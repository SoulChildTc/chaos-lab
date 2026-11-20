package cli

import (
	"syscall"
)

// processAlive reports whether pid exists. EPERM means it exists but belongs to
// someone else, which still counts as alive.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return err != syscall.ESRCH
}
