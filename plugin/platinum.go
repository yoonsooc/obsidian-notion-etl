// platinum is ysc's personal transform policy for migrating daily notes from
// the Yersona vault into the Notion Platinum DB. It implements
// pipeline.Plugin and registers itself at init; base.config.yaml selects it
// via pipeline.plugin: 'platinum'.
package plugin

import (
	"github.com/yoonsooc/obsidian-notion-etl/internal/config"
	"github.com/yoonsooc/obsidian-notion-etl/internal/pipeline"
	"github.com/yoonsooc/obsidian-notion-etl/internal/transform"
)

func init() { pipeline.Register(platinum{}) }

type platinum struct{}

func (platinum) Name() string { return "platinum" }

// DateRules is the date derivation chain for daily notes.
// Rules are tried top to bottom and the first match wins.
func (platinum) DateRules() []config.DateRule {
	return []config.DateRule{
		{FileLayout: "DN_060102"},        // DN_251101.md
		{FileLayout: "060102"},           // 251101.md (legacy, no prefix)
		{FrontmatterKey: "created_date"}, // fallback for notes without a date in the filename
	}
}

// Mapping is the property mapping for the Platinum DB. Every docu_type value
// converges to Todo, so fixed values suffice and no translation table is needed.
func (platinum) Mapping() []config.MappingEntry {
	return []config.MappingEntry{
		{NotionProperty: "Type", Value: "Todo"},
		{NotionProperty: "Status", Value: "Done"},
	}
}

// Transformers keeps NFC title normalization: the vault holds Korean
// filenames, which macOS stores in NFD.
func (platinum) Transformers() []transform.Transformer {
	return []transform.Transformer{pipeline.NFCTitle()}
}
