//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly || windows)

package memo

import "os"

// No file locking on this system: runs are not serialised (the README says
// which systems lock).
var caseInsensitiveFS = false

func foldCase(s string) string { return s }

func lockFile(f *os.File, block bool) (bool, error) { return true, nil }
func unlockFile(f *os.File) error                   { return nil }
