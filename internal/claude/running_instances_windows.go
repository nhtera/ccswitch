//go:build windows

package claude

import "os"

// isPIDAlive on Windows: os.FindProcess always succeeds (it just wraps
// the PID in a struct without actually opening a handle), so we can't
// rely on it for liveness. A best-effort approach is to attempt a
// `OpenProcess` via syscall — but for v1 we keep things simple and
// trust the lock/session file's presence. A stale file at worst shows
// one extra running-instance entry; it never corrupts state.
func isPIDAlive(pid int) bool {
	return pid > 1 && processExistsWindows(pid)
}

// processExistsWindows returns true if FindProcess returns nil error.
// On Windows this is a weak check (FindProcess almost always succeeds)
// but it's better than nothing. A future enhancement could call
// OpenProcess via golang.org/x/sys/windows for accuracy.
func processExistsWindows(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}
