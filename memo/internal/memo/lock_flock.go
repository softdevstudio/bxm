//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package memo

import (
	"os"
	"runtime"
	"strings"
	"syscall"
)

// macOS volumes are case-insensitive by default, so out.txt and OUT.txt are
// one file there and must share a lock.
var caseInsensitiveFS = runtime.GOOS == "darwin"

func foldCase(s string) string { return strings.ToLower(s) }

// lockFile tries once, without waiting, to take an exclusive or shared
// flock. It returns false when another process holds a conflicting lock.
func lockFile(f *os.File, exclusive bool) (bool, error) {
	how := syscall.LOCK_SH | syscall.LOCK_NB
	if exclusive {
		how = syscall.LOCK_EX | syscall.LOCK_NB
	}
	for {
		err := syscall.Flock(int(f.Fd()), how)
		switch err {
		case nil:
			return true, nil
		case syscall.EINTR:
			continue
		case syscall.EWOULDBLOCK:
			return false, nil
		}
		return false, err
	}
}

func unlockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
