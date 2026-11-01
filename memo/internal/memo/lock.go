package memo

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
)

// Two runs that write the same output at the same time could otherwise store
// a file the other one is still rewriting. A run therefore holds an exclusive
// lock on each declared output from before its cache lookup until its outputs
// are restored or stored. The lock files live in the cache, one per output
// path, so nothing is written into the project. The operating system drops a
// lock when its process ends, even when the process is killed.

// lockKey names the lock for an output: the SHA-256 of its absolute path,
// with symlinks in the parent folder resolved, so two spellings of one path
// share a lock.
func lockKey(out string) string {
	abs, err := filepath.Abs(out)
	if err != nil {
		abs = out
	}
	if parent, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		abs = filepath.Join(parent, filepath.Base(abs))
	}
	if caseInsensitiveFS {
		abs = foldCase(abs)
	}
	h := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(h[:])
}

// LockOutputs takes the lock for every output, in a fixed order so that two
// runs sharing several outputs cannot each hold one and wait for the other.
// waiting is called, at most once per output, when another run holds it.
// The returned function releases all of them.
func (c *Cache) LockOutputs(outs []string, waiting func(out string)) (func(), error) {
	byKey := map[string]string{}
	for _, o := range outs {
		byKey[lockKey(o)] = o
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	dir := filepath.Join(c.Dir, "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var held []*os.File
	release := func() {
		for i := len(held) - 1; i >= 0; i-- {
			unlockFile(held[i])
			held[i].Close()
		}
	}
	for _, k := range keys {
		f, err := os.OpenFile(filepath.Join(dir, k+".lock"), os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			release()
			return nil, err
		}
		got, err := lockFile(f, false)
		if err == nil && !got {
			if waiting != nil {
				waiting(byKey[k])
			}
			got, err = lockFile(f, true)
		}
		if err != nil || !got {
			f.Close()
			release()
			if err == nil {
				err = os.ErrDeadlineExceeded
			}
			return nil, err
		}
		held = append(held, f)
	}
	return release, nil
}
