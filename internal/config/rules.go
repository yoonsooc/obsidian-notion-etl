package config

import (
	"fmt"
	"strings"
	"time"
)

// DateRule은 dateFrom 체인의 한 단계다. 노트의 날짜를 얻는 방법 하나를
// 기술하며, 두 필드 중 정확히 하나만 설정되어야 한다.
type DateRule struct {
	// FileLayout은 파일명(확장자 제외)에 적용할 Go 시간 레이아웃이다 (예: 'DN_060102').
	FileLayout string `yaml:"fileLayout,omitempty"`
	// FrontmatterKey는 날짜를 읽을 frontmatter 키다. 값은 2006-01-02 형식을
	// 우선 시도하고, 실패하면 RFC3339 형식으로 해석한다.
	FrontmatterKey string `yaml:"frontmatterKey,omitempty"`
}

// MappingEntry는 옵시디언 프론트매터 키와 노션 속성의 대응 관계 하나를 나타낸다.
// Frontmatter와 Value 중 정확히 하나만 설정되어야 한다.
type MappingEntry struct {
	Frontmatter    string `yaml:"frontmatter,omitempty"`
	NotionProperty string `yaml:"notionProperty"`
	// Value는 고정값이다. 설정 시 Frontmatter/Values/Default는 비어야 한다.
	Value string `yaml:"value,omitempty"`
	// Values는 frontmatter 값 -> 노션 값 변환 테이블이다.
	Values map[string]string `yaml:"values,omitempty"`
	// Default는 Values에 없거나 frontmatter 값이 빈 경우의 폴백이다.
	Default string `yaml:"default,omitempty"`
}

// ValidateMapping은 매핑 규칙을 실제 노션 스키마와 대조한다 (init은 검증자).
// 반환된 warnings는 실행을 막지 않는 경고이며, 호출자가 warn 로그로 안내한다.
// err가 nil이 아니면 실행 중단 사유다.
// 검증 내용:
//   - notionProperty가 properties에 존재 (대소문자 정확 일치 우선, 무시 일치는 경고 후 수용)
//   - Value와 Frontmatter가 동시에 설정되면 에러, 둘 다 없어도 에러
//   - 같은 notionProperty를 가리키는 엔트리가 2개 이상이면 경고 후 뒤 엔트리 무시
//   - title/date/url 타입 속성을 가리키면 에러 (파이프라인이 파일명에서 파생)
func ValidateMapping(mapping []MappingEntry, properties []Property) (warnings []string, err error) {
	byName := make(map[string]Property, len(properties))
	byFold := make(map[string]Property, len(properties))
	for _, p := range properties {
		byName[p.Name] = p
		byFold[strings.ToLower(p.Name)] = p
	}

	// 확정된 속성 이름 -> 그 속성을 처음 가리킨 엔트리 인덱스
	seen := make(map[string]int, len(mapping))
	for i, m := range mapping {
		if strings.TrimSpace(m.NotionProperty) == "" {
			return warnings, fmt.Errorf("mapping[%d]의 notionProperty가 비어 있음", i)
		}

		hasValue := m.Value != ""
		hasFrontmatter := m.Frontmatter != ""
		switch {
		case hasValue && hasFrontmatter:
			return warnings, fmt.Errorf("mapping[%d](%s): value와 frontmatter는 동시에 설정할 수 없음", i, m.NotionProperty)
		case !hasValue && !hasFrontmatter:
			return warnings, fmt.Errorf("mapping[%d](%s): value와 frontmatter 중 하나는 설정해야 함", i, m.NotionProperty)
		case hasValue && (len(m.Values) > 0 || m.Default != ""):
			return warnings, fmt.Errorf("mapping[%d](%s): value가 설정되면 values/default는 비어야 함", i, m.NotionProperty)
		}

		prop, ok := byName[m.NotionProperty]
		if !ok {
			prop, ok = byFold[strings.ToLower(m.NotionProperty)]
			if !ok {
				return warnings, fmt.Errorf("mapping[%d]: 노션 속성 %q이 데이터베이스에 없음", i, m.NotionProperty)
			}
			warnings = append(warnings, fmt.Sprintf("mapping[%d]: 속성 이름 %q의 대소문자가 실제 속성 %q와 다름, %q로 처리함", i, m.NotionProperty, prop.Name, prop.Name))
		}

		switch prop.Type {
		case "title", "date", "url":
			return warnings, fmt.Errorf("mapping[%d]: %s 타입 속성 %q은 매핑할 수 없음(파이프라인이 파일명에서 파생)", i, prop.Type, prop.Name)
		case "select", "status", "rich_text":
			// v1이 페이로드를 만들 수 있는 타입 (PRD FR-2). 통과.
		default:
			return warnings, fmt.Errorf("mapping[%d]: %s 타입 속성 %q은 v1이 지원하지 않음(지원: select, status, rich_text)", i, prop.Type, prop.Name)
		}

		if first, dup := seen[prop.Name]; dup {
			warnings = append(warnings, fmt.Sprintf("mapping[%d]: 속성 %q이 mapping[%d]와 중복됨, 이 엔트리는 무시됨", i, prop.Name, first))
			continue
		}
		seen[prop.Name] = i
	}
	return warnings, nil
}

// ValidateDateRules는 dateFrom 규칙 목록을 검증한다. field는 에러 메시지에
// 표기할 설정 경로다. 각 단계는 fileLayout과 frontmatterKey 중 정확히 하나만
// 설정해야 하며, fileLayout은 그 레이아웃으로 포맷한 기준 시각을 같은
// 레이아웃으로 되읽는 왕복 검사로 유효성을 확인한다.
func ValidateDateRules(field string, rules []DateRule) error {
	for i, r := range rules {
		hasLayout := strings.TrimSpace(r.FileLayout) != ""
		hasKey := strings.TrimSpace(r.FrontmatterKey) != ""
		if hasLayout == hasKey {
			return fmt.Errorf("%s.dateFrom[%d]는 fileLayout과 frontmatterKey 중 정확히 하나만 설정해야 함", field, i)
		}
		if !hasLayout {
			continue
		}

		// 예: "12"처럼 월과 일이 붙어 자릿수를 구분할 수 없는 레이아웃은
		// 포맷은 되지만 되읽기가 실패하므로 여기서 걸러진다.
		ref := time.Date(2006, time.January, 2, 15, 4, 5, 0, time.UTC)
		if _, err := time.Parse(r.FileLayout, ref.Format(r.FileLayout)); err != nil {
			return fmt.Errorf("%s.dateFrom[%d].fileLayout %q이 유효한 시간 레이아웃이 아님: %w", field, i, r.FileLayout, err)
		}
	}
	return nil
}
