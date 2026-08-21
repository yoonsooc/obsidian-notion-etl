package markdown

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/yoonsooc/obsidian-notion-etl/internal/notion"
)

// style is the formatting state accumulated during inline parsing.
type style struct {
	bold      bool
	italic    bool
	strike    bool
	underline bool
	code      bool
}

// parseInline converts one block's text into a Notion rich_text array.
// Supported syntax: [[wikilink]] / [[page|alias]], `inline code`,
// ~~strikethrough~~, ***bold+italic***, **bold**, *italic*.
// Unmatched markers are kept as literal text.
func parseInline(text, vaultName string) []notion.RichText {
	return parseStyled(text, style{}, vaultName)
}

// parseStyled parses text with the st formatting already applied. Nested
// markers are handled by recursively parsing the inner text with the added style.
func parseStyled(text string, st style, vaultName string) []notion.RichText {
	out := make([]notion.RichText, 0, 4)
	var literal strings.Builder
	flush := func() {
		if literal.Len() > 0 {
			out = append(out, richSpan(literal.String(), st, ""))
			literal.Reset()
		}
	}

	i := 0
	for i < len(text) {
		rest := text[i:]

		if strings.HasPrefix(rest, "[[") {
			if label, target, consumed, ok := parseWikiLink(rest); ok {
				flush()
				// The Notion API rejects obsidian:// URLs in inline links, so
				// render the page name underlined and append the URI as plain
				// text for copying.
				underlined := st
				underlined.underline = true
				out = append(out, richSpan(label, underlined, ""))
				out = append(out, richSpan(" ("+wikiURI(vaultName, target)+")", st, ""))
				i += consumed
				continue
			}
		}
		// No other formatting is interpreted inside a code span.
		if strings.HasPrefix(rest, "`") {
			if inner, consumed, ok := between(rest, "`"); ok {
				flush()
				codeStyle := st
				codeStyle.code = true
				out = append(out, richSpan(inner, codeStyle, ""))
				i += consumed
				continue
			}
		}
		if strings.HasPrefix(rest, "~~") {
			if inner, consumed, ok := betweenStyled(rest, "~~"); ok {
				flush()
				next := st
				next.strike = true
				out = append(out, parseStyled(inner, next, vaultName)...)
				i += consumed
				continue
			}
		}
		// Check "***" before "**" so it parses as bold+italic.
		if strings.HasPrefix(rest, "***") {
			if inner, consumed, ok := betweenStyled(rest, "***"); ok {
				flush()
				next := st
				next.bold = true
				next.italic = true
				out = append(out, parseStyled(inner, next, vaultName)...)
				i += consumed
				continue
			}
		}
		if strings.HasPrefix(rest, "**") {
			if inner, consumed, ok := betweenStyled(rest, "**"); ok {
				flush()
				next := st
				next.bold = true
				out = append(out, parseStyled(inner, next, vaultName)...)
				i += consumed
				continue
			}
		}
		if strings.HasPrefix(rest, "*") {
			if inner, consumed, ok := betweenStyled(rest, "*"); ok {
				flush()
				next := st
				next.italic = true
				out = append(out, parseStyled(inner, next, vaultName)...)
				i += consumed
				continue
			}
		}

		r, size := utf8.DecodeRuneInString(rest)
		literal.WriteRune(r)
		i += size
	}
	flush()
	return out
}

// between returns the text between the leading marker and its next occurrence,
// plus the total bytes consumed. A missing closing marker or empty inner text
// means no match.
func between(rest, marker string) (inner string, consumed int, ok bool) {
	m := len(marker)
	end := strings.Index(rest[m:], marker)
	if end <= 0 {
		return "", 0, false
	}
	return rest[m : m+end], m + end + m, true
}

// betweenStyled is between for style markers (**, *, ~~, ***) with two guards:
//   - reject inner text starting or ending with whitespace, so text like
//     "2 ** 10, 2 ** 20" gains no formatting (matches CommonMark)
//   - reject inner text with an odd number of backticks, so a closing marker
//     inside a code span ("a**b `c**d`") cannot break the span
func betweenStyled(rest, marker string) (inner string, consumed int, ok bool) {
	inner, consumed, ok = between(rest, marker)
	if !ok || strings.Trim(inner, " \t") != inner || strings.Count(inner, "`")%2 != 0 {
		return "", 0, false
	}
	return inner, consumed, true
}

// parseWikiLink extracts the label, link target, and bytes consumed from text
// starting with "[[page]]" or "[[page|alias]]". Wikilinks cannot span lines
// (Obsidian rule), so inner newlines are rejected. Heading/block anchors
// ("page#section", "page#^block") are stripped because the Obsidian URI file
// parameter cannot resolve them.
func parseWikiLink(rest string) (label, target string, consumed int, ok bool) {
	end := strings.Index(rest[2:], "]]")
	if end < 0 {
		return "", "", 0, false
	}
	inner := rest[2 : 2+end]
	if strings.ContainsRune(inner, '\n') {
		return "", "", 0, false
	}
	target, label = inner, inner
	if p := strings.Index(inner, "|"); p >= 0 {
		target, label = inner[:p], inner[p+1:]
	}
	if p := strings.IndexAny(target, "#^"); p >= 0 {
		target = target[:p]
	}
	target = strings.TrimSpace(target)
	label = strings.TrimSpace(label)
	if target == "" || label == "" {
		return "", "", 0, false
	}
	return label, target, 2 + end + 2, true
}

// richSpan builds one rich text element with the given style and link.
func richSpan(text string, st style, linkURL string) notion.RichText {
	rt := notion.RichText{Type: "text", Text: notion.Text{Content: text}}
	if linkURL != "" {
		rt.Text.Link = &notion.Link{URL: linkURL}
	}
	if st != (style{}) {
		rt.Annotations = &notion.Annotations{
			Bold:          st.bold,
			Italic:        st.italic,
			Strikethrough: st.strike,
			Underline:     st.underline,
			Code:          st.code,
		}
	}
	return rt
}

// wikiURI builds the Obsidian URI for a wikilink target. Obsidian resolves a
// note anywhere in the vault by name, so the target directory is omitted.
func wikiURI(vaultName, target string) string {
	return "obsidian://open?vault=" + escapeURIComponent(vaultName) +
		"&file=" + escapeURIComponent(target)
}

// escapeURIComponent encodes a URI component with spaces as "%20" instead of
// '+' (same rule as the identically named helper in internal/transform;
// duplicated deliberately to avoid a package dependency).
func escapeURIComponent(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}
