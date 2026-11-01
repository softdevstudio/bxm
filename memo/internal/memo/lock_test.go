package memo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

func TestOutputLockExcludesAndWaits(t *testing.T) {
	chdir(t, t.TempDir())
	c := &Cache{Dir: t.TempDir()}
	release, _, err := c.LockOutputs([]string{"out"}, LockOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var waited atomic.Bool
	got := make(chan struct{})
	go func() {
		// Another spelling of the same path shares the lock.
		r2, _, err := c.LockOutputs([]string{"sub/../out"}, LockOptions{Waiting: func(string) { waited.Store(true) }})
		if err != nil {
			t.Error(err)
			return
		}
		close(got)
		r2()
	}()
	select {
	case <-got:
		t.Fatal("second lock taken while the first is held")
	case <-time.After(300 * time.Millisecond):
	}
	if !waited.Load() {
		t.Fatal("waiting callback not called")
	}
	release()
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("second lock not taken after release")
	}
}

func TestDifferentOutputsDoNotBlock(t *testing.T) {
	chdir(t, t.TempDir())
	c := &Cache{Dir: t.TempDir()}
	r1, _, err := c.LockOutputs([]string{"a"}, LockOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r1()
	done := make(chan error, 1)
	go func() {
		r2, _, err := c.LockOutputs([]string{"b", "c"}, LockOptions{Waiting: func(string) { t.Error("waited on an unrelated output") }})
		if err == nil {
			r2()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked on an unrelated output")
	}
}

func TestOutputLocksInAnyOrderDoNotDeadlock(t *testing.T) {
	chdir(t, t.TempDir())
	c := &Cache{Dir: t.TempDir()}
	var wg sync.WaitGroup
	var inside atomic.Int32
	for i := 0; i < 8; i++ {
		outs := []string{"a", "b", "c"}
		if i%2 == 1 {
			outs = []string{"c", "b", "a"}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				r, _, err := c.LockOutputs(outs, LockOptions{})
				if err != nil {
					t.Error(err)
					return
				}
				if n := inside.Add(1); n != 1 {
					t.Errorf("%d holders at once", n)
				}
				inside.Add(-1)
				r()
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("deadlock")
	}
}

func TestLocksStayInTheCache(t *testing.T) {
	proj := t.TempDir()
	chdir(t, proj)
	c := &Cache{Dir: t.TempDir()}
	r, _, err := c.LockOutputs([]string{"out", "dir/x"}, LockOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r()
	ents, _ := os.ReadDir(proj)
	if len(ents) != 0 {
		t.Fatalf("wrote into the project: %v", ents)
	}
	// out, dir/x, and the shared lock on dir.
	locks, _ := filepath.Glob(filepath.Join(c.Dir, "locks", "*.lock"))
	if len(locks) != 3 {
		t.Fatalf("locks: %v", locks)
	}
}

func TestOverlappingOutputsExclude(t *testing.T) {
	chdir(t, t.TempDir())
	c := &Cache{Dir: t.TempDir()}
	for _, pair := range [][2]string{{"out", "out/r.txt"}, {"out/r.txt", "out"}, {"a/b/c", "a"}} {
		r1, _, err := c.LockOutputs([]string{pair[0]}, LockOptions{})
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = c.LockOutputs([]string{pair[1]}, LockOptions{Wait: 100 * time.Millisecond})
		if err == nil || !strings.Contains(err.Error(), "gave up") {
			t.Fatalf("%v: second lock taken or wrong error: %v", pair, err)
		}
		r1()
	}
	// Siblings under one folder share it and run together.
	r1, _, err := c.LockOutputs([]string{"out/a"}, LockOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r2, _, err := c.LockOutputs([]string{"out/b"}, LockOptions{Wait: 100 * time.Millisecond})
	if err != nil {
		t.Fatal("siblings blocked each other:", err)
	}
	r2()
	r1()
}

func TestNestedRunFailsInsteadOfWaiting(t *testing.T) {
	chdir(t, t.TempDir())
	c := &Cache{Dir: t.TempDir()}
	r, keys, err := c.LockOutputs([]string{"out"}, LockOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r()
	t.Setenv(HeldEnv, strings.Join(keys, ","))
	start := time.Now()
	for _, o := range []string{"out", "out/x"} {
		_, _, err = c.LockOutputs([]string{o}, LockOptions{})
		if err == nil || !strings.Contains(err.Error(), "enclosing bxm-memo run") {
			t.Fatalf("%s: %v", o, err)
		}
	}
	if time.Since(start) > time.Second {
		t.Fatal("waited before failing")
	}
	// A different output is fine.
	r2, _, err := c.LockOutputs([]string{"other"}, LockOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r2()
}

func TestSymlinkSpellingsShareALock(t *testing.T) {
	proj := t.TempDir()
	chdir(t, proj)
	if err := os.Mkdir("real", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", "link"); err != nil {
		t.Skip(err)
	}
	// new/ does not exist yet; the existing part of the path is still resolved.
	if pathKey(resolvePath("link/new/f")) != pathKey(resolvePath("real/new/f")) {
		t.Fatal("two spellings, two locks")
	}
}

func TestUnusableLockFolderIsReported(t *testing.T) {
	chdir(t, t.TempDir())
	dir := t.TempDir()
	// A file where the locks folder should be.
	if err := os.WriteFile(filepath.Join(dir, "locks"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Cache{Dir: dir}
	if _, _, err := c.LockOutputs([]string{"out"}, LockOptions{}); !errors.Is(err, ErrLocksUnavailable) {
		t.Fatal(err)
	}
}
