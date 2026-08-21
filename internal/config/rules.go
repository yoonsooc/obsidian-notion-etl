package config

import (
	"fmt"
	"strings"
	"time"
)

// DateRule is one step in a dateFrom chain: a single way to derive a note's
// date. Exactly one field must be set.
type DateRule struct {
	// FileLayout is a Go time layout applied to the file name without
	// extension (e.g. "DN_060102").
	FileLayout string `yaml:"fileLayout,omitempty"`
	// FrontmatterKey names the frontmatter key to read the date from; the
	// value is parsed as 2006-01-02 first, then RFC3339.
	FrontmatterKey string `yaml:"frontmatterKey,omitempty"`
}

// MappingEntry maps one Obsidian frontmatter key to a Notion property.
// Exactly one of Frontmatter and Value must be set.
type MappingEntry struct {
	Frontmatter    string `yaml:"frontmatter,omitempty"`
	NotionProperty string `yaml:"notionProperty"`
	// Value is a fixed value; when set, Frontmatter/Values/Default must be empty.
	Value string `yaml:"value,omitempty"`
	// Values translates frontmatter values to Notion values.
	Values map[string]string `yaml:"values,omitempty"`
	// Default is the fallback for missing or unmapped frontmatter values.
	Default string `yaml:"default,omitempty"`
}

// ValidateMapping checks mapping rules against the actual Notion schema.
// warnings are non-fatal (caller logs them); a non-nil err aborts the run.
// Rules: notionProperty must exist (case-insensitive match is accepted with a
// warning), exactly one of Value/Frontmatter must be set, duplicate targets
// warn and ignore later entries, and title/date/url properties are rejected
// because the pipeline derives them from the file name.
func ValidateMapping(mapping []MappingEntry, properties []Property) (warnings []string, err error) {
	byName := make(map[string]Property, len(properties))
	byFold := make(map[string]Property, len(properties))
	for _, p := range properties {
		byName[p.Name] = p
		byFold[strings.ToLower(p.Name)] = p
	}

	// resolved property name -> index of the first entry targeting it
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
			// The only property types v1 can build payloads for.
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

// ValidateDateRules validates a dateFrom rule list; field is the config path
// used in error messages. Each step must set exactly one of fileLayout and
// frontmatterKey, and each fileLayout must survive a format/parse round trip
// of the reference time.
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

		// Catches layouts like "12" that format fine but cannot be parsed
		// back (month and day digits are ambiguous).
		ref := time.Date(2006, time.January, 2, 15, 4, 5, 0, time.UTC)
		if _, err := time.Parse(r.FileLayout, ref.Format(r.FileLayout)); err != nil {
			return fmt.Errorf("%s.dateFrom[%d].fileLayout %q이 유효한 시간 레이아웃이 아님: %w", field, i, r.FileLayout, err)
		}
	}
	return nil
}
