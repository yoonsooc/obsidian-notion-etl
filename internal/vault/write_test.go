package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteNoteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	draft := NoteDraft{
		FileName: "2026-08-29.md",
		Frontmatter: []FrontmatterField{
			{Key: "source_id", Value: "abc-123"},
			{Key: "last_edited", Value: "2026-08-29T10:00:00Z"},
			{Key: "source", Value: "cloud"},
		},
		Body: "# 제목\n\n본문",
	}
	if err := WriteNote(dir, draft); err != nil {
		t.Fatalf("WriteNote() error = %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, draft.FileName))
	if err != nil {
		t.Fatalf("stat written note: %v", err)
	}
	// CreateTemp's 0600 must be widened to the project-wide 0644.
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("perm = %o, want 644", perm)
	}

	content, err := os.ReadFile(filepath.Join(dir, draft.FileName))
	if err != nil {
		t.Fatalf("read written note: %v", err)
	}
	// The written file must parse back with this package's own parser.
	note := ParseNote(draft.FileName, string(content))
	if note.Frontmatter["source_id"] != "abc-123" {
		t.Errorf("source_id = %q, want abc-123", note.Frontmatter["source_id"])
	}
	if note.Frontmatter["last_edited"] != "2026-08-29T10:00:00Z" {
		t.Errorf("last_edited = %q (콜론 포함 값 보존 실패)", note.Frontmatter["last_edited"])
	}
	if note.Body != "# 제목\n\n본문" {
		t.Errorf("body = %q", note.Body)
	}
}

func TestWriteNoteOverwrites(t *testing.T) {
	dir := t.TempDir()
	first := NoteDraft{FileName: "note.md", Body: "old"}
	second := NoteDraft{FileName: "note.md", Body: "new"}
	if err := WriteNote(dir, first); err != nil {
		t.Fatalf("WriteNote(first) error = %v", err)
	}
	if err := WriteNote(dir, second); err != nil {
		t.Fatalf("WriteNote(second) error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "note.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	if string(content) != "new\n" {
		t.Errorf("content = %q, want overwrite to %q", content, "new\n")
	}
}

func TestWriteNoteNoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	if err := WriteNote(dir, NoteDraft{FileName: "plain.md", Body: "본문만"}); err != nil {
		t.Fatalf("WriteNote() error = %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, "plain.md"))
	if string(content) != "본문만\n" {
		t.Errorf("content = %q, want body only", content)
	}
}

func TestWriteNoteEmptyFileName(t *testing.T) {
	if err := WriteNote(t.TempDir(), NoteDraft{}); err == nil {
		t.Fatal("WriteNote(empty name) must error")
	}
}

func TestSanitizeFileName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"2026/08: 계획*안?", "2026-08- 계획-안-"},
		{"  공백 정리  ", "공백 정리"},
		{"trailing.", "trailing"},
		{"한글 제목", "한글 제목"},
		{`a\b"c<d>e|f#g^h[i]j`, "a-b-c-d-e-f-g-h-i-j"},
		{"///", "---"},
	}
	for _, tt := range tests {
		if got := SanitizeFileName(tt.in); got != tt.want {
			t.Errorf("SanitizeFileName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
