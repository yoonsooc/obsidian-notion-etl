package pipeline

import (
	"github.com/yoonsooc/obsidian-notion-etl/internal/config"
	"github.com/yoonsooc/obsidian-notion-etl/internal/transform"
)

// defaultPlugin is the built-in plugin used when no user plugin is selected.
// Rules come from base.config.yaml's pipeline section, so they are editable
// without writing Go code; absent rules fall back to generic Obsidian
// conventions so the tool functions with zero plugin code.
type defaultPlugin struct {
	dateFrom []config.DateRule
	mapping  []config.MappingEntry
}

var _ Plugin = defaultPlugin{}

func (defaultPlugin) Name() string { return DefaultName }

func (d defaultPlugin) DateRules() []config.DateRule {
	if len(d.dateFrom) > 0 {
		return d.dateFrom
	}
	return []config.DateRule{
		{FileLayout: "2006-01-02"}, // Obsidian's default daily-note filename
		{FrontmatterKey: "date"},
		{FrontmatterKey: "created"},
	}
}

// Mapping may be empty: title/date/url are derived by built-in stages.
func (d defaultPlugin) Mapping() []config.MappingEntry { return d.mapping }

func (defaultPlugin) Transformers() []transform.Transformer {
	return []transform.Transformer{NFCTitle()}
}
