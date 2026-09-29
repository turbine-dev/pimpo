//go:build windows

package main

import "golang.org/x/sys/windows"

// stillActive is what GetExitCodeProcess reports for a running process.
const stillActive = 259

// processAlive asks Windows, where signals do not exist: a process that
// can be opened and has no exit code yet is running.
func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == stillActive
}
