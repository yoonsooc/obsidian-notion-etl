# task-004: internal/vault 패키지

담당: 병렬 에이전트 C
상태: 완료

## 목표

옵시디언 볼트 파일 스캔과 Frontmatter 파싱을 담당하는 `internal/vault` 패키지를 구현한다. M1에서는 init이 쓰는 키 수집이 목적이고, 파서는 M2 migrate에서도 재사용된다.

## API 계약

```go
package vault

// Note는 파싱된 마크다운 노트 한 건이다.
type Note struct {
    Filename    string            // 예: "2025-11-05.md"
    Frontmatter map[string]string // 키 -> 값 (값의 양끝 따옴표 제거됨)
    Body        string            // frontmatter를 제외한 본문 (TrimSpace 적용)
}

// ParseNote는 노트 내용에서 frontmatter와 본문을 분리한다.
// frontmatter가 없으면 Frontmatter는 빈 맵, Body는 전체 내용.
// 파싱 규칙: 문서가 "---"로 시작할 때만 frontmatter로 취급,
// strings.SplitN(content, "---", 3) 기반, 각 줄은 "key: value" 형태만 수집
// (중첩 YAML, 리스트 값은 M1 범위 밖이므로 무시하되 키는 수집한다).
func ParseNote(filename, content string) Note

// ScanFrontmatterKeys는 dir의 *.md 파일들(하위 디렉토리 제외)을 읽어
// 등장하는 frontmatter 키의 중복 제거·정렬된 목록을 돌려준다.
// 개별 파일 읽기 실패는 stderr 로그 후 건너뛴다(PRD 6.2 Continue 정책).
func ScanFrontmatterKeys(dir string) ([]string, error)
```

## 제약

- 표준 라이브러리만 사용. 정규식 금지 (CLAUDE.md 규칙).
- docs/review-checklist.md 준수.
- 단위 테스트 (`vault_test.go`): 테이블 기반으로 ParseNote (frontmatter 없음/정상/본문에 "---" 포함/따옴표 값/코론 포함 값), ScanFrontmatterKeys는 t.TempDir()로 검증.

## 완료 기준

- `go test ./internal/vault/` 통과, `go vet` 통과, gofmt 적용
