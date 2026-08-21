package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// assertJSONEq checks that two JSON bodies are structurally equal.
func assertJSONEq(t *testing.T, got []byte, want string) {
	t.Helper()
	var gotV, wantV any
	if err := json.Unmarshal(got, &gotV); err != nil {
		t.Fatalf("unmarshal got JSON: %v (body=%s)", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantV); err != nil {
		t.Fatalf("unmarshal want JSON: %v", err)
	}
	if !reflect.DeepEqual(gotV, wantV) {
		t.Errorf("JSON 불일치\n got: %s\nwant: %s", got, want)
	}
}

func TestExistsByDate(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     bool
	}{
		{name: "페이지 존재", response: `{"results":[{"id":"p1"}]}`, want: true},
		{name: "페이지 부재", response: `{"results":[]}`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			var gotBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				var err error
				gotBody, err = io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request body: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(tt.response)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()

			c := newTestClient(server.URL)
			got, err := c.ExistsByDate(context.Background(), "ds-1", "Date", "2026-01-02")
			if err != nil {
				t.Fatalf("ExistsByDate() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ExistsByDate() = %v, want %v", got, tt.want)
			}
			if gotMethod != http.MethodPost {
				t.Errorf("request method = %q, want POST", gotMethod)
			}
			if want := "/v1/data_sources/ds-1/query"; gotPath != want {
				t.Errorf("request path = %q, want %q", gotPath, want)
			}
			assertJSONEq(t, gotBody, `{"filter":{"property":"Date","date":{"equals":"2026-01-02"}},"page_size":1}`)
		})
	}
}

func TestExistsByTitle(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     bool
	}{
		{name: "페이지 존재", response: `{"results":[{"id":"p1"}]}`, want: true},
		{name: "페이지 부재", response: `{"results":[]}`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			var gotBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				var err error
				gotBody, err = io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request body: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(tt.response)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()

			c := newTestClient(server.URL)
			got, err := c.ExistsByTitle(context.Background(), "ds-1", "Name", "DN_251101")
			if err != nil {
				t.Fatalf("ExistsByTitle() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ExistsByTitle() = %v, want %v", got, tt.want)
			}
			if want := "/v1/data_sources/ds-1/query"; gotPath != want {
				t.Errorf("request path = %q, want %q", gotPath, want)
			}
			assertJSONEq(t, gotBody, `{"filter":{"property":"Name","title":{"equals":"DN_251101"}},"page_size":1}`)
		})
	}
}

func TestCreatePagePropertySerialization(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"id":"page-123"}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	properties := map[string]PropertyValue{
		"Name":   {Type: "title", Value: "DN_251101"},
		"Date":   {Type: "date", Value: "2025-11-01"},
		"URI":    {Type: "url", Value: "obsidian://open?vault=v&file=f"},
		"Type":   {Type: "select", Value: "Diary"},
		"Status": {Type: "status", Value: "Done"},
		"Memo":   {Type: "rich_text", Value: "메모"},
		"Empty":  {Type: "select", Value: ""}, // empty Value must not be sent
	}
	pageID, err := c.CreatePage(context.Background(), "ds-1", properties, nil)
	if err != nil {
		t.Fatalf("CreatePage() error = %v", err)
	}
	if want := "page-123"; pageID != want {
		t.Errorf("pageID = %q, want %q", pageID, want)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("request method = %q, want POST", gotMethod)
	}
	if want := "/v1/pages"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}
	assertJSONEq(t, gotBody, `{
		"parent": {"type": "data_source_id", "data_source_id": "ds-1"},
		"properties": {
			"Name":   {"title": [{"text": {"content": "DN_251101"}}]},
			"Date":   {"date": {"start": "2025-11-01"}},
			"URI":    {"url": "obsidian://open?vault=v&file=f"},
			"Type":   {"select": {"name": "Diary"}},
			"Status": {"status": {"name": "Done"}},
			"Memo":   {"rich_text": [{"text": {"content": "메모"}}]}
		}
	}`)
}

