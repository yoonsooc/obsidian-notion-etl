package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/yoonsooc/obsidian-notion-etl/internal/config"
	"github.com/yoonsooc/obsidian-notion-etl/internal/logging"
	"github.com/yoonsooc/obsidian-notion-etl/internal/markdown"
	"github.com/yoonsooc/obsidian-notion-etl/internal/notion"
	"github.com/yoonsooc/obsidian-notion-etl/internal/pipeline"
	"github.com/yoonsooc/obsidian-notion-etl/internal/transform"
	"github.com/yoonsooc/obsidian-notion-etl/internal/vault"
)

// migrateWorkers is the worker pool size. API throughput is governed by the
// client's built-in rate limiter (2.5 TPS), not the worker count.
const migrateWorkers = 5

// migrateEnv is the execution environment shared by workers.
// Every field except claimed is read-only during a run.
type migrateEnv struct {
	client        *notion.Client
	logger        *logging.Logger
	chain         []transform.Transformer
	dataSourceID  string
	titleProp     string            // name of the title property
	dateProp      string            // name of the date property, empty if none
	urlProp       string            // name of the url property, empty if none
	typeByName    map[string]string // Notion property name -> type
	vaultName     string            // Obsidian vault name, used for wiki-link URIs
	effectiveDate time.Time         // zero value disables the gate
	dryRun        bool

	// claimed is the set of duplicate-check keys already taken in this run.
	// When two notes deriving the same date/title land on different workers,
	// it locally blocks the check-then-create race that a Notion query alone
	// cannot catch.
	claimedMu sync.Mutex
	claimed   map[string]struct{}
}

// claim reserves a duplicate-check key, returning false if already taken.
func (env *migrateEnv) claim(key string) bool {
	env.claimedMu.Lock()
	defer env.claimedMu.Unlock()
	if _, taken := env.claimed[key]; taken {
		return false
	}
	env.claimed[key] = struct{}{}
	return true
}

// migrateStats aggregates per-note outcomes across workers.
type migrateStats struct {
	mu          sync.Mutex
	migrated    int
	skippedDup  int
	skippedGate int
	failed      int
}

func (s *migrateStats) add(outcome string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch outcome {
	case "migrated":
		s.migrated++
	case "skippedDup":
		s.skippedDup++
	case "skippedGate":
		s.skippedGate++
	case "failed":
		s.failed++
	}
}

