package memo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
)

// FormatVersion changes whenever the meaning of a key changes, so old cache
// entries are never confused with new ones.
const FormatVersion = 1

// Input is one input file in a manifest.
type Input struct {
	Path string `json:"path"`
	Hash string `json:"sha256"`
	Exec bool   `json:"exec,omitempty"`
}

// Manifest is everything that decides whether a command's result can be
// reused. Two runs with equal manifests are expected to produce the same
// outputs. The key is the SHA-256 of the manifest's JSON.
type Manifest struct {
	Version int                `json:"v"`
	Command []string           `json:"cmd"`
	Env     map[string]*string `json:"env"`
	Inputs  []Input            `json:"inputs"`
	Outputs []string           `json:"outputs"`
	OS      string             `json:"os"`
	Arch    string             `json:"arch"`
	Salt    string             `json:"salt,omitempty"`
}

// Spec is what the user declared on the command line.
type Spec struct {
	Command []string
	Inputs  []string // files, directories or patterns
	Outputs []string
	Env     []string
	Salt    string
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Build resolves the spec against the file system and returns its manifest.
func Build(s Spec) (*Manifest, error) {
	if len(s.Command) == 0 {
		return nil, fmt.Errorf("no command given (put it after --)")
	}
	if len(s.Outputs) == 0 {
		return nil, fmt.Errorf("declare at least one output with -o")
	}
	m := &Manifest{
		Version: FormatVersion,
		Command: s.Command,
		Env:     map[string]*string{},
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Salt:    s.Salt,
	}
	for _, name := range s.Env {
		if v, ok := os.LookupEnv(name); ok {
			v := v
			m.Env[name] = &v
		} else {
			m.Env[name] = nil
		}
	}
	seen := map[string]bool{}
	for _, arg := range s.Inputs {
		files, err := expandInput(arg)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("input %s matches no files", arg)
		}
		for _, f := range files {
			if seen[f] {
				continue
			}
			seen[f] = true
			h, err := hashFile(f)
			if err != nil {
				return nil, fmt.Errorf("input %s: %w", f, err)
			}
			info, err := os.Stat(f)
			if err != nil {
				return nil, err
			}
			exec := runtime.GOOS != "windows" && info.Mode()&0o111 != 0
			m.Inputs = append(m.Inputs, Input{Path: f, Hash: h, Exec: exec})
		}
	}
	sort.Slice(m.Inputs, func(i, j int) bool { return m.Inputs[i].Path < m.Inputs[j].Path })
	outs := map[string]bool{}
	for _, o := range s.Outputs {
		o = cleanRel(o)
		if outs[o] {
			continue
		}
		outs[o] = true
		m.Outputs = append(m.Outputs, o)
	}
	sort.Strings(m.Outputs)
	for _, o := range m.Outputs {
		if seen[o] {
			return nil, fmt.Errorf("%s is declared as both an input and an output", o)
		}
		for _, in := range m.Inputs {
			if strings.HasPrefix(in.Path, o+"/") {
				return nil, fmt.Errorf("input %s is inside output %s", in.Path, o)
			}
		}
	}
	return m, nil
}

// Key is the cache key of the manifest. encoding/json writes map keys in
// sorted order and slices are already sorted, so the bytes are stable.
func (m *Manifest) Key() string {
	b, _ := json.Marshal(m)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// JobID names "the same job" across runs whose inputs differ, so a miss can
// be explained against the previous run. It is the name if one was given,
// otherwise a hash of the command and the outputs.
func JobID(name string, m *Manifest) string {
	if name != "" {
		return "name-" + safeName(name)
	}
	b, _ := json.Marshal([]any{m.Command, m.Outputs})
	sum := sha256.Sum256(b)
	return "job-" + hex.EncodeToString(sum[:8])
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// Explain lists why `now` differs from `before`, most useful reasons first.
func Explain(before, now *Manifest) []string {
	if before == nil {
		return []string{"no earlier run of this job in the cache"}
	}
	var r []string
	if strings.Join(before.Command, "\x00") != strings.Join(now.Command, "\x00") {
		r = append(r, "the command changed")
	}
	if before.Salt != now.Salt {
		r = append(r, "the salt changed")
	}
	if before.OS != now.OS || before.Arch != now.Arch {
		r = append(r, fmt.Sprintf("platform changed from %s/%s to %s/%s", before.OS, before.Arch, now.OS, now.Arch))
	}
	if before.Version != now.Version {
		r = append(r, "cache format changed")
	}
	old := map[string]Input{}
	for _, in := range before.Inputs {
		old[in.Path] = in
	}
	cur := map[string]bool{}
	for _, in := range now.Inputs {
		cur[in.Path] = true
		o, ok := old[in.Path]
		switch {
		case !ok:
			r = append(r, "input "+in.Path+" is new")
		case o.Hash != in.Hash:
			r = append(r, "input "+in.Path+" changed")
		case o.Exec != in.Exec:
			r = append(r, "input "+in.Path+" changed its executable bit")
		}
	}
	for _, in := range before.Inputs {
		if !cur[in.Path] {
			r = append(r, "input "+in.Path+" is gone")
		}
	}
	names := map[string]bool{}
	for k := range before.Env {
		names[k] = true
	}
	for k := range now.Env {
		names[k] = true
	}
	var sorted []string
	for k := range names {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		a, aok := before.Env[k]
		b, bok := now.Env[k]
		switch {
		case !aok:
			r = append(r, "env "+k+" is newly declared")
		case !bok:
			r = append(r, "env "+k+" is no longer declared")
		case (a == nil) != (b == nil) || (a != nil && *a != *b):
			r = append(r, "env "+k+" changed")
		}
	}
	if strings.Join(before.Outputs, "\x00") != strings.Join(now.Outputs, "\x00") {
		r = append(r, "the declared outputs changed")
	}
	if len(r) == 0 {
		r = append(r, "inputs match the last run, but its cache entry is missing or damaged")
	}
	return r
}