func TestCreatePageUnsupportedPropertyType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("지원하지 않는 타입은 요청 전에 실패해야 한다")
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	properties := map[string]PropertyValue{
		"Tags": {Type: "multi_select", Value: "a"},
	}
	if _, err := c.CreatePage(context.Background(), "ds-1", properties, nil); err == nil {
		t.Fatal("CreatePage() error = nil, want unsupported type error")
	} else if !strings.Contains(err.Error(), "multi_select") {
		t.Errorf("error = %v, want message containing multi_select", err)
	}
}

// countChildren returns the length of the children array in a request body.
func countChildren(t *testing.T, body []byte) int {
	t.Helper()
	var payload struct {
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal children: %v", err)
	}
	return len(payload.Children)
}

func TestCreatePageSplitsChildren(t *testing.T) {
	type call struct {
		method   string
		path     string
		children int
	}
	var calls []call
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		calls = append(calls, call{method: r.Method, path: r.URL.Path, children: countChildren(t, body)})
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"id":"page-123"}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	children := make([]Block, 0, 150)
	for i := 0; i < 150; i++ {
		children = append(children, NewParagraph(fmt.Sprintf("문단 %d", i)))
	}

	c := newTestClient(server.URL)
	properties := map[string]PropertyValue{"Name": {Type: "title", Value: "note"}}
	pageID, err := c.CreatePage(context.Background(), "ds-1", properties, children)
	if err != nil {
		t.Fatalf("CreatePage() error = %v", err)
	}
	if want := "page-123"; pageID != want {
		t.Errorf("pageID = %q, want %q", pageID, want)
	}

	want := []call{
		{method: http.MethodPost, path: "/v1/pages", children: 100},
		{method: http.MethodPatch, path: "/v1/blocks/page-123/children", children: 50},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %+v, want %+v", calls, want)
	}
}

func TestCreatePageAppendFailureIncludesPageID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"id":"page-err"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(`{"code":"validation_error"}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	children := make([]Block, 0, 150)
	for i := 0; i < 150; i++ {
		children = append(children, NewParagraph("문단"))
	}

	c := newTestClient(server.URL)
	properties := map[string]PropertyValue{"Name": {Type: "title", Value: "note"}}
	pageID, err := c.CreatePage(context.Background(), "ds-1", properties, children)
	if err == nil {
		t.Fatal("CreatePage() error = nil, want partial failure error")
	}
	if !strings.Contains(err.Error(), "page-err") {
		t.Errorf("error = %v, want message containing page-err", err)
	}
	if want := "page-err"; pageID != want {
		t.Errorf("pageID = %q, want %q (부분 실패 시에도 ID를 반환)", pageID, want)
	}
}

func TestAppendBlocksChunks(t *testing.T) {
	var paths []string
	var counts []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("request method = %q, want PATCH", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		paths = append(paths, r.URL.Path)
		counts = append(counts, countChildren(t, body))
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"object":"list","results":[]}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	children := make([]Block, 0, 250)
	for i := 0; i < 250; i++ {
		children = append(children, NewParagraph("문단"))
	}

	c := newTestClient(server.URL)
	if err := c.AppendBlocks(context.Background(), "block-1", children); err != nil {
		t.Fatalf("AppendBlocks() error = %v", err)
	}

	if wantCounts := []int{100, 100, 50}; !reflect.DeepEqual(counts, wantCounts) {
		t.Errorf("children counts = %v, want %v", counts, wantCounts)
	}
	for i, path := range paths {
		if want := "/v1/blocks/block-1/children"; path != want {
			t.Errorf("paths[%d] = %q, want %q", i, path, want)
		}
	}
}

func TestAppendBlocksEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("빈 children으로는 요청을 보내지 않아야 한다")
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	if err := c.AppendBlocks(context.Background(), "block-1", nil); err != nil {
		t.Fatalf("AppendBlocks() error = %v", err)
	}
}
