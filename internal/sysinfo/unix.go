//go:build !windows

package sysinfo

import (
	"syscall"
	"time"
)

// cpuTime is the user and system time this process used.
func cpuTime() time.Duration {
	var ru syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &ru) != nil {
		return 0
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

func disk(dir string) (free, size uint64) {
	var st syscall.Statfs_t
	if syscall.Statfs(dir, &st) != nil {
		return 0, 0
	}
	return st.Bavail * uint64(st.Bsize), st.Blocks * uint64(st.Bsize)
}
