// Package sysinfo reads how busy this computer and this process are, for
// the system status panel. What a platform cannot tell stays zero.
package sysinfo

import (
	"runtime"
	"sync"
	"time"
)

type Host struct {
	CPUs     int        `json:"cpus"`
	Load     [3]float64 `json:"load"`
	MemTotal uint64     `json:"mem_total"`
	DiskFree uint64     `json:"disk_free"`
	DiskSize uint64     `json:"disk_size"`
}

type Process struct {
	CPUPercent float64 `json:"cpu_percent"`
	HeapBytes  uint64  `json:"heap_bytes"`
	SysBytes   uint64  `json:"sys_bytes"`
	Goroutines int     `json:"goroutines"`
	Uptime     float64 `json:"uptime_s"`
}

var (
	start   = time.Now()
	mu      sync.Mutex
	lastAt  time.Time
	lastCPU time.Duration
)

// ReadProcess measures CPU use since the previous call.
func ReadProcess() Process {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	p := Process{HeapBytes: m.HeapAlloc, SysBytes: m.Sys, Goroutines: runtime.NumGoroutine(), Uptime: time.Since(start).Seconds()}
	mu.Lock()
	defer mu.Unlock()
	now, cpu := time.Now(), cpuTime()
	if !lastAt.IsZero() {
		if wall := now.Sub(lastAt); wall > 0 {
			p.CPUPercent = float64(cpu-lastCPU) / float64(wall) * 100 / float64(runtime.NumCPU())
		}
	}
	lastAt, lastCPU = now, cpu
	return p
}

// ReadHost reads load, memory and the disk that holds dir.
func ReadHost(dir string) Host {
	h := Host{CPUs: runtime.NumCPU(), Load: loadAvg(), MemTotal: memTotal()}
	h.DiskFree, h.DiskSize = disk(dir)
	return h
}
