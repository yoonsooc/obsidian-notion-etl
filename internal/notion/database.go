package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Property는 Notion 데이터 소스 컬럼(프로퍼티)의 이름과 타입이다.
type Property struct {
	Name string
	Type string // "title", "date", "select", "url", "rich_text", ...
}

// DataSourceRef는 데이터베이스가 담고 있는 데이터 소스 하나에 대한 참조다.
type DataSourceRef struct {
	ID   string
	Name string
}

// Database는 Notion 데이터베이스(컨테이너)의 정보다.
// API 2025-09-03부터 속성 스키마는 데이터베이스가 아니라 데이터 소스에 붙으므로,
// 스키마가 필요하면 DataSources의 ID로 RetrieveDataSource를 호출한다.
type Database struct {
	ID          string
	Title       string
	DataSources []DataSourceRef
}

// DataSource는 Notion 데이터 소스의 스키마 정보다.
type DataSource struct {
	ID         string
	Title      string
	Properties []Property
}

// databaseResponse는 GET /v1/databases/{id} 응답에서 필요한 부분만 매핑한다.
type databaseResponse struct {
	ID          string              `json:"id"`
	Title       []richText          `json:"title"`
	DataSources []dataSourceRefJSON `json:"data_sources"`
}

// dataSourceRefJSON은 데이터베이스 응답의 data_sources 배열 원소다.
type dataSourceRefJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// dataSourceResponse는 GET /v1/data_sources/{id} 응답에서 필요한 부분만 매핑한다.
// properties는 키(프로퍼티 이름)가 동적이므로 map을 사용한다 (CLAUDE.md 규칙의 예외).
type dataSourceResponse struct {
	ID         string                    `json:"id"`
	Title      []richText                `json:"title"`
	Properties map[string]propertyScheme `json:"properties"`
}

// richText는 Notion rich text 배열의 원소에서 plain_text만 매핑한다.
type richText struct {
	PlainText string `json:"plain_text"`
}

// propertyScheme은 프로퍼티 정의에서 type 필드만 매핑한다.
type propertyScheme struct {
	Type string `json:"type"`
}

// RetrieveDatabase는 GET /v1/databases/{id}로 데이터베이스 컨테이너를 조회한다.
// 반환값에는 데이터 소스 참조 목록이 담기며, 속성 스키마는 RetrieveDataSource로 조회한다.
func (c *Client) RetrieveDatabase(ctx context.Context, databaseID string) (*Database, error) {
	body, err := c.do(ctx, http.MethodGet, "/v1/databases/"+url.PathEscape(databaseID), nil)
	if err != nil {
		return nil, fmt.Errorf("retrieve database %s: %w", databaseID, err)
	}

	var resp databaseResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode database response %s: %w", databaseID, err)
	}

	refs := make([]DataSourceRef, 0, len(resp.DataSources))
	for _, ds := range resp.DataSources {
		refs = append(refs, DataSourceRef{ID: ds.ID, Name: ds.Name})
	}

	return &Database{
		ID:          resp.ID,
		Title:       joinRichText(resp.Title),
		DataSources: refs,
	}, nil
}

// RetrieveDataSource는 GET /v1/data_sources/{id}로 데이터 소스 스키마를 조회한다.
// Properties는 이름 기준 오름차순으로 정렬해 결정적인 순서를 보장한다.
func (c *Client) RetrieveDataSource(ctx context.Context, dataSourceID string) (*DataSource, error) {
	body, err := c.do(ctx, http.MethodGet, "/v1/data_sources/"+url.PathEscape(dataSourceID), nil)
	if err != nil {
		return nil, fmt.Errorf("retrieve data source %s: %w", dataSourceID, err)
	}

	var resp dataSourceResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode data source response %s: %w", dataSourceID, err)
	}

	properties := make([]Property, 0, len(resp.Properties))
	for name, scheme := range resp.Properties {
		properties = append(properties, Property{Name: name, Type: scheme.Type})
	}
	sort.Slice(properties, func(i, j int) bool {
		return properties[i].Name < properties[j].Name
	})

	return &DataSource{
		ID:         resp.ID,
		Title:      joinRichText(resp.Title),
		Properties: properties,
	}, nil
}

// joinRichText는 rich text 배열의 plain_text를 이어붙인다.
func joinRichText(parts []richText) string {
	var b strings.Builder
	for _, rt := range parts {
		b.WriteString(rt.PlainText)
	}
	return b.String()
}

// ExtractDatabaseID는 노션 DB URL에서 32자리 hex ID를 추출해
// 8-4-4-4-12 하이픈 형식의 UUID 문자열로 돌려준다.
// 예: https://www.notion.so/b8842240012340b9bff4a51f2a0fd668?v=...
//
//	-> b8842240-0123-40b9-bff4-a51f2a0fd668
//
// 경로 마지막 세그먼트가 "제목-ID" 형태일 수 있으므로 뒤에서 32자를 취해 검사한다.
func ExtractDatabaseID(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse notion URL %q: %w", rawURL, err)
	}

	segment := lastPathSegment(parsed.Path)
	if len(segment) < 32 {
		return "", fmt.Errorf("notion URL %q: 경로에서 32자리 hex ID를 찾을 수 없다", rawURL)
	}

	candidate := strings.ToLower(segment[len(segment)-32:])
	if !isHex(candidate) {
		return "", fmt.Errorf("notion URL %q: 경로 마지막 세그먼트 %q가 32자리 hex ID로 끝나지 않는다", rawURL, segment)
	}

	return candidate[0:8] + "-" + candidate[8:12] + "-" + candidate[12:16] + "-" +
		candidate[16:20] + "-" + candidate[20:32], nil
}

// lastPathSegment는 URL 경로의 마지막 세그먼트를 반환한다.
func lastPathSegment(path string) string {
	trimmed := strings.TrimSuffix(path, "/")
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		return trimmed[idx+1:]
	}
	return trimmed
}

// isHex는 문자열이 소문자 16진수 문자로만 이루어졌는지 검사한다.
func isHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
