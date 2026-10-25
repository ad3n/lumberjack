package lumberjack

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupNameCompatibility(t *testing.T) {
	savedTime := currentTime
	now := time.Date(2026, 10, 25, 1, 2, 3, 456000000, time.FixedZone("custom", 7*3600))
	currentTime = func() time.Time { return now }
	defer func() { currentTime = savedTime }()

	for _, name := range []string{"app.log", "app", ".log", "many.parts.log", "a/../b/log.%s", "dir/ログ.log", ""} {
		for _, local := range []bool{false, true} {
			stamp := now
			if !local {
				stamp = stamp.UTC()
			}

			base := filepath.Base(name)
			ext := filepath.Ext(base)
			want := filepath.Join(filepath.Dir(name), fmt.Sprintf("%s-%s%s", base[:len(base)-len(ext)], stamp.Format(backupTimeFormat), ext))
			if got := backupName(name, local); got != want {
				t.Fatalf("backupName(%q, %t) = %q, want %q", name, local, got, want)
			}
		}
	}
}
