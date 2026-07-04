package store

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func readText(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// sortedSubdirs returns the immediate subdirectory names of dir (excluding
// dotfile-prefixed names), sorted lexically.
func sortedSubdirs(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// sortedMarkdown returns the *.md file names in dir, sorted lexically.
func sortedMarkdown(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func stem(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// firstPreview returns the first non-empty, non-heading line trimmed to 120 chars.
func firstPreview(text string) string {
	for _, line := range strings.Split(text, "\n") {
		s := strings.TrimSpace(line)
		if s != "" && !strings.HasPrefix(s, "#") {
			if len(s) > 120 {
				return s[:120]
			}
			return s
		}
	}
	return ""
}

// titleCase capitalises the first letter of each space-separated word.
func titleCase(s string) string {
	words := strings.Split(s, " ")
	for i, w := range words {
		if w == "" {
			continue
		}
		r := []rune(w)
		words[i] = strings.ToUpper(string(r[0])) + strings.ToLower(string(r[1:]))
	}
	return strings.Join(words, " ")
}

// hookText replaces '-' and '_' with spaces (no case change).
func hookText(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "-", " "), "_", " ")
}

func dirEmpty(dir string) bool {
	if !isDir(dir) {
		return false
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(ents) == 0
}
