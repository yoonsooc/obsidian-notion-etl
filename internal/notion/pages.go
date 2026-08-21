package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// maxChildrenPerRequest is Notion's per-request limit on children blocks.
const maxChildrenPerRequest = 100

// PropertyValue is one page property value. Supported types: "title", "date",
// "url", "select", "status", "rich_text".
type PropertyValue struct {
	Type  string
	Value string
}

type propertyRichText struct {
	Text Text `json:"text"`
}

type dateValue struct {
	Start string `json:"start"`
}

// namedOption is the payload value for select/status properties.
type namedOption struct {
	Name string `json:"name"`
}

// propertyPayload is the request payload for one page property; only the field
// matching PropertyValue.Type is set.
type propertyPayload struct {
	Title    []propertyRichText `json:"title,omitempty"`
	Date     *dateValue         `json:"date,omitempty"`
	URL      string             `json:"url,omitempty"`
	Select   *namedOption       `json:"select,omitempty"`
	Status   *namedOption       `json:"status,omitempty"`
	RichText []propertyRichText `json:"rich_text,omitempty"`
}

// buildPropertyPayload converts a PropertyValue to its typed request payload,
// erroring on unsupported types.
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

type equalsCondition struct {
	Equals string `json:"equals"`
}

// queryFilter is a data source query filter; exactly one of Date or Title is set.
type queryFilter struct {
	Property string           `json:"property"`
	Date     *equalsCondition `json:"date,omitempty"`
	Title    *equalsCondition `json:"title,omitempty"`
}

type queryRequest struct {
	Filter   queryFilter `json:"filter"`
	PageSize int         `json:"page_size"`
}

// queryResponse maps only the results array; existence checks need only its length.
type queryResponse struct {
	Results []json.RawMessage `json:"results"`
}

// pageParent designates the parent data source of a new page.
type pageParent struct {
	Type         string `json:"type"`
	DataSourceID string `json:"data_source_id"`
}

// createPageRequest is the POST /v1/pages body; properties keys are dynamic,
// hence the map.
type createPageRequest struct {
	Parent     pageParent                 `json:"parent"`
	Properties map[string]propertyPayload `json:"properties"`
	Children   []Block                    `json:"children,omitempty"`
}

type createPageResponse struct {
	ID string `json:"id"`
}

type appendBlocksRequest struct {
	Children []Block `json:"children"`
}

// ExistsByDate reports whether a page exists with the given date property value.
func (c *Client) ExistsByDate(ctx context.Context, dataSourceID, propertyName, dateISO string) (bool, error) {
	return c.queryExists(ctx, dataSourceID, queryFilter{
		Property: propertyName,
		Date:     &equalsCondition{Equals: dateISO},
	})
}

// ExistsByTitle reports whether a page exists with the given title property value.
func (c *Client) ExistsByTitle(ctx context.Context, dataSourceID, propertyName, title string) (bool, error) {
	return c.queryExists(ctx, dataSourceID, queryFilter{
		Property: propertyName,
		Title:    &equalsCondition{Equals: title},
	})
}

// queryExists queries the data source with page_size 1 and reports whether any result exists.
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

// CreatePage creates a page in a data source. The first 100 children ride on
// the create request; the rest are sent via AppendBlocks in batches of 100.
// On partial failure (page created, append failed) the error carries the page
// ID for manual cleanup. Properties with an empty Value are omitted.
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

// AppendBlocks appends children to a block in batches of 100; empty input sends no request.
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
