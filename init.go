package main

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

// runInit validates base.config.yaml and the Notion DB schema, then writes
// configs/latest.config.yaml. Per-item warnings go to a per-run log file.
func runInit() (err error) {
	logger, err := logging.New("migration", time.Now())
	if err != nil {
		return err
	}
	defer func() {
		// Point the user at the log even on failure; Close removes the
		// log file if nothing was written.
		if err != nil && logger.Wrote() {
			fmt.Fprintf(os.Stderr, "상세 로그: %s\n", logger.Path())
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

	databaseID, err := notion.ExtractDatabaseID(base.Notion.DB.URL)
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := notion.NewClient(token)
	db, err := client.RetrieveDatabase(ctx, databaseID)
	if err != nil {
		return fmt.Errorf("노션 DB(%s) 조회 실패 (토큰 권한과 Integration 연결을 확인하세요): %w", base.Notion.DB.Name, err)
	}

	// Since Notion API 2025-09-03 the property schema lives on the data
	// source. Only single-data-source databases are supported.
	if len(db.DataSources) == 0 {
		return fmt.Errorf("노션 DB %q에 데이터 소스가 없음", db.Title)
	}
	if len(db.DataSources) > 1 {
		names := make([]string, 0, len(db.DataSources))
		for _, ds := range db.DataSources {
			names = append(names, ds.Name)
		}
		return fmt.Errorf("노션 DB %q에 데이터 소스가 %d개 있음(단일 소스만 지원): %s",
			db.Title, len(db.DataSources), strings.Join(names, ", "))
	}

	ds, err := client.RetrieveDataSource(ctx, db.DataSources[0].ID)
	if err != nil {
		return fmt.Errorf("노션 데이터 소스(%s) 스키마 조회 실패: %w", db.DataSources[0].Name, err)
	}

	keys, err := vault.ScanFrontmatterKeys(base.SourceDir(), base.Obsidian.Vault.ToNotion.Exclude, logger)
	if err != nil {
		return fmt.Errorf("소스 디렉토리 스캔 실패: %w", err)
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
		return fmt.Errorf("mapping 규칙(플러그인 %s) 검증 실패: %w", plug.Name(), err)
	}
	for _, w := range warnings {
		logger.Warnf("%s", w)
	}
	if err := config.ValidateDateRules("plugin."+plug.Name()+".dateRules", dateRules); err != nil {
		return fmt.Errorf("날짜 규칙(플러그인 %s) 검증 실패: %w", plug.Name(), err)
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

	fmt.Printf("노션 DB %q(%s): 데이터 소스 %q, 속성 %d개 확인\n", db.Title, db.ID, db.DataSources[0].Name, len(ds.Properties))
	fmt.Printf("옵시디언 소스(%s): frontmatter 키 %d개 수집\n", base.SourceDir(), len(keys))
	fmt.Printf("매핑 규칙 %d건 검증(경고 %d건), %s 저장 완료\n", len(mapping), len(warnings), latestConfigPath)
	if archived {
		fmt.Printf("설정이 변경되어 기존 설정을 %s에 아카이빙함\n", configBackupDir)
	}
	if logger.Wrote() {
		fmt.Printf("상세 로그: %s\n", logger.Path())
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
			fmt.Fprintf(warn, "mapping(%s): frontmatter 키 %q가 노트의 %q와 대소문자가 달라 매칭되지 않음\n", m.NotionProperty, m.Frontmatter, actual)
			continue
		}
		fmt.Fprintf(warn, "mapping(%s): frontmatter 키 %q가 스캔된 노트들에서 발견되지 않음\n", m.NotionProperty, m.Frontmatter)
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
