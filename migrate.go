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
	"github.com/yoonsooc/obsidian-notion-etl/internal/transform"
	"github.com/yoonsooc/obsidian-notion-etl/internal/vault"
)

// migrateWorkers는 마이그레이션 워커 풀 크기다 (PRD FR-2 7항).
// API 처리량은 워커 수가 아니라 클라이언트에 내장된 Limiter(2.5 TPS)가 결정한다.
const migrateWorkers = 5

// migrateEnv는 워커들이 공유하는 실행 환경이다.
// claimed를 제외한 모든 필드는 실행 중 읽기 전용이다.
type migrateEnv struct {
	client        *notion.Client
	logger        *logging.Logger
	chain         []transform.Transformer
	dataSourceID  string
	titleProp     string            // title 타입 속성 이름
	dateProp      string            // date 타입 속성 이름 (없으면 빈 문자열)
	urlProp       string            // url 타입 속성 이름 (없으면 빈 문자열)
	typeByName    map[string]string // 노션 속성 이름 -> 타입
	vaultName     string            // 옵시디언 볼트 이름 (본문 위키링크 URI 생성용)
	effectiveDate time.Time         // 제로값이면 게이트 없음
	dryRun        bool

	// claimed는 이번 실행에서 이미 처리(생성 예약)된 중복 검사 키의 집합이다.
	// 같은 날짜/제목으로 파생되는 두 노트를 서로 다른 워커가 동시에 처리할 때,
	// 노션 조회만으로는 못 잡는 검사-생성 사이의 경합을 로컬에서 차단한다.
	claimedMu sync.Mutex
	claimed   map[string]struct{}
}

// claim은 중복 검사 키를 선점한다. 이미 선점된 키면 거짓을 돌려준다.
func (env *migrateEnv) claim(key string) bool {
	env.claimedMu.Lock()
	defer env.claimedMu.Unlock()
	if _, taken := env.claimed[key]; taken {
		return false
	}
	env.claimed[key] = struct{}{}
	return true
}

// migrateStats는 실행 결과 집계다.
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

// runMigrate는 PRD FR-2의 마이그레이션을 수행한다.
// --dry-run이면 페이지 생성과 state 갱신 없이 무엇이 이관될지 로그로만 보여준다.
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
		// 요약이 이미 로그 경로를 안내했다면 중복 출력하지 않는다.
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

// newMigrateEnv는 검증된 설정에서 워커 실행 환경을 조립한다.
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

	// 변환 규칙은 코드에 있다 (D10-변환 규칙의 위치). latest 스냅샷은 기록용이다.
	env.vaultName = toNotion.Name
	env.chain = buildPipeline(toNotion.Name, toNotion.Target, latest.Notion.Properties)
	return env, nil
}

// processNote는 노트 하나를 파이프라인 -> 게이트 -> 중복 검사 -> 생성 순서로
// 처리하고 결과("migrated"/"skippedDup"/"skippedGate"/"failed")를 돌려준다.
// 실패는 로그에 남기고 다음 노트로 계속한다 (PRD 6.2 Continue 정책).
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

	// effectiveDate 게이트: 날짜가 파생된 노트만 필터하고, 미상은 통과 (D6).
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

	// 중복 검사 (D2-멱등성): 날짜가 있으면 Date, 없으면 제목 기준.
	// 먼저 실행 내 키 선점으로 워커 간 검사-생성 경합을 막고, 그다음 노션을 조회한다.
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
		// 부분 생성(페이지 생성 후 append 실패)의 페이지 ID와 정리 안내는
		// CreatePage의 에러 메시지에 이미 포함되어 있다.
		env.logger.Warnf("생성 실패 %s: %v", note.RelPath, err)
		return "failed"
	}
	env.logger.Infof("이관 완료 %s -> %s", note.RelPath, pageID)
	return "migrated"
}

// checkDuplicate는 초안과 같은 페이지가 이미 존재하는지 조회한다.
func (env *migrateEnv) checkDuplicate(ctx context.Context, draft *transform.PageDraft) (bool, error) {
	if draft.Date != "" && env.dateProp != "" {
		return env.client.ExistsByDate(ctx, env.dataSourceID, env.dateProp, draft.Date)
	}
	return env.client.ExistsByTitle(ctx, env.dataSourceID, env.titleProp, draft.Title)
}

// buildProperties는 초안을 노션 속성 페이로드로 변환한다.
// 전용 속성(title/date/url)은 초안의 전용 필드에서, 나머지는 매핑 결과에서 채운다.
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
			// init 검증을 통과했다면 없어야 하는 경우의 방어.
			env.logger.Warnf("%s: 노션 속성 %q의 타입을 알 수 없어 속성을 건너뜀", relPath, name)
			continue
		}
		properties[name] = notion.PropertyValue{Type: propType, Value: value}
	}
	return properties
}
