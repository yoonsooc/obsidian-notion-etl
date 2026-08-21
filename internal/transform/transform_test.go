package transform

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestDateDeriver verifies rule order and fallback of date derivation.
func TestDateDeriver(t *testing.T) {
	rules := []DateRule{
		{FileLayout: "DN_060102"},
		{FrontmatterKey: "created_date"},
	}

	tests := []struct {
		name         string
		note         Note
		wantDate     string
		wantWarnings int
	}{
		{
			name:     "파일명 레이아웃 성공",
			note:     Note{Filename: "DN_251101.md"},
			wantDate: "2025-11-01",
		},
		{
			name: "파일명 실패 시 프론트매터 폴백",
			note: Note{
				Filename:    "일기.md",
				Frontmatter: map[string]string{"created_date": "2025-11-02"},
			},
			wantDate: "2025-11-02",
		},
		{
			name: "프론트매터 RFC3339 값은 앞 10자로 재시도",
			note: Note{
				Filename:    "일기.md",
				Frontmatter: map[string]string{"created_date": "2025-11-03T10:30:00+09:00"},
			},
			wantDate: "2025-11-03",
		},
		{
			name:         "전부 실패 시 Date 비움과 경고",
			note:         Note{Filename: "메모.md", Frontmatter: map[string]string{}},
			wantDate:     "",
			wantWarnings: 1,
		},
		{
			name: "프론트매터 값이 날짜가 아니면 실패",
			note: Note{
				Filename:    "메모.md",
				Frontmatter: map[string]string{"created_date": "날짜아님텍스트값임"},
			},
			wantDate:     "",
			wantWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draft := &PageDraft{Properties: make(map[string]string)}
			if err := NewDateDeriver(rules).Transform(tt.note, draft); err != nil {
				t.Fatalf("Transform 에러: %v", err)
			}
			if draft.Date != tt.wantDate {
				t.Errorf("Date = %q, want %q", draft.Date, tt.wantDate)
			}
			if len(draft.Warnings) != tt.wantWarnings {
				t.Errorf("Warnings = %v, want %d개", draft.Warnings, tt.wantWarnings)
			}
		})
	}
}

// TestTitleFromFilename verifies the filename-based title rule.
func TestTitleFromFilename(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		wantTitle string
	}{
		{name: "md 확장자 제거", filename: "DN_251101.md", wantTitle: "DN_251101"},
		{name: "한글 파일명", filename: "하루 정리.md", wantTitle: "하루 정리"},
		{name: "확장자 없는 파일명은 그대로", filename: "README", wantTitle: "README"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draft := &PageDraft{Properties: make(map[string]string)}
			if err := NewTitleFromFilename().Transform(Note{Filename: tt.filename}, draft); err != nil {
				t.Fatalf("Transform 에러: %v", err)
			}
			if draft.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", draft.Title, tt.wantTitle)
			}
		})
	}
}

// TestObsidianURI verifies obsidian:// link assembly and encoding.
func TestObsidianURI(t *testing.T) {
	tests := []struct {
		name      string
		vaultName string
		target    string
		relPath   string
		wantURI   string
	}{
		{
			name:      "한글 경로 인코딩",
			vaultName: "Yersona",
			target:    "100. Inbox/Work/Daily",
			relPath:   "2025-11/하루 정리.md",
			wantURI: "obsidian://open?vault=Yersona&file=" +
				"100.%20Inbox%2FWork%2FDaily%2F2025-11%2F%ED%95%98%EB%A3%A8%20%EC%A0%95%EB%A6%AC",
		},
		{
			name:      "target 없이 상대경로만",
			vaultName: "My Vault",
			target:    "",
			relPath:   "DN_251101.md",
			wantURI:   "obsidian://open?vault=My%20Vault&file=DN_251101",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draft := &PageDraft{Properties: make(map[string]string)}
			tr := NewObsidianURI(tt.vaultName, tt.target)
			if err := tr.Transform(Note{RelPath: tt.relPath}, draft); err != nil {
				t.Fatalf("Transform 에러: %v", err)
			}
			if draft.ObsidianURI != tt.wantURI {
				t.Errorf("ObsidianURI = %q, want %q", draft.ObsidianURI, tt.wantURI)
			}
		})
	}
}

