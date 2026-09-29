package local

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// FreeBytes is the room left on the disk that holds Dir.
func FreeBytes(dir string) int64 {
	for d := dir; ; d = filepath.Dir(d) {
		p, err := windows.UTF16PtrFromString(d)
		if err != nil {
			return -1
		}
		var free, total, all uint64
		if windows.GetDiskFreeSpaceEx(p, &free, &total, &all) == nil {
			return int64(free)
		}
		if d == filepath.Dir(d) {
			return -1
		}
	}
}
