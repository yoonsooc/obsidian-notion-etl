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

// Property is the name and type of a Notion data source column.
type Property struct {
	Name string
	Type string // "title", "date", "select", "url", "rich_text", ...
}

// DataSourceRef is a reference to one data source contained in a database.
type DataSourceRef struct {
	ID   string
	Name string
}

// Database is a Notion database container. Since API 2025-09-03 the property
// schema belongs to the data source, not the database; use RetrieveDataSource
// with a DataSources ID to get it.
type Database struct {
	ID          string
	Title       string
	DataSources []DataSourceRef
}

// DataSource is the schema of a Notion data source.
type DataSource struct {
	ID         string
	Title      string
	Properties []Property
}

// databaseResponse maps the needed subset of GET /v1/databases/{id}.
type databaseResponse struct {
	ID          string              `json:"id"`
	Title       []richText          `json:"title"`
	DataSources []dataSourceRefJSON `json:"data_sources"`
}

type dataSourceRefJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// dataSourceResponse maps the needed subset of GET /v1/data_sources/{id};
// properties keys are dynamic, hence the map.
type dataSourceResponse struct {
	ID         string                    `json:"id"`
	Title      []richText                `json:"title"`
	Properties map[string]propertyScheme `json:"properties"`
}

// richText maps only plain_text from a Notion rich text element.
type richText struct {
	PlainText string `json:"plain_text"`
}

type propertyScheme struct {
	Type string `json:"type"`
}

// RetrieveDatabase fetches a database container and its data source references.
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

// RetrieveDataSource fetches a data source schema; Properties are sorted by
// name for deterministic order.
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

// joinRichText concatenates the plain_text of a rich text array.
func joinRichText(parts []richText) string {
	var b strings.Builder
	for _, rt := range parts {
		b.WriteString(rt.PlainText)
	}
	return b.String()
}

// ExtractDatabaseID extracts the 32-hex-digit ID from a Notion database URL
// and returns it as a hyphenated UUID. The last path segment may be
// "Title-ID", so the trailing 32 characters are used.
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

func lastPathSegment(path string) string {
	trimmed := strings.TrimSuffix(path, "/")
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		return trimmed[idx+1:]
	}
	return trimmed
}

// isHex reports whether s consists only of lowercase hex characters.
func isHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
