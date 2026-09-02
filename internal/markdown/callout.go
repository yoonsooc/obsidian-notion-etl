package markdown

// calloutEmojiByType maps Obsidian callout types to the Notion callout emoji
// icon. Aliases follow Obsidian's callout documentation; unknown types fall
// back to the note emoji.
var calloutEmojiByType = map[string]string{
	"note":      "📝",
	"info":      "ℹ️",
	"important": "❗",
	"tip":       "💡",
	"hint":      "💡",
	"warning":   "⚠️",
	"caution":   "⚠️",
	"attention": "⚠️",
	"danger":    "🚨",
	"error":     "🚨",
	"success":   "✅",
	"check":     "✅",
	"done":      "✅",
	"question":  "❓",
	"help":      "❓",
	"faq":       "❓",
	"bug":       "🐛",
	"example":   "📋",
	"abstract":  "📋",
	"summary":   "📋",
	"todo":      "☑️",
	"quote":     "💬",
	"cite":      "💬",
}

// calloutTypeByEmoji is the reverse mapping for backup: an emoji resolves to
// the primary type name (aliases collapse to it); unknown emoji become "note".
var calloutTypeByEmoji = map[string]string{
	"📝":  "note",
	"ℹ️": "info",
	"❗":  "important",
	"💡":  "tip",
	"⚠️": "warning",
	"🚨":  "danger",
	"✅":  "success",
	"❓":  "question",
	"🐛":  "bug",
	"📋":  "example",
	"☑️": "todo",
	"💬":  "quote",
}

// calloutEmoji returns the icon emoji for an Obsidian callout type.
func calloutEmoji(ctype string) string {
	if emoji, ok := calloutEmojiByType[ctype]; ok {
		return emoji
	}
	return calloutEmojiByType["note"]
}

// calloutType returns the Obsidian callout type for a Notion callout icon.
func calloutType(emoji string) string {
	if ctype, ok := calloutTypeByEmoji[emoji]; ok {
		return ctype
	}
	return "note"
}
