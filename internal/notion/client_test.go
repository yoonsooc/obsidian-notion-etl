package notion

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient returns a client pointed at an httptest server.
func newTestClient(serverURL string) *Client {
	c := NewClient("test-token")
	c.baseURL = serverURL
	return c
}

const sampleDatabaseJSON = `{
	"object": "database",
	"id": "b8842240-0123-40b9-bff4-a51f2a0fd668",
	"title": [
		{"type": "text", "plain_text": "Columbus"},
		{"type": "text", "plain_text": "/Views"}
	],
	"data_sources": [
		{"id": "d5f11111-2222-3333-4444-555566667777", "name": "Platinum"}
	]
}`

const sampleDataSourceJSON = `{
	"object": "data_source",
	"id": "d5f11111-2222-3333-4444-555566667777",
	"title": [
		{"type": "text", "plain_text": "Platinum"}
	],
	"properties": {
		"Name": {"id": "title", "name": "Name", "type": "title"},
		"Date": {"id": "abc1", "name": "Date", "type": "date"},
		"Tags": {"id": "abc2", "name": "Tags", "type": "multi_select"},
		"URL": {"id": "abc3", "name": "URL", "type": "url"}
	}
}`

func TestRetrieveDatabase(t *testing.T) {
	var gotPath, gotAuth, gotVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("Notion-Version")
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(sampleDatabaseJSON)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	db, err := c.RetrieveDatabase(context.Background(), "b8842240-0123-40b9-bff4-a51f2a0fd668")
	if err != nil {
		t.Fatalf("RetrieveDatabase() error = %v", err)
	}

	if want := "/v1/databases/b8842240-0123-40b9-bff4-a51f2a0fd668"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}
	if want := "Bearer test-token"; gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
	if gotVersion != apiVersion {
		t.Errorf("Notion-Version header = %q, want %q", gotVersion, apiVersion)
	}

	if want := "b8842240-0123-40b9-bff4-a51f2a0fd668"; db.ID != want {
		t.Errorf("db.ID = %q, want %q", db.ID, want)
	}
	if want := "Columbus/Views"; db.Title != want {
		t.Errorf("db.Title = %q, want %q", db.Title, want)
	}

	wantRefs := []DataSourceRef{
		{ID: "d5f11111-2222-3333-4444-555566667777", Name: "Platinum"},
	}
	if len(db.DataSources) != len(wantRefs) {
		t.Fatalf("len(db.DataSources) = %d, want %d", len(db.DataSources), len(wantRefs))
	}
	for i, want := range wantRefs {
		if db.DataSources[i] != want {
			t.Errorf("db.DataSources[%d] = %+v, want %+v", i, db.DataSources[i], want)
		}
	}
}

func TestRetrieveDataSource(t *testing.T) {
	var gotPath, gotVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotVersion = r.Header.Get("Notion-Version")
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(sampleDataSourceJSON)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	ds, err := c.RetrieveDataSource(context.Background(), "d5f11111-2222-3333-4444-555566667777")
	if err != nil {
		t.Fatalf("RetrieveDataSource() error = %v", err)
	}

	if want := "/v1/data_sources/d5f11111-2222-3333-4444-555566667777"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}
	if gotVersion != apiVersion {
		t.Errorf("Notion-Version header = %q, want %q", gotVersion, apiVersion)
	}
	if want := "Platinum"; ds.Title != want {
		t.Errorf("ds.Title = %q, want %q", ds.Title, want)
	}

	// Properties must be sorted by name.
	wantProps := []Property{
		{Name: "Date", Type: "date"},
		{Name: "Name", Type: "title"},
		{Name: "Tags", Type: "multi_select"},
		{Name: "URL", Type: "url"},
	}
	if len(ds.Properties) != len(wantProps) {
		t.Fatalf("len(ds.Properties) = %d, want %d", len(ds.Properties), len(wantProps))
	}
	for i, want := range wantProps {
		if ds.Properties[i] != want {
			t.Errorf("ds.Properties[%d] = %+v, want %+v", i, ds.Properties[i], want)
		}
	}
}

func TestRetrieveDatabaseRetryOn429(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(sampleDatabaseJSON)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	db, err := c.RetrieveDatabase(context.Background(), "b8842240012340b9bff4a51f2a0fd668")
	if err != nil {
		t.Fatalf("RetrieveDatabase() error = %v", err)
	}
	if calls != 3 {
		t.Errorf("server calls = %d, want 3 (429 두 번 후 성공)", calls)
	}
	if db.Title != "Columbus/Views" {
		t.Errorf("db.Title = %q, want %q", db.Title, "Columbus/Views")
	}
}

