package privilege

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Keep removes capabilities on every Go thread, retaining only the requested bits.
// It requires a non-cgo binary; never substitute a calling-thread-only capset.
func Keep(mask uint32) error {
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	err := unix.Capget(&header, &data[0])
	if err != nil {
		return fmt.Errorf("read capabilities: %w", err)
	}
	data[0].Effective &= mask
	data[0].Permitted &= mask
	data[0].Inheritable = 0
	data[1] = unix.CapUserData{}

	_, _, errno := syscall.AllThreadsSyscall(syscall.SYS_CAPSET, uintptr(unsafe.Pointer(&header)), uintptr(unsafe.Pointer(&data[0])), 0)
	if errno != 0 {
		return fmt.Errorf("restrict capabilities on all threads (requires CGO_ENABLED=0): %w", errno)
	}
	return nil
}

// Harden prevents session threads and their descendants from regaining privileges at exec.
func Harden() error {
	_, _, errno := syscall.AllThreadsSyscall(syscall.SYS_PRCTL, unix.PR_SET_NO_NEW_PRIVS, 1, 0)
	if errno != 0 {
		return fmt.Errorf("set no_new_privs on all threads (requires CGO_ENABLED=0): %w", errno)
	}
	_, _, errno = syscall.AllThreadsSyscall(syscall.SYS_PRCTL, unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0)
	if errno != 0 {
		return fmt.Errorf("clear ambient capabilities on all threads: %w", errno)
	}
	return nil
}
