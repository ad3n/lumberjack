package lumberjack

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	backupTimeFormat = "2006-01-02T15-04-05.000"
	compressSuffix   = ".gz"
	tmpSuffix        = ".tmp"
	defaultFileMode  = os.FileMode(0600)
	defaultMaxSize   = 100
)

var (
	_           io.WriteCloser = (*Logger)(nil)
	currentTime                = time.Now

	osStat = os.Stat

	megabyte = 1024 * 1024
)

type millConfig struct {
	now        time.Time
	dir        string
	prefix     string
	ext        string
	maxBackups int
	maxAge     int
	compress   bool
}

type logInfo struct {
	timestamp time.Time
	os.DirEntry
}

type Logger struct {
	file *os.File

	millCh   chan millConfig
	Filename string `json:"filename" yaml:"filename"`

	MaxSize int `json:"maxsize" yaml:"maxsize"`

	MaxAge int `json:"maxage" yaml:"maxage"`

	MaxBackups int `json:"maxbackups" yaml:"maxbackups"`

	size     int64
	millWg   sync.WaitGroup
	FileMode os.FileMode
	mu       sync.Mutex

	LocalTime bool `json:"localtime" yaml:"localtime"`

	Compress bool `json:"compress" yaml:"compress"`
}

func (l *Logger) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	writeLen := int64(len(p))
	maxSize := l.max()
	if writeLen > maxSize {
		return 0, fmt.Errorf(
			"write length %d exceeds maximum file size %d", writeLen, maxSize,
		)
	}

	if l.file == nil || fileWasRemoved(l.file, l) {
		if err = l.openExistingOrNew(len(p)); err != nil {
			return 0, err
		}
	}

	if l.size > maxSize-writeLen {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = l.file.Write(p)
	l.size += int64(n)

	return n, err
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.millCh != nil {
		close(l.millCh)
		l.millWg.Wait()
		l.millCh = nil
	}

	return l.close()
}

func (l *Logger) close() error {
	if l.file == nil {
		return nil
	}

	err := l.file.Close()
	l.file = nil

	return err
}

func (l *Logger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rotate()
}

func (l *Logger) rotate() error {
	if err := l.close(); err != nil {
		return err
	}

	if err := l.openNew(); err != nil {
		return err
	}

	l.mill()

	return nil
}

func (l *Logger) openNew() error {
	err := os.MkdirAll(l.dir(), 0755)
	if err != nil {
		return fmt.Errorf("can't make directories for new logfile: %s", err)
	}

	name := l.filename()
	mode := defaultFileMode
	if l.FileMode != 0 {
		mode = l.FileMode
	}

	info, err := osStat(name)
	if err == nil {
		if l.FileMode == 0 {
			mode = info.Mode()
		}

		newname := backupName(name, l.LocalTime)
		if err := os.Rename(name, newname); err != nil {
			return fmt.Errorf("can't rename log file: %s", err)
		}

		if err := chownWithMode(name, info, mode); err != nil {
			return err
		}
	}

	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("can't open new logfile: %s", err)
	}

	l.file = f
	l.size = 0

	return nil
}

func backupName(name string, local bool) string {
	dir := filepath.Dir(name)
	filename := filepath.Base(name)
	ext := filepath.Ext(filename)
	prefix := filename[:len(filename)-len(ext)]
	t := currentTime()
	if !local {
		t = t.UTC()
	}

	timestamp := t.Format(backupTimeFormat)
	return filepath.Join(dir, prefix+"-"+timestamp+ext)
}

func (l *Logger) openExistingOrNew(writeLen int) error {
	l.mill()

	filename := l.filename()
	info, err := osStat(filename)
	if os.IsNotExist(err) {
		return l.openNew()
	}

	if err != nil {
		return fmt.Errorf("error getting log file info: %s", err)
	}

	maxSize := l.max()
	if int64(writeLen) >= maxSize || info.Size() >= maxSize-int64(writeLen) {
		return l.rotate()
	}

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return l.openNew()
	}

	l.file = file
	l.size = info.Size()

	return nil
}

func (l *Logger) filename() string {
	if l.Filename != "" {
		return l.Filename
	}

	name := filepath.Base(os.Args[0]) + "-lumberjack.log"

	return filepath.Join(os.TempDir(), name)
}

func (l *Logger) millConfig() millConfig {
	filename := l.filename()
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	var now time.Time
	if l.MaxAge > 0 {
		now = currentTime()
	}

	return millConfig{
		dir:        filepath.Dir(filename),
		prefix:     base[:len(base)-len(ext)] + "-",
		ext:        ext,
		maxBackups: l.MaxBackups,
		maxAge:     l.MaxAge,
		compress:   l.Compress,
		now:        now,
	}
}

