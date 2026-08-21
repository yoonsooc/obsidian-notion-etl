package config

import (
	"fmt"
	"os"
	"strings"
)

const notionTokenKey = "NOTION_TOKEN"

// LoadNotionToken reads NOTION_TOKEN from a .env file. Parsing is line-based
// KEY=VALUE; blank lines and '#' comments are ignored. A missing file, key,
// or empty value is an error.
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

// trimQuotes removes one matching pair of surrounding quotes.
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
