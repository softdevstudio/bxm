package memo

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Cache is a directory holding entries (one JSON file per key), jobs (the
// latest manifest per job, for explaining misses) and objects (file contents
// named by their SHA-256). Every file is written to a temporary name and
// renamed into place, so a crash leaves at most a stray temporary file, which
// is never read.
type Cache struct {
	Dir string
}

// OutFile is one restored file of an output.
type OutFile struct {
	Path string `json:"path"`
	Hash string `json:"sha256"`
	Size int64  `json:"size"`
	Exec bool   `json:"exec,omitempty"`
}

// Output is one declared output: a file, or a directory with its files.
type Output struct {
	Path  string    `json:"path"`
	IsDir bool      `json:"dir,omitempty"`
	Files []OutFile `json:"files"`
}

// Entry is a stored result.
type Entry struct {
	Key      string    `json:"key"`
	Job      string    `json:"job"`
	Manifest *Manifest `json:"manifest"`
	Outputs  []Output  `json:"outputs"`
	Stdout   string    `json:"stdout,omitempty"` // object hash
	Stderr   string    `json:"stderr,omitempty"` // object hash
	Created  time.Time `json:"created"`
	RunMS    int64     `json:"run_ms"`
}

// DefaultDir is $BXM_MEMO_CACHE, or bxm-memo in the user cache directory.
func DefaultDir() string {
	if d := os.Getenv("BXM_MEMO_CACHE"); d != "" {
		return d
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "bxm-memo")
	}
	return ".bxm-memo"
}

func (c *Cache) entryPath(key string) string { return filepath.Join(c.Dir, "entries", key+".json") }
func (c *Cache) jobPath(job string) string   { return filepath.Join(c.Dir, "jobs", job+".json") }
func (c *Cache) objPath(h string) string     { return filepath.Join(c.Dir, "objects", h[:2], h) }

func tmpName(dir string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return filepath.Join(dir, ".tmp-"+hex.EncodeToString(b[:]))
}

// writeAtomic writes data to path via a temporary file and a rename.
func writeAtomic(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := tmpName(dir)
	if err := os.WriteFile(tmp, data, perm); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// putFile copies a file into the object store and returns its hash and size.
func (c *Cache) putFile(src string) (string, int64, error) {
	f, err := os.Open(src)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	dir := filepath.Join(c.Dir, "objects")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
	}
	tmp := tmpName(dir)
	t, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(t, h), f)
	if cerr := t.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	dst := c.objPath(sum)
	if _, err := os.Stat(dst); err == nil {
		os.Remove(tmp)
		return sum, n, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	return sum, n, nil
}

func (c *Cache) putBytes(b []byte) (string, error) {
	sum := sha256.Sum256(b)
	h := hex.EncodeToString(sum[:])
	if _, err := os.Stat(c.objPath(h)); err == nil {
		return h, nil
	}
	return h, writeAtomic(c.objPath(h), b, 0o644)
}

// readObject returns an object's bytes after checking them against the hash.
func (c *Cache) readObject(h string) ([]byte, error) {
	b, err := os.ReadFile(c.objPath(h))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != h {
		return nil, fmt.Errorf("object %s is damaged", h[:12])
	}
	return b, nil
}

// Lookup returns the entry for key, or nil if there is none.
func (c *Cache) Lookup(key string) (*Entry, error) {
	b, err := os.ReadFile(c.entryPath(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil || e.Key != key || e.Manifest == nil {
		return nil, fmt.Errorf("entry %s is damaged", key[:12])
	}
	return &e, nil
}

// LastManifest returns the manifest of the latest stored run of a job.
func (c *Cache) LastManifest(job string) *Manifest {
	b, err := os.ReadFile(c.jobPath(job))
	if err != nil {
		return nil
	}
	var m Manifest
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return &m
}

// Drop removes an entry, for example after finding it damaged.
func (c *Cache) Drop(key string) { os.Remove(c.entryPath(key)) }

func collectOutput(path string) (Output, []string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Output{}, nil, fmt.Errorf("output %s was not produced", path)
	}
	if !info.IsDir() {
		return Output{Path: path}, []string{path}, nil
	}
	var files []string
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("output %s: symlinks are not supported", p)
		}
		if d.Type().IsRegular() {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	return Output{Path: path, IsDir: true}, files, err
}

// Store saves the outputs, captured stdout and stderr under the manifest's key.
func (c *Cache) Store(job string, m *Manifest, stdout, stderr []byte, run time.Duration) (*Entry, error) {
	e := &Entry{Key: m.Key(), Job: job, Manifest: m, Created: time.Now().UTC(), RunMS: run.Milliseconds()}
	for _, o := range m.Outputs {
		out, files, err := collectOutput(o)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			h, n, err := c.putFile(f)
			if err != nil {
				return nil, fmt.Errorf("storing %s: %w", f, err)
			}
			info, err := os.Stat(f)
			if err != nil {
				return nil, err
			}
			rel := f
			if out.IsDir {
				rel, _ = filepath.Rel(o, f)
			}
			exec := runtime.GOOS != "windows" && info.Mode()&0o111 != 0
			out.Files = append(out.Files, OutFile{Path: filepath.ToSlash(rel), Hash: h, Size: n, Exec: exec})
		}
		e.Outputs = append(e.Outputs, out)
	}
	var err error
	if len(stdout) > 0 {
		if e.Stdout, err = c.putBytes(stdout); err != nil {
			return nil, err
		}
	}
	if len(stderr) > 0 {
		if e.Stderr, err = c.putBytes(stderr); err != nil {
			return nil, err
		}
	}
	b, _ := json.MarshalIndent(e, "", " ")
	// The entry goes in last: once it exists, all its objects exist.
	if err := writeAtomic(c.entryPath(e.Key), b, 0o644); err != nil {
		return nil, err
	}
	mb, _ := json.MarshalIndent(m, "", " ")
	if err := writeAtomic(c.jobPath(job), mb, 0o644); err != nil {
		return nil, err
	}
	return e, nil
}

