package sysinfo

import (
	"os"
	"strconv"
	"strings"
)

func loadAvg() [3]float64 {
	var out [3]float64
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return out
	}
	for i, f := range strings.Fields(string(b)) {
		if i == 3 {
			break
		}
		out[i], _ = strconv.ParseFloat(f, 64)
	}
	return out
}

func memTotal() uint64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(line, "MemTotal:"); ok {
			kb, _ := strconv.ParseUint(strings.Fields(rest)[0], 10, 64)
			return kb * 1024
		}
	}
	return 0
}
