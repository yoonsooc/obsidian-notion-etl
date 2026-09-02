# task-009: internal/markdown (마크다운 -> 노션 블록)

담당: 병렬 에이전트 C
상태: 완료 (2026-08-21)

> 변경 이력 (2026-08-21, M2 리뷰 8번-빈 체크박스 줄 반영): to_do 마커 인식이
> 후행 공백을 요구하지 않도록 완화됨. 본문 없는 `- [ ]`/`- [x]` 단독 줄도
> 빈 텍스트의 to_do 블록으로 유지된다 (기존에는 불릿 `[ ]`로 오변환).
선행: internal/notion/blocks.go의 블록 타입·생성자 (메인 세션이 선작성, 수정 금지)

## 목표

PRD D4·FR-2 5항의 본문 변환을 구현한다. 마크다운 본문을 줄 단위로 파싱해
노션 블록 슬라이스로 변환한다.

## API 계약

```go
package markdown

// ToBlocks는 마크다운 본문을 노션 블록으로 변환한다.
// 변환 규칙 (PRD D4, 줄 단위):
//   "# " / "## " / "### "        -> heading_1/2/3 ("#### " 이상은 heading_3 폴백)
//   "- [ ] " / "- [x] "(대소문자) -> to_do (checked 반영)
//   "- " / "* "                  -> bulleted_list_item (중첩은 평탄화: 선행 공백 무시)
//   그 외 비어 있지 않은 줄       -> 연속된 줄들을 빈 줄 경계로 묶어 하나의 paragraph
// 모든 블록 텍스트는 notion.ChunkText(text, 2000)를 통과한다. 한 블록의
// 텍스트가 2,000자(rune)를 넘으면 같은 타입 블록 여러 개로 이어 붙인다.
// 빈 본문은 빈 슬라이스(nil 아님도 허용)를 반환한다.
func ToBlocks(body string) []notion.Block
```

internal/notion/blocks.go가 제공하는 것 (이미 존재, 수정 금지):
- `notion.Block` 구조체와 생성자 `notion.NewParagraph / NewHeading(level int, text string) / NewBulletedItem / NewToDo(text string, checked bool)` — 각 생성자는 텍스트 하나로 rich_text 1원소 블록을 만든다
- `notion.ChunkText(text string, limit int) []string` — []rune 기반 청킹

## 제약

- import는 표준 라이브러리 + github.com/yoonsooc/obsidian-notion-etl/internal/notion 만.
- internal/markdown/ 밖 수정 금지 (이 문서 상태 줄 제외). go.mod 수정 금지.
- 정규식 금지 (strings.HasPrefix/TrimPrefix 사용), panic 금지.
- docs/review-checklist.md 18항목 준수.
- 테이블 기반 테스트: heading 3종+h4 폴백, todo 체크/미체크, 불릿(-, *, 중첩 평탄화),
  문단 묶음(빈 줄 경계), 2,000자 초과 청킹(한글 rune 경계), 빈 본문, 수평선 "---"
  단독 줄 처리(문단으로 취급하거나 무시 중 택일하고 테스트로 고정).

## 완료 기준

- gofmt -l internal/markdown/ 비어 있음, go vet, go test ./internal/markdown/ 통과
