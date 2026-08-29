// Package pipeline is the single seam between the common transform machinery
// and user-defined plugins. Plugins implement the Plugin interface and
// register themselves at init time; common code selects one through Select
// (by the name in base.config.yaml) and assembles the chain with BuildChain,
// so nothing outside main's blank import depends on a plugin package.
//
// File layout: pipeline.go holds the contract and plugin selection,
// default.go the built-in config-driven plugin, chain.go the chain assembly.
package pipeline

import (
	"fmt"
	"sort"
	"strings"

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

func registeredNames() string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return "[" + strings.Join(names, ", ") + "]"
}
