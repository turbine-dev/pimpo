package sysinfo

import (
	"runtime"
	"testing"
)

func TestReadsThisMachine(t *testing.T) {
	h := ReadHost(t.TempDir())
	if h.CPUs < 1 || h.DiskSize == 0 || h.DiskFree > h.DiskSize {
		t.Fatalf("%+v", h)
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if h.MemTotal == 0 || h.Load[0] <= 0 {
			t.Fatalf("load and memory missing: %+v", h)
		}
	}
	ReadProcess()
	for i := 0; i < 3e7; i++ {
		_ = i * i
	}
	p := ReadProcess()
	if p.HeapBytes == 0 || p.Goroutines < 1 || p.CPUPercent < 0 {
		t.Fatalf("%+v", p)
	}
}
