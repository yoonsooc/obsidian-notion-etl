package main

// pipeline.go는 이 프로젝트의 변환 정책이 코드로 모여 있는 곳이다
// (D10-변환 규칙의 위치: 코드 파이프라인). 설정 파일(base.config.yaml)에는
// 환경 정보(경로, DB, 제외 목록)만 두고, 도메인 규칙(날짜 파생, 속성 매핑)은
// 여기서 선언한다. 규칙 변경은 이 파일 수정 + 리빌드로 이뤄지며, init이
// 실행 시점에 실제 노션 스키마·노트와 대조해 검증한다 (D5-설정 역할 분리).

import (
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/yoonsooc/obsidian-notion-etl/internal/config"
	"github.com/yoonsooc/obsidian-notion-etl/internal/transform"
)

// dailyDateRules는 데일리 노트의 날짜 파생 체인이다
// (D6-대상 선정과 날짜 파생의 분리). 위에서부터 시도해 처음 성공한 값을 쓴다.
func dailyDateRules() []config.DateRule {
	return []config.DateRule{
		{FileLayout: "DN_060102"},        // DN_251101.md
		{FileLayout: "060102"},           // 251101.md (접두사 없는 구형 파일명)
		{FrontmatterKey: "created_date"}, // 파일명에 날짜가 없는 노트의 폴백
	}
}

// platinumMapping은 노션 Platinum DB의 속성 매핑 규칙이다 (D9-실제 매핑 규칙).
// docu_type 전 값이 Todo로 수렴하므로 값 변환 테이블 없이 고정값만 쓴다.
func platinumMapping() []config.MappingEntry {
	return []config.MappingEntry{
		{NotionProperty: "Type", Value: "Todo"},
		{NotionProperty: "Status", Value: "Done"},
	}
}

// buildPipeline은 마이그레이션 변환 체인을 조립한다 (D8-변환 아키텍처).
// 내장 Transformer는 코드 규칙으로 조립하고, 설정으로 표현할 수 없는 로직은
// 이 파일의 커스텀 Transformer(nfcTitle 등)로 같은 체인에 추가한다.
func buildPipeline(vaultName, target string, properties []config.Property) []transform.Transformer {
	return []transform.Transformer{
		transform.NewDateDeriver(toTransformDateRules(dailyDateRules())),
		transform.NewTitleFromFilename(),
		nfcTitle{},
		transform.NewObsidianURI(vaultName, target),
		transform.NewPropertyMapper(toTransformMappingRules(platinumMapping(), properties)),
	}
}

// nfcTitle은 제목을 NFC로 정규화하는 커스텀 Transformer다.
// macOS 파일명은 NFD로 저장되므로, 한글 파일명 노트의 제목이 NFD인 채
// 노션에 들어가면 재실행 시 title.equals 중복 검사가 불일치할 수 있다.
type nfcTitle struct{}

// Name은 단계 이름을 반환한다.
func (nfcTitle) Name() string { return "nfcTitle" }

// Transform은 draft.Title을 NFC로 정규화한다.
func (nfcTitle) Transform(_ transform.Note, draft *transform.PageDraft) error {
	draft.Title = norm.NFC.String(draft.Title)
	return nil
}

// toTransformDateRules는 규칙 정의를 파이프라인 타입으로 변환한다.
func toTransformDateRules(rules []config.DateRule) []transform.DateRule {
	out := make([]transform.DateRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, transform.DateRule{FileLayout: r.FileLayout, FrontmatterKey: r.FrontmatterKey})
	}
	return out
}

// toTransformMappingRules는 검증된 매핑을 파이프라인 타입으로 변환한다.
// ValidateMapping의 "중복 엔트리는 무시" 규칙을 여기서 실제로 적용한다.
// 속성 이름은 ValidateMapping과 같은 순서로 해석한다: 정확 일치가 있으면
// 그대로 쓰고, 없을 때만 대소문자 무시 일치로 스키마의 실제 이름을 취한다
// (대소문자만 다른 속성이 공존하는 스키마에서 오라우팅을 막기 위함).
func toTransformMappingRules(entries []config.MappingEntry, properties []config.Property) []transform.MappingRule {
	exact := make(map[string]struct{}, len(properties))
	actualByFold := make(map[string]string, len(properties))
	for _, p := range properties {
		exact[p.Name] = struct{}{}
		actualByFold[strings.ToLower(p.Name)] = p.Name
	}

	seen := make(map[string]struct{}, len(entries))
	rules := make([]transform.MappingRule, 0, len(entries))
	for _, e := range entries {
		name := e.NotionProperty
		if _, ok := exact[name]; !ok {
			if actual, ok := actualByFold[strings.ToLower(name)]; ok {
				name = actual
			}
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		rules = append(rules, transform.MappingRule{
			Frontmatter:    e.Frontmatter,
			NotionProperty: name,
			Value:          e.Value,
			Values:         e.Values,
			Default:        e.Default,
		})
	}
	return rules
}
