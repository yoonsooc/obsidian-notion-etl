// Package vault scans an Obsidian vault for markdown notes and parses
// their YAML frontmatter using plain string functions (no regex).
package vault

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Note is a single parsed markdown note.
type Note struct {
	Filename    string            // e.g. "2025-11-05.md"
	RelPath     string            // slash-separated path relative to the scan root, set by CollectNotes
	Frontmatter map[string]string // key -> value (surrounding quotes stripped)
	Body        string            // content after the frontmatter, trimmed
}

// ParseNote splits note content into frontmatter and body.
// Frontmatter is recognized only when the first line is exactly "---" and a
// closing "---" line exists; line-level matching keeps "---" inside values or
// horizontal rules ("----") from being misread. Only "key: value" lines are
// collected; nested YAML and list values are ignored (the key is still kept).
func ParseNote(filename, content string) Note {
	note := Note{
		Filename:    filename,
		Frontmatter: make(map[string]string),
		Body:        strings.TrimSpace(content),
	}

	lines := strings.Split(content, "\n")
	if trimLineEnd(lines[0]) != "---" {
		return note
	}

	closing := -1
	for i := 1; i < len(lines); i++ {
		if trimLineEnd(lines[i]) == "---" {
			closing = i
			break
		}
	}
	if closing < 0 {
		return note
	}

	note.Frontmatter = parseFrontmatter(strings.Join(lines[1:closing], "\n"))
	note.Body = strings.TrimSpace(strings.Join(lines[closing+1:], "\n"))
	return note
}

// trimLineEnd strips trailing whitespace and carriage returns (CRLF files).
func trimLineEnd(line string) string {
	return strings.TrimRight(line, "\r \t")
}

// parseFrontmatter collects "key: value" lines from a frontmatter block.
func parseFrontmatter(block string) map[string]string {
	fm := make(map[string]string)
	for line := range strings.SplitSeq(block, "\n") {
		line = strings.TrimSpace(line)
		// Skip blank lines, list items ("- a"), and YAML comments ("# ...").
		if line == "" || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := splitKeyValue(line)
		if !ok || key == "" {
			continue
		}
		fm[key] = trimQuotes(value)
	}
	return fm
}

// splitKeyValue splits a frontmatter line into key and value.
// A quoted key may contain a colon (e.g. `"docu_type:": Plan`, a typo seen in
// real vaults), so the quoted span is taken as the key; otherwise only the
// first colon splits, preserving colons in values ("time: 10:30").
// A trailing colon inside the key is treated as a typo and removed.
func splitKeyValue(line string) (key, value string, ok bool) {
	if quote := line[0]; quote == '"' || quote == '\'' {
		if end := strings.IndexByte(line[1:], quote); end >= 0 {
			rest := strings.TrimSpace(line[end+2:])
			if after, found := strings.CutPrefix(rest, ":"); found {
				key = strings.TrimSuffix(line[1:end+1], ":")
				return key, strings.TrimSpace(after), true
			}
		}
		// Unbalanced quote or no colon after it: fall back to the plain rule.
	}
	k, v, found := strings.Cut(line, ":")
	if !found {
		return "", "", false
	}
	// Strip stray quote characters so an unbalanced quote typo
	// (`"created: ...`) does not pollute the key.
	key = strings.TrimSuffix(strings.Trim(strings.TrimSpace(k), `"'`), ":")
	return key, strings.TrimSpace(v), true
}

// trimQuotes removes one layer of matching single or double quotes.
func trimQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) ||
		(strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`)) {
		return s[1 : len(s)-1]
	}
	return s
}

// CollectNotes recursively reads *.md files under dir and returns parsed
// notes sorted by relative path. Files matching an exclude glob are skipped
// (see Excluded). Individual read failures are logged to warn and skipped.
func CollectNotes(dir string, exclude []string, warn io.Writer) ([]Note, error) {
	var notes []Note

	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable root makes the whole scan pointless; abort.
			if p == dir {
				return err
			}
			fmt.Fprintf(warn, "vault: skip %s: %v\n", p, err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			rel = d.Name()
		}

		// Prune excluded directories to avoid walking their subtrees.
		if d.IsDir() {
			if p != dir && Excluded(rel, exclude) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		if Excluded(rel, exclude) {
			return nil
		}

		content, readErr := os.ReadFile(p)
		if readErr != nil {
			fmt.Fprintf(warn, "vault: skip %s: %v\n", p, readErr)
			return nil
		}
		note := ParseNote(d.Name(), string(content))
		note.RelPath = filepath.ToSlash(rel)
		notes = append(notes, note)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("scan dir %s: %w", dir, walkErr)
	}
	// WalkDir already walks lexically; sort anyway for a deterministic order.
	sort.Slice(notes, func(i, j int) bool { return notes[i].RelPath < notes[j].RelPath })
	return notes, nil
}

// ScanFrontmatterKeys recursively reads *.md files under dir and returns the
// deduplicated, sorted list of frontmatter keys. File selection matches CollectNotes.
func ScanFrontmatterKeys(dir string, exclude []string, warn io.Writer) ([]string, error) {
	notes, err := CollectNotes(dir, exclude, warn)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	for _, note := range notes {
		for key := range note.Frontmatter {
			seen[key] = struct{}{}
		}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

// Excluded reports whether relPath matches any exclude glob, gitignore-style.
// Each pattern is tried against the full relative path, the basename, and
// every ancestor directory path; ancestor matching lets "templates" or
// "templates/*" exclude deeply nested files, since path.Match's '*' does not
// cross '/'. Both sides are NFC-normalized before comparing because macOS
// stores Korean filenames in NFD while config patterns are typically NFC.
// Pattern syntax errors are validated at config load time and ignored here.
func Excluded(relPath string, exclude []string) bool {
	rel := norm.NFC.String(filepath.ToSlash(relPath))

	// Candidates: full path, basename, and ancestor directory paths
	// (e.g. templates/2025/note.md -> [full, note.md, templates, templates/2025]).
	segments := strings.Split(rel, "/")
	candidates := make([]string, 0, len(segments)+1)
	candidates = append(candidates, rel, path.Base(rel))
	for i := 1; i < len(segments); i++ {
		candidates = append(candidates, strings.Join(segments[:i], "/"))
	}

	for _, pattern := range exclude {
		pat := strings.TrimSuffix(norm.NFC.String(pattern), "/")
		for _, candidate := range candidates {
			if ok, err := path.Match(pat, candidate); err == nil && ok {
				return true
			}
		}
	}
	return false
}
