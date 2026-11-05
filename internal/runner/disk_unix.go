//go:build !windows

package runner

import (
	"os"
	"path/filepath"
	"syscall"
)

func freeBytes(dir string) (int64, bool) {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil {
			var st syscall.Statfs_t
			if syscall.Statfs(d, &st) != nil {
				return 0, false
			}
			return int64(st.Bavail) * int64(st.Bsize), true
		}
		if d == filepath.Dir(d) {
			return 0, false
		}
	}
}
