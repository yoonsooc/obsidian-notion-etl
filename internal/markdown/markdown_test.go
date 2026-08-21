package markdown

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yoonsooc/obsidian-notion-etl/internal/notion"
)

func TestToBlocks(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []notion.Block
	}{
		{
			name: "헤딩 3종",
			body: "# 제목1\n## 제목2\n### 제목3",
			want: []notion.Block{
				notion.NewHeading(1, "제목1"),
				notion.NewHeading(2, "제목2"),
				notion.NewHeading(3, "제목3"),
			},
		},
		{
			name: "h4 이상은 heading_3 폴백",
			body: "#### 제목4\n##### 제목5",
			want: []notion.Block{
				notion.NewHeading(3, "제목4"),
				notion.NewHeading(3, "제목5"),
			},
		},
		{
			name: "공백 없는 해시는 문단",
			body: "#태그아님",
			want: []notion.Block{notion.NewParagraph("#태그아님")},
		},
		{
			name: "todo 미체크와 체크(대소문자)",
			body: "- [ ] 할 일\n- [x] 끝난 일\n- [X] 대문자 끝난 일",
			want: []notion.Block{
				notion.NewToDo("할 일", false),
				notion.NewToDo("끝난 일", true),
				notion.NewToDo("대문자 끝난 일", true),
			},
		},
		{
			name: "본문 없는 체크박스 줄도 to_do로 유지",
			body: "- [ ]\n- [x]",
			want: []notion.Block{
				notion.NewToDo("", false),
				notion.NewToDo("", true),
			},
		},
		{
			name: "마커 뒤에 공백 없이 글자가 붙으면 to_do가 아니라 불릿",
			body: "- [ ]abc",
			want: []notion.Block{
				notion.NewBulletedItem("[ ]abc"),
			},
		},
		{
			name: "불릿 대시와 별표",
			body: "- 첫째\n* 둘째",
			want: []notion.Block{
				notion.NewBulletedItem("첫째"),
				notion.NewBulletedItem("둘째"),
			},
		},
		{
			name: "중첩 불릿은 평탄화",
			body: "- 부모\n  - 자식\n\t* 탭 자식\n    - [ ] 중첩 todo",
			want: []notion.Block{
				notion.NewBulletedItem("부모"),
				notion.NewBulletedItem("자식"),
				notion.NewBulletedItem("탭 자식"),
				notion.NewToDo("중첩 todo", false),
			},
		},
		{
			name: "문단은 빈 줄 경계로 묶음",
			body: "첫 줄\n둘째 줄\n\n다른 문단",
			want: []notion.Block{
				notion.NewParagraph("첫 줄\n둘째 줄"),
				notion.NewParagraph("다른 문단"),
			},
		},
		{
			name: "마커 줄은 문단을 끊는다",
			body: "문단 하나\n# 제목\n문단 둘",
			want: []notion.Block{
				notion.NewParagraph("문단 하나"),
				notion.NewHeading(1, "제목"),
				notion.NewParagraph("문단 둘"),
			},
		},
		{
			name: "CRLF 줄바꿈 처리",
			body: "# 제목\r\n본문\r\n",
			want: []notion.Block{
				notion.NewHeading(1, "제목"),
				notion.NewParagraph("본문"),
			},
		},
		{
			name: "빈 본문",
			body: "",
			want: []notion.Block{},
		},
		{
			name: "공백만 있는 본문",
			body: "  \n\t\n",
			want: []notion.Block{},
		},
		{
			name: "수평선 단독 줄은 무시",
			body: "---",
			want: []notion.Block{},
		},
		{
			name: "수평선은 문단 경계로 작동",
			body: "위 문단\n---\n아래 문단\n-----\n긴 대시 뒤 문단",
			want: []notion.Block{
				notion.NewParagraph("위 문단"),
				notion.NewParagraph("아래 문단"),
				notion.NewParagraph("긴 대시 뒤 문단"),
			},
		},
		{
			name: "대시 두 개는 문단",
			body: "--",
			want: []notion.Block{notion.NewParagraph("--")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToBlocks(tt.body, "TestVault")
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToBlocks(%q) = %+v, want %+v", tt.body, got, tt.want)
			}
		})
	}
}

