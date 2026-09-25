//go:build windows

package sysinfo

import "time"

func cpuTime() time.Duration          { return 0 }
func disk(string) (free, size uint64) { return 0, 0 }
