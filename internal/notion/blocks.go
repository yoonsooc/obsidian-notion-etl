package notion

// Block은 노션 블록 생성/append 페이로드다. Type에 해당하는 필드 하나만 채운다.
type Block struct {
	Object           string         `json:"object"`
	Type             string         `json:"type"`
	Paragraph        *RichTextBlock `json:"paragraph,omitempty"`
	Heading1         *RichTextBlock `json:"heading_1,omitempty"`
	Heading2         *RichTextBlock `json:"heading_2,omitempty"`
	Heading3         *RichTextBlock `json:"heading_3,omitempty"`
	BulletedListItem *RichTextBlock `json:"bulleted_list_item,omitempty"`
	ToDo             *ToDoBlock     `json:"to_do,omitempty"`
}

// RichTextBlock은 rich_text 배열만 갖는 블록 본문이다 (paragraph, heading 등).
type RichTextBlock struct {
	RichText []RichText `json:"rich_text"`
}

// ToDoBlock은 to_do 블록 본문이다.
type ToDoBlock struct {
	RichText []RichText `json:"rich_text"`
	Checked  bool       `json:"checked"`
}

// RichText는 rich text 배열의 원소다. v1은 스타일 없는 text 타입만 쓴다.
type RichText struct {
	Type string `json:"type"`
	Text Text   `json:"text"`
}

// Text는 rich text의 실제 내용이다.
type Text struct {
	Content string `json:"content"`
}

// newRichText는 텍스트 하나로 rich_text 1원소 배열을 만든다.
func newRichText(text string) []RichText {
	return []RichText{{Type: "text", Text: Text{Content: text}}}
}

// NewParagraph는 paragraph 블록을 만든다.
func NewParagraph(text string) Block {
	return Block{Object: "block", Type: "paragraph", Paragraph: &RichTextBlock{RichText: newRichText(text)}}
}

// NewHeading은 heading_1/2/3 블록을 만든다. level이 1~3 밖이면 3으로 클램프한다.
func NewHeading(level int, text string) Block {
	body := &RichTextBlock{RichText: newRichText(text)}
	switch level {
	case 1:
		return Block{Object: "block", Type: "heading_1", Heading1: body}
	case 2:
		return Block{Object: "block", Type: "heading_2", Heading2: body}
	default:
		return Block{Object: "block", Type: "heading_3", Heading3: body}
	}
}

// NewBulletedItem은 bulleted_list_item 블록을 만든다.
func NewBulletedItem(text string) Block {
	return Block{Object: "block", Type: "bulleted_list_item", BulletedListItem: &RichTextBlock{RichText: newRichText(text)}}
}

// NewToDo는 to_do 블록을 만든다.
func NewToDo(text string, checked bool) Block {
	return Block{Object: "block", Type: "to_do", ToDo: &ToDoBlock{RichText: newRichText(text), Checked: checked}}
}

// ChunkText는 텍스트를 limit자(rune 기준) 단위로 나눈다.
// 노션 rich_text content의 2,000자 제한 대응이다 (CLAUDE.md 규칙: []rune 기반).
func ChunkText(text string, limit int) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	chunks := make([]string, 0, (len(runes)+limit-1)/limit)
	for i := 0; i < len(runes); i += limit {
		end := min(i+limit, len(runes))
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}
