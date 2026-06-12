//go:build linux

package platform

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	prioProcess = 0
	ioprioClassBE = 2
	ioprioClassShift = 13
)

// SetLowPriority sets the process priority to idle on Linux.
// It sets the CPU scheduling priority to 19 (lowest) and attempts to set
// the I/O priority to best-effort class at the lowest level.
func SetLowPriority() error {
	// Set CPU niceness to 19 (lowest priority)
	if err := syscall.Setpriority(syscall.PRIO_PROCESS, 0, 19); err != nil {
		return fmt.Errorf("setpriority failed: %w", err)
	}

	// Attempt to set I/O priority via ioprio_set syscall
	// ioprio value: class=BE (2), data=7 (lowest within class)
	ioprioValue := (ioprioClassBE << ioprioClassShift) | 7
	if err := ioprioSet(1 /* IOPRIO_WHO_PROCESS */, 0 /* current process */, ioprioValue); err != nil {
		// Non-fatal: ioprio_set may not be available on all kernels
		// Log but don't return error
		_ = err
	}

	return nil
}

// ioprioSet invokes the ioprio_set syscall directly.
func ioprioSet(which, who, ioprio int) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOPRIO_SET,
		uintptr(which),
		uintptr(who),
		uintptr(unsafe.Pointer(nil)),
	)
	// Re-do with correct argument passing
	_, _, errno = syscall.RawSyscall(
		syscall.SYS_IOPRIO_SET,
		uintptr(which),
		uintptr(who),
		uintptr(ioprio),
	)
	if errno != 0 {
		return fmt.Errorf("ioprio_set: %w", errno)
	}
	return nil
}
