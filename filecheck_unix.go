//go:build darwin || linux

package lumberjack

import (
	"os"
	"syscall"
)

// fileWasRemoved reports whether an open file has been unlinked. Using fstat
// avoids both the allocation and the pathname lookup performed by os.Stat on
// every Write.
func fileWasRemoved(file *os.File, _ *Logger) bool {
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &stat); err != nil {
		return false
	}

	return stat.Nlink == 0
}
