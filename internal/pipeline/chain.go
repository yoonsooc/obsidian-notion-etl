package pipeline

import (
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/yoonsooc/obsidian-notion-etl/internal/config"
	"github.com/yoonsooc/obsidian-notion-etl/internal/transform"
)

// BuildChain assembles the migration transform chain: built-in stages around
// the plugin's rules and custom transformers.
func BuildChain(p Plugin, vaultName, target string, properties []config.Property) []transform.Transformer {
	chain := []transform.Transformer{
		transform.NewDateDeriver(dateRules(p.DateRules())),
		transform.NewTitleFromFilename(),
	}
	chain = append(chain, p.Transformers()...)
	return append(chain,
		transform.NewObsidianURI(vaultName, target),
		transform.NewPropertyMapper(mappingRules(p.Mapping(), properties)),
	)
}

// NFCTitle returns a transformer that normalizes the title to NFC. macOS
// stores filenames in NFD, so a title left in NFD (e.g. Korean filenames)
// would make the title-equality duplicate check miss on re-runs.
func NFCTitle() transform.Transformer { return nfcTitle{} }

type nfcTitle struct{}

var _ transform.Transformer = nfcTitle{}

func (nfcTitle) Name() string { return "nfcTitle" }

func (nfcTitle) Transform(_ transform.Note, draft *transform.PageDraft) error {
	draft.Title = norm.NFC.String(draft.Title)
	return nil
}

// dateRules converts rule definitions to the pipeline type.
func dateRules(rules []config.DateRule) []transform.DateRule {
	out := make([]transform.DateRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, transform.DateRule{FileLayout: r.FileLayout, FrontmatterKey: r.FrontmatterKey})
	}
	return out
}

// mappingRules converts validated mapping entries to the pipeline type,
// enforcing ValidateMapping's "duplicate entries are ignored" rule.
// Property names resolve in the same order as ValidateMapping: exact match
// first, then case-insensitive match to the schema's actual name, so schemas
// with case-only property variants do not get misrouted.
func mappingRules(entries []config.MappingEntry, properties []config.Property) []transform.MappingRule {
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
