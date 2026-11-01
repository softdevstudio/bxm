package memo

import (
	"os"
	"path/filepath"
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
	release, err := c.LockOutputs([]string{"out"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var waited atomic.Bool
	got := make(chan struct{})
	go func() {
		// Another spelling of the same path shares the lock.
		r2, err := c.LockOutputs([]string{"sub/../out"}, func(string) { waited.Store(true) })
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
	r1, err := c.LockOutputs([]string{"a"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r1()
	done := make(chan error, 1)
	go func() {
		r2, err := c.LockOutputs([]string{"b", "c"}, func(string) { t.Error("waited on an unrelated output") })
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
				r, err := c.LockOutputs(outs, nil)
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
	r, err := c.LockOutputs([]string{"out", "dir/x"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r()
	ents, _ := os.ReadDir(proj)
	if len(ents) != 0 {
		t.Fatalf("wrote into the project: %v", ents)
	}
	locks, _ := filepath.Glob(filepath.Join(c.Dir, "locks", "*.lock"))
	if len(locks) != 2 {
		t.Fatalf("locks: %v", locks)
	}
}
