package memo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// MaxCapture is the most stdout plus stderr a run may print and still be
// cached. Larger output is passed through but the result is not stored.
const MaxCapture = 64 << 20

func cleanRel(p string) string { return filepath.ToSlash(filepath.Clean(p)) }

// capture holds stdout and stderr up to a shared limit. The two streams are
// copied by separate goroutines, hence the lock.
type capture struct {
	mu       sync.Mutex
	out, err bytes.Buffer
	total    int
	overflow bool
}

type capWriter struct {
	c   *capture
	buf *bytes.Buffer
}

func (w capWriter) Write(p []byte) (int, error) {
	w.c.mu.Lock()
	defer w.c.mu.Unlock()
	if !w.c.overflow {
		if w.c.total+len(p) > MaxCapture {
			w.c.overflow = true
			w.c.out.Reset()
			w.c.err.Reset()
		} else {
			w.buf.Write(p)
			w.c.total += len(p)
		}
	}
	return len(p), nil
}

// Options control one run.
type Options struct {
	Spec   Spec
	Name   string
	Cache  *Cache
	Quiet  bool
	Stdout io.Writer
	Stderr io.Writer
	Log    io.Writer // where hit/miss lines go
}

// Result describes what happened.
type Result struct {
	Hit          bool
	Key          string
	Reasons      []string // why it missed
	ExitCode     int
	Restored     int
	Stored       bool
	NotStoredWhy string
}

func (o *Options) logf(format string, a ...any) {
	if !o.Quiet && o.Log != nil {
		fmt.Fprintf(o.Log, "bxm-memo: "+format+"\n", a...)
	}
}

func label(name string, m *Manifest) string {
	if name != "" {
		return name
	}
	return filepath.Base(m.Command[0])
}

// Check builds the manifest and reports hit or miss without running anything.
func Check(o *Options) (*Result, *Manifest, error) {
	m, err := Build(o.Spec)
	if err != nil {
		return nil, nil, err
	}
	key := m.Key()
	r := &Result{Key: key}
	e, err := o.Cache.Lookup(key)
	if err == nil && e != nil {
		r.Hit = true
		return r, m, nil
	}
	r.Reasons = Explain(o.Cache.LastManifest(JobID(o.Name, m)), m)
	return r, m, nil
}

// Run restores the outputs on a hit, or runs the command on a miss and stores
// the result when it succeeds.
func Run(o *Options) (*Result, error) {
	m, err := Build(o.Spec)
	if err != nil {
		return nil, err
	}
	key := m.Key()
	job := JobID(o.Name, m)
	name := label(o.Name, m)
	r := &Result{Key: key}

	if e, err := o.Cache.Lookup(key); err != nil {
		o.logf("cache entry damaged, running instead: %v", err)
		o.Cache.Drop(key)
	} else if e != nil {
		start := time.Now()
		stdout, stderr, rerr := o.Cache.Replay(e)
		var n int
		if rerr == nil {
			n, _, rerr = o.Cache.Restore(e)
		}
		if rerr == nil {
			o.Stdout.Write(stdout)
			o.Stderr.Write(stderr)
			r.Hit, r.Restored = true, n
			o.logf("hit %s: restored %d file(s) in %s (the run took %s)", name, n,
				time.Since(start).Round(time.Microsecond), (time.Duration(e.RunMS) * time.Millisecond))
			return r, nil
		}
		o.logf("cache entry damaged, running instead: %v", rerr)
		o.Cache.Drop(key)
	}

	r.Reasons = Explain(o.Cache.LastManifest(job), m)
	for i, why := range r.Reasons {
		if i == 5 {
			o.logf("  ... and %d more", len(r.Reasons)-5)
			break
		}
		if i == 0 {
			o.logf("miss %s: %s", name, why)
		} else {
			o.logf("  %s", why)
		}
	}

	capt := &capture{}
	cmd := exec.Command(m.Command[0], m.Command[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(o.Stdout, capWriter{capt, &capt.out})
	cmd.Stderr = io.MultiWriter(o.Stderr, capWriter{capt, &capt.err})
	start := time.Now()
	err = cmd.Run()
	took := time.Since(start)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			r.ExitCode = ee.ExitCode()
			if r.ExitCode < 0 {
				r.ExitCode = 1
			}
			r.NotStoredWhy = fmt.Sprintf("the command failed (exit %d)", r.ExitCode)
			o.logf("not cached: %s", r.NotStoredWhy)
			return r, nil
		}
		return nil, fmt.Errorf("running %s: %w", m.Command[0], err)
	}
	if capt.overflow {
		r.NotStoredWhy = "the command printed more than 64 MiB"
		o.logf("not cached: %s", r.NotStoredWhy)
		return r, nil
	}
	// If an input changed while the command ran, the outputs may belong to
	// neither version. Do not store them.
	if after, err := Build(o.Spec); err != nil || after.Key() != key {
		r.NotStoredWhy = "an input changed while the command was running"
		o.logf("not cached: %s", r.NotStoredWhy)
		return r, nil
	}
	for _, out := range m.Outputs {
		if _, err := os.Stat(out); err != nil {
			r.NotStoredWhy = "the command did not produce the declared output " + out
			o.logf("not cached: %s", r.NotStoredWhy)
			return r, nil
		}
	}
	if _, err := o.Cache.Store(job, m, capt.out.Bytes(), capt.err.Bytes(), took); err != nil {
		return r, fmt.Errorf("storing the result: %w", err)
	}
	r.Stored = true
	return r, nil
}
