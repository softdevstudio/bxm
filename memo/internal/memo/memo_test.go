package memo

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// project makes a temporary project directory, makes it the working
// directory for the test and returns a cache in a separate directory.
func project(t *testing.T, files map[string]string) *Cache {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("tests use sh scripts")
	}
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
	for name, body := range files {
		os.MkdirAll(filepath.Dir(name), 0o755)
		perm := os.FileMode(0o644)
		if strings.HasSuffix(name, ".sh") {
			perm = 0o755
		}
		if err := os.WriteFile(name, []byte(body), perm); err != nil {
			t.Fatal(err)
		}
	}
	return &Cache{Dir: filepath.Join(t.TempDir(), "cache")}
}

func write(t *testing.T, name, body string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type runner struct {
	t     *testing.T
	cache *Cache
	spec  Spec
	name  string
	out   bytes.Buffer
	err   bytes.Buffer
	log   bytes.Buffer
}

func (r *runner) run() *Result {
	r.t.Helper()
	r.out.Reset()
	r.err.Reset()
	r.log.Reset()
	res, err := Run(&Options{Spec: r.spec, Name: r.name, Cache: r.cache,
		Stdout: &r.out, Stderr: &r.err, Log: &r.log})
	if err != nil {
		r.t.Fatalf("run: %v", err)
	}
	return res
}

// The build script counts its own runs in a file outside the outputs.
const build = `#!/bin/sh
echo run >> runs.log
mkdir -p out
cat src/*.txt > out/all.txt
printf 'made\n'
printf 'note\n' >&2
`

func runs(t *testing.T) int {
	b, _ := os.ReadFile("runs.log")
	return strings.Count(string(b), "run")
}

func TestHitRestoresOutputsAndReplaysOutput(t *testing.T) {
	c := project(t, map[string]string{"build.sh": build, "src/a.txt": "A\n", "src/b.txt": "B\n"})
	r := &runner{t: t, cache: c, spec: Spec{Command: []string{"./build.sh"},
		Inputs: []string{"src", "build.sh"}, Outputs: []string{"out"}}}

	if res := r.run(); res.Hit || !res.Stored {
		t.Fatalf("first run: %+v", res)
	}
	os.RemoveAll("out")
	res := r.run()
	if !res.Hit || res.Restored != 1 {
		t.Fatalf("second run should hit: %+v", res)
	}
	if runs(t) != 1 {
		t.Fatalf("command ran %d times, want 1", runs(t))
	}
	if got := read(t, "out/all.txt"); got != "A\nB\n" {
		t.Fatalf("restored %q", got)
	}
	if r.out.String() != "made\n" || r.err.String() != "note\n" {
		t.Fatalf("replayed %q %q", r.out.String(), r.err.String())
	}
}

func TestMissReasons(t *testing.T) {
	c := project(t, map[string]string{"build.sh": build, "src/a.txt": "A\n", "src/b.txt": "B\n"})
	r := &runner{t: t, cache: c, name: "cat", spec: Spec{Command: []string{"./build.sh"},
		Inputs: []string{"src/*.txt"}, Outputs: []string{"out"}, Env: []string{"BXM_TEST_MODE"}}}
	r.run()

	write(t, "src/b.txt", "B2\n")
	write(t, "src/c.txt", "C\n")
	os.Remove("src/a.txt")
	t.Setenv("BXM_TEST_MODE", "fast")
	res := r.run()
	want := []string{"input src/b.txt changed", "input src/c.txt is new", "input src/a.txt is gone", "env BXM_TEST_MODE changed"}
	if strings.Join(res.Reasons, "|") != strings.Join(want, "|") {
		t.Fatalf("reasons %q", res.Reasons)
	}
	if !strings.Contains(r.log.String(), "miss cat: input src/b.txt changed") {
		t.Fatalf("log %q", r.log.String())
	}
	r.spec.Command = []string{"./build.sh", "--extra"}
	if res := r.run(); res.Reasons[0] != "the command changed" {
		t.Fatalf("reasons %q", res.Reasons)
	}
}

func TestKeyDoesNotDependOnFlagOrder(t *testing.T) {
	project(t, map[string]string{"a.txt": "a", "b.txt": "b"})
	m1, err := Build(Spec{Command: []string{"x"}, Inputs: []string{"a.txt", "b.txt"}, Outputs: []string{"o1", "o2"}, Env: []string{"A", "B"}})
	if err != nil {
		t.Fatal(err)
	}
	m2, _ := Build(Spec{Command: []string{"x"}, Inputs: []string{"*.txt", "b.txt"}, Outputs: []string{"o2", "./o1"}, Env: []string{"B", "A"}})
	if m1.Key() != m2.Key() {
		t.Fatal("same inputs in another order gave another key")
	}
	m3, _ := Build(Spec{Command: []string{"x"}, Inputs: []string{"a.txt", "b.txt"}, Outputs: []string{"o1", "o2"}, Env: []string{"A", "B"}, Salt: "1"})
	if m1.Key() == m3.Key() {
		t.Fatal("salt did not change the key")
	}
}

func TestFailedCommandIsNotCached(t *testing.T) {
	c := project(t, map[string]string{"fail.sh": "#!/bin/sh\necho run >> runs.log\nmkdir -p out; echo x > out/f\nexit 3\n", "in.txt": "i"})
	r := &runner{t: t, cache: c, spec: Spec{Command: []string{"./fail.sh"}, Inputs: []string{"in.txt"}, Outputs: []string{"out"}}}
	for i := 0; i < 2; i++ {
		res := r.run()
		if res.ExitCode != 3 || res.Stored || res.Hit {
			t.Fatalf("run %d: %+v", i, res)
		}
	}
	if runs(t) != 2 {
		t.Fatalf("runs %d", runs(t))
	}
}

func TestMissingOutputIsNotCached(t *testing.T) {
	c := project(t, map[string]string{"in.txt": "i"})
	r := &runner{t: t, cache: c, spec: Spec{Command: []string{"true"}, Inputs: []string{"in.txt"}, Outputs: []string{"nothing"}}}
	res := r.run()
	if res.Stored || !strings.Contains(res.NotStoredWhy, "did not produce") {
		t.Fatalf("%+v", res)
	}
}

func TestDamagedObjectFallsBackToRunning(t *testing.T) {
	c := project(t, map[string]string{"build.sh": build, "src/a.txt": "A\n"})
	r := &runner{t: t, cache: c, spec: Spec{Command: []string{"./build.sh"}, Inputs: []string{"src"}, Outputs: []string{"out"}}}
	r.run()
	// Damage every object in the cache.
	filepath.Walk(filepath.Join(c.Dir, "objects"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			os.WriteFile(p, []byte("garbage"), 0o644)
		}
		return nil
	})
	write(t, "out/all.txt", "kept until the rerun\n")
	res := r.run()
	if res.Hit || !res.Stored {
		t.Fatalf("damaged cache should rerun and store again: %+v", res)
	}
	if !strings.Contains(r.log.String(), "damaged") {
		t.Fatalf("log %q", r.log.String())
	}
	if got := read(t, "out/all.txt"); got != "A\n" || runs(t) != 2 {
		t.Fatalf("got %q after %d runs", got, runs(t))
	}
}

