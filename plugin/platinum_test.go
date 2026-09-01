package plugin

import (
	"testing"
	"time"

	"github.com/yoonsooc/obsidian-notion-etl/internal/config"
	"github.com/yoonsooc/obsidian-notion-etl/internal/transform"
)

func TestDateRulesValidateAndParseMonthly(t *testing.T) {
	if err := config.ValidateDateRules("plugin.platinum", platinum{}.DateRules()); err != nil {
		t.Fatalf("DateRules must validate: %v", err)
	}
	// The MG layout parses a monthly filename to the month's first day.
	got, err := time.Parse("MG_200601", "MG_202603")
	if err != nil {
		t.Fatalf("monthly layout parse: %v", err)
	}
	if want := "2026-03-01"; got.Format("2006-01-02") != want {
		t.Errorf("MG_202603 -> %s, want %s", got.Format("2006-01-02"), want)
	}
}

func TestWeeklyStartDate(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		preset   string // date already set by the declarative chain
		want     string
		warned   bool
	}{
		{name: "주간 시작일 파생", filename: "WG_260513-0517.md", want: "2026-05-13"},
		{name: "물결 구분자", filename: "WG_260330~0405.md", want: "2026-03-30"},
		{name: "끝이 6자리(월 경계)", filename: "WG_260831~260906.md", want: "2026-08-31"},
		{name: "created_date 폴백을 덮어씀", filename: "WG_260513-0517.md", preset: "2026-05-11", want: "2026-05-13"},
		{name: "주간 아님(데일리)", filename: "DN_251101.md", preset: "2025-11-01", want: "2025-11-01"},
		{name: "범위 구분자 없음", filename: "WG_260513.md", want: ""},
		{name: "시작 날짜 형식 오류", filename: "WG_26051x-0517.md", want: "", warned: true},
		{name: "끝 부분 길이 오류", filename: "WG_260513-051.md", want: "", warned: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draft := &transform.PageDraft{Date: tt.preset}
			err := weeklyStartDate{}.Transform(transform.Note{Filename: tt.filename}, draft)
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			if draft.Date != tt.want {
				t.Errorf("Date = %q, want %q", draft.Date, tt.want)
			}
			if got := len(draft.Warnings) > 0; got != tt.warned {
				t.Errorf("warned = %v, want %v (%v)", got, tt.warned, draft.Warnings)
			}
		})
	}
}
