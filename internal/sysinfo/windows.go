//go:build windows

package sysinfo

import (
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows has no load average; the rest comes from kernel32.

func loadAvg() [3]float64 { return [3]float64{} }

type memoryStatusEx struct {
	length                                 uint32
	memoryLoad                             uint32
	totalPhys, availPhys                   uint64
	totalPageFile, availPageFile           uint64
	totalVirtual, availVirtual, availExtra uint64
}

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

func memTotal() uint64 {
	m := memoryStatusEx{}
	m.length = uint32(unsafe.Sizeof(m))
	if ok, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); ok == 0 {
		return 0
	}
	return m.totalPhys
}

func cpuTime() time.Duration {
	var creation, exit, kernel, user windows.Filetime
	if windows.GetProcessTimes(windows.CurrentProcess(), &creation, &exit, &kernel, &user) != nil {
		return 0
	}
	// Filetimes count 100-nanosecond intervals.
	ticks := func(f windows.Filetime) int64 { return int64(f.HighDateTime)<<32 | int64(f.LowDateTime) }
	return time.Duration(ticks(kernel)+ticks(user)) * 100
}

func disk(dir string) (free, size uint64) {
	for d := dir; ; d = filepath.Dir(d) {
		p, err := windows.UTF16PtrFromString(d)
		if err != nil {
			return 0, 0
		}
		var avail, total, all uint64
		if windows.GetDiskFreeSpaceEx(p, &avail, &total, &all) == nil {
			return avail, total
		}
		if d == filepath.Dir(d) {
			return 0, 0
		}
	}
}