func TestRestoreLeavesOutputsAloneWhenAnyObjectIsBad(t *testing.T) {
	c := project(t, map[string]string{"in.txt": "i",
		"two.sh": "#!/bin/sh\nmkdir -p out; echo one > out/1; echo two > out/2\n"})
	m, _ := Build(Spec{Command: []string{"./two.sh"}, Inputs: []string{"in.txt"}, Outputs: []string{"out"}})
	r := &runner{t: t, cache: c, spec: Spec{Command: []string{"./two.sh"}, Inputs: []string{"in.txt"}, Outputs: []string{"out"}}}
	r.run()
	e, _ := c.Lookup(m.Key())
	// Damage only the second file's object.
	os.WriteFile(c.objPath(e.Outputs[0].Files[1].Hash), []byte("bad"), 0o644)
	write(t, "out/1", "old one\n")
	if _, _, err := c.Restore(e); err == nil {
		t.Fatal("restore should fail")
	}
	if got := read(t, "out/1"); got != "old one\n" {
		t.Fatalf("out/1 was half restored: %q", got)
	}
	left, _ := filepath.Glob("out/.tmp-*")
	if len(left) != 0 {
		t.Fatalf("temporary files left behind: %v", left)
	}
}

func TestDirectoryOutputMatchesCachedRun(t *testing.T) {
	c := project(t, map[string]string{"build.sh": build, "src/a.txt": "A\n"})
	r := &runner{t: t, cache: c, spec: Spec{Command: []string{"./build.sh"}, Inputs: []string{"src"}, Outputs: []string{"out"}}}
	r.run()
	write(t, "out/stale.txt", "left from something else")
	if res := r.run(); !res.Hit {
		t.Fatal("want hit")
	}
	if _, err := os.Stat("out/stale.txt"); err == nil {
		t.Fatal("stale file in a directory output survived the restore")
	}
}

func TestExecutableBitIsRestored(t *testing.T) {
	c := project(t, map[string]string{"in.txt": "i",
		"mk.sh": "#!/bin/sh\nprintf '#!/bin/sh\\necho hi\\n' > tool; chmod +x tool\n"})
	r := &runner{t: t, cache: c, spec: Spec{Command: []string{"./mk.sh"}, Inputs: []string{"in.txt"}, Outputs: []string{"tool"}}}
	r.run()
	os.Remove("tool")
	if res := r.run(); !res.Hit {
		t.Fatal("want hit")
	}
	info, err := os.Stat("tool")
	if err != nil || info.Mode()&0o100 == 0 {
		t.Fatalf("tool not executable after restore: %v %v", info.Mode(), err)
	}
}

