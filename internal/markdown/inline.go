package markdown

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/yoonsooc/obsidian-notion-etl/internal/notion"
)

// style은 인라인 파싱 중 누적되는 서식 상태다.
type style struct {
	bold      bool
	italic    bool
	strike    bool
	underline bool
	code      bool
}

// parseInline은 한 블록의 텍스트를 노션 rich_text 배열로 변환한다 (task-012).
// 지원 문법: [[위키링크]] / [[문서|별칭]], `인라인코드`, ~~취소선~~,
// ***bold+italic***, **bold**, *italic*. 짝이 없는 마커는 리터럴로 남긴다.
// 정규식 없이 표준 문자열 함수로만 구현한다 (CLAUDE.md 규칙).
func parseInline(text, vaultName string) []notion.RichText {
	return parseStyled(text, style{}, vaultName)
}

// parseStyled는 st 서식이 적용된 상태에서 text를 파싱한다. 중첩 마커는
// 내부 텍스트를 서식을 더한 채 재귀 파싱해 처리한다.
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
				// 노션 API가 본문 인라인 링크의 obsidian:// 스킴을 거부하므로
				// (link.url 검증, task-012 변경 이력 참조) 밑줄 문서명 뒤에
				// 복사용 URI를 일반 텍스트로 병기한다 (사용자 결정).
				underlined := st
				underlined.underline = true
				out = append(out, richSpan(label, underlined, ""))
				out = append(out, richSpan(" ("+wikiURI(vaultName, target)+")", st, ""))
				i += consumed
				continue
			}
		}
		// 코드 스팬 내부는 다른 서식을 해석하지 않는다.
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
		// "***"는 "**"보다 먼저 검사해 bold+italic으로 해석한다.
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

// between은 rest가 marker로 시작할 때, 다음 marker까지의 내부 문자열과
// 전체 소비 길이(바이트)를 돌려준다. 닫는 marker가 없거나 내부가 비어 있으면
// 마커로 취급하지 않는다.
func between(rest, marker string) (inner string, consumed int, ok bool) {
	m := len(marker)
	end := strings.Index(rest[m:], marker)
	if end <= 0 {
		return "", 0, false
	}
	return rest[m : m+end], m + end + m, true
}

// betweenStyled는 서식 마커(**, *, ~~, ***)용 between이다. 두 가지 가드를 더한다.
//   - 내부가 공백/탭으로 시작하거나 끝나면 거부: "2 ** 10, 2 ** 20" 같은
//     문장에서 원문에 없던 서식이 생기는 오탐 방지 (CommonMark 규칙과 일치)
//   - 내부에 홀수 개의 백틱이 있으면 거부: 닫는 마커가 코드 스팬 안에 있는
//     경우("a**b `c**d`")를 마커로 잘못 매칭해 코드 스팬을 파괴하는 것 방지
func betweenStyled(rest, marker string) (inner string, consumed int, ok bool) {
	inner, consumed, ok = between(rest, marker)
	if !ok || strings.Trim(inner, " \t") != inner || strings.Count(inner, "`")%2 != 0 {
		return "", 0, false
	}
	return inner, consumed, true
}

// parseWikiLink는 "[[문서명]]" 또는 "[[문서명|별칭]]"으로 시작하는 텍스트에서
// 표시 텍스트, 링크 대상, 소비 길이를 꺼낸다.
// 위키링크는 줄을 넘을 수 없으므로(옵시디언 규칙) 내부에 개행이 있으면 거부한다.
// 링크 대상의 헤딩/블록 앵커("문서#섹션", "문서#^블록")는 옵시디언 URI의 file
// 파라미터가 해석하지 못하므로 잘라내고 문서명만 대상에 남긴다.
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

// richSpan은 서식과 링크가 적용된 rich text 원소 하나를 만든다.
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

// wikiURI는 위키링크 대상의 옵시디언 URI를 만든다. 옵시디언은 문서명만으로
// 볼트 전체에서 노트를 찾으므로 target 디렉토리는 넣지 않는다.
// 공백은 %20으로 인코딩한다 (transform의 Obsidian_URI와 같은 규칙).
func wikiURI(vaultName, target string) string {
	return "obsidian://open?vault=" + escapeURIComponent(vaultName) +
		"&file=" + escapeURIComponent(target)
}

// escapeURIComponent는 URI 컴포넌트를 인코딩하되 공백을 '+'가 아니라 "%20"으로
// 쓴다 (internal/transform의 동명 헬퍼와 같은 규칙. 패키지 간 의존을 만들지
// 않기 위한 의도적 중복).
func escapeURIComponent(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}
