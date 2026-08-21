package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// maxChildrenPerRequest는 페이지 생성/append 요청 하나에 실을 수 있는
// children 블록의 최대 개수다 (PRD 6.1).
const maxChildrenPerRequest = 100

// PropertyValue는 페이지 속성 값 하나다. Type에 따라 페이로드가 결정된다.
// 지원 타입: "title", "date", "url", "select", "status", "rich_text" (PRD FR-2).
type PropertyValue struct {
	Type  string
	Value string
}

// propertyRichText는 속성 페이로드용 rich text 원소다. text.content만 사용한다.
type propertyRichText struct {
	Text Text `json:"text"`
}

// dateValue는 date 속성 페이로드의 값이다.
type dateValue struct {
	Start string `json:"start"`
}

// namedOption은 select/status 속성 페이로드의 값이다.
type namedOption struct {
	Name string `json:"name"`
}

// propertyPayload는 페이지 속성 하나의 요청 페이로드다.
// PropertyValue.Type에 해당하는 필드 하나만 채운다.
type propertyPayload struct {
	Title    []propertyRichText `json:"title,omitempty"`
	Date     *dateValue         `json:"date,omitempty"`
	URL      string             `json:"url,omitempty"`
	Select   *namedOption       `json:"select,omitempty"`
	Status   *namedOption       `json:"status,omitempty"`
	RichText []propertyRichText `json:"rich_text,omitempty"`
}

// buildPropertyPayload는 PropertyValue를 타입별 요청 페이로드로 변환한다.
// 지원하지 않는 타입이면 에러를 반환한다.
func buildPropertyPayload(pv PropertyValue) (propertyPayload, error) {
	switch pv.Type {
	case "title":
		return propertyPayload{Title: []propertyRichText{{Text: Text{Content: pv.Value}}}}, nil
	case "date":
		return propertyPayload{Date: &dateValue{Start: pv.Value}}, nil
	case "url":
		return propertyPayload{URL: pv.Value}, nil
	case "select":
		return propertyPayload{Select: &namedOption{Name: pv.Value}}, nil
	case "status":
		return propertyPayload{Status: &namedOption{Name: pv.Value}}, nil
	case "rich_text":
		return propertyPayload{RichText: []propertyRichText{{Text: Text{Content: pv.Value}}}}, nil
	default:
		return propertyPayload{}, fmt.Errorf("지원하지 않는 속성 타입 %q", pv.Type)
	}
}

// equalsCondition은 쿼리 필터의 equals 조건이다.
type equalsCondition struct {
	Equals string `json:"equals"`
}

// queryFilter는 데이터 소스 쿼리의 필터다. Date 또는 Title 중 하나만 채운다.
type queryFilter struct {
	Property string           `json:"property"`
	Date     *equalsCondition `json:"date,omitempty"`
	Title    *equalsCondition `json:"title,omitempty"`
}

// queryRequest는 POST /v1/data_sources/{id}/query 요청 본문이다.
type queryRequest struct {
	Filter   queryFilter `json:"filter"`
	PageSize int         `json:"page_size"`
}

// queryResponse는 쿼리 응답에서 results 배열만 매핑한다.
// 존재 여부 확인에는 배열 길이만 필요하므로 원소 내용은 디코딩하지 않는다.
type queryResponse struct {
	Results []json.RawMessage `json:"results"`
}

// pageParent는 페이지 생성 요청의 parent다. 데이터 소스를 부모로 지정한다.
type pageParent struct {
	Type         string `json:"type"`
	DataSourceID string `json:"data_source_id"`
}

// createPageRequest는 POST /v1/pages 요청 본문이다.
// properties는 키(속성 이름)가 동적이므로 map을 사용한다 (CLAUDE.md 규칙의 예외).
type createPageRequest struct {
	Parent     pageParent                 `json:"parent"`
	Properties map[string]propertyPayload `json:"properties"`
	Children   []Block                    `json:"children,omitempty"`
}

// createPageResponse는 페이지 생성 응답에서 id만 매핑한다.
type createPageResponse struct {
	ID string `json:"id"`
}

// appendBlocksRequest는 PATCH /v1/blocks/{id}/children 요청 본문이다.
type appendBlocksRequest struct {
	Children []Block `json:"children"`
}

