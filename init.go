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
	"github.com/yoonsooc/obsidian-notion-etl/internal/vault"
)

const (
	envPath          = ".env"
	baseConfigPath   = "base.config.yaml"
	latestConfigPath = "configs/latest.config.yaml"
	configBackupDir  = "configs/backups"
)

// runInit은 PRD FR-1의 설정 부트스트랩을 수행한다.
// base.config.yaml과 노션 DB 스키마를 검증하고 configs/latest.config.yaml을 생성한다.
// 항목 단위 경고는 logs/migration/의 실행별 로그 파일에 남긴다 (PRD 6.2).
func runInit() (err error) {
	logger, err := logging.New("migration", time.Now())
	if err != nil {
		return err
	}
	defer func() {
		// 실패로 끝나도 경고가 기록됐다면 사용자가 로그를 찾을 수 있게 안내한다.
		// 기록이 없으면 Close가 빈 로그 파일을 제거한다.
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

	// API 2025-09-03부터 속성 스키마는 데이터 소스에 붙는다. 이 도구는 단일
	// 데이터 소스 DB만 지원한다 (PRD 1.1 비목표: 데이터베이스 중첩/다중 소스).
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

	// init은 매핑 규칙의 작성자가 아니라 검증자다 (D5-설정 역할 분리).
	// 코드에 정의된 규칙(pipeline.go)을 실제 노션 스키마·실제 노트와 대조하고,
	// 통과한 규칙을 latest에 기록용 스냅샷으로 남긴다 (D10-변환 규칙의 위치).
	properties := toConfigProperties(ds.Properties)
	mapping := platinumMapping()
	dateRules := dailyDateRules()
	warnings, err := config.ValidateMapping(mapping, properties)
	if err != nil {
		return fmt.Errorf("mapping 규칙(pipeline.go) 검증 실패: %w", err)
	}
	for _, w := range warnings {
		logger.Warnf("%s", w)
	}
	if err := config.ValidateDateRules("pipeline.dailyDateRules", dateRules); err != nil {
		return fmt.Errorf("날짜 규칙(pipeline.go) 검증 실패: %w", err)
	}
	warnMissingFrontmatterKeys(mapping, keys, logger)

	latest := &config.LatestConfig{}
	latest.Obsidian.FrontmatterKeys = keys
	latest.Notion.DatabaseID = db.ID
	latest.Notion.DataSourceID = ds.ID
	latest.Notion.Properties = properties
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

// warnMissingFrontmatterKeys는 매핑이 참조하는 frontmatter 키가 실제 노트들에서
// 발견되지 않는 경우를 경고한다 (FR-1 5항: 규칙과 실데이터의 대조).
// 실제 매핑 조회(PropertyMapper)는 정확 일치이므로 검증도 정확 일치가 기준이고,
// 대소문자만 다른 키가 있으면 별도 경고로 구분해 안내한다.
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

// toConfigProperties는 노션 스키마 속성을 설정 파일 표현으로 변환한다.
func toConfigProperties(properties []notion.Property) []config.Property {
	out := make([]config.Property, 0, len(properties))
	for _, p := range properties {
		out = append(out, config.Property{Name: p.Name, Type: p.Type})
	}
	return out
}
