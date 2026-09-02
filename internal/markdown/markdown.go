// Package markdown converts a markdown body into a slice of Notion blocks.
// Parsing is line-based and uses plain string functions (no regex).
package markdown

import (
	"strings"

	"github.com/yoonsooc/obsidian-notion-etl/internal/notion"
)

// chunkLimit is the Notion maximum length of a rich_text content, in runes.
const chunkLimit = 2000

// maxRichTextPerBlock is the Notion maximum rich_text array size per block.
const maxRichTextPerBlock = 100

// ToBlocks converts a markdown body into Notion blocks. Line-based rules:
//   - "# " / "## " / "### "              -> heading_1/2/3 ("#### "+ falls back to heading_3)
//   - "- [ ] " / "- [x] " (case-insensitive) -> to_do
//   - "- " / "* "                        -> bulleted_list_item (nesting flattened)
//   - "> [!type] ..." + following "> "   -> callout (Obsidian callout; emoji icon by type)
//   - "> "                               -> consecutive quote lines grouped into one quote
//   - other non-empty lines              -> consecutive lines grouped into one paragraph
//
// Horizontal rules (a standalone line of 3+ dashes) emit no block and only
// act as paragraph boundaries, since the supported block set has no divider
// and omitting them is closer to the source intent than a literal "---".
//
// Block text goes through the inline parser to produce styled rich_text;
// vaultName is used to build Obsidian URIs for [[wikilinks]]. Rich text
// elements are split at 2,000 runes, and blocks exceeding 100 elements are
// continued as additional blocks of the same type. An empty body yields an
// empty slice.
func ToBlocks(body, vaultName string) []notion.Block {
	lines := strings.Split(body, "\n")
	blocks := make([]notion.Block, 0, len(lines))

	// para and quote accumulate consecutive lines of their kind; at most one
	// of the two is non-empty at a time (entering one flushes the other).
	para := make([]string, 0, len(lines))
	quote := make([]string, 0)
	flush := func() {
		if len(para) == 0 {
			return
		}
		text := strings.Join(para, "\n")
		para = para[:0]
		blocks = appendRich(blocks, text, vaultName, notion.NewParagraphRich)
	}
	flushQuote := func() {
		if len(quote) == 0 {
			return
		}
		group := quote
		quote = quote[:0]
		if ctype, title, ok := parseCalloutHeader(group[0]); ok {
			text := title
			if body := strings.Join(group[1:], "\n"); body != "" {
				if text == "" {
					text = body
				} else {
					text += "\n" + body
				}
			}
			blocks = appendRich(blocks, text, vaultName, func(rt []notion.RichText) notion.Block {
				return notion.NewCalloutRich(rt, calloutEmoji(ctype))
			})
			return
		}
		blocks = appendRich(blocks, strings.Join(group, "\n"), vaultName, notion.NewQuoteRich)
	}

	for _, raw := range lines {
		line := strings.TrimSuffix(raw, "\r")
		// Detect markers on the trimmed line so nested items are flattened.
		marker := strings.TrimSpace(line)

		if content, ok := parseQuoteLine(marker); ok {
			flush()
			quote = append(quote, content)
			continue
		}
		flushQuote()

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
		// The to_do marker ("- [ ] ") contains the bullet marker, so check it first.
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
	flushQuote()
	return blocks
}

// parseQuoteLine extracts the content of a blockquote line (">", "> text",
// ">text"); ok is false for non-quote lines.
func parseQuoteLine(s string) (content string, ok bool) {
	rest, found := strings.CutPrefix(s, ">")
	if !found {
		return "", false
	}
	return strings.TrimPrefix(rest, " "), true
}

// parseCalloutHeader recognizes an Obsidian callout header ("[!type] title",
// already stripped of the "> " prefix). The optional fold marker ("+"/"-")
// after the bracket is dropped; ok is false when the line is a plain quote.
func parseCalloutHeader(s string) (ctype, title string, ok bool) {
	rest, found := strings.CutPrefix(s, "[!")
	if !found {
		return "", "", false
	}
	end := strings.IndexByte(rest, ']')
	if end <= 0 {
		return "", "", false
	}
	ctype = strings.ToLower(rest[:end])
	for _, r := range ctype {
		if r < 'a' || r > 'z' {
			return "", "", false
		}
	}
	title = strings.TrimSpace(strings.TrimLeft(rest[end+1:], "+-"))
	return ctype, title, true
}

// appendRich inline-parses text and appends one or more blocks of the same
// type, honoring the Notion limits (2,000 runes per element, 100 elements per
// block). Empty text still produces one empty block (marker-only lines survive).
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

// splitLongSpans splits rich text elements over 2,000 runes into multiple
// elements with identical styling.
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

// isRule reports whether s is a horizontal rule: 3+ characters, all dashes.
func isRule(s string) bool {
	if len(s) < 3 {
		return false
	}
	return strings.Count(s, "-") == len(s)
}

// parseHeading extracts the level and text from a heading line ('#'s followed
// by a space); ok is false otherwise. Levels above 3 are clamped by notion.NewHeading.
func parseHeading(s string) (level int, text string, ok bool) {
	rest := strings.TrimLeft(s, "#")
	if len(rest) == len(s) || !strings.HasPrefix(rest, " ") {
		return 0, "", false
	}
	return len(s) - len(rest), strings.TrimSpace(rest), true
}

// parseToDo extracts the text and checked state from a to_do line
// ("- [ ] ", "- [x] ", "- [X] "). A bare marker with no text ("- [ ]") is an
// empty checkbox, as commonly written in Obsidian; ok is false otherwise.
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
		// The marker must end the line or be followed by whitespace
		// ("- [ ]abc" is not a to_do).
		if rest != "" && !strings.HasPrefix(rest, " ") && !strings.HasPrefix(rest, "\t") {
			continue
		}
		return strings.TrimSpace(rest), m.checked, true
	}
	return "", false, false
}

// parseBullet extracts the text from a bullet line ("- ", "* "); ok is false otherwise.
func parseBullet(s string) (text string, ok bool) {
	if !strings.HasPrefix(s, "- ") && !strings.HasPrefix(s, "* ") {
		return "", false
	}
	return strings.TrimSpace(s[len("- "):]), true
}
