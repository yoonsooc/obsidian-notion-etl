package pipeline

import (
	"strings"
	"testing"

	"github.com/yoonsooc/obsidian-notion-etl/internal/config"
	"github.com/yoonsooc/obsidian-notion-etl/internal/transform"
)

type fakePlugin struct{ name string }

func (f fakePlugin) Name() string                        { return f.name }
func (fakePlugin) DateRules() []config.DateRule          { return []config.DateRule{{FileLayout: "060102"}} }
func (fakePlugin) Mapping() []config.MappingEntry        { return nil }
func (fakePlugin) Transformers() []transform.Transformer { return nil }

// withRegistry swaps the global registry for the test's lifetime.
func withRegistry(t *testing.T, plugins ...Plugin) {
	t.Helper()
	saved := registry
	registry = map[string]Plugin{}
	for _, p := range plugins {
		registry[p.Name()] = p
	}
	t.Cleanup(func() { registry = saved })
}

func TestSelectDefaultWhenNothingRegistered(t *testing.T) {
	withRegistry(t)

	p, err := Select(config.PipelineConfig{})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if p.Name() != DefaultName {
		t.Fatalf("Name = %q, want %q", p.Name(), DefaultName)
	}
	rules := p.DateRules()
	if len(rules) == 0 || rules[0].FileLayout != "2006-01-02" {
		t.Fatalf("fallback DateRules = %+v, want 2006-01-02 first", rules)
	}
	if err := config.ValidateDateRules("plugin.default", rules); err != nil {
		t.Fatalf("fallback rules must validate: %v", err)
	}
	if len(p.Mapping()) != 0 {
		t.Fatalf("fallback Mapping = %+v, want empty", p.Mapping())
	}
	if len(p.Transformers()) != 1 || p.Transformers()[0].Name() != "nfcTitle" {
		t.Fatalf("default Transformers = %+v, want [nfcTitle]", p.Transformers())
	}
}

func TestSelectDefaultUsesConfigRules(t *testing.T) {
	withRegistry(t, fakePlugin{name: "user"})

	cfg := config.PipelineConfig{
		Plugin:   DefaultName,
		DateFrom: []config.DateRule{{FrontmatterKey: "made_at"}},
		Mapping:  []config.MappingEntry{{NotionProperty: "Type", Value: "Todo"}},
	}
	p, err := Select(cfg)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if p.Name() != DefaultName {
		t.Fatalf("Name = %q, want %q", p.Name(), DefaultName)
	}
	if rules := p.DateRules(); len(rules) != 1 || rules[0].FrontmatterKey != "made_at" {
		t.Fatalf("DateRules = %+v, want config rule", rules)
	}
	if m := p.Mapping(); len(m) != 1 || m[0].NotionProperty != "Type" {
		t.Fatalf("Mapping = %+v, want config mapping", m)
	}
}

func TestSelectNamedAndSoleFallback(t *testing.T) {
	withRegistry(t, fakePlugin{name: "user"})

	if p, err := Select(config.PipelineConfig{Plugin: "user"}); err != nil || p.Name() != "user" {
		t.Fatalf("named select = %v, %v", p, err)
	}
	// Empty name with exactly one registered plugin selects it.
	if p, err := Select(config.PipelineConfig{}); err != nil || p.Name() != "user" {
		t.Fatalf("sole fallback = %v, %v", p, err)
	}
}

func TestSelectErrors(t *testing.T) {
	withRegistry(t, fakePlugin{name: "a"}, fakePlugin{name: "b"})

	if _, err := Select(config.PipelineConfig{}); err == nil || !strings.Contains(err.Error(), "[a, b]") {
		t.Fatalf("ambiguous select error = %v, want registered names listed", err)
	}
	if _, err := Select(config.PipelineConfig{Plugin: "nope"}); err == nil {
		t.Fatal("unknown name must error")
	}
	// Config rule fields are default-plugin-only.
	cfg := config.PipelineConfig{Plugin: "a", DateFrom: []config.DateRule{{FrontmatterKey: "date"}}}
	if _, err := Select(cfg); err == nil || !strings.Contains(err.Error(), DefaultName) {
		t.Fatalf("rules with user plugin = %v, want error mentioning %q", err, DefaultName)
	}
}

func TestRegisterRejectsReservedAndDuplicate(t *testing.T) {
	withRegistry(t)

	mustPanic := func(name string, p Plugin) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatalf("Register(%s) must panic", name)
			}
		}()
		Register(p)
	}
	mustPanic("default", fakePlugin{name: DefaultName})
	mustPanic("empty", fakePlugin{name: " "})
	Register(fakePlugin{name: "once"})
	mustPanic("duplicate", fakePlugin{name: "once"})
}

func TestBuildChainOrder(t *testing.T) {
	withRegistry(t)

	p, err := Select(config.PipelineConfig{})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	chain := BuildChain(p, "MyVault", "Daily", nil)
	names := make([]string, 0, len(chain))
	for _, s := range chain {
		names = append(names, s.Name())
	}
	// Custom stages sit after title derivation, before URI/property mapping.
	want := []string{"dateDeriver", "titleFromFilename", "nfcTitle", "obsidianURI", "propertyMapper"}
	if len(names) != len(want) {
		t.Fatalf("chain = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("chain[%d] = %q, want %q (full: %v)", i, names[i], want[i], names)
		}
	}
}
