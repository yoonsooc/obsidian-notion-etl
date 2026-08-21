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

	mapping := buildMapping(keys, ds.Properties, logger)

	latest := &config.LatestConfig{}
	latest.Obsidian.FrontmatterKeys = keys
	latest.Notion.DatabaseID = db.ID
	latest.Notion.DataSourceID = ds.ID
	latest.Notion.Properties = toConfigProperties(ds.Properties)
	latest.Mapping = mapping

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
	fmt.Printf("매핑 %d건 생성, %s 저장 완료\n", len(mapping), latestConfigPath)
	if archived {
		fmt.Printf("설정이 변경되어 기존 설정을 %s에 아카이빙함\n", configBackupDir)
	}
	if logger.Wrote() {
		fmt.Printf("상세 로그: %s\n", logger.Path())
	}
	return nil
}

// buildMapping은 frontmatter 키와 노션 속성 이름을 대소문자 무시로 매칭한다.
// title/date/url 타입 속성은 파일명에서 파생되므로 매핑 대상에서 제외하고,
// 매칭에 실패한 키는 warn(실행 로그 파일)에 안내한다.
func buildMapping(keys []string, properties []notion.Property, warn io.Writer) []config.MappingEntry {
	byLowerName := make(map[string]notion.Property, len(properties))
	for _, p := range properties {
		byLowerName[strings.ToLower(p.Name)] = p
	}

	mapping := make([]config.MappingEntry, 0, len(keys))
	for _, key := range keys {
		p, ok := byLowerName[strings.ToLower(key)]
		if !ok {
			fmt.Fprintf(warn, "매핑 제외: frontmatter 키 %q에 대응하는 노션 속성이 없음\n", key)
			continue
		}
		switch p.Type {
		case "title", "date", "url":
			fmt.Fprintf(warn, "매핑 제외: 노션 속성 %q(%s)는 파일명에서 파생되므로 건너뜀\n", p.Name, p.Type)
			continue
		}
		mapping = append(mapping, config.MappingEntry{Frontmatter: key, NotionProperty: p.Name})
	}
	return mapping
}

// toConfigProperties는 노션 스키마 속성을 설정 파일 표현으로 변환한다.
func toConfigProperties(properties []notion.Property) []config.Property {
	out := make([]config.Property, 0, len(properties))
	for _, p := range properties {
		out = append(out, config.Property{Name: p.Name, Type: p.Type})
	}
	return out
}
