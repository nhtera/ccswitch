//go:build !windows

package claude

import (
	"errors"
	"os"
	"syscall"
)

// isPIDAlive uses Signal(0) — a no-op probe that succeeds iff the
// kernel knows about the PID. ESRCH (no such process) means dead;
// EPERM means the process exists but we lack permission to signal it.
func isPIDAlive(pid int) bool {
	if pid <= 1 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}
