package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testProperties is a sample Notion schema for ValidateMapping tests.
func testProperties() []Property {
	return []Property{
		{Name: "Name", Type: "title"},
		{Name: "Date", Type: "date"},
		{Name: "Link", Type: "url"},
		{Name: "Type", Type: "select"},
		{Name: "Status", Type: "status"},
		{Name: "Tags", Type: "multi_select"},
	}
}

func TestValidateMapping(t *testing.T) {
	tests := []struct {
		name         string
		mapping      []MappingEntry
		wantWarnings []string // substrings expected per warning, in order
		wantErr      string   // substring expected in the error; empty means success
	}{
		{
			name:    "빈 매핑",
			mapping: nil,
		},
		{
			name: "value와 frontmatter 매핑 정상",
			mapping: []MappingEntry{
				{NotionProperty: "Type", Value: "Todo"},
				{NotionProperty: "Status", Frontmatter: "state"},
			},
		},
		{
			// Types v1 cannot build payloads for must be rejected at init,
			// or every item would fail during migrate.
			name: "미지원 타입(multi_select) 매핑 금지",
			mapping: []MappingEntry{
				{NotionProperty: "Tags", Frontmatter: "tags"},
			},
			wantErr: "지원하지 않음",
		},
		{
			name: "values와 default를 동반한 frontmatter 매핑 정상",
			mapping: []MappingEntry{
				{NotionProperty: "Status", Frontmatter: "status", Values: map[string]string{"진행": "Doing"}, Default: "Done"},
			},
		},
		{
			name: "대소문자 무시 일치는 경고 후 수용",
			mapping: []MappingEntry{
				{NotionProperty: "type", Value: "Todo"},
			},
			wantWarnings: []string{"대소문자"},
		},
		{
			name: "존재하지 않는 속성",
			mapping: []MappingEntry{
				{NotionProperty: "Nope", Value: "x"},
			},
			wantErr: "데이터베이스에 없음",
		},
		{
			name: "notionProperty 비어 있음",
			mapping: []MappingEntry{
				{NotionProperty: "  ", Value: "x"},
			},
			wantErr: "notionProperty가 비어 있음",
		},
		{
			name: "value와 frontmatter 동시 설정",
			mapping: []MappingEntry{
				{NotionProperty: "Type", Value: "Todo", Frontmatter: "type"},
			},
			wantErr: "동시에 설정할 수 없음",
		},
		{
			name: "value와 frontmatter 둘 다 없음",
			mapping: []MappingEntry{
				{NotionProperty: "Type"},
			},
			wantErr: "하나는 설정해야 함",
		},
		{
			name: "value에 values 동반",
			mapping: []MappingEntry{
				{NotionProperty: "Type", Value: "Todo", Values: map[string]string{"a": "b"}},
			},
			wantErr: "values/default는 비어야 함",
		},
		{
			name: "value에 default 동반",
			mapping: []MappingEntry{
				{NotionProperty: "Type", Value: "Todo", Default: "x"},
			},
			wantErr: "values/default는 비어야 함",
		},
		{
			name: "중복 속성은 경고 후 뒤 엔트리 무시",
			mapping: []MappingEntry{
				{NotionProperty: "Type", Value: "Todo"},
				{NotionProperty: "Type", Value: "Done"},
			},
			wantWarnings: []string{"중복"},
		},
		{
			name: "대소문자만 다른 중복도 잡음",
			mapping: []MappingEntry{
				{NotionProperty: "Type", Value: "Todo"},
				{NotionProperty: "type", Value: "Done"},
			},
			wantWarnings: []string{"대소문자", "중복"},
		},
		{
			name: "title 타입 속성 매핑 금지",
			mapping: []MappingEntry{
				{NotionProperty: "Name", Frontmatter: "title"},
			},
			wantErr: "title 타입",
		},
		{
			name: "date 타입 속성 매핑 금지",
			mapping: []MappingEntry{
				{NotionProperty: "Date", Frontmatter: "date"},
			},
			wantErr: "date 타입",
		},
		{
			name: "url 타입 속성 매핑 금지",
			mapping: []MappingEntry{
				{NotionProperty: "Link", Frontmatter: "link"},
			},
			wantErr: "url 타입",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings, err := ValidateMapping(tt.mapping, testProperties())
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("에러를 기대했지만 nil을 받음 (warnings=%v)", warnings)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("에러 메시지에 %q가 없음: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateMapping 실패: %v", err)
			}
			if len(warnings) != len(tt.wantWarnings) {
				t.Fatalf("경고 개수 = %d, want %d (warnings=%v)", len(warnings), len(tt.wantWarnings), warnings)
			}
			for i, want := range tt.wantWarnings {
				if !strings.Contains(warnings[i], want) {
					t.Errorf("경고[%d]에 %q가 없음: %q", i, want, warnings[i])
				}
			}
		})
	}
}