// runMigrate migrates Obsidian notes to Notion. With --dry-run it only logs
// what would be migrated, without creating pages or updating state.
func runMigrate(args []string) (err error) {
	dryRun := false
	for _, a := range args {
		if a != "--dry-run" {
			return fmt.Errorf("migrate: 알 수 없는 인자: %s", a)
		}
		dryRun = true
	}

	logger, err := logging.New("migration", time.Now())
	if err != nil {
		return err
	}
	summaryPrinted := false
	defer func() {
		// Skip the log path hint if the summary already printed it.
		if err != nil && logger.Wrote() && !summaryPrinted {
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
	latest, err := config.LoadLatest(latestConfigPath)
	if err != nil {
		return err
	}
	if latest == nil || latest.Notion.DataSourceID == "" {
		return errors.New("검증된 설정이 없음: 먼저 'etl-worker init'을 실행하세요")
	}

	env, err := newMigrateEnv(token, base, latest, logger, dryRun)
	if err != nil {
		return err
	}

	toNotion := base.Obsidian.Vault.ToNotion
	notes, err := vault.CollectNotes(base.SourceDir(), toNotion.Exclude, logger)
	if err != nil {
		return fmt.Errorf("소스 디렉토리 스캔 실패: %w", err)
	}

	var stats migrateStats
	jobs := make(chan vault.Note)
	var wg sync.WaitGroup
	for range migrateWorkers {
		wg.Go(func() {
			for note := range jobs {
				stats.add(env.processNote(context.Background(), note))
			}
		})
	}
	for _, note := range notes {
		jobs <- note
	}
	close(jobs)
	wg.Wait()

	if !dryRun {
		now := time.Now()
		if latest.State.FirstRunAt == "" {
			latest.State.FirstRunAt = now.Format(time.RFC3339)
		}
		latest.State.LastMigrateRunAt = now.Format(time.RFC3339)
		if _, err := config.SaveLatest(latestConfigPath, configBackupDir, latest, now); err != nil {
			return err
		}
	}

	mode := ""
	if dryRun {
		mode = " (dry-run: 실제 생성 없음)"
	}
	fmt.Printf("마이그레이션 완료%s: 대상 %d건 중 이관 %d, 중복 스킵 %d, 날짜 게이트 스킵 %d, 실패 %d\n",
		mode, len(notes), stats.migrated, stats.skippedDup, stats.skippedGate, stats.failed)
	if logger.Wrote() {
		fmt.Printf("상세 로그: %s\n", logger.Path())
		summaryPrinted = true
	}
	if stats.failed > 0 {
		return fmt.Errorf("%d건 실패 (상세는 로그 참조)", stats.failed)
	}
	return nil
}

// newMigrateEnv assembles the worker environment from the validated config.
func newMigrateEnv(token string, base *config.BaseConfig, latest *config.LatestConfig, logger *logging.Logger, dryRun bool) (*migrateEnv, error) {
	env := &migrateEnv{
		client:       notion.NewClient(token),
		logger:       logger,
		dataSourceID: latest.Notion.DataSourceID,
		typeByName:   make(map[string]string, len(latest.Notion.Properties)),
		dryRun:       dryRun,
		claimed:      make(map[string]struct{}),
	}

	for _, p := range latest.Notion.Properties {
		env.typeByName[p.Name] = p.Type
		switch p.Type {
		case "title":
			env.titleProp = p.Name
		case "date":
			if env.dateProp == "" {
				env.dateProp = p.Name
			}
		case "url":
			if env.urlProp == "" {
				env.urlProp = p.Name
			}
		}
	}
	if env.titleProp == "" {
		return nil, errors.New("노션 스키마에 title 속성이 없음: init을 다시 실행하세요")
	}

	toNotion := base.Obsidian.Vault.ToNotion
	if toNotion.EffectiveDate != "" {
		effective, err := time.Parse("2006-01-02", toNotion.EffectiveDate)
		if err != nil {
			return nil, fmt.Errorf("effectiveDate 해석 실패: %w", err)
		}
		env.effectiveDate = effective
	}

	// Transform rules live in the selected plugin; the latest snapshot is for record only.
	plug, err := pipeline.Select(base.Pipeline)
	if err != nil {
		return nil, err
	}
	env.vaultName = toNotion.Name
	env.chain = pipeline.BuildChain(plug, toNotion.Name, toNotion.Target, latest.Notion.Properties)
	return env, nil
}

// processNote runs one note through pipeline -> gate -> duplicate check ->
// create, returning "migrated", "skippedDup", "skippedGate", or "failed".
// Failures are logged and processing continues with the next note.
func (env *migrateEnv) processNote(ctx context.Context, note vault.Note) string {
	draft, err := transform.Run(transform.Note{
		Filename:    note.Filename,
		RelPath:     note.RelPath,
		Frontmatter: note.Frontmatter,
		Body:        note.Body,
	}, env.chain)
	if err != nil {
		env.logger.Warnf("변환 실패 %s: %v", note.RelPath, err)
		return "failed"
	}
	for _, w := range draft.Warnings {
		env.logger.Warnf("%s: %s", note.RelPath, w)
	}

	// effectiveDate gate: filters only notes with a derived date; notes
	// without one pass through.
	if draft.Date != "" && !env.effectiveDate.IsZero() {
		d, err := time.Parse("2006-01-02", draft.Date)
		if err != nil {
			env.logger.Warnf("파생 날짜 해석 실패 %s (%q): %v", note.RelPath, draft.Date, err)
			return "failed"
		}
		if d.Before(env.effectiveDate) {
			env.logger.Infof("게이트 스킵 %s: 날짜 %s가 effectiveDate 이전", note.RelPath, draft.Date)
			return "skippedGate"
		}
	}

	// Duplicate check: by date when available, otherwise by title. Claim the
	// key within this run first to block the check-then-create race between
	// workers, then query Notion.
	dupKey := "title:" + draft.Title
	if draft.Date != "" && env.dateProp != "" {
		dupKey = "date:" + draft.Date
	}
	if !env.claim(dupKey) {
		env.logger.Infof("중복 스킵 %s: 이번 실행의 다른 노트와 %s 겹침", note.RelPath, dupKey)
		return "skippedDup"
	}
	exists, err := env.checkDuplicate(ctx, draft)
	if err != nil {
		env.logger.Warnf("중복 검사 실패 %s: %v", note.RelPath, err)
		return "failed"
	}
	if exists {
		env.logger.Infof("중복 스킵 %s", note.RelPath)
		return "skippedDup"
	}

	properties := env.buildProperties(draft, note.RelPath)
	blocks := markdown.ToBlocks(note.Body, env.vaultName)

	if env.dryRun {
		env.logger.Infof("[dry-run] 생성 예정 %s: 제목 %q, 날짜 %q, 블록 %d개, 속성 %d개",
			note.RelPath, draft.Title, draft.Date, len(blocks), len(properties))
		return "migrated"
	}

	pageID, err := env.client.CreatePage(ctx, env.dataSourceID, properties, blocks)
	if err != nil {
		// On partial creation (page created, block append failed) the page ID
		// and cleanup hint are already part of CreatePage's error message.
		env.logger.Warnf("생성 실패 %s: %v", note.RelPath, err)
		return "failed"
	}
	env.logger.Infof("이관 완료 %s -> %s", note.RelPath, pageID)
	return "migrated"
}

// checkDuplicate queries Notion for an existing page matching the draft.
func (env *migrateEnv) checkDuplicate(ctx context.Context, draft *transform.PageDraft) (bool, error) {
	if draft.Date != "" && env.dateProp != "" {
		return env.client.ExistsByDate(ctx, env.dataSourceID, env.dateProp, draft.Date)
	}
	return env.client.ExistsByTitle(ctx, env.dataSourceID, env.titleProp, draft.Title)
}

// buildProperties converts a draft into the Notion property payload.
// Dedicated properties (title/date/url) come from the draft's dedicated
// fields; the rest come from the mapping results.
func (env *migrateEnv) buildProperties(draft *transform.PageDraft, relPath string) map[string]notion.PropertyValue {
	properties := make(map[string]notion.PropertyValue, len(draft.Properties)+3)
	properties[env.titleProp] = notion.PropertyValue{Type: "title", Value: draft.Title}
	if draft.Date != "" && env.dateProp != "" {
		properties[env.dateProp] = notion.PropertyValue{Type: "date", Value: draft.Date}
	}
	if draft.ObsidianURI != "" && env.urlProp != "" {
		properties[env.urlProp] = notion.PropertyValue{Type: "url", Value: draft.ObsidianURI}
	}
	for name, value := range draft.Properties {
		propType, ok := env.typeByName[name]
		if !ok {
			// Defensive: should not happen once init validation passed.
			env.logger.Warnf("%s: 노션 속성 %q의 타입을 알 수 없어 속성을 건너뜀", relPath, name)
			continue
		}
		properties[name] = notion.PropertyValue{Type: propType, Value: value}
	}
	return properties
}
