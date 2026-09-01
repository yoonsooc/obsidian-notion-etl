package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// queryPageSize is Notion's maximum page_size for query/list endpoints.
const queryPageSize = 100

// Page is a queried page's data used by the backup flow.
type Page struct {
	ID             string
	LastEditedTime string
	Title          string // plain text of the title property
	Date           string // start of the date property, empty if unset
}

type timestampCondition struct {
	OnOrAfter string `json:"on_or_after"`
}

// timestampFilter filters a query by page timestamps (last_edited_time).
type timestampFilter struct {
	Timestamp      string              `json:"timestamp"`
	LastEditedTime *timestampCondition `json:"last_edited_time"`
}

type timestampSort struct {
	Timestamp string `json:"timestamp"`
	Direction string `json:"direction"`
}

type pagesQueryRequest struct {
	Filter      *timestampFilter `json:"filter,omitempty"`
	Sorts       []timestampSort  `json:"sorts,omitempty"`
	PageSize    int              `json:"page_size"`
	StartCursor string           `json:"start_cursor,omitempty"`
}

// pagePropertyJSON maps only the property types the backup flow reads.
type pagePropertyJSON struct {
	Type  string     `json:"type"`
	Title []RichText `json:"title,omitempty"`
	Date  *dateValue `json:"date,omitempty"`
}

type queryPageJSON struct {
	ID             string                      `json:"id"`
	LastEditedTime string                      `json:"last_edited_time"`
	Properties     map[string]pagePropertyJSON `json:"properties"`
}

type pagesQueryResponse struct {
	Results    []queryPageJSON `json:"results"`
	HasMore    bool            `json:"has_more"`
	NextCursor string          `json:"next_cursor"`
}

// QueryPagesSince returns the data source's pages edited on or after sinceISO
// (RFC3339), oldest first; an empty sinceISO returns every page. All result
// pages are fetched via cursor pagination. titleProp/dateProp name the
// properties to extract (property lookup falls back to a type scan, since the
// schema snapshot may lag a renamed property).
func (c *Client) QueryPagesSince(ctx context.Context, dataSourceID, sinceISO, titleProp, dateProp string) ([]Page, error) {
	req := pagesQueryRequest{
		Sorts:    []timestampSort{{Timestamp: "last_edited_time", Direction: "ascending"}},
		PageSize: queryPageSize,
	}
	if sinceISO != "" {
		req.Filter = &timestampFilter{
			Timestamp:      "last_edited_time",
			LastEditedTime: &timestampCondition{OnOrAfter: sinceISO},
		}
	}

	var pages []Page
	for {
		reqBody, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("encode pages query request: %w", err)
		}
		body, err := c.do(ctx, http.MethodPost, "/v1/data_sources/"+url.PathEscape(dataSourceID)+"/query", reqBody)
		if err != nil {
			return nil, fmt.Errorf("query pages of data source %s: %w", dataSourceID, err)
		}

		var resp pagesQueryResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode pages query response %s: %w", dataSourceID, err)
		}
		for _, raw := range resp.Results {
			pages = append(pages, toPage(raw, titleProp, dateProp))
		}
		if !resp.HasMore || resp.NextCursor == "" {
			return pages, nil
		}
		req.StartCursor = resp.NextCursor
	}
}

// toPage extracts the title and date property values from a raw query result.
func toPage(raw queryPageJSON, titleProp, dateProp string) Page {
	page := Page{ID: raw.ID, LastEditedTime: raw.LastEditedTime}

	if p, ok := raw.Properties[titleProp]; ok && p.Type == "title" {
		page.Title = JoinPlainText(p.Title)
	} else {
		for _, p := range raw.Properties {
			if p.Type == "title" {
				page.Title = JoinPlainText(p.Title)
				break
			}
		}
	}

	if p, ok := raw.Properties[dateProp]; ok && p.Type == "date" && p.Date != nil {
		page.Date = p.Date.Start
	}
	return page
}

type blockChildrenResponse struct {
	Results    []Block `json:"results"`
	HasMore    bool    `json:"has_more"`
	NextCursor string  `json:"next_cursor"`
}

// ListBlockChildren returns a block's direct children in order, fetching all
// result pages via cursor pagination. Nested children are not descended into (v1).
func (c *Client) ListBlockChildren(ctx context.Context, blockID string) ([]Block, error) {
	var blocks []Block
	cursor := ""
	for {
		path := "/v1/blocks/" + url.PathEscape(blockID) + "/children?page_size=" + fmt.Sprint(queryPageSize)
		if cursor != "" {
			path += "&start_cursor=" + url.QueryEscape(cursor)
		}
		body, err := c.do(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("list block children of %s: %w", blockID, err)
		}

		var resp blockChildrenResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode block children response %s: %w", blockID, err)
		}
		blocks = append(blocks, resp.Results...)
		if !resp.HasMore || resp.NextCursor == "" {
			return blocks, nil
		}
		cursor = resp.NextCursor
	}
}
