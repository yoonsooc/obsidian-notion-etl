# task-003: internal/notion 클라이언트

담당: 병렬 에이전트 B
상태: 완료

> 변경 이력 (2026-08-21): API 버전을 2022-06-28에서 2026-03-11로 상향(사용자 결정).
> RetrieveDatabase는 데이터 소스 참조 목록을 반환하도록 바뀌었고, 속성 스키마 조회는
> 새 메서드 RetrieveDataSource(GET /v1/data_sources/{id})가 담당한다.

## 목표

Rate Limiter와 429 재시도가 내장된 Notion API 클라이언트의 M1 범위(스키마 조회)를 구현한다. M2/M3에서 페이지 생성·쿼리 메서드가 추가될 수 있는 구조로 만든다.

## API 계약

```go
package notion

const apiVersion = "2022-06-28" // Notion-Version 헤더

type Property struct {
    Name string
    Type string // "title", "date", "select", "url", "rich_text", ...
}

type Database struct {
    ID         string
    Title      string
    Properties []Property
}

type Client struct { /* 내부: token, *http.Client(timeout 10s), *rate.Limiter */ }

// NewClient는 rate.NewLimiter(rate.Limit(2.5), 3)을 내장한 클라이언트를 만든다.
// 모든 API 호출은 반드시 이 Limiter를 통과한다(우회 불가 구조).
func NewClient(token string) *Client

// RetrieveDatabase는 GET /v1/databases/{id}로 스키마를 조회한다.
func (c *Client) RetrieveDatabase(ctx context.Context, databaseID string) (*Database, error)

// ExtractDatabaseID는 노션 DB URL에서 32자리 hex ID를 추출해
// 8-4-4-4-12 하이픈 형식의 UUID 문자열로 돌려준다.
// 예: https://www.notion.so/0123456789abcdef0123456789abcdef?v=...
//  -> 01234567-89ab-cdef-0123-456789abcdef
// 정규식 금지: url.Parse + strings 함수로 경로 마지막 세그먼트에서
// 32자리 hex를 찾는다 (세그먼트가 "제목-ID" 형태일 수 있으므로 뒤에서부터).
func ExtractDatabaseID(rawURL string) (string, error)
```

## 내부 설계 요구사항

- 공통 요청 헬퍼 `do(ctx, method, path, body) ([]byte, error)`:
  1. `limiter.Wait(ctx)`
  2. 요청 실행 (Authorization: Bearer, Notion-Version, Content-Type 헤더)
  3. 429면 Retry-After 헤더(초)만큼 대기 후 재시도, 최대 3회. 헤더가 없으면 1초.
  4. 2xx가 아니면 상태코드와 응답 본문을 담은 에러 반환.
- API 응답 매핑은 명시적 struct 사용. 단, properties처럼 키가 동적인 부분만 `map[string]...` 허용 (CLAUDE.md 규칙).
- 401/404는 재시도하지 않고 즉시 에러 반환 (원인 구분 가능한 메시지).

## 제약

- 의존성: 표준 라이브러리 + `golang.org/x/time/rate`만.
- docs/review-checklist.md 준수 (특히: defer로 Body.Close, 타입 어설션 v,ok, 채널/전역 금지).
- 단위 테스트 (`client_test.go`): httptest 서버로 RetrieveDatabase 파싱, 429 재시도(Retry-After 준수), 401 즉시 실패, ExtractDatabaseID 테이블 테스트 (쿼리스트링/제목 포함 URL/잘못된 URL).

## 완료 기준

- `go test ./internal/notion/` 통과, `go vet` 통과, gofmt 적용
