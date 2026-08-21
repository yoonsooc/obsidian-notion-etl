package notion

// Block is a Notion block payload for create/append; only the field matching Type is set.
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

// RichTextBlock is a block body holding only a rich_text array (paragraph, heading, etc.).
type RichTextBlock struct {
	RichText []RichText `json:"rich_text"`
}

// ToDoBlock is a to_do block body.
type ToDoBlock struct {
	RichText []RichText `json:"rich_text"`
	Checked  bool       `json:"checked"`
}

// RichText is an element of a rich text array; Annotations is set only when
// inline formatting is present.
type RichText struct {
	Type        string       `json:"type"`
	Text        Text         `json:"text"`
	Annotations *Annotations `json:"annotations,omitempty"`
}

// Annotations is inline formatting for a rich text element.
type Annotations struct {
	Bold          bool `json:"bold,omitempty"`
	Italic        bool `json:"italic,omitempty"`
	Strikethrough bool `json:"strikethrough,omitempty"`
	Underline     bool `json:"underline,omitempty"`
	Code          bool `json:"code,omitempty"`
}

// Text is the content of a rich text element; a non-nil Link makes it an inline link.
type Text struct {
	Content string `json:"content"`
	Link    *Link  `json:"link,omitempty"`
}

// Link is the target URL of an inline link.
type Link struct {
	URL string `json:"url"`
}

// PlainText builds a single-element rich_text array with no formatting.
func PlainText(text string) []RichText {
	return []RichText{{Type: "text", Text: Text{Content: text}}}
}

// NewParagraph builds an unformatted paragraph block.
func NewParagraph(text string) Block {
	return NewParagraphRich(PlainText(text))
}

// NewParagraphRich builds a paragraph block from a rich text array.
func NewParagraphRich(richText []RichText) Block {
	return Block{Object: "block", Type: "paragraph", Paragraph: &RichTextBlock{RichText: richText}}
}

// NewHeading builds an unformatted heading_1/2/3 block; levels outside 1-3 clamp to 3.
func NewHeading(level int, text string) Block {
	return NewHeadingRich(level, PlainText(text))
}

// NewHeadingRich builds a heading_1/2/3 block from a rich text array.
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

// NewBulletedItem builds an unformatted bulleted_list_item block.
func NewBulletedItem(text string) Block {
	return NewBulletedItemRich(PlainText(text))
}

// NewBulletedItemRich builds a bulleted_list_item block from a rich text array.
func NewBulletedItemRich(richText []RichText) Block {
	return Block{Object: "block", Type: "bulleted_list_item", BulletedListItem: &RichTextBlock{RichText: richText}}
}

// NewToDo builds an unformatted to_do block.
func NewToDo(text string, checked bool) Block {
	return NewToDoRich(PlainText(text), checked)
}

// NewToDoRich builds a to_do block from a rich text array.
func NewToDoRich(richText []RichText, checked bool) Block {
	return Block{Object: "block", Type: "to_do", ToDo: &ToDoBlock{RichText: richText, Checked: checked}}
}

// ChunkText splits text into limit-sized chunks counted in runes, for Notion's
// 2,000-character rich_text content limit.
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
