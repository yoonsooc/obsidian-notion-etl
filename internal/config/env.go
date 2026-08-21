package config

import (
	"fmt"
	"os"
	"strings"
)

// notionTokenKey는 .env 파일에서 찾을 노션 API 토큰의 키 이름이다.
const notionTokenKey = "NOTION_TOKEN"

// LoadNotionToken은 .env 파일에서 NOTION_TOKEN 값을 읽는다.
// 파싱은 줄 단위 KEY=VALUE 형식이며, '#'으로 시작하는 줄과 빈 줄은 무시한다.
// 파일이 없거나 키가 없거나 값이 비어 있으면 에러를 반환한다.
func LoadNotionToken(envPath string) (string, error) {
	data, err := os.ReadFile(envPath)
	if err != nil {
		return "", fmt.Errorf(".env 파일 읽기 실패: %w", err)
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) != notionTokenKey {
			continue
		}
		value = trimQuotes(strings.TrimSpace(value))
		if value == "" {
			return "", fmt.Errorf("%s에서 %s 값이 비어 있음", envPath, notionTokenKey)
		}
		return value, nil
	}
	return "", fmt.Errorf("%s에 %s 키가 없음", envPath, notionTokenKey)
}

// trimQuotes는 문자열을 감싼 같은 종류의 따옴표 한 쌍을 제거한다.
func trimQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	first, last := s[0], s[len(s)-1]
	if first == last && (first == '"' || first == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}
