package markdown

import (
	"strings"
	"testing"

	"github.com/yoonsooc/obsidian-notion-etl/internal/notion"
)

func rt(text string) []notion.RichText {
	return []notion.RichText{{Type: "text", Text: notion.Text{Content: text}}}
}

func TestFromBlocksMapping(t *testing.T) {
	blocks := []notion.Block{
		{Type: "heading_1", Heading1: &notion.RichTextBlock{RichText: rt("제목")}},
		{Type: "paragraph", Paragraph: &notion.RichTextBlock{RichText: rt("첫 문단")}},
		{Type: "bulleted_list_item", BulletedListItem: &notion.RichTextBlock{RichText: rt("항목 1")}},
		{Type: "bulleted_list_item", BulletedListItem: &notion.RichTextBlock{RichText: rt("항목 2")}},
		{Type: "to_do", ToDo: &notion.ToDoBlock{RichText: rt("할 일"), Checked: false}},
		{Type: "to_do", ToDo: &notion.ToDoBlock{RichText: rt("한 일"), Checked: true}},
		{Type: "heading_3", Heading3: &notion.RichTextBlock{RichText: rt("소제목")}},
		{Type: "paragraph", Paragraph: &notion.RichTextBlock{RichText: rt("둘째 문단")}},
	}

	md, warnings := FromBlocks(blocks)
	want := strings.Join([]string{
		"# 제목",
		"",
		"첫 문단",
		"",
		"- 항목 1",
		"- 항목 2",
		"- [ ] 할 일",
		"- [x] 한 일",
		"",
		"### 소제목",
		"",
		"둘째 문단",
	}, "\n")
	if md != want {
		t.Errorf("FromBlocks() =\n%s\nwant:\n%s", md, want)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
}

func TestFromBlocksFallbackAndSkip(t *testing.T) {
	blocks := []notion.Block{
		{Type: "paragraph", Paragraph: &notion.RichTextBlock{RichText: rt("본문")}},
		{Type: "toggle", Fallback: rt("토글 텍스트")}, // unsupported with text -> paragraph fallback
		{Type: "divider"},   // unsupported without text -> skipped
		{Type: "paragraph"}, // malformed known type (nil body) -> skipped
		{Type: "paragraph", Paragraph: &notion.RichTextBlock{}}, // empty paragraph -> spacing only
	}

	md, warnings := FromBlocks(blocks)
	want := "본문\n\n토글 텍스트"
	if md != want {
		t.Errorf("FromBlocks() = %q, want %q", md, want)
	}
	if len(warnings) != 3 {
		t.Fatalf("warnings = %v, want 3 (fallback, divider skip, malformed skip)", warnings)
	}
}

func TestFromBlocksNestedWarning(t *testing.T) {
	blocks := []notion.Block{
		{Type: "bulleted_list_item", BulletedListItem: &notion.RichTextBlock{RichText: rt("부모")}, HasChildren: true},
	}
	md, warnings := FromBlocks(blocks)
	if md != "- 부모" {
		t.Errorf("FromBlocks() = %q, want %q", md, "- 부모")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "nested") {
		t.Errorf("warnings = %v, want nested-children warning", warnings)
	}
}

func TestFromBlocksPrefersPlainText(t *testing.T) {
	// Read responses carry plain_text (e.g. mention elements have no text.content).
	blocks := []notion.Block{
		{Type: "paragraph", Paragraph: &notion.RichTextBlock{RichText: []notion.RichText{
			{Type: "mention", PlainText: "@언급"},
			{Type: "text", Text: notion.Text{Content: " 뒤 텍스트"}},
		}}},
	}
	md, _ := FromBlocks(blocks)
	if md != "@언급 뒤 텍스트" {
		t.Errorf("FromBlocks() = %q, want mention plain_text used", md)
	}
}
