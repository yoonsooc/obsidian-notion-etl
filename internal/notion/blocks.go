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

// RichText는 rich text 배열의 원소다. Annotations와 Link는 인라인 서식이
// 있을 때만 채워진다 (task-012).
type RichText struct {
	Type        string       `json:"type"`
	Text        Text         `json:"text"`
	Annotations *Annotations `json:"annotations,omitempty"`
}

// Annotations는 rich text 원소의 인라인 서식이다.
type Annotations struct {
	Bold          bool `json:"bold,omitempty"`
	Italic        bool `json:"italic,omitempty"`
	Strikethrough bool `json:"strikethrough,omitempty"`
	Underline     bool `json:"underline,omitempty"`
	Code          bool `json:"code,omitempty"`
}

// Text는 rich text의 실제 내용이다. Link가 있으면 인라인 링크가 된다.
type Text struct {
	Content string `json:"content"`
	Link    *Link  `json:"link,omitempty"`
}

// Link는 인라인 링크의 대상 URL이다.
type Link struct {
	URL string `json:"url"`
}

// PlainText는 서식 없는 텍스트 하나로 rich_text 1원소 배열을 만든다.
func PlainText(text string) []RichText {
	return []RichText{{Type: "text", Text: Text{Content: text}}}
}

// NewParagraph는 서식 없는 paragraph 블록을 만든다.
func NewParagraph(text string) Block {
	return NewParagraphRich(PlainText(text))
}

// NewParagraphRich는 rich text 배열로 paragraph 블록을 만든다.
func NewParagraphRich(richText []RichText) Block {
	return Block{Object: "block", Type: "paragraph", Paragraph: &RichTextBlock{RichText: richText}}
}

// NewHeading은 서식 없는 heading_1/2/3 블록을 만든다. level이 1~3 밖이면 3으로 클램프한다.
func NewHeading(level int, text string) Block {
	return NewHeadingRich(level, PlainText(text))
}

// NewHeadingRich는 rich text 배열로 heading_1/2/3 블록을 만든다.
func NewHeadingRich(level int, richText []RichText) Block {
	body := &RichTextBlock{RichText: richText}
	switch level {
	case 1:
		return Block{Object: "block", Type: "heading_1", Heading1: body}
	case 2:
		return Block{Object: "block", Type: "heading_2", Heading2: body}
	default:
		return Block{Object: "block", Type: "heading_3", Heading3: body}
	}
}

// NewBulletedItem은 서식 없는 bulleted_list_item 블록을 만든다.
func NewBulletedItem(text string) Block {
	return NewBulletedItemRich(PlainText(text))
}

// NewBulletedItemRich는 rich text 배열로 bulleted_list_item 블록을 만든다.
func NewBulletedItemRich(richText []RichText) Block {
	return Block{Object: "block", Type: "bulleted_list_item", BulletedListItem: &RichTextBlock{RichText: richText}}
}

// NewToDo는 서식 없는 to_do 블록을 만든다.
func NewToDo(text string, checked bool) Block {
	return NewToDoRich(PlainText(text), checked)
}

// NewToDoRich는 rich text 배열로 to_do 블록을 만든다.
func NewToDoRich(richText []RichText, checked bool) Block {
	return Block{Object: "block", Type: "to_do", ToDo: &ToDoBlock{RichText: richText, Checked: checked}}
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
