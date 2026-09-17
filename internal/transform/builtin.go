package transform

import (
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"
)

// dateLayout is the canonical format for draft.Date and frontmatter dates.
const dateLayout = "2006-01-02"

// stem returns the filename without its ".md" extension.
func stem(filename string) string {
	return strings.TrimSuffix(filename, ".md")
}

// DateRule is one date derivation rule (field-compatible with config.DateRule).
// Set exactly one of FileLayout or FrontmatterKey.
type DateRule struct {
	FileLayout     string // Go time layout applied to the filename (without extension)
	FrontmatterKey string // frontmatter key to read the date from
}

// dateDeriver derives draft.Date from a chain of DateRules.
type dateDeriver struct {
	rules []DateRule
}

// NewDateDeriver returns a Transformer that tries rules in order to fill
// draft.Date. FileLayout applies time.Parse to the filename (without
// extension); FrontmatterKey parses the value as 2006-01-02, retrying with
// the first 10 characters for RFC3339-style values. If all rules fail, Date
// stays empty and a warning is recorded; it is not an error.
func NewDateDeriver(rules []DateRule) Transformer {
	return &dateDeriver{rules: rules}
}

func (d *dateDeriver) Name() string { return "dateDeriver" }

func (d *dateDeriver) Transform(note Note, draft *PageDraft) error {
	for _, rule := range d.rules {
		if rule.FileLayout != "" {
			if t, err := time.Parse(rule.FileLayout, stem(note.Filename)); err == nil {
				draft.Date = t.Format(dateLayout)
				return nil
			}
			continue
		}
		if rule.FrontmatterKey == "" {
			continue
		}
		v := note.Frontmatter[rule.FrontmatterKey]
		if v == "" {
			continue
		}
		if t, err := time.Parse(dateLayout, v); err == nil {
			draft.Date = t.Format(dateLayout)
			return nil
		}
		// Retry with the first 10 characters for values with a time suffix (RFC3339).
		if r := []rune(v); len(r) > 10 {
			if t, err := time.Parse(dateLayout, string(r[:10])); err == nil {
				draft.Date = t.Format(dateLayout)
				return nil
			}
		}
	}
	draft.Date = ""
	draft.Warnings = append(draft.Warnings,
		fmt.Sprintf("dateDeriver: could not derive a date from %s, leaving Date empty", note.Filename))
	return nil
}

// titleFromFilename derives the title from the filename.
type titleFromFilename struct{}

// NewTitleFromFilename returns a Transformer that sets draft.Title to the
// filename without its ".md" extension.
func NewTitleFromFilename() Transformer {
	return titleFromFilename{}
}

func (titleFromFilename) Name() string { return "titleFromFilename" }

func (titleFromFilename) Transform(note Note, draft *PageDraft) error {
	draft.Title = stem(note.Filename)
	return nil
}

// obsidianURI builds the obsidian:// link that opens the note.
type obsidianURI struct {
	vaultName string
	target    string
}

// NewObsidianURI returns a Transformer that sets draft.ObsidianURI to
// obsidian://open?vault=<vault>&file=<target/relPath without .md>.
// target is the note directory relative to the vault root.
func NewObsidianURI(vaultName, target string) Transformer {
	return &obsidianURI{vaultName: vaultName, target: target}
}

func (o *obsidianURI) Name() string { return "obsidianURI" }

func (o *obsidianURI) Transform(note Note, draft *PageDraft) error {
	file := path.Join(o.target, stem(note.RelPath))
	draft.ObsidianURI = "obsidian://open?vault=" + escapeURIComponent(o.vaultName) +
		"&file=" + escapeURIComponent(file)
	return nil
}

// escapeURIComponent encodes a URI component with spaces as "%20" instead of
// '+': Obsidian decodes URI values decodeURIComponent-style and does not turn
// '+' back into a space, so plain url.QueryEscape breaks paths with spaces.
func escapeURIComponent(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// MappingRule is one property mapping rule (field-compatible with
// config.MappingEntry). Value injects a fixed value; Frontmatter maps a note value.
type MappingRule struct {
	Frontmatter    string            // frontmatter key to read the value from
	NotionProperty string            // Notion property name to fill
	Value          string            // fixed value (takes precedence over Frontmatter)
	Values         map[string]string // note value -> Notion value table (optional)
	Default        string            // fallback when the value is empty or unmapped (optional)
}

// propertyMapper fills draft.Properties from a chain of MappingRules.
type propertyMapper struct {
	rules []MappingRule
}

// NewPropertyMapper returns a Transformer that applies rules in order to fill
// draft.Properties. A rule with Value injects that fixed value. A rule with
// Frontmatter maps the note value through the Values table; when the value is
// empty or unmapped, Default is used, and without a Default the property is
// skipped with a warning. Without a Values table the note value is used as is.
func NewPropertyMapper(rules []MappingRule) Transformer {
	return &propertyMapper{rules: rules}
}

func (p *propertyMapper) Name() string { return "propertyMapper" }

func (p *propertyMapper) Transform(note Note, draft *PageDraft) error {
	for _, rule := range p.rules {
		if rule.Value != "" {
			draft.Properties[rule.NotionProperty] = rule.Value
			continue
		}
		if rule.Frontmatter == "" {
			draft.Warnings = append(draft.Warnings,
				fmt.Sprintf("propertyMapper: rule for %s has neither value nor frontmatter, skipped",
					rule.NotionProperty))
			continue
		}
		raw := note.Frontmatter[rule.Frontmatter]
		resolved, ok := resolveValue(rule, raw)
		if !ok {
			draft.Warnings = append(draft.Warnings,
				fmt.Sprintf("propertyMapper: property %s skipped (frontmatter %q value %q has no translation and no default)",
					rule.NotionProperty, rule.Frontmatter, raw))
			continue
		}
		draft.Properties[rule.NotionProperty] = resolved
	}
	return nil
}

// resolveValue applies the rule's mapping table and fallback to raw.
// A false second return means there is no value to fill.
func resolveValue(rule MappingRule, raw string) (string, bool) {
	if raw == "" {
		if rule.Default == "" {
			return "", false
		}
		return rule.Default, true
	}
	if len(rule.Values) == 0 {
		return raw, true
	}
	if mapped, ok := rule.Values[raw]; ok {
		return mapped, true
	}
	if rule.Default == "" {
		return "", false
	}
	return rule.Default, true
}
