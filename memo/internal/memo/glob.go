package memo

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// hasMeta reports whether a path segment uses glob syntax.
func hasMeta(s string) bool { return strings.ContainsAny(s, "*?[") }

// matchSegments matches slash-separated path segments against a pattern whose
// segments may be "**" (any number of segments, including none).
func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(name); i++ {
				if matchSegments(pat[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		ok, err := path.Match(pat[0], name[0])
		if err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// expandInput turns one --in argument into the regular files it names. A plain
// file names itself, a directory names every file under it, and a pattern with
// *, ?, [...] or ** names the files it matches. Results use forward slashes.
func expandInput(arg string) ([]string, error) {
	clean := filepath.ToSlash(filepath.Clean(arg))
	segs := strings.Split(clean, "/")
	firstMeta := -1
	for i, s := range segs {
		if hasMeta(s) {
			firstMeta = i
			break
		}
	}
	if firstMeta < 0 {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, fmt.Errorf("input %s: %w", arg, err)
		}
		if !info.IsDir() {
			return []string{clean}, nil
		}
		return walkFiles(clean, nil)
	}
	for _, s := range segs {
		if s != "**" && strings.Contains(s, "**") {
			return nil, fmt.Errorf("input %s: ** must be a whole path segment", arg)
		}
		if _, err := path.Match(s, ""); err != nil {
			return nil, fmt.Errorf("input %s: bad pattern", arg)
		}
	}
	root := strings.Join(segs[:firstMeta], "/")
	if root == "" {
		root = "."
	}
	if strings.HasPrefix(clean, "/") && firstMeta == 1 {
		root = "/"
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, nil
	}
	pat := segs[firstMeta:]
	return walkFiles(root, func(rel string) bool {
		return matchSegments(pat, strings.Split(rel, "/"))
	})
}

func walkFiles(root string, keep func(rel string) bool) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Follow symlinks to files; skip anything that is not a regular file.
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("input %s: %w", p, err)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if keep == nil || keep(rel) {
			out = append(out, filepath.ToSlash(p))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}
