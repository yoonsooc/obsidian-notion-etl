package markdown

import (
	"fmt"
	"strings"

	"github.com/yoonsooc/obsidian-notion-etl/internal/notion"
)

// FromBlocks renders top-level Notion blocks back to markdown (the reverse of
// ToBlocks, backup direction). Mapping: heading_1/2/3 -> #/##/###, to_do ->
// - [ ]/- [x], bulleted_list_item -> "- ", paragraph -> plain text. An
// unsupported block type falls back to a plain-text paragraph when its text
// is extractable, and is skipped with a warning otherwise. Inline formatting
// is not reconstructed (plain text only); round-trip loss is accepted.
func FromBlocks(blocks []notion.Block) (md string, warnings []string) {
	var lines []string
	prevList := false

	emit := func(line string, isList bool) {
		// Blank line between blocks, except inside a run of list items.
		if len(lines) > 0 && !(prevList && isList) {
			lines = append(lines, "")
		}
		lines = append(lines, line)
		prevList = isList
	}

	for i, b := range blocks {
		if b.HasChildren {
			warnings = append(warnings, fmt.Sprintf("블록 %d(%s): 중첩 블록은 수집하지 않음(v1), 최상위 텍스트만 백업됨", i, b.Type))
		}
		// The nil guards double as malformed-input protection: a known type
		// with a missing body drops to the fallback path instead of panicking.
		switch {
		case b.Type == "paragraph" && b.Paragraph != nil:
			text := notion.JoinPlainText(b.Paragraph.RichText)
			if text == "" {
				// Empty paragraph is intentional spacing; the block separator
				// already yields a blank line, so nothing extra is emitted.
				continue
			}
			emit(text, false)
		case b.Type == "heading_1" && b.Heading1 != nil:
			emit("# "+notion.JoinPlainText(b.Heading1.RichText), false)
		case b.Type == "heading_2" && b.Heading2 != nil:
			emit("## "+notion.JoinPlainText(b.Heading2.RichText), false)
		case b.Type == "heading_3" && b.Heading3 != nil:
			emit("### "+notion.JoinPlainText(b.Heading3.RichText), false)
		case b.Type == "bulleted_list_item" && b.BulletedListItem != nil:
			emit("- "+notion.JoinPlainText(b.BulletedListItem.RichText), true)
		case b.Type == "to_do" && b.ToDo != nil:
			marker := "- [ ] "
			if b.ToDo.Checked {
				marker = "- [x] "
			}
			emit(marker+notion.JoinPlainText(b.ToDo.RichText), true)
		default:
			text := notion.JoinPlainText(b.Fallback)
			if text == "" {
				warnings = append(warnings, fmt.Sprintf("블록 %d(%s): 텍스트를 추출할 수 없어 건너뜀", i, b.Type))
				continue
			}
			warnings = append(warnings, fmt.Sprintf("블록 %d(%s): 미지원 타입, 일반 문단으로 폴백", i, b.Type))
			emit(text, false)
		}
	}
	return strings.Join(lines, "\n"), warnings
}
