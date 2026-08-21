package main

// pipeline.go is the single source of the transform rules (date derivation,
// property mapping). base.config.yaml holds only environment info (paths, DB,
// excludes); changing a rule means editing this file and rebuilding, and init
// validates the rules against the live Notion schema and actual notes.

import (
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/yoonsooc/obsidian-notion-etl/internal/config"
	"github.com/yoonsooc/obsidian-notion-etl/internal/transform"
)

// dailyDateRules is the date derivation chain for daily notes.
// Rules are tried top to bottom and the first match wins.
func dailyDateRules() []config.DateRule {
	return []config.DateRule{
		{FileLayout: "DN_060102"},        // DN_251101.md
		{FileLayout: "060102"},           // 251101.md (legacy, no prefix)
		{FrontmatterKey: "created_date"}, // fallback for notes without a date in the filename
	}
}

// platinumMapping is the property mapping for the Notion Platinum DB.
// Every docu_type value converges to Todo, so fixed values suffice and no
// value translation table is needed.
func platinumMapping() []config.MappingEntry {
	return []config.MappingEntry{
		{NotionProperty: "Type", Value: "Todo"},
		{NotionProperty: "Status", Value: "Done"},
	}
}

// buildPipeline assembles the migration transform chain from the built-in
// transformers plus custom ones defined in this file (e.g. nfcTitle).
func buildPipeline(vaultName, target string, properties []config.Property) []transform.Transformer {
	return []transform.Transformer{
		transform.NewDateDeriver(toTransformDateRules(dailyDateRules())),
		transform.NewTitleFromFilename(),
		nfcTitle{},
		transform.NewObsidianURI(vaultName, target),
		transform.NewPropertyMapper(toTransformMappingRules(platinumMapping(), properties)),
	}
}

// nfcTitle normalizes the title to NFC. macOS stores filenames in NFD, so a
// title left in NFD (e.g. Korean filenames) would make the title.equals
// duplicate check miss on re-runs.
type nfcTitle struct{}

func (nfcTitle) Name() string { return "nfcTitle" }

func (nfcTitle) Transform(_ transform.Note, draft *transform.PageDraft) error {
	draft.Title = norm.NFC.String(draft.Title)
	return nil
}

// toTransformDateRules converts rule definitions to the pipeline type.
func toTransformDateRules(rules []config.DateRule) []transform.DateRule {
	out := make([]transform.DateRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, transform.DateRule{FileLayout: r.FileLayout, FrontmatterKey: r.FrontmatterKey})
	}
	return out
}

// toTransformMappingRules converts validated mapping entries to the pipeline
// type, enforcing ValidateMapping's "duplicate entries are ignored" rule.
// Property names resolve in the same order as ValidateMapping: exact match
// first, then case-insensitive match to the schema's actual name, so schemas
// with case-only property variants do not get misrouted.
func toTransformMappingRules(entries []config.MappingEntry, properties []config.Property) []transform.MappingRule {
	exact := make(map[string]struct{}, len(properties))
	actualByFold := make(map[string]string, len(properties))
	for _, p := range properties {
		exact[p.Name] = struct{}{}
		actualByFold[strings.ToLower(p.Name)] = p.Name
	}

	seen := make(map[string]struct{}, len(entries))
	rules := make([]transform.MappingRule, 0, len(entries))
	for _, e := range entries {
		name := e.NotionProperty
		if _, ok := exact[name]; !ok {
			if actual, ok := actualByFold[strings.ToLower(name)]; ok {
				name = actual
			}
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		rules = append(rules, transform.MappingRule{
			Frontmatter:    e.Frontmatter,
			NotionProperty: name,
			Value:          e.Value,
			Values:         e.Values,
			Default:        e.Default,
		})
	}
	return rules
}
