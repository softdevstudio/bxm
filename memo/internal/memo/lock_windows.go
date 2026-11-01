//go:build windows

package memo

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// Windows paths are case-insensitive.
var caseInsensitiveFS = true

func foldCase(s string) string { return strings.ToLower(s) }

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileFailImmediately = 0x1
	lockfileExclusiveLock   = 0x2
	errorLockViolation      = syscall.Errno(33)
	errorIOPending          = syscall.Errno(997)
)

// lockFile tries once, without waiting, to lock the first byte of f with
// LockFileEx, exclusive or shared. It returns false when another process
// holds a conflicting lock.
func lockFile(f *os.File, exclusive bool) (bool, error) {
	flags := uintptr(lockfileFailImmediately)
	if exclusive {
		flags |= lockfileExclusiveLock
	}
	var ol syscall.Overlapped
	r, _, err := procLockFileEx.Call(f.Fd(), flags, 0, 1, 0, uintptr(unsafe.Pointer(&ol)))
	if r != 0 {
		return true, nil
	}
	if err == errorLockViolation || err == errorIOPending {
		return false, nil
	}
	return false, err
}

func unlockFile(f *os.File) error {
	var ol syscall.Overlapped
	r, _, err := procUnlockFileEx.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&ol)))
	if r == 0 {
		return err
	}
	return nil
}
