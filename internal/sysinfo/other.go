//go:build !darwin && !linux

package sysinfo

func loadAvg() [3]float64 { return [3]float64{} }
func memTotal() uint64    { return 0 }
