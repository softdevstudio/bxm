package memo

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Two runs that write the same output at the same time could otherwise store
// a file the other one is still rewriting. A run therefore locks its outputs
// from before its cache lookup until its outputs are restored or stored.
//
// Outputs can overlap without being equal: one run writes the folder out,
// another writes out/r.txt. So each output takes an exclusive lock on itself
// and a shared lock on every folder above it inside the working directory.
// An exclusive lock on out then conflicts with the shared lock on out that a
// run writing out/r.txt holds, while runs writing out/a and out/b only share
// out and proceed together. All locks are taken in one sorted order, so two
// runs can't each hold one and wait for the other.
//
// The lock files live in the cache, one per path, so nothing is written into
// the project. The operating system drops a lock when its process ends, even
// when the process is killed.

// HeldEnv names the environment variable a run passes to its command with the
// locks it holds, so a bxm-memo started inside it can't wait for its parent.
const HeldEnv = "BXM_MEMO_HELD"

// DefaultLockWait is how long a run waits for another run's outputs.
const DefaultLockWait = time.Hour

// ErrLocksUnavailable means locking itself failed (a read-only cache, a file
// system without locks). Run carries on without locks and says so.
var ErrLocksUnavailable = errors.New("output locks unavailable")

// resolvePath makes p absolute and resolves symlinks in the longest part of
// it that exists, so two spellings of one path share a lock.
func resolvePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	var rest []string
	dir := abs
	for {
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			dir = r
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		rest = append([]string{filepath.Base(dir)}, rest...)
		dir = parent
	}
	return filepath.Join(append([]string{dir}, rest...)...)
}

func pathKey(p string) string {
	if caseInsensitiveFS {
		p = foldCase(p)
	}
	h := sha256.Sum256([]byte(p))
	return hex.EncodeToString(h[:])
}

type lockWant struct {
	key       string
	exclusive bool
	label     string // the output it was taken for, for messages
}

// wants lists the locks for a set of outputs: exclusive on each output,
// shared on each folder above it inside the working directory. Where both
// apply to one path, exclusive wins.
func wants(outs []string) []lockWant {
	byKey := map[string]*lockWant{}
	add := func(p string, exclusive bool, label string) {
		k := pathKey(resolvePath(p))
		if w, ok := byKey[k]; ok {
			if exclusive && !w.exclusive {
				w.exclusive, w.label = true, label
			}
			return
		}
		byKey[k] = &lockWant{k, exclusive, label}
	}
	for _, o := range outs {
		clean := filepath.Clean(o)
		add(clean, true, o)
		for d := filepath.Dir(clean); d != "." && d != string(filepath.Separator) && d != filepath.Dir(d); d = filepath.Dir(d) {
			add(d, false, o)
		}
	}
	list := make([]lockWant, 0, len(byKey))
	for _, w := range byKey {
		list = append(list, *w)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].key < list[j].key })
	return list
}

// LockOptions controls LockOutputs.
type LockOptions struct {
	Wait    time.Duration                     // give up after this long; 0 means DefaultLockWait
	Waiting func(out string)                  // called once per output another run holds
	Still   func(out string, d time.Duration) // called every minute of waiting
}

// LockOutputs takes the locks for the outputs and returns a function that
// releases them, and the keys held (for HeldEnv). It fails at once when an
// enclosing bxm-memo run already holds one of them, rather than waiting for
// itself. Any other failure to lock wraps ErrLocksUnavailable.
func (c *Cache) LockOutputs(outs []string, opt LockOptions) (func(), []string, error) {
	list := wants(outs)
	held := map[string]bool{}
	for _, k := range strings.Split(os.Getenv(HeldEnv), ",") {
		if k != "" {
			held[k] = true
		}
	}
	for _, w := range list {
		if held[w.key] {
			return nil, nil, fmt.Errorf("an enclosing bxm-memo run is writing %s; a command run by bxm-memo can't wrap its own outputs", w.label)
		}
	}
	wait := opt.Wait
	if wait <= 0 {
		wait = DefaultLockWait
	}
	dir := filepath.Join(c.Dir, "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrLocksUnavailable, err)
	}
	var files []*os.File
	var keys []string
	release := func() {
		for i := len(files) - 1; i >= 0; i-- {
			unlockFile(files[i])
			files[i].Close()
		}
	}
	deadline := time.Now().Add(wait)
	for _, w := range list {
		f, err := os.OpenFile(filepath.Join(dir, w.key+".lock"), os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			release()
			return nil, nil, fmt.Errorf("%w: %v", ErrLocksUnavailable, err)
		}
		start, nextNote := time.Now(), time.Now().Add(time.Minute)
		pause := 5 * time.Millisecond
		for told := false; ; {
			got, err := lockFile(f, w.exclusive)
			if err != nil {
				f.Close()
				release()
				return nil, nil, fmt.Errorf("%w: %v", ErrLocksUnavailable, err)
			}
			if got {
				break
			}
			if !told && opt.Waiting != nil {
				opt.Waiting(w.label)
			}
			told = true
			if time.Now().After(deadline) {
				f.Close()
				release()
				return nil, nil, fmt.Errorf("gave up after %s waiting for another run writing %s", wait, w.label)
			}
			if time.Now().After(nextNote) && opt.Still != nil {
				opt.Still(w.label, time.Since(start).Round(time.Second))
				nextNote = time.Now().Add(time.Minute)
			}
			time.Sleep(pause)
			if pause < 200*time.Millisecond {
				pause *= 2
			}
		}
		files = append(files, f)
		keys = append(keys, w.key)
	}
	return release, keys, nil
}