func millRunOnceConfig(config millConfig) error {
	if config.maxBackups == 0 && config.maxAge == 0 && !config.compress {
		return nil
	}

	files, err := oldLogFiles(config.dir, config.prefix, config.ext)
	if err != nil {
		return err
	}

	var compress, remove []logInfo

	if config.maxBackups > 0 && config.maxBackups < len(files) {
		preserved := make(map[string]struct{}, config.maxBackups+1)
		remaining := make([]logInfo, 0, config.maxBackups)
		for _, f := range files {
			fn := f.Name()

			fn = strings.TrimSuffix(fn, compressSuffix)
			preserved[fn] = struct{}{}

			if len(preserved) > config.maxBackups {
				remove = append(remove, f)
				continue
			}

			remaining = append(remaining, f)
		}

		files = remaining
	}

	if config.maxAge > 0 {
		diff := maxAgeDuration(config.maxAge)
		cutoff := config.now.Add(-diff)

		remaining := make([]logInfo, 0, len(files))
		for _, f := range files {
			if f.timestamp.Before(cutoff) {
				remove = append(remove, f)
				continue
			}

			remaining = append(remaining, f)
		}

		files = remaining
	}

	if config.compress {
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), compressSuffix) {
				compress = append(compress, f)
			}
		}
	}

	for _, f := range remove {
		errRemove := os.Remove(filepath.Join(config.dir, f.Name()))
		if err == nil && errRemove != nil {
			err = errRemove
		}
	}

	for _, f := range compress {
		fn := filepath.Join(config.dir, f.Name())
		errCompress := compressLogFile(fn, fn+compressSuffix)
		if err == nil && errCompress != nil {
			err = errCompress
		}
	}

	return err
}

func (l *Logger) millRun() {
	defer l.millWg.Done()

	for config := range l.millCh {
		_ = millRunOnceConfig(config)
	}
}

func (l *Logger) mill() {
	if l.MaxBackups == 0 && l.MaxAge == 0 && !l.Compress {
		return
	}

	config := l.millConfig()
	if l.millCh == nil {
		l.millCh = make(chan millConfig, 1)
		l.millWg.Add(1)
		go l.millRun()
	}

	select {
	case l.millCh <- config:
	default:
	}
}

func (l *Logger) oldLogFiles() ([]logInfo, error) {
	config := l.millConfig()

	return oldLogFiles(config.dir, config.prefix, config.ext)
}

func oldLogFiles(dir, prefix, ext string) ([]logInfo, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("can't read log file directory: %s", err)
	}

	capacity := len(files)
	if capacity > 64 {
		capacity = 64
	}

	logFiles := make([]logInfo, 0, capacity)

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		if t, err := timeFromName(f.Name(), prefix, ext); err == nil {
			logFiles = append(logFiles, logInfo{t, f})
			continue
		}

		if t, err := timeFromName(f.Name(), prefix, ext+compressSuffix); err == nil {
			logFiles = append(logFiles, logInfo{t, f})
			continue
		}
	}

	slices.SortFunc(logFiles, func(a, b logInfo) int {
		return b.timestamp.Compare(a.timestamp)
	})

	return logFiles, nil
}

func (l *Logger) timeFromName(filename, prefix, ext string) (time.Time, error) {
	return timeFromName(filename, prefix, ext)
}

func timeFromName(filename, prefix, ext string) (time.Time, error) {
	if !strings.HasPrefix(filename, prefix) {
		return time.Time{}, errors.New("mismatched prefix")
	}

	if !strings.HasSuffix(filename, ext) {
		return time.Time{}, errors.New("mismatched extension")
	}

	ts := filename[len(prefix) : len(filename)-len(ext)]

	return time.Parse(backupTimeFormat, ts)
}

func (l *Logger) max() int64 {
	if l.MaxSize == 0 {
		return int64(defaultMaxSize * megabyte)
	}

	maxSize := int64(l.MaxSize)
	unit := int64(megabyte)
	if maxSize > 0 && unit > 0 && maxSize > math.MaxInt64/unit {
		return math.MaxInt64
	}

	return maxSize * unit
}

func maxAgeDuration(days int) time.Duration {
	const day = 24 * time.Hour
	if days > int(math.MaxInt64/int64(day)) {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(days) * day
}

func (l *Logger) dir() string {
	return filepath.Dir(l.filename())
}

func (l *Logger) prefixAndExt() (prefix, ext string) {
	filename := filepath.Base(l.filename())
	ext = filepath.Ext(filename)
	prefix = filename[:len(filename)-len(ext)] + "-"
	return prefix, ext
}

func compressLogFile(src, dst string) (err error) {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}
	sourceClosed := false
	defer func() {
		if !sourceClosed {
			_ = f.Close()
		}
	}()

	fi, err := osStat(src)
	if err != nil {
		return fmt.Errorf("failed to stat log file: %v", err)
	}

	tmpDst := dst + tmpSuffix
	defer func() {
		if err != nil {
			_ = os.Remove(tmpDst)
		}
	}()

	if err := chown(tmpDst, fi); err != nil {
		return fmt.Errorf("failed to chown compressed log file: %v", err)
	}

	gzf, err := os.OpenFile(tmpDst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fi.Mode())
	if err != nil {
		return fmt.Errorf("failed to open compressed log file: %v", err)
	}
	destinationClosed := false
	defer func() {
		if !destinationClosed {
			_ = gzf.Close()
		}
	}()

	gz := compressionWriters.get(gzf)
	defer func() {
		compressionWriters.put(gz)
		if err != nil {
			err = fmt.Errorf("failed to compress log file: %v", err)
		}
	}()

	if _, err := compressionBuffers.copy(gz, f); err != nil {
		return err
	}

	if err := gz.Close(); err != nil {
		return err
	}

	if err := gzf.Sync(); err != nil {
		return err
	}

	destinationClosed = true
	if err := gzf.Close(); err != nil {
		return err
	}

	sourceClosed = true
	if err := f.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpDst, dst); err != nil {
		return err
	}

	if err := os.Remove(src); err != nil {
		return err
	}

	return nil
}
