// Package markdown은 마크다운 본문을 노션 블록 슬라이스로 변환한다.
// 줄 단위 파싱만 수행하며, 정규식 없이 표준 문자열 함수로 구현한다 (PRD D4, FR-2 5항).
package markdown

import (
	"strings"

	"github.com/yoonsooc/obsidian-notion-etl/internal/notion"
)

// chunkLimit은 노션 rich_text content의 최대 길이(rune 기준)다.
const chunkLimit = 2000

// maxRichTextPerBlock은 블록 하나의 rich_text 배열이 가질 수 있는 최대 원소 수다.
const maxRichTextPerBlock = 100

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
// 각 블록의 텍스트는 인라인 파서(parseInline)를 거쳐 서식 있는 rich_text로
// 변환된다 (task-012). vaultName은 [[위키링크]]의 옵시디언 URI 생성에 쓰인다.
// rich_text 원소는 2,000자(rune) 단위로 분할되고, 블록당 원소 100개를 넘으면
// 같은 타입 블록 여러 개로 이어 붙인다. 빈 본문은 빈 슬라이스를 반환한다.
func ToBlocks(body, vaultName string) []notion.Block {
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
		blocks = appendRich(blocks, text, vaultName, notion.NewParagraphRich)
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
			blocks = appendRich(blocks, text, vaultName, func(rt []notion.RichText) notion.Block {
				return notion.NewHeadingRich(level, rt)
			})
			continue
		}
		// to_do 마커("- [ ] ")는 불릿 마커("- ")를 포함하므로 먼저 검사한다.
		if text, checked, ok := parseToDo(marker); ok {
			flush()
			blocks = appendRich(blocks, text, vaultName, func(rt []notion.RichText) notion.Block {
				return notion.NewToDoRich(rt, checked)
			})
			continue
		}
		if text, ok := parseBullet(marker); ok {
			flush()
			blocks = appendRich(blocks, text, vaultName, notion.NewBulletedItemRich)
			continue
		}
		para = append(para, line)
	}
	flush()
	return blocks
}

// appendRich는 텍스트를 인라인 파싱한 뒤 노션 제약(원소당 2,000자, 블록당
// 원소 100개)에 맞춰 같은 타입 블록 하나 이상으로 덧붙인다.
// 텍스트가 비어 있으면 빈 텍스트 블록 하나를 만든다 (마커만 있는 줄도 블록으로 유지).
func appendRich(blocks []notion.Block, text, vaultName string, build func([]notion.RichText) notion.Block) []notion.Block {
	spans := splitLongSpans(parseInline(text, vaultName))
	if len(spans) == 0 {
		return append(blocks, build(notion.PlainText("")))
	}
	for start := 0; start < len(spans); start += maxRichTextPerBlock {
		end := min(start+maxRichTextPerBlock, len(spans))
		blocks = append(blocks, build(spans[start:end]))
	}
	return blocks
}

// splitLongSpans는 2,000자(rune)를 넘는 rich text 원소를 같은 서식의
// 원소 여러 개로 분할한다.
func splitLongSpans(spans []notion.RichText) []notion.RichText {
	out := make([]notion.RichText, 0, len(spans))
	for _, span := range spans {
		chunks := notion.ChunkText(span.Text.Content, chunkLimit)
		if len(chunks) <= 1 {
			out = append(out, span)
			continue
		}
		for _, chunk := range chunks {
			split := span
			split.Text.Content = chunk
			out = append(out, split)
		}
	}
	return out
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
