package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testProperties는 ValidateMapping 테스트에 쓰는 노션 스키마 표본이다.
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
		wantWarnings []string // 각 경고 메시지에 포함되어야 하는 문자열 (순서 일치)
		wantErr      string   // 에러 메시지에 포함되어야 하는 문자열. 빈 값이면 성공 기대.
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
			// v1이 페이로드를 만들 수 없는 타입은 init에서 미리 거른다
			// (통과시키면 migrate에서 전 건 실패한다).
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

// TestValidateDateRules는 날짜 규칙 검증을 직접 검사한다. 규칙은 설정이 아니라
// 코드(pipeline.go)에 있으므로(D10-변환 규칙의 위치) yaml 왕복 없이 검증만 본다.
func TestValidateDateRules(t *testing.T) {
	tests := []struct {
		name    string
		rules   []DateRule
		wantErr string // 에러 메시지에 포함되어야 하는 문자열. 빈 값이면 성공 기대.
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
			// "12"는 월(1)과 일(2)이 붙어 있어 포맷("12")을 되읽을 때
			// 월이 12로 읽히고 일이 남지 않아 실패한다.
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
		wantErr string // 에러 메시지에 포함되어야 하는 문자열. 빈 값이면 성공 기대.
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
			// 변환 규칙이 코드로 이동하면서(D10-변환 규칙의 위치) 설정의
			// dateFrom/mapping 키는 미지 키로 거부된다 (낡은 설정 감지).
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

// TestDecodeBaseRealFile은 저장소의 실제 base.config.yaml이 엄격 파싱을
// 통과하는지 확인한다. 경로 검증은 실볼트가 필요하므로 파싱 단계만 검사한다.
func TestDecodeBaseRealFile(t *testing.T) {
	path := filepath.Join("..", "..", "base.config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("실제 base.config.yaml이 없어 건너뜀: %v", err)
		}
		t.Fatalf("실제 base.config.yaml 읽기 실패: %v", err)
	}

	// 사용자 설정은 수시로 바뀌므로 내용에 대한 단언은 하지 않는다.
	// 엄격 파싱(미지 키 없음) 통과만 확인한다.
	if _, err := decodeBase(data); err != nil {
		t.Fatalf("실제 base.config.yaml 엄격 파싱 실패: %v", err)
	}
}

// TestLatestRoundTripRules는 확장된 MappingEntry와 DateFrom이 latest 스냅샷에
// 저장·복원되는지 검증한다.
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
