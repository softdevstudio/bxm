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

// lockFile takes an exclusive flock. Without block it returns false when
// another process holds the lock.
func lockFile(f *os.File, block bool) (bool, error) {
	how := syscall.LOCK_EX
	if !block {
		how |= syscall.LOCK_NB
	}
	for {
		err := syscall.Flock(int(f.Fd()), how)
		switch err {
		case nil:
			return true, nil
		case syscall.EINTR:
			continue
		case syscall.EWOULDBLOCK:
			if !block {
				return false, nil
			}
		}
		return false, err
	}
}

func unlockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
