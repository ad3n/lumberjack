package lumberjack

import (
	"bufio"
	"log"
)

// To use lumberjack with the standard library's log package, just pass it into
// the SetOutput function when your application starts.
func Example() {
	log.SetOutput(&Logger{
		Filename:   "/var/log/myapp/foo.log",
		MaxSize:    500, // megabytes
		MaxBackups: 3,
		MaxAge:     28,   // days
		Compress:   true, // disabled by default
	})
}

// Buffering amortizes filesystem and file-removal-check syscalls for
// high-volume logging. Flush the buffer before closing or rotating the logger.
func ExampleLogger_buffered() {
	l := &Logger{
		Filename:   "/var/log/myapp/foo.log",
		MaxSize:    500,
		MaxBackups: 3,
	}
	w := bufio.NewWriterSize(l, 4096)
	log.SetOutput(w)

	// Before application shutdown:
	_ = w.Flush()
	_ = l.Close()
}
