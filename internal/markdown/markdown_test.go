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
			got := ToBlocks(tt.body)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToBlocks(%q) = %+v, want %+v", tt.body, got, tt.want)
			}
		})
	}
}

func TestToBlocksChunking(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []notion.Block
	}{
		{
			name: "2000자 초과 문단은 같은 타입으로 분할(한글 rune 경계)",
			body: strings.Repeat("가", 2500),
			want: []notion.Block{
				notion.NewParagraph(strings.Repeat("가", 2000)),
				notion.NewParagraph(strings.Repeat("가", 500)),
			},
		},
		{
			name: "정확히 2000자는 블록 하나",
			body: strings.Repeat("나", 2000),
			want: []notion.Block{notion.NewParagraph(strings.Repeat("나", 2000))},
		},
		{
			name: "todo 분할 시 checked 유지",
			body: "- [x] " + strings.Repeat("다", 2100),
			want: []notion.Block{
				notion.NewToDo(strings.Repeat("다", 2000), true),
				notion.NewToDo(strings.Repeat("다", 100), true),
			},
		},
		{
			name: "불릿 분할",
			body: "- " + strings.Repeat("라", 2001),
			want: []notion.Block{
				notion.NewBulletedItem(strings.Repeat("라", 2000)),
				notion.NewBulletedItem("라"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToBlocks(tt.body); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("블록 수 %d, want %d (본문 길이 %d rune)",
					len(got), len(tt.want), len([]rune(tt.body)))
			}
		})
	}
}
