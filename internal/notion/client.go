// Package notion은 Rate Limiter와 429 재시도가 내장된 Notion API 클라이언트를 제공한다.
// M1 범위는 데이터베이스 스키마 조회이며, M2/M3에서 페이지 생성·쿼리 메서드가
// 같은 Client 위에 추가될 수 있는 구조를 갖는다.
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

// apiVersion은 모든 요청의 Notion-Version 헤더에 사용하는 API 버전이다.
// 2025-09-03부터 데이터베이스와 데이터 소스(data source)가 분리되어,
// 속성 스키마 조회와 쿼리는 /v1/data_sources 계열 엔드포인트를 사용한다.
const apiVersion = "2026-03-11"

// defaultBaseURL은 Notion API의 기본 엔드포인트다.
// 테스트에서는 Client.baseURL을 httptest 서버 주소로 교체한다.
const defaultBaseURL = "https://api.notion.com"

// maxRetries429는 429 응답에 대한 최대 재시도 횟수다.
const maxRetries429 = 3

// defaultRetryAfter는 429 응답에 Retry-After 헤더가 없을 때 대기하는 기본 시간이다.
const defaultRetryAfter = 1 * time.Second

// maxRetryAfter는 Retry-After 헤더 값의 상한이다. 서버가 비정상적으로 큰 값
// (예: 3600초)을 돌려줘도 조용히 장시간 대기하지 않도록 클램프한다.
const maxRetryAfter = 60 * time.Second

// Client는 Notion API 클라이언트다.
// 모든 API 호출은 내장된 rate.Limiter를 통과하며, 외부에서 Limiter를
// 주입하거나 우회할 수 없다.
type Client struct {
	token      string
	httpClient *http.Client
	limiter    *rate.Limiter
	baseURL    string
}

// NewClient는 rate.NewLimiter(rate.Limit(2.5), 3)을 내장한 클라이언트를 만든다.
// 모든 API 호출은 반드시 이 Limiter를 통과한다(우회 불가 구조).
func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		limiter:    rate.NewLimiter(rate.Limit(2.5), 3),
		baseURL:    defaultBaseURL,
	}
}

// do는 공통 요청 헬퍼다. Limiter 대기 후 요청을 실행하고,
// 429 응답이면 Retry-After 헤더(초)만큼 대기한 뒤 최대 maxRetries429회 재시도한다.
// 401/404는 재시도 없이 즉시 에러를 반환하며, 그 외 2xx가 아닌 응답도
// 상태코드와 응답 본문을 담은 에러로 반환한다.
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

// send는 단일 HTTP 요청을 실행하고 응답 본문, 상태코드, Retry-After 헤더 값을 반환한다.
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

// parseRetryAfter는 Retry-After 헤더(초 단위 정수)를 time.Duration으로 변환한다.
// 헤더가 없거나 파싱할 수 없으면 defaultRetryAfter를, maxRetryAfter를 넘으면
// maxRetryAfter를 반환한다.
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

// sleepContext는 지정된 시간만큼 대기하되, 컨텍스트 취소 시 즉시 반환한다.
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