// TestPropertyMapper verifies fixed values, value mapping, fallback, and warnings.
func TestPropertyMapper(t *testing.T) {
	tests := []struct {
		name         string
		rules        []MappingRule
		note         Note
		wantProps    map[string]string
		wantWarnings int
	}{
		{
			name: "value 고정값 주입",
			rules: []MappingRule{
				{NotionProperty: "Type", Value: "Todo"},
				{NotionProperty: "Status", Value: "Done"},
			},
			note:      Note{},
			wantProps: map[string]string{"Type": "Todo", "Status": "Done"},
		},
		{
			name: "values 테이블로 값 변환",
			rules: []MappingRule{{
				Frontmatter:    "priority",
				NotionProperty: "Priority",
				Values:         map[string]string{"high": "High", "low": "Low"},
			}},
			note:      Note{Frontmatter: map[string]string{"priority": "high"}},
			wantProps: map[string]string{"Priority": "High"},
		},
		{
			name: "values 없으면 노트 값 그대로",
			rules: []MappingRule{{
				Frontmatter:    "mood",
				NotionProperty: "Mood",
			}},
			note:      Note{Frontmatter: map[string]string{"mood": "좋음"}},
			wantProps: map[string]string{"Mood": "좋음"},
		},
		{
			name: "테이블에 없는 값은 default",
			rules: []MappingRule{{
				Frontmatter:    "priority",
				NotionProperty: "Priority",
				Values:         map[string]string{"high": "High"},
				Default:        "Normal",
			}},
			note:      Note{Frontmatter: map[string]string{"priority": "unknown"}},
			wantProps: map[string]string{"Priority": "Normal"},
		},
		{
			name: "값이 비면 default",
			rules: []MappingRule{{
				Frontmatter:    "priority",
				NotionProperty: "Priority",
				Default:        "Normal",
			}},
			note:      Note{Frontmatter: map[string]string{}},
			wantProps: map[string]string{"Priority": "Normal"},
		},
		{
			name: "default도 없으면 속성 건너뛰고 경고",
			rules: []MappingRule{{
				Frontmatter:    "priority",
				NotionProperty: "Priority",
				Values:         map[string]string{"high": "High"},
			}},
			note:         Note{Frontmatter: map[string]string{"priority": "unknown"}},
			wantProps:    map[string]string{},
			wantWarnings: 1,
		},
		{
			name:         "value와 frontmatter 둘 다 없는 규칙은 경고",
			rules:        []MappingRule{{NotionProperty: "Broken"}},
			note:         Note{},
			wantProps:    map[string]string{},
			wantWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draft := &PageDraft{Properties: make(map[string]string)}
			if err := NewPropertyMapper(tt.rules).Transform(tt.note, draft); err != nil {
				t.Fatalf("Transform 에러: %v", err)
			}
			if len(draft.Properties) != len(tt.wantProps) {
				t.Errorf("Properties = %v, want %v", draft.Properties, tt.wantProps)
			}
			for k, want := range tt.wantProps {
				if got := draft.Properties[k]; got != want {
					t.Errorf("Properties[%q] = %q, want %q", k, got, want)
				}
			}
			if len(draft.Warnings) != tt.wantWarnings {
				t.Errorf("Warnings = %v, want %d개", draft.Warnings, tt.wantWarnings)
			}
		})
	}
}

// recorder is a test Transformer that records execution order.
type recorder struct {
	name string
	log  *[]string
	err  error
}

func (r recorder) Name() string { return r.name }

func (r recorder) Transform(note Note, draft *PageDraft) error {
	if r.err != nil {
		return r.err
	}
	*r.log = append(*r.log, r.name)
	return nil
}

// TestRunOrder verifies the chain runs once in registration order.
func TestRunOrder(t *testing.T) {
	var log []string
	chain := []Transformer{
		recorder{name: "first", log: &log},
		recorder{name: "second", log: &log},
		recorder{name: "third", log: &log},
	}

	draft, err := Run(Note{Filename: "DN_251101.md"}, chain)
	if err != nil {
		t.Fatalf("Run 에러: %v", err)
	}
	if draft.Properties == nil {
		t.Error("Run은 Properties를 초기화해야 한다")
	}
	if got, want := strings.Join(log, ","), "first,second,third"; got != want {
		t.Errorf("실행 순서 = %q, want %q", got, want)
	}
}

// TestRunError verifies abort on stage error and stage-name wrapping.
func TestRunError(t *testing.T) {
	var log []string
	cause := errors.New("본문이 비어 있음")
	chain := []Transformer{
		recorder{name: "first", log: &log},
		recorder{name: "failing", log: &log, err: cause},
		recorder{name: "third", log: &log},
	}

	draft, err := Run(Note{}, chain)
	if draft != nil {
		t.Errorf("에러 시 draft는 nil이어야 하는데 %v", draft)
	}
	if !errors.Is(err, cause) {
		t.Errorf("원인 에러가 래핑되어야 하는데 err = %v", err)
	}
	if want := fmt.Sprintf("failing: %s", cause); err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
	if got, want := strings.Join(log, ","), "first"; got != want {
		t.Errorf("에러 이후 단계가 실행됨: 실행 순서 = %q, want %q", got, want)
	}
}

// TestRunFullChain is an integration test of the full built-in chain.
func TestRunFullChain(t *testing.T) {
	chain := []Transformer{
		NewDateDeriver([]DateRule{
			{FileLayout: "DN_060102"},
			{FrontmatterKey: "created_date"},
		}),
		NewTitleFromFilename(),
		NewObsidianURI("Yersona", "100. Inbox/Work/Daily"),
		NewPropertyMapper([]MappingRule{
			{NotionProperty: "Type", Value: "Todo"},
			{NotionProperty: "Status", Value: "Done"},
		}),
	}
	note := Note{
		Filename:    "DN_251101.md",
		RelPath:     "2025-11/DN_251101.md",
		Frontmatter: map[string]string{},
		Body:        "본문",
	}

	draft, err := Run(note, chain)
	if err != nil {
		t.Fatalf("Run 에러: %v", err)
	}
	if draft.Date != "2025-11-01" {
		t.Errorf("Date = %q, want %q", draft.Date, "2025-11-01")
	}
	if draft.Title != "DN_251101" {
		t.Errorf("Title = %q, want %q", draft.Title, "DN_251101")
	}
	wantURI := "obsidian://open?vault=Yersona&file=100.%20Inbox%2FWork%2FDaily%2F2025-11%2FDN_251101"
	if draft.ObsidianURI != wantURI {
		t.Errorf("ObsidianURI = %q, want %q", draft.ObsidianURI, wantURI)
	}
	if got := draft.Properties["Type"]; got != "Todo" {
		t.Errorf("Properties[Type] = %q, want %q", got, "Todo")
	}
	if got := draft.Properties["Status"]; got != "Done" {
		t.Errorf("Properties[Status] = %q, want %q", got, "Done")
	}
	if len(draft.Warnings) != 0 {
		t.Errorf("Warnings = %v, want 없음", draft.Warnings)
	}
}
