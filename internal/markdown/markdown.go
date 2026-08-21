// Package markdown은 마크다운 본문을 노션 블록 슬라이스로 변환한다.
// 줄 단위 파싱만 수행하며, 정규식 없이 표준 문자열 함수로 구현한다 (PRD D4, FR-2 5항).
package markdown

import (
	"strings"

	"github.com/yoonsooc/obsidian-notion-etl/internal/notion"
)

// chunkLimit은 노션 rich_text content의 최대 길이(rune 기준)다.
const chunkLimit = 2000

// ToBlocks는 마크다운 본문을 노션 블록으로 변환한다.
// 변환 규칙 (줄 단위):
//   - "# " / "## " / "### "         -> heading_1/2/3 ("#### " 이상은 heading_3 폴백)
//   - "- [ ] " / "- [x] "(대소문자) -> to_do (checked 반영)
//   - "- " / "* "                   -> bulleted_list_item (중첩은 평탄화: 선행 공백 무시)
//   - 그 외 비어 있지 않은 줄        -> 연속된 줄들을 빈 줄 경계로 묶어 하나의 paragraph
//
// 수평선("---" 등 대시로만 이루어진 3자 이상의 단독 줄)은 블록을 만들지 않고
// 문단 경계로만 작동한다. 현재 블록 타입 집합에 divider가 없으므로,
// 리터럴 "---" 문단을 남기는 것보다 생략하는 쪽이 원문 의도에 가깝기 때문이다.
//
// 모든 블록 텍스트는 notion.ChunkText(text, 2000)를 통과하며, 2,000자(rune)를
// 넘는 텍스트는 같은 타입 블록 여러 개로 이어 붙인다. 빈 본문은 빈 슬라이스를 반환한다.
func ToBlocks(body string) []notion.Block {
	lines := strings.Split(body, "\n")
	blocks := make([]notion.Block, 0, len(lines))

	// para는 빈 줄 경계로 하나의 paragraph가 될 연속 줄 묶음이다.
	para := make([]string, 0, len(lines))
	flush := func() {
		if len(para) == 0 {
			return
		}
		text := strings.Join(para, "\n")
		para = para[:0]
		blocks = appendChunked(blocks, text, notion.NewParagraph)
	}

	for _, raw := range lines {
		line := strings.TrimSuffix(raw, "\r")
		// 마커 판별은 선행/후행 공백을 무시한 문자열로 수행한다 (중첩 평탄화).
		marker := strings.TrimSpace(line)

		if marker == "" || isRule(marker) {
			flush()
			continue
		}
		if level, text, ok := parseHeading(marker); ok {
			flush()
			blocks = appendChunked(blocks, text, func(chunk string) notion.Block {
				return notion.NewHeading(level, chunk)
			})
			continue
		}
		// to_do 마커("- [ ] ")는 불릿 마커("- ")를 포함하므로 먼저 검사한다.
		if text, checked, ok := parseToDo(marker); ok {
			flush()
			blocks = appendChunked(blocks, text, func(chunk string) notion.Block {
				return notion.NewToDo(chunk, checked)
			})
			continue
		}
		if text, ok := parseBullet(marker); ok {
			flush()
			blocks = appendChunked(blocks, text, notion.NewBulletedItem)
			continue
		}
		para = append(para, line)
	}
	flush()
	return blocks
}

// appendChunked는 텍스트를 2,000자 단위로 나눠 같은 타입 블록 여러 개로 덧붙인다.
// 텍스트가 비어 있으면 빈 텍스트 블록 하나를 만든다 (마커만 있는 줄도 블록으로 유지).
func appendChunked(blocks []notion.Block, text string, build func(string) notion.Block) []notion.Block {
	chunks := notion.ChunkText(text, chunkLimit)
	if len(chunks) == 0 {
		return append(blocks, build(""))
	}
	for _, chunk := range chunks {
		blocks = append(blocks, build(chunk))
	}
	return blocks
}

// isRule은 대시('-')로만 이루어진 3자 이상의 줄(수평선)인지 판별한다.
func isRule(s string) bool {
	if len(s) < 3 {
		return false
	}
	return strings.Count(s, "-") == len(s)
}

// parseHeading은 '#' 1개 이상과 공백으로 시작하는 헤딩 줄에서 레벨과 본문을 꺼낸다.
// 헤딩 줄이 아니면 ok가 거짓이다. 레벨 4 이상은 notion.NewHeading이 heading_3으로 클램프한다.
func parseHeading(s string) (level int, text string, ok bool) {
	rest := strings.TrimLeft(s, "#")
	if len(rest) == len(s) || !strings.HasPrefix(rest, " ") {
		return 0, "", false
	}
	return len(s) - len(rest), strings.TrimSpace(rest), true
}

// parseToDo는 to_do 마커("- [ ] ", "- [x] ", "- [X] ")로 시작하는 줄에서
// 본문과 체크 여부를 꺼낸다. 본문 없이 마커만 있는 줄("- [ ]")도 빈 체크박스로
// 인정한다 (Obsidian에서 흔한 형태). to_do 줄이 아니면 ok가 거짓이다.
func parseToDo(s string) (text string, checked bool, ok bool) {
	markers := []struct {
		prefix  string
		checked bool
	}{
		{prefix: "- [ ]", checked: false},
		{prefix: "- [x]", checked: true},
		{prefix: "- [X]", checked: true},
	}
	for _, m := range markers {
		rest, found := strings.CutPrefix(s, m.prefix)
		if !found {
			continue
		}
		// 마커 뒤는 줄 끝이거나 공백이어야 한다 ("- [ ]abc"는 to_do가 아님).
		if rest != "" && !strings.HasPrefix(rest, " ") && !strings.HasPrefix(rest, "\t") {
			continue
		}
		return strings.TrimSpace(rest), m.checked, true
	}
	return "", false, false
}

// parseBullet은 불릿 마커("- ", "* ")로 시작하는 줄에서 본문을 꺼낸다.
// 불릿 줄이 아니면 ok가 거짓이다.
func parseBullet(s string) (text string, ok bool) {
	if !strings.HasPrefix(s, "- ") && !strings.HasPrefix(s, "* ") {
		return "", false
	}
	return strings.TrimSpace(s[len("- "):]), true
}
