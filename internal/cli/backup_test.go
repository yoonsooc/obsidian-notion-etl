package cli

import (
	"testing"

	"github.com/yoonsooc/obsidian-notion-etl/internal/notion"
)

func TestBackupFileName(t *testing.T) {
	used := make(map[string]int)
	tests := []struct {
		name string
		page notion.Page
		want string
	}{
		// Title first: same-date daily/weekly notes keep their original names.
		{name: "데일리 제목", page: notion.Page{Title: "DN_260513", Date: "2026-05-13"}, want: "DN_260513.md"},
		{name: "주간 제목(같은 날짜)", page: notion.Page{Title: "WG_260513-0517", Date: "2026-05-13"}, want: "WG_260513-0517.md"},
		{name: "노션 작성 제목 정리", page: notion.Page{Title: "회의: 계획/초안", Date: ""}, want: "회의- 계획-초안.md"},
		{name: "제목 없음은 날짜 폴백", page: notion.Page{Title: "", Date: "2026-05-13"}, want: "2026-05-13.md"},
		{name: "제목·날짜 없음은 ID 폴백", page: notion.Page{ID: "abc-123"}, want: "abc-123.md"},
		// Same title twice in one run gets a suffix.
		{name: "실행 내 제목 충돌", page: notion.Page{Title: "DN_260513", Date: ""}, want: "DN_260513-2.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backupFileName(tt.page, used); got != tt.want {
				t.Errorf("backupFileName() = %q, want %q", got, tt.want)
			}
		})
	}
}