// styled builds a styled rich text element for tests.
func styled(text string, ann notion.Annotations) notion.RichText {
	return notion.RichText{Type: "text", Text: notion.Text{Content: text}, Annotations: &ann}
}

// wikiSpans builds the two elements a wikilink produces: the underlined page
// name plus the URI as plain text (the API rejects inline obsidian:// links).
func wikiSpans(label, uri string) []notion.RichText {
	return []notion.RichText{
		styled(label, notion.Annotations{Underline: true}),
		notion.PlainText(" (" + uri + ")")[0],
	}
}

func TestParseInline(t *testing.T) {
	plain := func(s string) notion.RichText { return notion.PlainText(s)[0] }

	tests := []struct {
		name string
		text string
		want []notion.RichText
	}{
		{
			name: "서식 없는 텍스트",
			text: "그냥 텍스트",
			want: []notion.RichText{plain("그냥 텍스트")},
		},
		{
			name: "bold",
			text: "앞 **굵게** 뒤",
			want: []notion.RichText{
				plain("앞 "),
				styled("굵게", notion.Annotations{Bold: true}),
				plain(" 뒤"),
			},
		},
		{
			name: "italic",
			text: "앞 *기울임* 뒤",
			want: []notion.RichText{
				plain("앞 "),
				styled("기울임", notion.Annotations{Italic: true}),
				plain(" 뒤"),
			},
		},
		{
			name: "취소선과 인라인 코드",
			text: "~~취소~~ `code`",
			want: []notion.RichText{
				styled("취소", notion.Annotations{Strikethrough: true}),
				plain(" "),
				styled("code", notion.Annotations{Code: true}),
			},
		},
		{
			name: "bold+italic 삼중 마커",
			text: "***둘 다***",
			want: []notion.RichText{
				styled("둘 다", notion.Annotations{Bold: true, Italic: true}),
			},
		},
		{
			name: "bold 안의 italic 중첩",
			text: "**굵게 *기울임* 굵게**",
			want: []notion.RichText{
				styled("굵게 ", notion.Annotations{Bold: true}),
				styled("기울임", notion.Annotations{Bold: true, Italic: true}),
				styled(" 굵게", notion.Annotations{Bold: true}),
			},
		},
		{
			name: "코드 스팬 내부는 서식 해석 안 함",
			text: "`**not bold**`",
			want: []notion.RichText{
				styled("**not bold**", notion.Annotations{Code: true}),
			},
		},
		{
			name: "짝 없는 마커는 리터럴",
			text: "짝 없는 **마커",
			want: []notion.RichText{plain("짝 없는 **마커")},
		},
		{
			name: "수식의 별표는 리터럴 (공백 가드)",
			text: "2 * 3 * 6",
			want: []notion.RichText{plain("2 * 3 * 6")},
		},
		{
			name: "위키링크는 밑줄 문서명 + URI 텍스트",
			text: "참고: [[다른 문서]]",
			want: append([]notion.RichText{plain("참고: ")},
				wikiSpans("다른 문서", "obsidian://open?vault=TestVault&file=%EB%8B%A4%EB%A5%B8%20%EB%AC%B8%EC%84%9C")...),
		},
		{
			name: "별칭 위키링크",
			text: "[[DN_260101|새해 노트]] 참조",
			want: append(wikiSpans("새해 노트", "obsidian://open?vault=TestVault&file=DN_260101"),
				plain(" 참조")),
		},
		{
			name: "닫히지 않은 위키링크는 리터럴",
			text: "[[미완성",
			want: []notion.RichText{plain("[[미완성")},
		},
		{
			// An unclosed [[ must not swallow a real wikilink on the next line.
			name: "위키링크는 줄을 넘지 않음",
			text: "가 [[미완성\n나 [[진짜]] 다",
			want: append(append([]notion.RichText{plain("가 [[미완성\n나 ")},
				wikiSpans("진짜", "obsidian://open?vault=TestVault&file=%EC%A7%84%EC%A7%9C")...),
				plain(" 다")),
		},
		{
			// A closing ** inside a code span must not match as a marker.
			name: "코드 스팬 안의 마커와 매칭 금지",
			text: "a**b `c**d`",
			want: []notion.RichText{
				plain("a**b "),
				styled("c**d", notion.Annotations{Code: true}),
			},
		},
		{
			// The whitespace guard also applies to bold markers.
			name: "bold 공백 가드",
			text: "2 ** 10은 1024, 2 ** 20은",
			want: []notion.RichText{plain("2 ** 10은 1024, 2 ** 20은")},
		},
		{
			name: "위키링크의 헤딩 앵커는 URI 대상에서 제거",
			text: "[[문서#섹션]]",
			want: wikiSpans("문서#섹션", "obsidian://open?vault=TestVault&file=%EB%AC%B8%EC%84%9C"),
		},
		{
			name: "위키링크의 블록 앵커도 제거",
			text: "[[문서#^abc123|별칭]]",
			want: wikiSpans("별칭", "obsidian://open?vault=TestVault&file=%EB%AC%B8%EC%84%9C"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseInline(tt.text, "TestVault")
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseInline(%q) = %+v, want %+v", tt.text, got, tt.want)
			}
		})
	}
}

