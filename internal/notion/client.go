// Package notion provides a Notion API client with built-in rate limiting
// and 429 retry handling.
package notion

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

// apiVersion is sent in the Notion-Version header. Since API 2025-09-03,
// databases and data sources are separate: schema retrieval and queries use
// the /v1/data_sources endpoints.
const apiVersion = "2026-03-11"

const defaultBaseURL = "https://api.notion.com"

// maxRetries429 is the maximum number of retries for 429 responses.
const maxRetries429 = 3

// defaultRetryAfter is used when a 429 response has no Retry-After header.
const defaultRetryAfter = 1 * time.Second

// maxRetryAfter clamps the Retry-After header so an abnormally large value
// cannot cause a silent long wait.
const maxRetryAfter = 60 * time.Second

// Client is a Notion API client. Every API call passes through the built-in
// rate.Limiter, which cannot be injected or bypassed.
type Client struct {
	token      string
	httpClient *http.Client
	limiter    *rate.Limiter
	baseURL    string
}

// NewClient returns a Client limited to 2.5 req/s (burst 3), staying under
// Notion's 3 req/s rate limit.
func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		limiter:    rate.NewLimiter(rate.Limit(2.5), 3),
		baseURL:    defaultBaseURL,
	}
}

// do waits on the limiter, sends the request, and retries 429 responses up to
// maxRetries429 times honoring Retry-After. 401/404 fail immediately without retry.
func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter wait: %w", err)
		}

		respBody, status, retryAfter, err := c.send(ctx, method, path, body)
		if err != nil {
			return nil, err
		}

		switch {
		case status >= 200 && status < 300:
			return respBody, nil
		case status == http.StatusUnauthorized:
			return nil, fmt.Errorf("notion API unauthorized (401): 통합(integration) 토큰을 확인하라: %s", respBody)
		case status == http.StatusNotFound:
			return nil, fmt.Errorf("notion API not found (404): 데이터베이스 ID와 통합 연결(share) 여부를 확인하라: %s", respBody)
		case status == http.StatusTooManyRequests:
			if attempt >= maxRetries429 {
				return nil, fmt.Errorf("notion API rate limited (429): %d회 재시도 후 실패: %s", maxRetries429, respBody)
			}
			if err := sleepContext(ctx, parseRetryAfter(retryAfter)); err != nil {
				return nil, fmt.Errorf("retry wait: %w", err)
			}
		default:
			return nil, fmt.Errorf("notion API request failed: status %d: %s", status, respBody)
		}
	}
}

// send performs a single HTTP request and returns the body, status code, and Retry-After header.
func (c *Client) send(ctx context.Context, method, path string, body []byte) ([]byte, int, string, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, 0, "", fmt.Errorf("build request %s %s: %w", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Notion-Version", apiVersion)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, "", fmt.Errorf("request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, "", fmt.Errorf("read response body %s %s: %w", method, path, err)
	}
	return respBody, resp.StatusCode, resp.Header.Get("Retry-After"), nil
}

// parseRetryAfter converts a Retry-After header (integer seconds) to a duration,
// falling back to defaultRetryAfter and clamping at maxRetryAfter.
func parseRetryAfter(header string) time.Duration {
	seconds, err := strconv.Atoi(header)
	if err != nil || seconds < 0 {
		return defaultRetryAfter
	}
	d := time.Duration(seconds) * time.Second
	if d > maxRetryAfter {
		return maxRetryAfter
	}
	return d
}

// sleepContext sleeps for d or until the context is canceled.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
