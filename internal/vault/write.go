package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NoteDraft is the markdown note artifact a backup run produces (the reverse
// direction's counterpart of a parsed Note). Frontmatter is ordered so the
// written file is deterministic.
type NoteDraft struct {
	FileName    string // e.g. "2025-11-05.md"
	Frontmatter []FrontmatterField
	Body        string
}

// FrontmatterField is one "key: value" frontmatter line.
type FrontmatterField struct {
	Key   string
	Value string
}

// Render serializes the draft to file content: a frontmatter block (when any
// field is set) followed by the body and a trailing newline.
func (d NoteDraft) Render() string {
	var sb strings.Builder
	if len(d.Frontmatter) > 0 {
		sb.WriteString("---\n")
		for _, f := range d.Frontmatter {
			sb.WriteString(f.Key)
			sb.WriteString(": ")
			sb.WriteString(f.Value)
			sb.WriteString("\n")
		}
		sb.WriteString("---\n\n")
	}
	sb.WriteString(d.Body)
	sb.WriteString("\n")
	return sb.String()
}

// WriteNote writes the draft into dir, overwriting any existing file: the
// backup directory is a mirror of its source of truth. The write is atomic
// (temp file + rename) so a mid-write failure never leaves a corrupt note.
func WriteNote(dir string, draft NoteDraft) error {
	if draft.FileName == "" {
		return fmt.Errorf("빈 파일명으로는 노트를 쓸 수 없음")
	}
	path := filepath.Join(dir, draft.FileName)

	tmp, err := os.CreateTemp(dir, draft.FileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("임시 파일 생성 실패: %w", err)
	}
	tmpPath := tmp.Name()

	_, writeErr := tmp.WriteString(draft.Render())
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		if writeErr != nil {
			return fmt.Errorf("노트 쓰기 실패 %s: %w", draft.FileName, writeErr)
		}
		return fmt.Errorf("노트 닫기 실패 %s: %w", draft.FileName, closeErr)
	}
	// CreateTemp uses 0600; match the usual 0644.
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("노트 권한 설정 실패 %s: %w", draft.FileName, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("노트 저장 실패 %s: %w", draft.FileName, err)
	}
	return nil
}

// fileNameReplacer maps filesystem- and Obsidian-hostile characters to "-".
var fileNameReplacer = strings.NewReplacer(
	"/", "-", "\\", "-", ":", "-", "*", "-", "?", "-",
	"\"", "-", "<", "-", ">", "-", "|", "-", "#", "-", "^", "-", "[", "-", "]", "-",
)

// SanitizeFileName makes a note title safe to use as a file name: forbidden
// characters become "-" and surrounding whitespace/dots are trimmed. An empty
// result is the caller's problem (fall back to another name source).
func SanitizeFileName(name string) string {
	cleaned := fileNameReplacer.Replace(name)
	return strings.Trim(cleaned, " .\t")
}
