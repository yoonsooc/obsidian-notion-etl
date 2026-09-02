package markdown

import (
	"strings"
	"testing"

	"github.com/yoonsooc/obsidian-notion-etl/internal/notion"
)

func TestToBlocksQuote(t *testing.T) {
	body := strings.Join([]string{
		"문단",
		"> 첫 줄",
		"> 둘째 줄",
		"",
		"> 별도 인용",
		">스페이스 없는 인용",
	}, "\n")

	blocks := ToBlocks(body, "V")
	if len(blocks) != 3 {
		t.Fatalf("blocks = %d (%+v), want 3 (문단, 인용, 인용)", len(blocks), blocks)
	}
	if blocks[1].Type != "quote" || notion.JoinPlainText(blocks[1].Quote.RichText) != "첫 줄\n둘째 줄" {
		t.Errorf("blocks[1] = %+v, want 두 줄 quote", blocks[1])
	}
	// A blank line separates quote groups; ">text" without a space still counts.
	if blocks[2].Type != "quote" || notion.JoinPlainText(blocks[2].Quote.RichText) != "별도 인용\n스페이스 없는 인용" {
		t.Errorf("blocks[2] = %+v", blocks[2])
	}
}

func TestToBlocksCallout(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantEmoji string
		wantText  string
	}{
		{
			name:      "제목과 본문",
			body:      "> [!info] 제목\n> 본문 줄",
			wantEmoji: "ℹ️",
			wantText:  "제목\n본문 줄",
		},
		{
			name:      "제목 없는 콜아웃",
			body:      "> [!warning]\n> 경고 본문",
			wantEmoji: "⚠️",
			wantText:  "경고 본문",
		},
		{
			name:      "접힘 마커와 미지의 타입",
			body:      "> [!mystery]- 접힌 제목",
			wantEmoji: "📝", // unknown type falls back to note
			wantText:  "접힌 제목",
		},
		{
			name:      "attention 별칭",
			body:      "> [!attention] 주의",
			wantEmoji: "⚠️",
			wantText:  "주의",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := ToBlocks(tt.body, "V")
			if len(blocks) != 1 || blocks[0].Type != "callout" {
				t.Fatalf("blocks = %+v, want single callout", blocks)
			}
			c := blocks[0].Callout
			if c.Icon == nil || c.Icon.Emoji != tt.wantEmoji {
				t.Errorf("emoji = %+v, want %q", c.Icon, tt.wantEmoji)
			}
			if got := notion.JoinPlainText(c.RichText); got != tt.wantText {
				t.Errorf("text = %q, want %q", got, tt.wantText)
			}
		})
	}
}

func TestToBlocksCalloutFalsePositive(t *testing.T) {
	// "[!...]" with non-letters is a plain quote, not a callout.
	blocks := ToBlocks("> [!x1] 텍스트", "V")
	if len(blocks) != 1 || blocks[0].Type != "quote" {
		t.Fatalf("blocks = %+v, want plain quote", blocks)
	}
}

func TestFromBlocksQuoteAndCalloutRoundTrip(t *testing.T) {
	body := strings.Join([]string{
		"> [!info] 제목",
		"> 본문 줄",
		"",
		"> 인용 한 줄",
		"> 인용 두 줄",
	}, "\n")

	md, warnings := FromBlocks(ToBlocks(body, "V"))
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if md != body {
		t.Errorf("round trip:\n got %q\nwant %q", md, body)
	}
}

func TestFromBlocksBareCallout(t *testing.T) {
	// A callout with no text renders as a bare header line.
	md, _ := FromBlocks([]notion.Block{notion.NewCalloutRich(nil, "🚨")})
	if md != "> [!danger]" {
		t.Errorf("FromBlocks() = %q, want bare header", md)
	}
}
