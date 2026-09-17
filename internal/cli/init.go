// Package cli implements the subcommand entry points (init, migrate, backup).
// It assembles both domains and the plugin seam; main only routes here and
// blank-imports the plugin package for registration.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/yoonsooc/obsidian-notion-etl/internal/config"
	"github.com/yoonsooc/obsidian-notion-etl/internal/logging"
	"github.com/yoonsooc/obsidian-notion-etl/internal/notion"
	"github.com/yoonsooc/obsidian-notion-etl/internal/pipeline"
	"github.com/yoonsooc/obsidian-notion-etl/internal/vault"
)

const (
	envPath          = ".env"
	baseConfigPath   = "base.config.yaml"
	latestConfigPath = "configs/latest.config.yaml"
	configBackupDir  = "configs/backups"
)

// RunInit validates base.config.yaml and the Notion DB schema, then writes
// configs/latest.config.yaml. Per-item warnings go to a per-run log file.
func RunInit() (err error) {
	logger, err := logging.New("migration", time.Now())
	if err != nil {
		return err
	}
	p := NewPrinter("")
	defer func() {
		// Point the user at the log even on failure; Close removes the
		// log file if nothing was written.
		if err != nil && logger.Wrote() {
			p.Fprintf(os.Stderr, "Details: %s\n", logger.Path())
		}
		_ = logger.Close()
	}()

	token, err := config.LoadNotionToken(envPath)
	if err != nil {
		return err
	}

	base, err := config.LoadBase(baseConfigPath)
	if err != nil {
		return err
	}
	p = NewPrinter(base.Lang)

	databaseID, err := notion.ExtractDatabaseID(base.Notion.DB.URL)
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := notion.NewClient(token)
	db, err := client.RetrieveDatabase(ctx, databaseID)
	if err != nil {
		return fmt.Errorf("retrieve Notion DB (%s) (check the token and integration connection): %w", base.Notion.DB.Name, err)
	}

	// Since Notion API 2025-09-03 the property schema lives on the data
	// source. Only single-data-source databases are supported.
	if len(db.DataSources) == 0 {
		return fmt.Errorf("notion DB %q has no data source", db.Title)
	}
	if len(db.DataSources) > 1 {
		names := make([]string, 0, len(db.DataSources))
		for _, ds := range db.DataSources {
			names = append(names, ds.Name)
		}
		return fmt.Errorf("notion DB %q has %d data sources (only a single source is supported): %s",
			db.Title, len(db.DataSources), strings.Join(names, ", "))
	}

	ds, err := client.RetrieveDataSource(ctx, db.DataSources[0].ID)
	if err != nil {
		return fmt.Errorf("retrieve Notion data source (%s) schema: %w", db.DataSources[0].Name, err)
	}

	keys, err := vault.ScanFrontmatterKeys(base.SourceDir(), base.Obsidian.Vault.ToNotion.Exclude, logger)
	if err != nil {
		return fmt.Errorf("scan source directory: %w", err)
	}

	// init is a validator, not the author, of the mapping rules: it checks
	// the rules declared by the selected plugin against the live Notion schema
	// and actual notes, and snapshots the passing rules into latest for record.
	plug, err := pipeline.Select(base.Pipeline)
	if err != nil {
		return err
	}
	properties := toConfigProperties(ds.Properties)
	mapping := plug.Mapping()
	dateRules := plug.DateRules()
	warnings, err := config.ValidateMapping(mapping, properties)
	if err != nil {
		return fmt.Errorf("validate mapping rules (plugin %s): %w", plug.Name(), err)
	}
	for _, w := range warnings {
		logger.Warnf("%s", w)
	}
	if err := config.ValidateDateRules("plugin."+plug.Name()+".dateRules", dateRules); err != nil {
		return fmt.Errorf("validate date rules (plugin %s): %w", plug.Name(), err)
	}
	warnMissingFrontmatterKeys(mapping, keys, logger)

	latest := &config.LatestConfig{}
	latest.Obsidian.FrontmatterKeys = keys
	latest.Notion.DatabaseID = db.ID
	latest.Notion.DataSourceID = ds.ID
	latest.Notion.Properties = properties
	latest.Plugin = plug.Name()
	latest.Mapping = mapping
	latest.DateFrom = dateRules

	prev, err := config.LoadLatest(latestConfigPath)
	if err != nil {
		return err
	}
	if prev != nil {
		latest.State = prev.State
	}

	archived, err := config.SaveLatest(latestConfigPath, configBackupDir, latest, time.Now())
	if err != nil {
		return err
	}

	p.Printf("Notion DB %q(%s): data source %q, %d properties verified\n", db.Title, db.ID, db.DataSources[0].Name, len(ds.Properties))
	p.Printf("Obsidian source (%s): collected %d frontmatter keys\n", base.SourceDir(), len(keys))
	p.Printf("Validated %d mapping rules (%d warnings), saved %s\n", len(mapping), len(warnings), latestConfigPath)
	if archived {
		p.Printf("Config changed; previous version archived to %s\n", configBackupDir)
	}
	if logger.Wrote() {
		p.Printf("Details: %s\n", logger.Path())
	}
	return nil
}

// warnMissingFrontmatterKeys warns when a mapping references a frontmatter key
// not found in the scanned notes. Mapping lookup is exact-match, so a key that
// differs only in case gets its own distinct warning.
func warnMissingFrontmatterKeys(mapping []config.MappingEntry, keys []string, warn io.Writer) {
	exact := make(map[string]struct{}, len(keys))
	byFold := make(map[string]string, len(keys))
	for _, key := range keys {
		exact[key] = struct{}{}
		byFold[strings.ToLower(key)] = key
	}
	for _, m := range mapping {
		if m.Frontmatter == "" {
			continue
		}
		if _, ok := exact[m.Frontmatter]; ok {
			continue
		}
		if actual, ok := byFold[strings.ToLower(m.Frontmatter)]; ok {
			fmt.Fprintf(warn, "mapping(%s): frontmatter key %q does not match the notes' %q due to letter case\n", m.NotionProperty, m.Frontmatter, actual)
			continue
		}
		fmt.Fprintf(warn, "mapping(%s): frontmatter key %q not found in the scanned notes\n", m.NotionProperty, m.Frontmatter)
	}
}

// toConfigProperties converts Notion schema properties to the config representation.
func toConfigProperties(properties []notion.Property) []config.Property {
	out := make([]config.Property, 0, len(properties))
	for _, p := range properties {
		out = append(out, config.Property{Name: p.Name, Type: p.Type})
	}
	return out
}
