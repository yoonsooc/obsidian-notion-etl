// Package pipeline is the single seam between the common transform machinery
// and user-defined plugins. Plugins implement the Plugin interface and
// register themselves at init time; common code selects one through Lookup
// (by the name in base.config.yaml) and assembles the chain with BuildChain,
// so nothing outside main's blank import depends on a plugin package.
package pipeline

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/yoonsooc/obsidian-notion-etl/internal/config"
	"github.com/yoonsooc/obsidian-notion-etl/internal/transform"
)

// Plugin is the contract a user-defined transform policy implements.
// All methods are declarative: they return rule data or transformer values and
// must not fail. Rule problems surface later through init's validation
// (config.ValidateMapping / ValidateDateRules), and per-note transform errors
// are contained by transform.Run, so no plugin error propagates to this seam.
type Plugin interface {
	// Name identifies the plugin; pipeline.plugin in base.config.yaml selects it.
	Name() string
	// DateRules returns the date derivation chain, tried top to bottom.
	DateRules() []config.DateRule
	// Mapping returns the Notion property mapping entries.
	Mapping() []config.MappingEntry
	// Transformers returns custom stages inserted after title derivation
	// and before the URI/property-mapping built-ins.
	Transformers() []transform.Transformer
}

// DefaultName is the built-in config-driven plugin's name.
const DefaultName = "default"

var registry = map[string]Plugin{}

// Register adds a plugin to the registry. It is meant to be called from a
// plugin package's init; a blank, reserved, or duplicate name is a
// programming error and panics (initial-setup failure, consistent with the
// project's panic policy).
func Register(p Plugin) {
	name := p.Name()
	if strings.TrimSpace(name) == "" {
		panic("pipeline: plugin with empty name")
	}
	if name == DefaultName {
		panic("pipeline: plugin name " + DefaultName + " is reserved for the built-in plugin")
	}
	if _, dup := registry[name]; dup {
		panic("pipeline: duplicate plugin name " + name)
	}
	registry[name] = p
}

// Select resolves the plugin for a run. "default" (or an empty name with no
// plugin registered) yields the built-in plugin whose rules come from the
// config; otherwise the name resolves against the registry, with the sole
// registered plugin as the empty-name fallback. The config rule fields belong
// to the default plugin, so combining them with a user plugin is an error
// rather than a silent drop.
func Select(cfg config.PipelineConfig) (Plugin, error) {
	if cfg.Plugin == DefaultName || (cfg.Plugin == "" && len(registry) == 0) {
		return defaultPlugin{dateFrom: cfg.DateFrom, mapping: cfg.Mapping}, nil
	}
	p, err := lookup(cfg.Plugin)
	if err != nil {
		return nil, err
	}
	if len(cfg.DateFrom) > 0 || len(cfg.Mapping) > 0 {
		return nil, fmt.Errorf("pipeline.dateFrom/mapping은 default 플러그인 전용입니다: plugin을 %q 대신 %q로 지정하거나 규칙을 제거하세요", p.Name(), DefaultName)
	}
	return p, nil
}

// lookup selects a registered plugin by name. An empty name selects the sole
// registered plugin; with several registered, the config must name one.
func lookup(name string) (Plugin, error) {
	if name == "" {
		if len(registry) == 1 {
			for _, p := range registry {
				return p, nil
			}
		}
		return nil, fmt.Errorf("pipeline.plugin 미지정: 등록된 플러그인 %s 중 하나 또는 %q를 base.config.yaml에 지정하세요", registeredNames(), DefaultName)
	}
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("알 수 없는 플러그인 %q: 등록된 플러그인은 %s", name, registeredNames())
	}
	return p, nil
}

// defaultPlugin is the built-in plugin used when no user plugin is selected.
// Rules come from base.config.yaml's pipeline section, so they are editable
// without writing Go code; absent rules fall back to generic Obsidian
// conventions so the tool functions with zero plugin code.
type defaultPlugin struct {
	dateFrom []config.DateRule
	mapping  []config.MappingEntry
}

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

// NFCTitle returns a transformer that normalizes the title to NFC. macOS
// stores filenames in NFD, so a title left in NFD (e.g. Korean filenames)
// would make the title-equality duplicate check miss on re-runs.
func NFCTitle() transform.Transformer { return nfcTitle{} }

type nfcTitle struct{}

func (nfcTitle) Name() string { return "nfcTitle" }

func (nfcTitle) Transform(_ transform.Note, draft *transform.PageDraft) error {
	draft.Title = norm.NFC.String(draft.Title)
	return nil
}

func registeredNames() string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return "[" + strings.Join(names, ", ") + "]"
}

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
