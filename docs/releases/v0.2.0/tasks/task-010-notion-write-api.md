# task-010: internal/notion M2 메서드 (쿼리·페이지 생성·블록 append)

담당: 병렬 에이전트 D
상태: 완료 (2026-08-21, pages.go / pages_test.go 추가)
선행: internal/notion/blocks.go의 블록 타입 (메인 세션이 선작성, 수정 금지)

## 목표

migrate가 쓰는 세 가지 API를 기존 Client(do 헬퍼, Limiter 내장) 위에 추가한다.
모두 API 2026-03-11 기준이며 data_source 계열 엔드포인트를 쓴다.

## API 계약 (internal/notion에 추가, 새 파일 pages.go 권장)

```go
// PropertyValue는 페이지 속성 값 하나다. Type에 따라 페이로드가 결정된다.
// 지원 타입: "title", "date", "url", "select", "status", "rich_text" (PRD FR-2)
type PropertyValue struct {
    Type  string
    Value string
}

// ExistsByDate는 Date 속성(date.equals)으로 페이지 존재 여부를 확인한다.
// POST /v1/data_sources/{id}/query, page_size 1.
func (c *Client) ExistsByDate(ctx context.Context, dataSourceID, propertyName, dateISO string) (bool, error)

// ExistsByTitle은 title 속성(title.equals)으로 페이지 존재 여부를 확인한다.
func (c *Client) ExistsByTitle(ctx context.Context, dataSourceID, propertyName, title string) (bool, error)

// CreatePage는 데이터 소스에 페이지를 생성한다. children은 최대 100개까지만
// 본 요청에 싣고, 초과분은 AppendBlocks로 100개씩 나눠 보낸다.
// 부분 실패(생성 성공 후 append 실패) 시 생성된 페이지 ID를 담은 에러를 반환한다
// (로그로 수동 정리를 돕기 위함, PRD FR-2 6항).
// properties는 노션 속성 이름 -> PropertyValue. 빈 Value인 속성은 보내지 않는다.
func (c *Client) CreatePage(ctx context.Context, dataSourceID string, properties map[string]PropertyValue, children []Block) (pageID string, err error)

// AppendBlocks는 PATCH /v1/blocks/{blockID}/children로 블록을 100개씩 나눠 추가한다.
func (c *Client) AppendBlocks(ctx context.Context, blockID string, children []Block) error
```

페이로드 세부:
- 페이지 생성 parent: `{"type": "data_source_id", "data_source_id": "..."}`
- 속성 페이로드: title -> `{"title":[{"text":{"content":v}}]}`, date -> `{"date":{"start":v}}`,
  url -> `{"url":v}`, select -> `{"select":{"name":v}}`, status -> `{"status":{"name":v}}`,
  rich_text -> `{"rich_text":[{"text":{"content":v}}]}`. 명시적 struct로 정의하되
  properties 맵 키가 동적인 부분만 map 허용 (CLAUDE.md 규칙).
- 쿼리 필터: `{"filter":{"property":name,"date":{"equals":v}},"page_size":1}` /
  title은 `{"title":{"equals":v}}`. 응답은 results 배열 길이만 보면 된다.

## 제약

- internal/notion/ 안의 새 파일만 추가하라. client.go, database.go, blocks.go 수정 금지
  (do 헬퍼와 블록 타입을 그대로 사용). go.mod 수정 금지.
- docs/review-checklist.md 18항목 준수.
- httptest 기반 테스트: 쿼리 필터 페이로드 검증, 존재/부재 응답, CreatePage의
  100개 분할(150개 children -> 생성 1회 + append 1회, 각 요청의 children 수 검증),
  append 실패 시 에러에 페이지 ID 포함, 속성 페이로드 타입별 직렬화.

## 완료 기준

- gofmt -l internal/notion/ 비어 있음, go vet, go test ./internal/notion/ 통과
