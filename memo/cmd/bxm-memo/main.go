// Command bxm-memo reuses the outputs of a command when its inputs have not
// changed. See https://bxm.net and the README.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/softdevstudio/bxm/memo/internal/memo"
)

// Version is raised with each release; CI checks it matches the VERSION file.
const Version = "0.1.0"

// Exit code for bxm-memo's own errors, so they are not confused with the
// wrapped command's exit codes (the same convention as env and docker).
const exitOwnError = 125

const usage = `bxm-memo: skip a command when its inputs have not changed

usage:
  bxm-memo run     [flags] -- COMMAND [ARGS...]   restore outputs on a hit, run on a miss
  bxm-memo explain [flags] -- COMMAND [ARGS...]   say hit or miss and why; runs nothing
  bxm-memo list                                   cached results, newest first
  bxm-memo gc [--older-than 30d] [--max-size 2G]  remove old entries and unused files
  bxm-memo clear                                  remove the whole cache
  bxm-memo version

flags for run and explain:
  -i, --in PATH      an input file, directory or pattern (*, ?, [..], **); repeat
  -o, --out PATH     an output file or directory the command writes; repeat
  -e, --env NAME     an environment variable that changes the result; repeat
  -n, --name NAME    a name for this job, used when explaining misses
      --salt TEXT    extra text in the key, to invalidate on purpose
      --cache DIR    cache directory (default $BXM_MEMO_CACHE or the user cache dir)
  -q, --quiet        no hit/miss lines on stderr

On a hit the outputs are restored and the command's original stdout and
stderr are replayed. A command that fails is never cached. Exit codes are the
command's own; 125 means bxm-memo itself failed.`

type flags struct {
	spec  memo.Spec
	name  string
	cache string
	quiet bool
	older time.Duration
	max   int64
	limit bool // --older-than or --max-size was given
}

func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	mult := int64(1)
	for _, u := range []struct {
		suf string
		m   int64
	}{{"K", 1 << 10}, {"M", 1 << 20}, {"G", 1 << 30}, {"T", 1 << 40}} {
		if strings.HasSuffix(s, u.suf) || strings.HasSuffix(s, u.suf+"B") {
			mult = u.m
			s = strings.TrimSuffix(strings.TrimSuffix(s, "B"), u.suf)
			break
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("bad size %q", s)
	}
	return int64(n * float64(mult)), nil
}

func parseAge(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n < 0 {
			return 0, fmt.Errorf("bad age %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func parse(args []string) (*flags, error) {
	f := &flags{cache: memo.DefaultDir()}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			f.spec.Command = args[i+1:]
			break
		}
		val := func() (string, error) {
			if eq := strings.IndexByte(a, '='); eq > 0 && strings.HasPrefix(a, "--") {
				return a[eq+1:], nil
			}
			i++
			if i >= len(args) {
				return "", fmt.Errorf("%s needs a value", a)
			}
			return args[i], nil
		}
		name := a
		if eq := strings.IndexByte(a, '='); eq > 0 && strings.HasPrefix(a, "--") {
			name = a[:eq]
		}
		var err error
		var v string
		switch name {
		case "-i", "--in":
			v, err = val()
			f.spec.Inputs = append(f.spec.Inputs, v)
		case "-o", "--out":
			v, err = val()
			f.spec.Outputs = append(f.spec.Outputs, v)
		case "-e", "--env":
			v, err = val()
			f.spec.Env = append(f.spec.Env, v)
		case "-n", "--name":
			f.name, err = val()
		case "--salt":
			f.spec.Salt, err = val()
		case "--cache":
			f.cache, err = val()
		case "-q", "--quiet":
			f.quiet = true
		case "--older-than":
			if v, err = val(); err == nil {
				f.older, err = parseAge(v)
				f.limit = true
			}
		case "--max-size":
			if v, err = val(); err == nil {
				f.max, err = parseSize(v)
				f.limit = true
			}
		default:
			return nil, fmt.Errorf("unknown flag %s (put the command after --)", a)
		}
		if err != nil {
			return nil, err
		}
	}
	return f, nil
}

func human(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Println(usage)
		return 2
	}
	cmd := args[0]
	switch cmd {
	case "help", "-h", "--help":
		fmt.Println(usage)
		return 0
	case "version", "--version":
		fmt.Printf("bxm-memo %s (cache format %d)\n", Version, memo.FormatVersion)
		return 0
	}
	f, err := parse(args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "bxm-memo:", err)
		return exitOwnError
	}
	c := &memo.Cache{Dir: f.cache}
	opts := &memo.Options{Spec: f.spec, Name: f.name, Cache: c, Quiet: f.quiet,
		Stdout: os.Stdout, Stderr: os.Stderr, Log: os.Stderr}
	switch cmd {
	case "run":
		r, err := memo.Run(opts)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bxm-memo:", err)
			return exitOwnError
		}
		return r.ExitCode
	case "explain":
		r, m, err := memo.Check(opts)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bxm-memo:", err)
			return exitOwnError
		}
		fmt.Printf("key %s\n", r.Key)
		fmt.Printf("inputs %d file(s), outputs %s\n", len(m.Inputs), strings.Join(m.Outputs, ", "))
		if r.Hit {
			fmt.Println("hit: a run with these exact inputs is cached")
			return 0
		}
		fmt.Println("miss:")
		for _, why := range r.Reasons {
			fmt.Println("  " + why)
		}
		return 1
	case "list":
		es, err := c.Entries()
		if err != nil {
			fmt.Fprintln(os.Stderr, "bxm-memo:", err)
			return exitOwnError
		}
		for _, e := range es {
			fmt.Printf("%s  %s  %-10s %-9s %s\n", e.Key[:12], e.Created.Local().Format("2006-01-02 15:04"),
				human(e.Size()), (time.Duration(e.RunMS) * time.Millisecond).String(), strings.Join(e.Manifest.Command, " "))
		}
		fmt.Printf("%d cached result(s) in %s\n", len(es), c.Dir)
		return 0
	case "gc":
		if !f.limit {
			f.older = 30 * 24 * time.Hour
		}
		older := f.older
		if f.limit && older == 0 && f.max == 0 {
			older = time.Nanosecond // --older-than 0 means everything
		}
		n, freed, err := c.GC(older, f.max)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bxm-memo:", err)
			return exitOwnError
		}
		fmt.Printf("removed %d result(s), freed %s\n", n, human(freed))
		return 0
	case "clear":
		if err := os.RemoveAll(c.Dir); err != nil {
			fmt.Fprintln(os.Stderr, "bxm-memo:", err)
			return exitOwnError
		}
		fmt.Printf("cleared %s\n", c.Dir)
		return 0
	}
	fmt.Fprintf(os.Stderr, "bxm-memo: unknown command %q; run bxm-memo help\n", cmd)
	return exitOwnError
}

func main() { os.Exit(run(os.Args[1:])) }
