package sysinfo

import (
	"encoding/binary"

	"golang.org/x/sys/unix"
)

func loadAvg() [3]float64 {
	raw, err := unix.SysctlRaw("vm.loadavg")
	if err != nil || len(raw) < 16 {
		return [3]float64{}
	}
	scale := 2048.0
	if len(raw) >= 24 {
		if v := binary.LittleEndian.Uint64(raw[16:24]); v > 0 {
			scale = float64(v)
		}
	}
	var out [3]float64
	for i := range out {
		out[i] = float64(binary.LittleEndian.Uint32(raw[i*4:])) / scale
	}
	return out
}

func memTotal() uint64 {
	v, _ := unix.SysctlUint64("hw.memsize")
	return v
}
