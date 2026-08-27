//go:build !darwin && !linux

package lumberjack

import "os"

// fileWasRemoved uses the portable pathname check on platforms where the
// open-file link count is not available through syscall.Stat_t.
func fileWasRemoved(_ *os.File, logger *Logger) bool {
	_, err := osStat(logger.filename())
	return os.IsNotExist(err)
}