func TestInputChangedDuringRunIsNotStored(t *testing.T) {
	c := project(t, map[string]string{"in.txt": "before",
		"edit.sh": "#!/bin/sh\necho during > in.txt\necho x > out.txt\n"})
	r := &runner{t: t, cache: c, spec: Spec{Command: []string{"./edit.sh"}, Inputs: []string{"in.txt"}, Outputs: []string{"out.txt"}}}
	res := r.run()
	if res.Stored || !strings.Contains(res.NotStoredWhy, "changed while") {
		t.Fatalf("%+v", res)
	}
}

func TestInputPatternMustMatch(t *testing.T) {
	c := project(t, map[string]string{"a.txt": "a"})
	_, err := Run(&Options{Spec: Spec{Command: []string{"true"}, Inputs: []string{"src/**/*.go"}, Outputs: []string{"o"}},
		Cache: c, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "matches no files") {
		t.Fatalf("err %v", err)
	}
	_, err = Build(Spec{Command: []string{"x"}, Inputs: []string{"a.txt"}, Outputs: []string{"a.txt"}})
	if err == nil {
		t.Fatal("an output that is also an input should be refused")
	}
}

func TestGlobs(t *testing.T) {
	project(t, map[string]string{"src/a.go": "", "src/x/b.go": "", "src/x/y/c.go": "", "src/x/c.txt": "", "top.go": ""})
	cases := map[string]string{
		"src/**/*.go": "src/a.go src/x/b.go src/x/y/c.go",
		"src/*/*.go":  "src/x/b.go",
		"**/c.*":      "src/x/c.txt src/x/y/c.go",
		"*.go":        "top.go",
		"src/x":       "src/x/b.go src/x/c.txt src/x/y/c.go",
		"src/[ab].go": "src/a.go",
	}
	for pat, want := range cases {
		got, err := expandInput(pat)
		if err != nil {
			t.Fatalf("%s: %v", pat, err)
		}
		if strings.Join(got, " ") != want {
			t.Errorf("%s: got %q want %q", pat, got, want)
		}
	}
}

func TestConcurrentRunsOfTheSameJob(t *testing.T) {
	c := project(t, map[string]string{"in.txt": "i",
		"slow.sh": "#!/bin/sh\nsleep 0.2\nmkdir -p out\necho result > out/r.txt\n"})
	spec := Spec{Command: []string{"./slow.sh"}, Inputs: []string{"in.txt"}, Outputs: []string{"out"}}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Run(&Options{Spec: spec, Cache: c, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	os.RemoveAll("out")
	r := &runner{t: t, cache: c, spec: spec}
	if res := r.run(); !res.Hit || read(t, "out/r.txt") != "result\n" {
		t.Fatalf("after concurrent stores: %+v", res)
	}
}

func TestGCKeepsNewestWithinSize(t *testing.T) {
	c := project(t, map[string]string{"in.txt": "1",
		"big.sh": "#!/bin/sh\nhead -c 1000 /dev/zero > out.bin; cat in.txt >> out.bin\n"})
	r := &runner{t: t, cache: c, spec: Spec{Command: []string{"./big.sh"}, Inputs: []string{"in.txt"}, Outputs: []string{"out.bin"}}}
	for _, v := range []string{"1", "2", "3"} {
		write(t, "in.txt", v)
		r.run()
		time.Sleep(10 * time.Millisecond)
	}
	// Age every object so GC is allowed to remove unreferenced ones.
	old := time.Now().Add(-2 * time.Hour)
	filepath.Walk(filepath.Join(c.Dir, "objects"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			os.Chtimes(p, old, old)
		}
		return nil
	})
	n, freed, err := c.GC(0, 1500)
	if err != nil || n != 2 || freed < 2000 {
		t.Fatalf("gc removed %d, freed %d, err %v", n, freed, err)
	}
	if res := r.run(); !res.Hit {
		t.Fatal("newest result should survive gc")
	}
	write(t, "in.txt", "1")
	if res := r.run(); res.Hit {
		t.Fatal("oldest result should be gone")
	}
}

func TestLeftoversFromAKilledStoreAreIgnored(t *testing.T) {
	c := project(t, map[string]string{"build.sh": build, "src/a.txt": "A\n"})
	// What a store killed half way leaves: temporary files and objects with no entry.
	os.MkdirAll(filepath.Join(c.Dir, "objects", "ab"), 0o755)
	os.MkdirAll(filepath.Join(c.Dir, "entries"), 0o755)
	write(t, filepath.Join(c.Dir, "objects", ".tmp-deadbeef"), "partial")
	write(t, filepath.Join(c.Dir, "objects", "ab", "ab00"), "orphan")
	write(t, filepath.Join(c.Dir, "entries", ".tmp-cafe"), "{half a json")
	r := &runner{t: t, cache: c, spec: Spec{Command: []string{"./build.sh"}, Inputs: []string{"src"}, Outputs: []string{"out"}}}
	if res := r.run(); res.Hit || !res.Stored {
		t.Fatalf("%+v", res)
	}
	if res := r.run(); !res.Hit {
		t.Fatal("want hit")
	}
	if es, _ := c.Entries(); len(es) != 1 {
		t.Fatalf("entries %d", len(es))
	}
}

func TestOutputsMustStayInsideTheWorkingDirectory(t *testing.T) {
	project(t, map[string]string{"in.txt": "i", "sub/x.txt": "x"})
	for _, o := range []string{".", "./", "..", "../x", "/tmp/x", "sub/../.."} {
		if _, err := Build(Spec{Command: []string{"true"}, Inputs: []string{"in.txt"}, Outputs: []string{o}}); err == nil {
			t.Errorf("output %q was accepted", o)
		}
	}
	if _, err := Build(Spec{Command: []string{"true"}, Inputs: []string{"sub"}, Outputs: []string{"sub"}}); err == nil {
		t.Error("an output that contains an input was accepted")
	}
	if _, err := Build(Spec{Command: []string{"true"}, Inputs: []string{"in.txt"}, Outputs: []string{"out", "out/a"}}); err == nil {
		t.Error("nested outputs were accepted")
	}
}

func TestInputsBehindDirectorySymlinksAreHashed(t *testing.T) {
	c := project(t, map[string]string{"real/a.txt": "1", "cp.sh": "#!/bin/sh\ncat src/linked/a.txt > out.txt\n"})
	os.MkdirAll("src", 0o755)
	if err := os.Symlink("../real", "src/linked"); err != nil {
		t.Skip("no symlinks here")
	}
	os.Symlink("..", "src/loop") // a loop must not hang the walk
	r := &runner{t: t, cache: c, spec: Spec{Command: []string{"./cp.sh"}, Inputs: []string{"src"}, Outputs: []string{"out.txt"}}}
	r.run()
	write(t, "real/a.txt", "2")
	if res := r.run(); res.Hit {
		t.Fatal("a change behind a directory symlink gave a stale hit")
	}
	if read(t, "out.txt") != "2" {
		t.Fatal("wrong output")
	}
	os.Symlink("nowhere", "src/broken")
	if _, err := Build(r.spec); err == nil || !strings.Contains(err.Error(), "broken link") {
		t.Fatalf("a broken link should be an error, got %v", err)
	}
}

func TestPlantedEntriesAreRefused(t *testing.T) {
	c := project(t, map[string]string{"in.txt": "i", "mk.sh": "#!/bin/sh\nmkdir -p out; echo x > out/f\n"})
	spec := Spec{Command: []string{"./mk.sh"}, Inputs: []string{"in.txt"}, Outputs: []string{"out"}}
	r := &runner{t: t, cache: c, spec: spec}
	r.run()
	m, _ := Build(spec)
	path := c.entryPath(m.Key())
	orig, _ := os.ReadFile(path)
	for name, edit := range map[string]func(string) string{
		"escaping file": func(s string) string { return strings.Replace(s, `"path": "f"`, `"path": "../../escape.txt"`, 1) },
		"other output":  func(s string) string { return strings.Replace(s, `"path": "out"`, `"path": "elsewhere"`, 1) },
		"short hash": func(s string) string {
			i := strings.Index(s, `"sha256": "`) + len(`"sha256": "`)
			return s[:i] + "a" + s[i+64:]
		},
	} {
		os.WriteFile(path, []byte(edit(string(orig))), 0o644)
		res := r.run() // must not panic or write outside
		if res.Hit {
			t.Errorf("%s: planted entry was used", name)
		}
		if _, err := os.Stat("../escape.txt"); err == nil {
			t.Fatalf("%s: wrote outside the working directory", name)
		}
	}
}

func TestEmptyDirectoryOutputIsRestored(t *testing.T) {
	c := project(t, map[string]string{"in.txt": "i"})
	r := &runner{t: t, cache: c, spec: Spec{Command: []string{"mkdir", "-p", "emptyd"}, Inputs: []string{"in.txt"}, Outputs: []string{"emptyd"}}}
	r.run()
	os.RemoveAll("emptyd")
	if res := r.run(); !res.Hit {
		t.Fatal("want hit")
	}
	if info, err := os.Stat("emptyd"); err != nil || !info.IsDir() {
		t.Fatal("empty directory output not restored")
	}
}