type staged struct {
	tmp, dst string
}

// Restore puts an entry's outputs back. Every file is read, checked against
// its hash and written to a temporary name first; only when all of them are
// good are the old outputs replaced. A damaged cache never leaves outputs
// half restored. It returns the number of files and bytes restored.
func (c *Cache) Restore(e *Entry) (int, int64, error) {
	var stage []staged
	cleanup := func() {
		for _, s := range stage {
			os.Remove(s.tmp)
		}
	}
	var bytes int64
	for _, out := range e.Outputs {
		for _, f := range out.Files {
			dst := filepath.FromSlash(f.Path)
			if out.IsDir {
				dst = filepath.Join(filepath.FromSlash(out.Path), dst)
			}
			data, err := c.readObject(f.Hash)
			if err != nil {
				cleanup()
				return 0, 0, err
			}
			dir := filepath.Dir(dst)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				cleanup()
				return 0, 0, err
			}
			perm := fs.FileMode(0o644)
			if f.Exec {
				perm = 0o755
			}
			tmp := tmpName(dir)
			if err := os.WriteFile(tmp, data, perm); err != nil {
				cleanup()
				return 0, 0, err
			}
			if f.Exec {
				os.Chmod(tmp, perm) // umask may have dropped the bits
			}
			stage = append(stage, staged{tmp, dst})
			bytes += int64(len(data))
		}
	}
	// Directory outputs are replaced as a whole: files that the cached run did
	// not produce are removed, so the result matches a fresh run.
	keep := map[string]bool{}
	for _, s := range stage {
		keep[s.dst] = true
	}
	for _, out := range e.Outputs {
		if !out.IsDir {
			continue
		}
		root := filepath.FromSlash(out.Path)
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && !keep[p] && !strings.HasPrefix(d.Name(), ".tmp-") {
				os.Remove(p)
			}
			return nil
		})
	}
	for _, s := range stage {
		if err := os.Rename(s.tmp, s.dst); err != nil {
			cleanup()
			return 0, 0, err
		}
	}
	return len(stage), bytes, nil
}

// Replay returns the captured stdout and stderr of an entry.
func (c *Cache) Replay(e *Entry) (stdout, stderr []byte, err error) {
	if e.Stdout != "" {
		if stdout, err = c.readObject(e.Stdout); err != nil {
			return nil, nil, err
		}
	}
	if e.Stderr != "" {
		if stderr, err = c.readObject(e.Stderr); err != nil {
			return nil, nil, err
		}
	}
	return stdout, stderr, nil
}

// Entries lists all readable entries, newest first.
func (c *Cache) Entries() ([]*Entry, error) {
	dir := filepath.Join(c.Dir, "entries")
	names, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Entry
	for _, n := range names {
		if !strings.HasSuffix(n.Name(), ".json") {
			continue
		}
		e, err := c.Lookup(strings.TrimSuffix(n.Name(), ".json"))
		if err == nil && e != nil {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

func (e *Entry) Size() int64 {
	var n int64
	for _, o := range e.Outputs {
		for _, f := range o.Files {
			n += f.Size
		}
	}
	return n
}

// GC removes entries older than maxAge (when > 0), then the oldest entries
// until the outputs they hold fit in maxBytes (when > 0), then every object
// and temporary file no remaining entry refers to.
func (c *Cache) GC(maxAge time.Duration, maxBytes int64) (removedEntries int, freed int64, err error) {
	entries, err := c.Entries()
	if err != nil {
		return 0, 0, err
	}
	var keep []*Entry
	var total int64
	now := time.Now()
	for _, e := range entries { // newest first
		tooOld := maxAge > 0 && now.Sub(e.Created) > maxAge
		tooBig := maxBytes > 0 && total+e.Size() > maxBytes
		if tooOld || tooBig {
			c.Drop(e.Key)
			removedEntries++
			continue
		}
		total += e.Size()
		keep = append(keep, e)
	}
	live := map[string]bool{}
	for _, e := range keep {
		for _, o := range e.Outputs {
			for _, f := range o.Files {
				live[f.Hash] = true
			}
		}
		live[e.Stdout] = true
		live[e.Stderr] = true
	}
	objects := filepath.Join(c.Dir, "objects")
	filepath.WalkDir(objects, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if live[name] {
			return nil
		}
		// Leave recent unreferenced files alone: a run that is storing right
		// now writes its objects before its entry.
		if info, err := d.Info(); err == nil && now.Sub(info.ModTime()) < time.Hour {
			return nil
		}
		{
			if info, err := d.Info(); err == nil {
				freed += info.Size()
			}
			os.Remove(p)
		}
		return nil
	})
	return removedEntries, freed, nil
}
