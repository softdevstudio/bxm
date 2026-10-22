package main

// End-to-end tests: build the real binary and run it as separate processes.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

var bin string

func TestMain(m *testing.M) {
	if runtime.GOOS == "windows" {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "bxm-memo-bin")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(dir, "bxm-memo")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type env struct {
	t     *testing.T
	dir   string
	cache string
}

func newEnv(t *testing.T) *env {
	e := &env{t: t, dir: t.TempDir(), cache: filepath.Join(t.TempDir(), "cache")}
	return e
}

func (e *env) file(name, body string, perm os.FileMode) {
	p := filepath.Join(e.dir, name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), perm); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) cmd(args ...string) *exec.Cmd {
	c := exec.Command(bin, args...)
	c.Dir = e.dir
	c.Env = append(os.Environ(), "BXM_MEMO_CACHE="+e.cache)
	return c
}

func (e *env) run(args ...string) (stdout, stderr string, code int) {
	c := e.cmd(args...)
	var o, er bytes.Buffer
	c.Stdout, c.Stderr = &o, &er
	err := c.Run()
	code = 0
	if x, ok := err.(*exec.ExitError); ok {
		code = x.ExitCode()
	} else if err != nil {
		e.t.Fatal(err)
	}
	return o.String(), er.String(), code
}

func TestExitCodesPassThrough(t *testing.T) {
	e := newEnv(t)
	e.file("in.txt", "x", 0o644)
	e.file("seven.sh", "#!/bin/sh\nexit 7\n", 0o755)
	if _, _, code := e.run("run", "-i", "in.txt", "-o", "o", "--", "./seven.sh"); code != 7 {
		t.Fatalf("code %d, want 7", code)
	}
	if _, errOut, code := e.run("run", "-i", "missing/*.c", "-o", "o", "--", "true"); code != 125 || !strings.Contains(errOut, "matches no files") {
		t.Fatalf("own error: code %d, %q", code, errOut)
	}
	if _, _, code := e.run("run", "-i", "in.txt", "-o", "o", "--", "./no-such-command"); code != 125 {
		t.Fatalf("missing command: code %d", code)
	}
}

func TestManyProcessesShareOneCache(t *testing.T) {
	// Sixteen checkouts of the same job (as on sixteen CI runners or in
	// sixteen worktrees) run at once against one shared cache. Each checkout
	// has its own working directory, as it would in real use: processes that
	// write the same output path in one directory race whether Memo is there
	// or not.
	const n = 16
	cache := filepath.Join(t.TempDir(), "cache")
	envs := make([]*env, n)
	for i := range envs {
		e := &env{t: t, dir: t.TempDir(), cache: cache}
		e.file("src/a.txt", "A", 0o644)
		e.file("build.sh", "#!/bin/sh\necho run >> runs.log\nsleep 0.3\nmkdir -p out\ncat src/a.txt > out/r.txt\necho done\n", 0o755)
		envs[i] = e
	}
	var wg sync.WaitGroup
	fails := make(chan string, n)
	for _, e := range envs {
		wg.Add(1)
		go func(e *env) {
			defer wg.Done()
			out, errOut, code := e.run("run", "-q", "-i", "src", "-o", "out", "--", "./build.sh")
			b, _ := os.ReadFile(filepath.Join(e.dir, "out/r.txt"))
			if code != 0 || out != "done\n" || string(b) != "A" {
				fails <- fmt.Sprintf("code %d out %q file %q err %q", code, out, b, errOut)
			}
		}(e)
	}
	wg.Wait()
	close(fails)
	for f := range fails {
		t.Fatal(f)
	}
	e := envs[0]
	os.RemoveAll(filepath.Join(e.dir, "out"))
	_, errOut, _ := e.run("run", "-i", "src", "-o", "out", "--", "./build.sh")
	if !strings.Contains(errOut, "hit") {
		t.Fatalf("expected a hit after the parallel runs: %q", errOut)
	}
	b, _ := os.ReadFile(filepath.Join(e.dir, "out/r.txt"))
	if string(b) != "A" {
		t.Fatalf("restored %q", b)
	}
	list, _, _ := e.run("list")
	if !strings.Contains(list, "1 cached result") {
		t.Fatalf("list: %q", list)
	}
}

func TestKilledRunLeavesNoEntry(t *testing.T) {
	e := newEnv(t)
	e.file("in.txt", "x", 0o644)
	e.file("slow.sh", "#!/bin/sh\nmkdir -p out\necho partial > out/r.txt\nsleep 5\necho full > out/r.txt\n", 0o755)
	c := e.cmd("run", "-i", "in.txt", "-o", "out", "--", "./slow.sh")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	c.Process.Signal(syscall.SIGKILL)
	c.Wait()
	_, _, code := e.run("explain", "-i", "in.txt", "-o", "out", "--", "./slow.sh")
	if code != 1 {
		t.Fatal("a killed run must not leave a cache entry")
	}
}

func TestExplainSaysWhy(t *testing.T) {
	e := newEnv(t)
	e.file("a.txt", "1", 0o644)
	e.file("mk.sh", "#!/bin/sh\ncp a.txt b.txt\n", 0o755)
	args := []string{"-n", "copy", "-i", "a.txt", "-e", "BXM_E2E", "-o", "b.txt", "--", "./mk.sh"}
	e.run(append([]string{"run"}, args...)...)
	if out, _, code := e.run(append([]string{"explain"}, args...)...); code != 0 || !strings.Contains(out, "hit") {
		t.Fatalf("explain after run: %d %q", code, out)
	}
	e.file("a.txt", "2", 0o644)
	out, _, code := e.run(append([]string{"explain"}, args...)...)
	if code != 1 || !strings.Contains(out, "input a.txt changed") {
		t.Fatalf("explain after edit: %d %q", code, out)
	}
}

func TestGCAndClear(t *testing.T) {
	e := newEnv(t)
	e.file("a.txt", "1", 0o644)
	e.run("run", "-i", "a.txt", "-o", "b.txt", "--", "cp", "a.txt", "b.txt")
	out, _, code := e.run("gc", "--max-size", "0.5K")
	if code != 0 || !strings.Contains(out, "removed 0 result") {
		t.Fatalf("gc: %d %q", code, out)
	}
	out, _, _ = e.run("gc", "--older-than", "0s")
	if !strings.Contains(out, "removed 1 result") {
		t.Fatalf("gc older-than: %q", out)
	}
	if out, _, code := e.run("clear"); code != 0 || !strings.Contains(out, "cleared") {
		t.Fatalf("clear: %q", out)
	}
	if _, err := os.Stat(e.cache); !os.IsNotExist(err) {
		t.Fatal("cache still there after clear")
	}
}
