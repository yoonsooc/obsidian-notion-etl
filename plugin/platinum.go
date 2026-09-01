// platinum is ysc's personal transform policy for migrating daily, weekly,
// and monthly notes from the Yersona vault into the Notion Platinum DB. It
// implements pipeline.Plugin and registers itself at init; base.config.yaml
// selects it via pipeline.plugin: 'platinum'.
package plugin

import (
	"strings"
	"time"

	"github.com/yoonsooc/obsidian-notion-etl/internal/config"
	"github.com/yoonsooc/obsidian-notion-etl/internal/pipeline"
	"github.com/yoonsooc/obsidian-notion-etl/internal/transform"
)

func init() { pipeline.Register(platinum{}) }

type platinum struct{}

var _ pipeline.Plugin = platinum{}

func (platinum) Name() string { return "platinum" }

// DateRules is the date derivation chain, tried top to bottom per note type.
// Weekly (WG_260513-0517) is not expressible as a time layout because of the
// range suffix; weeklyStartDate below covers it as a custom stage.
func (platinum) DateRules() []config.DateRule {
	return []config.DateRule{
		{FileLayout: "DN_060102"},        // daily: DN_251101.md
		{FileLayout: "060102"},           // daily legacy: 251101.md (no prefix)
		{FileLayout: "MG_200601"},        // monthly: MG_202603.md -> month start (day defaults to 1)
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

// Transformers adds the weekly start-date stage and keeps NFC title
// normalization (the vault holds Korean filenames, which macOS stores in NFD).
func (platinum) Transformers() []transform.Transformer {
	return []transform.Transformer{weeklyStartDate{}, pipeline.NFCTitle()}
}

// weeklyStartDate derives the schedule start date from weekly filenames:
// WG_<start YYMMDD><sep><end>, where the real vault mixes "-" and "~" as the
// separator and MMDD/YYMMDD as the end (WG_260513-0517, WG_260831~260906).
// Go time layouts cannot express "parse the start, ignore the range suffix",
// hence a custom stage (D8). For a WG_ filename it overrides whatever the
// declarative chain set: weekly notes share daily frontmatter, so the
// created_date fallback would otherwise fill the note's creation date
// instead of the schedule start.
type weeklyStartDate struct{}

func (weeklyStartDate) Name() string { return "weeklyStartDate" }

func (weeklyStartDate) Transform(note transform.Note, draft *transform.PageDraft) error {
	stem := strings.TrimSuffix(note.Filename, ".md")
	rest, ok := strings.CutPrefix(stem, "WG_")
	if !ok {
		return nil
	}
	sep := strings.IndexAny(rest, "-~")
	if sep < 0 {
		return nil
	}
	start, end := rest[:sep], rest[sep+1:]
	t, err := time.Parse("060102", start)
	if err != nil || (len(end) != 4 && len(end) != 6) {
		draft.Warnings = append(draft.Warnings, "주간 노트 파일명이지만 날짜 파싱 실패: "+note.Filename)
		return nil
	}
	draft.Date = t.Format("2006-01-02")
	return nil
}