func TestRetrieveDatabaseRetryExhausted(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	if _, err := c.RetrieveDatabase(context.Background(), "b8842240012340b9bff4a51f2a0fd668"); err == nil {
		t.Fatal("RetrieveDatabase() error = nil, want rate limited error")
	} else if !strings.Contains(err.Error(), "429") {
		t.Errorf("error = %v, want message containing 429", err)
	}
	// 1 initial call + 3 retries = 4 calls.
	if calls != 4 {
		t.Errorf("server calls = %d, want 4", calls)
	}
}

func TestRetrieveDatabaseUnauthorized(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
		if _, err := w.Write([]byte(`{"code":"unauthorized"}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	if _, err := c.RetrieveDatabase(context.Background(), "b8842240012340b9bff4a51f2a0fd668"); err == nil {
		t.Fatal("RetrieveDatabase() error = nil, want unauthorized error")
	} else if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %v, want message containing 401", err)
	}
	if calls != 1 {
		t.Errorf("server calls = %d, want 1 (401은 재시도 없음)", calls)
	}
}

func TestRetrieveDatabaseNotFound(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	if _, err := c.RetrieveDatabase(context.Background(), "b8842240012340b9bff4a51f2a0fd668"); err == nil {
		t.Fatal("RetrieveDatabase() error = nil, want not found error")
	} else if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %v, want message containing 404", err)
	}
	if calls != 1 {
		t.Errorf("server calls = %d, want 1 (404는 재시도 없음)", calls)
	}
}

func TestExtractDatabaseID(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		want    string
		wantErr bool
	}{
		{
			name:   "ID만 있는 URL과 쿼리스트링",
			rawURL: "https://www.notion.so/b8842240012340b9bff4a51f2a0fd668?v=abcdef1234567890abcdef1234567890",
			want:   "b8842240-0123-40b9-bff4-a51f2a0fd668",
		},
		{
			name:   "제목-ID 형태의 세그먼트",
			rawURL: "https://www.notion.so/My-Database-b8842240012340b9bff4a51f2a0fd668",
			want:   "b8842240-0123-40b9-bff4-a51f2a0fd668",
		},
		{
			name:   "워크스페이스 경로 포함",
			rawURL: "https://www.notion.so/myworkspace/Daily-Notes-b8842240012340b9bff4a51f2a0fd668?v=1&pvs=4",
			want:   "b8842240-0123-40b9-bff4-a51f2a0fd668",
		},
		{
			name:   "대문자 hex ID",
			rawURL: "https://www.notion.so/B8842240012340B9BFF4A51F2A0FD668",
			want:   "b8842240-0123-40b9-bff4-a51f2a0fd668",
		},
		{
			name:   "끝에 슬래시가 붙은 URL",
			rawURL: "https://www.notion.so/b8842240012340b9bff4a51f2a0fd668/",
			want:   "b8842240-0123-40b9-bff4-a51f2a0fd668",
		},
		{
			name:    "ID가 없는 URL",
			rawURL:  "https://www.notion.so/",
			wantErr: true,
		},
		{
			name:    "세그먼트가 32자보다 짧은 URL",
			rawURL:  "https://www.notion.so/short-segment",
			wantErr: true,
		},
		{
			name:    "32자이지만 hex가 아닌 세그먼트",
			rawURL:  "https://www.notion.so/zzzz2240012340b9bff4a51f2a0fd6zz",
			wantErr: true,
		},
		{
			name:    "파싱할 수 없는 URL",
			rawURL:  "://invalid-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractDatabaseID(tt.rawURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ExtractDatabaseID(%q) error = nil, want error", tt.rawURL)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExtractDatabaseID(%q) error = %v", tt.rawURL, err)
			}
			if got != tt.want {
				t.Errorf("ExtractDatabaseID(%q) = %q, want %q", tt.rawURL, got, tt.want)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string // time.Duration.String()
	}{
		{name: "정수 초", header: "2", want: "2s"},
		{name: "0초", header: "0", want: "0s"},
		{name: "헤더 없음", header: "", want: "1s"},
		{name: "숫자가 아닌 값", header: "soon", want: "1s"},
		{name: "음수", header: "-1", want: "1s"},
		{name: "상한 초과는 60초로 클램프", header: "3600", want: "1m0s"},
		{name: "상한 값 그대로", header: "60", want: "1m0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRetryAfter(tt.header); got.String() != tt.want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}
