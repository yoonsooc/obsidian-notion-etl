package transform

import (
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"
)

// dateLayout은 draft.Date와 frontmatter 날짜 값의 표준 형식이다.
const dateLayout = "2006-01-02"

// stem은 파일명에서 ".md" 확장자를 제거한 이름을 반환한다.
func stem(filename string) string {
	return strings.TrimSuffix(filename, ".md")
}

// DateRule은 날짜 파생 규칙 하나다 (config.DateRule과 필드 호환).
// FileLayout과 FrontmatterKey 중 하나만 설정한다.
type DateRule struct {
	FileLayout     string // 파일명(확장자 제외)을 해석할 Go 시간 레이아웃
	FrontmatterKey string // 날짜 값을 읽을 frontmatter 키
}

// dateDeriver는 dateFrom 규칙 체인으로 draft.Date를 파생한다 (D6).
type dateDeriver struct {
	rules []DateRule
}

// NewDateDeriver는 규칙을 순서대로 시도해 draft.Date를 채우는 Transformer를
// 만든다. FileLayout은 파일명(확장자 제외)에 time.Parse를 적용하고,
// FrontmatterKey는 값을 2006-01-02로 파싱하되 실패하면 앞 10자만
// 재시도한다(RFC3339 대응). 전부 실패하면 Date를 비우고 Warnings에
// 기록한다 (에러가 아니다, D6).
func NewDateDeriver(rules []DateRule) Transformer {
	return &dateDeriver{rules: rules}
}

func (d *dateDeriver) Name() string { return "dateDeriver" }

func (d *dateDeriver) Transform(note Note, draft *PageDraft) error {
	for _, rule := range d.rules {
		if rule.FileLayout != "" {
			if t, err := time.Parse(rule.FileLayout, stem(note.Filename)); err == nil {
				draft.Date = t.Format(dateLayout)
				return nil
			}
			continue
		}
		if rule.FrontmatterKey == "" {
			continue
		}
		v := note.Frontmatter[rule.FrontmatterKey]
		if v == "" {
			continue
		}
		if t, err := time.Parse(dateLayout, v); err == nil {
			draft.Date = t.Format(dateLayout)
			return nil
		}
		// RFC3339처럼 날짜 뒤에 시각이 붙은 값은 앞 10자만 다시 시도한다.
		if r := []rune(v); len(r) > 10 {
			if t, err := time.Parse(dateLayout, string(r[:10])); err == nil {
				draft.Date = t.Format(dateLayout)
				return nil
			}
		}
	}
	draft.Date = ""
	draft.Warnings = append(draft.Warnings,
		fmt.Sprintf("dateDeriver: %s에서 날짜를 파생하지 못해 Date를 비움", note.Filename))
	return nil
}

// titleFromFilename은 파일명으로 제목을 정한다 (D7).
type titleFromFilename struct{}

// NewTitleFromFilename은 draft.Title을 파일명에서 ".md"를 제거한 값으로
// 채우는 Transformer를 만든다 (D7).
func NewTitleFromFilename() Transformer {
	return titleFromFilename{}
}

func (titleFromFilename) Name() string { return "titleFromFilename" }

func (titleFromFilename) Transform(note Note, draft *PageDraft) error {
	draft.Title = stem(note.Filename)
	return nil
}

// obsidianURI는 노트를 여는 obsidian:// 링크를 만든다.
type obsidianURI struct {
	vaultName string
	target    string
}

// NewObsidianURI는 draft.ObsidianURI를
// obsidian://open?vault=<vault>&file=<target/relPath에서 .md 제거> 형태로
// 채우는 Transformer를 만든다. vaultName은 볼트 이름, target은 볼트 기준
// 상대 디렉토리다.
func NewObsidianURI(vaultName, target string) Transformer {
	return &obsidianURI{vaultName: vaultName, target: target}
}

func (o *obsidianURI) Name() string { return "obsidianURI" }

func (o *obsidianURI) Transform(note Note, draft *PageDraft) error {
	file := path.Join(o.target, stem(note.RelPath))
	draft.ObsidianURI = "obsidian://open?vault=" + escapeURIComponent(o.vaultName) +
		"&file=" + escapeURIComponent(file)
	return nil
}

// escapeURIComponent는 URI 컴포넌트를 인코딩하되 공백을 '+'가 아니라 "%20"으로
// 쓴다. Obsidian은 URI 값을 decodeURIComponent 방식으로 해석하므로 '+'를
// 공백으로 되돌리지 않아, url.QueryEscape 그대로는 공백 포함 경로가 깨진다.
func escapeURIComponent(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// MappingRule은 속성 매핑 규칙 하나다 (config.MappingEntry와 필드 호환).
// Value를 설정하면 고정값 주입, Frontmatter를 설정하면 노트 값 매핑이다.
type MappingRule struct {
	Frontmatter    string            // 값을 읽을 frontmatter 키
	NotionProperty string            // 채울 노션 속성 이름
	Value          string            // 고정값 (설정 시 Frontmatter보다 우선)
	Values         map[string]string // 노트 값 -> 노션 값 변환 테이블 (선택)
	Default        string            // 값이 비거나 테이블에 없을 때의 폴백 (선택)
}

// propertyMapper는 mapping 규칙 체인으로 draft.Properties를 채운다 (D8, D9).
type propertyMapper struct {
	rules []MappingRule
}

// NewPropertyMapper는 규칙을 순서대로 적용해 draft.Properties를 채우는
// Transformer를 만든다. Value가 설정된 규칙은 고정값을 주입한다.
// Frontmatter가 설정된 규칙은 노트 값을 Values 테이블로 변환하며, 값이
// 비거나 테이블에 없으면 Default를 쓰고, Default도 없으면 그 속성은
// 건너뛰고 Warnings에 기록한다. Values 테이블이 없으면 노트 값을 그대로
// 쓴다.
func NewPropertyMapper(rules []MappingRule) Transformer {
	return &propertyMapper{rules: rules}
}

func (p *propertyMapper) Name() string { return "propertyMapper" }

func (p *propertyMapper) Transform(note Note, draft *PageDraft) error {
	for _, rule := range p.rules {
		if rule.Value != "" {
			draft.Properties[rule.NotionProperty] = rule.Value
			continue
		}
		if rule.Frontmatter == "" {
			draft.Warnings = append(draft.Warnings,
				fmt.Sprintf("propertyMapper: %s 규칙에 value와 frontmatter가 모두 없어 건너뜀",
					rule.NotionProperty))
			continue
		}
		raw := note.Frontmatter[rule.Frontmatter]
		resolved, ok := resolveValue(rule, raw)
		if !ok {
			draft.Warnings = append(draft.Warnings,
				fmt.Sprintf("propertyMapper: %s 속성 건너뜀 (frontmatter %q 값 %q 변환 불가, default 없음)",
					rule.NotionProperty, rule.Frontmatter, raw))
			continue
		}
		draft.Properties[rule.NotionProperty] = resolved
	}
	return nil
}

// resolveValue는 frontmatter 값 raw에 규칙의 변환 테이블과 폴백을 적용한다.
// 두 번째 반환값이 false면 채울 값이 없다는 뜻이다.
func resolveValue(rule MappingRule, raw string) (string, bool) {
	if raw == "" {
		if rule.Default == "" {
			return "", false
		}
		return rule.Default, true
	}
	if len(rule.Values) == 0 {
		return raw, true
	}
	if mapped, ok := rule.Values[raw]; ok {
		return mapped, true
	}
	if rule.Default == "" {
		return "", false
	}
	return rule.Default, true
}