// TestValidateDateRules exercises date-rule validation directly (rules live
// in code, not config, so no YAML round trip is needed).
func TestValidateDateRules(t *testing.T) {
	tests := []struct {
		name    string
		rules   []DateRule
		wantErr string // substring expected in the error; empty means success
	}{
		{
			name: "fileLayout과 frontmatterKey 체인 정상",
			rules: []DateRule{
				{FileLayout: "DN_060102"},
				{FrontmatterKey: "created_date"},
			},
		},
		{
			name:    "두 필드 동시 설정",
			rules:   []DateRule{{FileLayout: "DN_060102", FrontmatterKey: "created_date"}},
			wantErr: "정확히 하나만 설정",
		},
		{
			name:    "두 필드 모두 없음",
			rules:   []DateRule{{}},
			wantErr: "정확히 하나만 설정",
		},
		{
			name: "왕복 불가능한 레이아웃",
			// "12" formats but cannot be parsed back: the month consumes
			// both digits, leaving nothing for the day.
			rules:   []DateRule{{FileLayout: "12"}},
			wantErr: "유효한 시간 레이아웃이 아님",
		},
		{
			name:    "에러 메시지에 field 경로가 포함됨",
			rules:   []DateRule{{}},
			wantErr: "pipeline.dailyDateRules.dateFrom[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDateRules("pipeline.dailyDateRules", tt.rules)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("에러를 기대했지만 nil을 받음")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("에러 메시지에 %q가 없음: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateDateRules 실패: %v", err)
			}
		})
	}
}

func TestDecodeBaseKnownFields(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string // substring expected in the error; empty means success
	}{
		{
			name: "알려진 키만 있으면 통과",
			yaml: "obsidian:\n  vault:\n    toNotion:\n      name: 'v'\n",
		},
		{
			name:    "최상위 미지 키",
			yaml:    "obsidian:\n  vault: {}\nmaping: []\n",
			wantErr: "maping",
		},
		{
			name:    "중첩 미지 키",
			yaml:    "obsidian:\n  vault:\n    toNotion:\n      name: 'v'\n      targett: 'Daily'\n",
			wantErr: "targett",
		},
		{
			// Transform rules moved to code, so config-level dateFrom/mapping
			// keys are rejected as unknown (detects stale configs).
			name:    "코드로 이동한 dateFrom 키는 거부",
			yaml:    "obsidian:\n  vault:\n    toNotion:\n      dateFrom:\n        - fileLayout: '060102'\n",
			wantErr: "dateFrom",
		},
		{
			name:    "코드로 이동한 mapping 키는 거부",
			yaml:    "obsidian:\n  vault: {}\nmapping:\n  - notionProperty: Type\n",
			wantErr: "mapping",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeBase([]byte(tt.yaml))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("에러를 기대했지만 nil을 받음")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("에러 메시지에 %q가 없음: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeBase 실패: %v", err)
			}
		})
	}
}

// TestDecodeBaseRealFile checks that the repo's real base.config.yaml passes
// strict parsing; path validation needs a real vault, so only parsing is tested.
func TestDecodeBaseRealFile(t *testing.T) {
	path := filepath.Join("..", "..", "base.config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("실제 base.config.yaml이 없어 건너뜀: %v", err)
		}
		t.Fatalf("실제 base.config.yaml 읽기 실패: %v", err)
	}

	// User config changes often; assert only that strict parsing passes.
	if _, err := decodeBase(data); err != nil {
		t.Fatalf("실제 base.config.yaml 엄격 파싱 실패: %v", err)
	}
}

// TestLatestRoundTripRules verifies extended MappingEntry and DateFrom fields
// survive a save/load round trip of the latest snapshot.
func TestLatestRoundTripRules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "latest.config.yaml")
	now := time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC)

	cfg := newLatest()
	cfg.Mapping = []MappingEntry{
		{NotionProperty: "Type", Value: "Todo"},
		{NotionProperty: "Status", Frontmatter: "status", Values: map[string]string{"진행": "Doing"}, Default: "Done"},
	}
	cfg.DateFrom = []DateRule{
		{FileLayout: "DN_060102"},
		{FrontmatterKey: "created_date"},
	}

	if _, err := SaveLatest(path, filepath.Join(dir, "backups"), cfg, now); err != nil {
		t.Fatalf("SaveLatest 실패: %v", err)
	}
	got, err := LoadLatest(path)
	if err != nil {
		t.Fatalf("LoadLatest 실패: %v", err)
	}
	if got == nil {
		t.Fatal("nil이 아닌 설정을 기대")
	}

	if len(got.Mapping) != 2 {
		t.Fatalf("mapping 개수 = %d, want 2", len(got.Mapping))
	}
	if got.Mapping[0].Value != "Todo" {
		t.Errorf("Mapping[0].Value = %q, want %q", got.Mapping[0].Value, "Todo")
	}
	if got.Mapping[1].Values["진행"] != "Doing" || got.Mapping[1].Default != "Done" {
		t.Errorf("Mapping[1] 변환 테이블 복원 실패: %+v", got.Mapping[1])
	}
	if len(got.DateFrom) != 2 {
		t.Fatalf("dateFrom 개수 = %d, want 2", len(got.DateFrom))
	}
	if got.DateFrom[0].FileLayout != "DN_060102" || got.DateFrom[1].FrontmatterKey != "created_date" {
		t.Errorf("dateFrom 복원 실패: %+v", got.DateFrom)
	}
}