// TestToBlocksInline verifies inline formatting works through block conversion.
func TestToBlocksInline(t *testing.T) {
	got := ToBlocks("- [ ] **중요** 할 일", "TestVault")
	want := []notion.Block{
		notion.NewToDoRich([]notion.RichText{
			styled("중요", notion.Annotations{Bold: true}),
			notion.PlainText(" 할 일")[0],
		}, false),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ToBlocks 인라인 통합 = %+v, want %+v", got, want)
	}
}

// plainSpans builds a rich_text array from unstyled text pieces.
func plainSpans(texts ...string) []notion.RichText {
	spans := make([]notion.RichText, 0, len(texts))
	for _, text := range texts {
		spans = append(spans, notion.PlainText(text)...)
	}
	return spans
}

func TestToBlocksChunking(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []notion.Block
	}{
		{
			// Over 2,000 runes only the rich_text elements are split, not the
			// block, so Notion renders one continuous paragraph.
			name: "2000자 초과 문단은 한 블록 안에서 원소 분할(한글 rune 경계)",
			body: strings.Repeat("가", 2500),
			want: []notion.Block{
				notion.NewParagraphRich(plainSpans(strings.Repeat("가", 2000), strings.Repeat("가", 500))),
			},
		},
		{
			name: "정확히 2000자는 원소 하나",
			body: strings.Repeat("나", 2000),
			want: []notion.Block{notion.NewParagraph(strings.Repeat("나", 2000))},
		},
		{
			name: "todo 분할 시 한 체크박스 유지",
			body: "- [x] " + strings.Repeat("다", 2100),
			want: []notion.Block{
				notion.NewToDoRich(plainSpans(strings.Repeat("다", 2000), strings.Repeat("다", 100)), true),
			},
		},
		{
			name: "불릿 분할",
			body: "- " + strings.Repeat("라", 2001),
			want: []notion.Block{
				notion.NewBulletedItemRich(plainSpans(strings.Repeat("라", 2000), "라")),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToBlocks(tt.body, "TestVault"); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("블록 수 %d, want %d (본문 길이 %d rune)",
					len(got), len(tt.want), len([]rune(tt.body)))
			}
		})
	}
}
