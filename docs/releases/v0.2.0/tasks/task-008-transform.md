# task-008: internal/transform 변환 파이프라인

담당: 병렬 에이전트 B
상태: 완료 (2026-08-21)

> 변경 이력 (2026-08-21, M2 리뷰 반영):
> - NewObsidianURI의 인코딩이 url.QueryEscape 그대로가 아니라 공백을 `%20`으로
>   치환하는 방식으로 변경됨 (M2 리뷰 1번-URI 공백 인코딩. Obsidian은 `+`를
>   공백으로 해석하지 않음). 이 문서의 계약 중 "url.QueryEscape 인코딩" 문구는 구버전.
> - 체인 조립 주체가 명확해짐: D10-변환 규칙의 위치에 따라 규칙과 조립은
>   main 패키지의 pipeline.go가 담당하고, 커스텀 Transformer(nfcTitle)도 거기에 있다.
선행: 없음 (자체 타입으로 완결, config/notion과 컴파일 의존 없음)

## 목표

PRD D8의 Transformer 파이프라인을 구현한다. 노트 하나가 체인을 순서대로 한 번씩
통과하며 노션 페이지 초안(PageDraft)이 완성된다. **이 패키지는 config·notion
패키지를 import하지 않는다.** 규칙은 자체 타입으로 받고, 조립은 migrate(task-011)가 한다.

## API 계약

```go
package transform

// Note는 파이프라인 입력이다 (vault.Note와 필드 호환, 의존은 없음).
type Note struct {
    Filename    string            // 예: "DN_251101.md"
    RelPath     string            // target 기준 상대경로 (Obsidian URI 생성용)
    Frontmatter map[string]string
    Body        string
}

// PageDraft는 파이프라인이 완성하는 노션 페이지 초안이다.
type PageDraft struct {
    Title       string            // Name(title) 속성
    Date        string            // "2006-01-02". 파생 실패 시 빈 문자열 (D6)
    ObsidianURI string            // url 속성
    Properties  map[string]string // 노션 속성 이름 -> 값 (타입 해석은 소비자 몫)
    Warnings    []string          // 파이프라인 중 발생한 항목 단위 경고 (로그용)
}

type Transformer interface {
    Name() string
    Transform(note Note, draft *PageDraft) error // error는 해당 노트 스킵 사유
}

// Run은 체인을 등록 순서대로 한 번씩 적용해 초안을 완성한다.
func Run(note Note, chain []Transformer) (*PageDraft, error)

// 내장 Transformer 생성자:

// DateRule은 config.DateRule과 필드 호환 (FileLayout / FrontmatterKey 중 하나).
type DateRule struct {
    FileLayout     string
    FrontmatterKey string
}
// NewDateDeriver: 규칙을 순서대로 시도해 draft.Date를 채운다.
// FileLayout은 파일명(확장자 제외)에 time.Parse. FrontmatterKey는 값을
// 2006-01-02로 파싱하고 실패 시 앞 10자만 재시도(RFC3339 대응).
// 전부 실패하면 Date를 비우고 Warnings에 기록 (에러 아님, D6).
func NewDateDeriver(rules []DateRule) Transformer

// NewTitleFromFilename: draft.Title = 파일명에서 ".md" 제거 (D7)
func NewTitleFromFilename() Transformer

// NewObsidianURI: obsidian://open?vault=<vault>&file=<target/relPath에서 .md 제거>
// url.QueryEscape 인코딩. vaultName과 target(볼트 기준 상대 디렉토리)을 받는다.
func NewObsidianURI(vaultName, target string) Transformer

// MappingRule은 config.MappingEntry와 필드 호환.
type MappingRule struct {
    Frontmatter    string
    NotionProperty string
    Value          string
    Values         map[string]string
    Default        string
}
// NewPropertyMapper: 규칙을 순서대로 적용해 draft.Properties를 채운다.
//   Value 설정 시 고정값. Frontmatter 설정 시 노트 값을 Values로 변환,
//   테이블에 없거나 값이 비면 Default, Default도 없으면 그 속성은 건너뛰고 Warnings 기록.
func NewPropertyMapper(rules []MappingRule) Transformer
```

## 제약

- 표준 라이브러리만 import (net/url, time, strings, fmt). 정규식 금지, panic 금지.
- internal/transform/ 밖 수정 금지 (이 문서 상태 줄 제외). go.mod 수정 금지.
- docs/review-checklist.md 18항목 준수.
- 테이블 기반 테스트: 날짜 체인(파일명 성공/프론트매터 폴백/전부 실패), 제목,
  URI 인코딩(한글 경로 포함), 매핑(고정값/값 변환/Default/누락 경고), Run 순서 보장.

## 완료 기준

- gofmt -l internal/transform/ 비어 있음, go vet, go test ./internal/transform/ 통과