// ExistsByDate는 Date 속성(date.equals)으로 페이지 존재 여부를 확인한다.
// POST /v1/data_sources/{id}/query를 page_size 1로 호출한다.
func (c *Client) ExistsByDate(ctx context.Context, dataSourceID, propertyName, dateISO string) (bool, error) {
	return c.queryExists(ctx, dataSourceID, queryFilter{
		Property: propertyName,
		Date:     &equalsCondition{Equals: dateISO},
	})
}

// ExistsByTitle은 title 속성(title.equals)으로 페이지 존재 여부를 확인한다.
func (c *Client) ExistsByTitle(ctx context.Context, dataSourceID, propertyName, title string) (bool, error) {
	return c.queryExists(ctx, dataSourceID, queryFilter{
		Property: propertyName,
		Title:    &equalsCondition{Equals: title},
	})
}

// queryExists는 필터로 데이터 소스를 page_size 1로 쿼리하여
// 결과가 하나라도 있는지 반환한다.
func (c *Client) queryExists(ctx context.Context, dataSourceID string, filter queryFilter) (bool, error) {
	reqBody, err := json.Marshal(queryRequest{Filter: filter, PageSize: 1})
	if err != nil {
		return false, fmt.Errorf("encode query request: %w", err)
	}

	body, err := c.do(ctx, http.MethodPost, "/v1/data_sources/"+url.PathEscape(dataSourceID)+"/query", reqBody)
	if err != nil {
		return false, fmt.Errorf("query data source %s: %w", dataSourceID, err)
	}

	var resp queryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, fmt.Errorf("decode query response %s: %w", dataSourceID, err)
	}
	return len(resp.Results) > 0, nil
}

// CreatePage는 데이터 소스에 페이지를 생성한다. children은 최대 100개까지만
// 본 요청에 싣고, 초과분은 AppendBlocks로 100개씩 나눠 보낸다.
// 부분 실패(생성 성공 후 append 실패) 시 생성된 페이지 ID를 담은 에러를 반환한다
// (로그로 수동 정리를 돕기 위함, PRD FR-2 6항).
// properties는 노션 속성 이름 -> PropertyValue이며, 빈 Value인 속성은 보내지 않는다.
func (c *Client) CreatePage(ctx context.Context, dataSourceID string, properties map[string]PropertyValue, children []Block) (string, error) {
	props := make(map[string]propertyPayload, len(properties))
	for name, pv := range properties {
		if pv.Value == "" {
			continue
		}
		payload, err := buildPropertyPayload(pv)
		if err != nil {
			return "", fmt.Errorf("build property %q: %w", name, err)
		}
		props[name] = payload
	}

	first := children
	var rest []Block
	if len(children) > maxChildrenPerRequest {
		first = children[:maxChildrenPerRequest]
		rest = children[maxChildrenPerRequest:]
	}

	reqBody, err := json.Marshal(createPageRequest{
		Parent:     pageParent{Type: "data_source_id", DataSourceID: dataSourceID},
		Properties: props,
		Children:   first,
	})
	if err != nil {
		return "", fmt.Errorf("encode create page request: %w", err)
	}

	body, err := c.do(ctx, http.MethodPost, "/v1/pages", reqBody)
	if err != nil {
		return "", fmt.Errorf("create page in data source %s: %w", dataSourceID, err)
	}

	var resp createPageResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode create page response: %w", err)
	}

	if err := c.AppendBlocks(ctx, resp.ID, rest); err != nil {
		return resp.ID, fmt.Errorf("페이지 %s가 부분 생성됨(블록 append 실패, 수동 정리 필요): %w", resp.ID, err)
	}
	return resp.ID, nil
}

// AppendBlocks는 PATCH /v1/blocks/{blockID}/children로 블록을 100개씩 나눠 추가한다.
// children이 비어 있으면 아무 요청도 보내지 않는다.
func (c *Client) AppendBlocks(ctx context.Context, blockID string, children []Block) error {
	for start := 0; start < len(children); start += maxChildrenPerRequest {
		end := min(start+maxChildrenPerRequest, len(children))
		reqBody, err := json.Marshal(appendBlocksRequest{Children: children[start:end]})
		if err != nil {
			return fmt.Errorf("encode append blocks request: %w", err)
		}
		if _, err := c.do(ctx, http.MethodPatch, "/v1/blocks/"+url.PathEscape(blockID)+"/children", reqBody); err != nil {
			return fmt.Errorf("append blocks %d-%d to %s: %w", start, end, blockID, err)
		}
	}
	return nil
}
