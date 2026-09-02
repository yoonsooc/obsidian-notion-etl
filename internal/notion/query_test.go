package notion

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pageJSON(id, edited, title, date string) string {
	dateVal := "null"
	if date != "" {
		dateVal = `{"type":"date","date":{"start":"` + date + `"}}`
	} else {
		dateVal = `{"type":"date","date":null}`
	}
	return `{
		"id": "` + id + `",
		"last_edited_time": "` + edited + `",
		"properties": {
			"Name": {"type":"title","title":[{"type":"text","text":{"content":"` + title + `"},"plain_text":"` + title + `"}]},
			"Date": ` + dateVal + `
		}
	}`
}

func TestQueryPagesSincePaginatesAndFilters(t *testing.T) {
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		if len(bodies) == 1 {
			_, _ = w.Write([]byte(`{"results":[` + pageJSON("p1", "2026-08-01T00:00:00Z", "첫 노트", "2026-08-01") + `],
				"has_more": true, "next_cursor": "cur-2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[` + pageJSON("p2", "2026-08-02T00:00:00Z", "제목: 콜론", "") + `],
			"has_more": false, "next_cursor": null}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	pages, err := c.QueryPagesSince(context.Background(), "ds-1", "2026-07-31T00:00:00Z", "Name", "Date")
	if err != nil {
		t.Fatalf("QueryPagesSince() error = %v", err)
	}

	if len(bodies) != 2 {
		t.Fatalf("request count = %d, want 2 (pagination)", len(bodies))
	}
	var first, second map[string]any
	if err := json.Unmarshal(bodies[0], &first); err != nil {
		t.Fatalf("decode first request: %v", err)
	}
	if err := json.Unmarshal(bodies[1], &second); err != nil {
		t.Fatalf("decode second request: %v", err)
	}
	if _, ok := first["filter"]; !ok {
		t.Error("first request has no last_edited_time filter")
	}
	if _, ok := first["start_cursor"]; ok {
		t.Error("first request must not carry start_cursor")
	}
	if got := second["start_cursor"]; got != "cur-2" {
		t.Errorf("second request start_cursor = %v, want cur-2", got)
	}

	want := []Page{
		{ID: "p1", LastEditedTime: "2026-08-01T00:00:00Z", Title: "첫 노트", Date: "2026-08-01"},
		{ID: "p2", LastEditedTime: "2026-08-02T00:00:00Z", Title: "제목: 콜론", Date: ""},
	}
	if len(pages) != len(want) {
		t.Fatalf("pages = %+v, want %+v", pages, want)
	}
	for i := range want {
		if pages[i] != want[i] {
			t.Errorf("pages[%d] = %+v, want %+v", i, pages[i], want[i])
		}
	}
}

func TestQueryPagesSinceEmptyWatermarkOmitsFilter(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[],"has_more":false}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	if _, err := c.QueryPagesSince(context.Background(), "ds-1", "", "Name", "Date"); err != nil {
		t.Fatalf("QueryPagesSince() error = %v", err)
	}
	if strings.Contains(string(body), "filter") {
		t.Errorf("empty watermark must send no filter, got body %s", body)
	}
}

func TestQueryPagesSinceFallsBackToTitleTypeScan(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Title property renamed to "이름": lookup by snapshot name "Name" misses.
		_, _ = w.Write([]byte(`{"results":[{
			"id": "p1", "last_edited_time": "2026-08-01T00:00:00Z",
			"properties": {"이름": {"type":"title","title":[{"type":"text","text":{"content":""},"plain_text":"이름으로 찾음"}]}}
		}], "has_more": false}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	pages, err := c.QueryPagesSince(context.Background(), "ds-1", "", "Name", "Date")
	if err != nil {
		t.Fatalf("QueryPagesSince() error = %v", err)
	}
	if len(pages) != 1 || pages[0].Title != "이름으로 찾음" {
		t.Errorf("pages = %+v, want title found via type scan", pages)
	}
}

func TestListBlockChildrenPaginates(t *testing.T) {
	var cursors []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursors = append(cursors, r.URL.Query().Get("start_cursor"))
		w.Header().Set("Content-Type", "application/json")
		if len(cursors) == 1 {
			_, _ = w.Write([]byte(`{"results":[
				{"object":"block","type":"paragraph","paragraph":{"rich_text":[{"type":"text","text":{"content":"a"},"plain_text":"a"}]}}
			], "has_more": true, "next_cursor": "cur-2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[
			{"object":"block","type":"to_do","to_do":{"rich_text":[{"type":"text","text":{"content":"b"},"plain_text":"b"}],"checked":true},"has_children":true}
		], "has_more": false}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	blocks, err := c.ListBlockChildren(context.Background(), "page-1")
	if err != nil {
		t.Fatalf("ListBlockChildren() error = %v", err)
	}
	if len(cursors) != 2 || cursors[0] != "" || cursors[1] != "cur-2" {
		t.Fatalf("cursors = %v, want [\"\", cur-2]", cursors)
	}
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if blocks[0].Type != "paragraph" || JoinPlainText(blocks[0].Paragraph.RichText) != "a" {
		t.Errorf("blocks[0] = %+v, want paragraph a", blocks[0])
	}
	if blocks[1].Type != "to_do" || !blocks[1].ToDo.Checked || !blocks[1].HasChildren {
		t.Errorf("blocks[1] = %+v, want checked to_do with has_children", blocks[1])
	}
}

func TestBlockUnmarshalFallback(t *testing.T) {
	// toggle: unsupported type with rich_text -> Fallback extracted.
	var toggle Block
	if err := json.Unmarshal([]byte(`{"object":"block","type":"toggle",
		"toggle":{"rich_text":[{"type":"text","text":{"content":"토글"},"plain_text":"토글"}]}}`), &toggle); err != nil {
		t.Fatalf("unmarshal toggle: %v", err)
	}
	if got := JoinPlainText(toggle.Fallback); got != "토글" {
		t.Errorf("toggle Fallback = %q, want 토글", got)
	}

	// quote/callout are typed now: no Fallback, bodies decode into their fields.
	var quote Block
	if err := json.Unmarshal([]byte(`{"object":"block","type":"quote",
		"quote":{"rich_text":[{"type":"text","text":{"content":"인용문"},"plain_text":"인용문"}]}}`), &quote); err != nil {
		t.Fatalf("unmarshal quote: %v", err)
	}
	if quote.Quote == nil || len(quote.Fallback) != 0 {
		t.Errorf("quote = %+v, want typed body without Fallback", quote)
	}

	// divider: no rich_text -> Fallback stays empty.
	var divider Block
	if err := json.Unmarshal([]byte(`{"object":"block","type":"divider","divider":{}}`), &divider); err != nil {
		t.Fatalf("unmarshal divider: %v", err)
	}
	if len(divider.Fallback) != 0 {
		t.Errorf("divider Fallback = %+v, want empty", divider.Fallback)
	}

	// paragraph: known type must not populate Fallback.
	var para Block
	if err := json.Unmarshal([]byte(`{"object":"block","type":"paragraph",
		"paragraph":{"rich_text":[{"type":"text","text":{"content":"x"},"plain_text":"x"}]}}`), &para); err != nil {
		t.Fatalf("unmarshal paragraph: %v", err)
	}
	if para.Paragraph == nil || len(para.Fallback) != 0 {
		t.Errorf("paragraph = %+v, want typed body only", para)
	}
}
