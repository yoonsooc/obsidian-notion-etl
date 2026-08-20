package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ---------------------------------------------------------
// Configuration
// ---------------------------------------------------------
const (
	NotionAPIURL  = "https://api.notion.com/v1/pages"
	NotionToken   = "secret_YOUR_NOTION_TOKEN" // 노션 API 토큰
	DatabaseID    = "YOUR_DATABASE_ID"         // 노션 DB ID
	VaultPath     = "/Users/username/Documents/Obsidian/Daily"
	VaultName     = "MyVault" // 옵시디언 볼트 이름 (URI 생성용)
	TargetDateStr = "2025-11-01"
)

// ---------------------------------------------------------
// 2. 노션 API 페이로드 구조체 정의 (필요한 속성만 엄격하게 매핑)
// ---------------------------------------------------------
type NotionPayload struct {
	Parent     Parent                 `json:"parent"`
	Properties map[string]interface{} `json:"properties"`
	Children   []Block                `json:"children"`
}

type Parent struct {
	DatabaseID string `json:"database_id"`
}

type Block struct {
	Object    string    `json:"object"`
	Type      string    `json:"type"`
	Paragraph Paragraph `json:"paragraph"`
}

type Paragraph struct {
	RichText []RichText `json:"rich_text"`
}

type RichText struct {
	Type string `json:"type"`
	Text Text   `json:"text"`
}

type Text struct {
	Content string `json:"content"`
}

func main() {
	targetDate, err := time.Parse("2006-01-02", TargetDateStr)
	if err != nil {
		log.Fatalf("Invalid target date format: %v", err)
	}

	files, err := os.ReadDir(VaultPath)
	if err != nil {
		log.Fatalf("Failed to read directory: %v", err)
	}

	// 작업 큐 생성
	jobs := make(chan string, len(files))
	
	// 토큰 버킷 알고리즘을 사용한 Rate Limiter (초당 2.5회 허용, 버스트 3)
	// 노션 제한인 3 TPS를 넘지 않도록 보수적으로 2.5로 설정
	limiter := rate.NewLimiter(rate.Limit(2.5), 3)
	ctx := context.Background()

	var wg sync.WaitGroup

	// 워커 풀(Worker Pool) 생성 (동시성 제어)
	numWorkers := 5
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(ctx, &wg, jobs, limiter, targetDate)
	}

	// 큐에 작업 푸시
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".md") {
			jobs <- file.Name()
		}
	}
	close(jobs)

	// 모든 작업이 끝날 때까지 대기
	wg.Wait()
	fmt.Println("Migration completed successfully.")
}

// ---------------------------------------------------------
// 워커 로직: 큐에서 파일을 꺼내어 처리
// ---------------------------------------------------------
func worker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan string, limiter *rate.Limiter, targetDate time.Time) {
	defer wg.Done()
	for filename := range jobs {
		// 파일명(예: 2025-11-05.md)에서 날짜 파싱
		dateStr := strings.TrimSuffix(filename, ".md")
		fileDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue // 날짜 형식이 아닌 파일은 무시
		}

		if fileDate.Before(targetDate) {
			continue // 타겟 날짜 이전 파일 스킵
		}

		// Rate Limiter 대기 (API 속도 제한 준수)
		if err := limiter.Wait(ctx); err != nil {
			log.Printf("Limiter error: %v", err)
			continue
		}

		processFile(filename, dateStr)
	}
}

// ---------------------------------------------------------
// 파일 파싱 및 API 전송
// ---------------------------------------------------------
func processFile(filename, dateStr string) {
	filePath := filepath.Join(VaultPath, filename)
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("Failed to read %s: %v", filename, err)
		return
	}

	content := string(contentBytes)
	category, body := parseFrontmatter(content)

	// 본문을 2000자 단위로 청크 분할 (노션 API Block 텍스트 제한)
	chunks := chunkText(body, 2000)
	var children []Block
	for _, chunk := range chunks {
		children = append(children, Block{
			Object: "block",
			Type:   "paragraph",
			Paragraph: Paragraph{
				RichText: []RichText{
					{Type: "text", Text: Text{Content: chunk}},
				},
			},
		})
	}

	// Obsidian URI 생성 (경로 인코딩 필수)
	encodedPath := url.QueryEscape("Daily/" + filename)
	obsidianURI := fmt.Sprintf("obsidian://open?vault=%s&file=%s", url.QueryEscape(VaultName), encodedPath)

	// 페이로드 프로퍼티 매핑
	properties := map[string]interface{}{
		"Name": map[string]interface{}{
			"title": []map[string]interface{}{
				{"text": map[string]interface{}{"content": dateStr}},
			},
		},
		"Date": map[string]interface{}{
			"date": map[string]interface{}{"start": dateStr},
		},
		"Obsidian_URI": map[string]interface{}{
			"url": obsidianURI,
		},
	}

	// 카테고리가 존재하면 매핑
	if category != "" {
		properties["Type"] = map[string]interface{}{
			"select": map[string]interface{}{"name": category},
		}
	}

	payload := NotionPayload{
		Parent:     Parent{DatabaseID: DatabaseID},
		Properties: properties,
		Children:   children,
	}

	sendToNotion(payload, filename)
}

// YAML Frontmatter 파싱 (정규식 대신 단순 문자열 처리로 오버헤드 최소화)
func parseFrontmatter(content string) (string, string) {
	category := "Diary" // 기본값
	body := content

	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) >= 3 {
			yamlPart := parts[1]
			body = strings.TrimSpace(parts[2])

			lines := strings.Split(yamlPart, "
")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "type:") || strings.HasPrefix(line, "category:") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						// 쌍따옴표 등 제거
						category = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
					}
				}
			}
		}
	}
	return category, body
}

// 노션 블록 제약을 위한 청킹 함수
func chunkText(text string, limit int) []string {
	var chunks []string
	runes := []rune(text)
	for i := 0; i < len(runes); i += limit {
		end := i + limit
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

func sendToNotion(payload NotionPayload, filename string) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal payload for %s: %v", filename, err)
		return
	}

	req, err := http.NewRequest("POST", NotionAPIURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("Failed to create request for %s: %v", filename, err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+NotionToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Notion-Version", "2022-06-28")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("HTTP Request failed for %s: %v", filename, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Notion API Error for %s (Status: %d): %s", filename, resp.StatusCode, string(bodyBytes))
	} else {
		fmt.Printf("Successfully migrated: %s
", filename)
	}
}