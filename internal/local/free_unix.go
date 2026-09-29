//go:build !windows

package local

import (
	"path/filepath"
	"syscall"
)

// FreeBytes is the room left on the disk that holds Dir.
func FreeBytes(dir string) int64 {
	var st syscall.Statfs_t
	for d := dir; ; d = filepath.Dir(d) {
		if syscall.Statfs(d, &st) == nil {
			return int64(st.Bavail) * int64(st.Bsize)
		}
		if d == filepath.Dir(d) {
			return -1
		}
	}
}
