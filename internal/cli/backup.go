package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/text/message"

	"github.com/yoonsooc/obsidian-notion-etl/internal/config"
	"github.com/yoonsooc/obsidian-notion-etl/internal/logging"
	"github.com/yoonsooc/obsidian-notion-etl/internal/markdown"
	"github.com/yoonsooc/obsidian-notion-etl/internal/notion"
	"github.com/yoonsooc/obsidian-notion-etl/internal/vault"
)

// RunBackup incrementally mirrors Notion pages into the backup directory.
// Pages edited on or after the watermark (state.lastBackupRunAt) are fetched
// and unconditionally overwritten as markdown notes; the watermark advances to
// this run's start time only when every page succeeded, so a partial failure
// retries on the next run (overwrites make reprocessing safe).
func RunBackup(args []string) (err error) {
	dryRun := false
	scheduleMode := ""
	cronSpec := defaultCronSpec
	setMode := func(mode string) error {
		if scheduleMode != "" {
			return fmt.Errorf("backup: --%s and --%s cannot be combined", scheduleMode, mode)
		}
		scheduleMode = mode
		return nil
	}
	for _, a := range args {
		switch {
		case a == "--dry-run":
			dryRun = true
		case a == "--schedule":
			if err := setMode("schedule"); err != nil {
				return err
			}
		case strings.HasPrefix(a, "--schedule="):
			if err := setMode("schedule"); err != nil {
				return err
			}
			cronSpec = strings.TrimPrefix(a, "--schedule=")
		case a == "--unschedule":
			if err := setMode("unschedule"); err != nil {
				return err
			}
		case a == "--status":
			if err := setMode("status"); err != nil {
				return err
			}
		default:
			return fmt.Errorf("backup: unknown argument: %s (see etl-worker help)", a)
		}
	}
	if scheduleMode != "" {
		if dryRun {
			return fmt.Errorf("backup: --dry-run cannot be combined with --%s", scheduleMode)
		}
		return runSchedule(scheduleMode, cronSpec)
	}

	logger, err := logging.New("backup", time.Now())
	if err != nil {
		return err
	}
	p := NewPrinter("")
	summaryPrinted := false
	defer func() {
		if err != nil && logger.Wrote() && !summaryPrinted {
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
	latest, err := config.LoadLatest(latestConfigPath)
	if err != nil {
		return err
	}
	if latest == nil || latest.Notion.DataSourceID == "" {
		return errors.New("no validated config: run 'etl-worker init' first")
	}
	p = NewPrinter(base.Lang)

	titleProp, dateProp := "", ""
	for _, p := range latest.Notion.Properties {
		switch p.Type {
		case "title":
			titleProp = p.Name
		case "date":
			if dateProp == "" {
				dateProp = p.Name
			}
		}
	}

	ctx := context.Background()
	client := notion.NewClient(token)
	backupDir := base.BackupDir()
	watermark := latest.State.LastBackupRunAt
	runStart := time.Now()

	pages, err := client.QueryPagesSince(ctx, latest.Notion.DataSourceID, watermark, titleProp, dateProp)
	if err != nil {
		return fmt.Errorf("incremental query: %w", err)
	}

	written, failed := 0, 0
	usedNames := make(map[string]int, len(pages))
	for _, page := range pages {
		fileName := backupFileName(page, usedNames)
		draft, buildErr := buildNoteDraft(ctx, client, page, fileName, logger)
		if buildErr != nil {
			logger.Warnf("backup: %s (%s) failed: %v", fileName, page.ID, buildErr)
			failed++
			continue
		}
		if dryRun {
			logger.Infof("backup (dry-run): %s <- page %s (last_edited %s)", fileName, page.ID, page.LastEditedTime)
			written++
			continue
		}
		if writeErr := vault.WriteNote(backupDir, *draft); writeErr != nil {
			logger.Warnf("backup: %s (%s) failed: %v", fileName, page.ID, writeErr)
			failed++
			continue
		}
		logger.Infof("backup: %s <- page %s", fileName, page.ID)
		written++
	}

	// The watermark advances only on a fully clean, real run (PRD FR-3.6).
	if !dryRun && failed == 0 {
		latest.State.LastBackupRunAt = runStart.Format(time.RFC3339)
		if _, err := config.SaveLatest(latestConfigPath, configBackupDir, latest, runStart); err != nil {
			return err
		}
	}

	mode := ""
	if dryRun {
		mode = p.Sprintf(" (dry-run: nothing written)")
	}
	p.Printf("Backup complete%s: %d targets, %d backed up, %d failed (watermark: %s)\n",
		mode, len(pages), written, failed, watermarkLabel(p, watermark))
	if logger.Wrote() {
		p.Printf("Details: %s\n", logger.Path())
		summaryPrinted = true
	}
	if failed > 0 {
		return fmt.Errorf("%d items failed; the watermark was not advanced, so the next run retries (see the log)", failed)
	}
	return nil
}

// buildNoteDraft fetches a page's blocks and assembles the markdown note draft.
func buildNoteDraft(ctx context.Context, client *notion.Client, page notion.Page, fileName string, logger *logging.Logger) (*vault.NoteDraft, error) {
	blocks, err := client.ListBlockChildren(ctx, page.ID)
	if err != nil {
		return nil, fmt.Errorf("collect blocks: %w", err)
	}
	body, warnings := markdown.FromBlocks(blocks)
	for _, w := range warnings {
		logger.Warnf("backup: %s(%s): %s", fileName, page.ID, w)
	}
	return &vault.NoteDraft{
		FileName: fileName,
		Frontmatter: []vault.FrontmatterField{
			{Key: "notion_id", Value: page.ID},
			{Key: "notion_last_edited", Value: page.LastEditedTime},
			{Key: "source", Value: "notion"},
		},
		Body: body,
	}, nil
}

// backupFileName derives a note file name from the sanitized page title: the
// title is the note's identity in both directions (migrate keeps the filename
// stem as the title, D14 keys duplicates on it), so title naming restores the
// original Obsidian names and keeps same-date notes (daily vs weekly start)
// from colliding. Fallbacks: the Date property as YYYY-MM-DD, then the page
// ID. In-run collisions get -2, -3, ... suffixes (PRD 9.2); collisions with
// earlier runs are plain overwrites by design.
func backupFileName(page notion.Page, used map[string]int) string {
	name := vault.SanitizeFileName(page.Title)
	if name == "" && len(page.Date) >= 10 {
		if _, err := time.Parse("2006-01-02", page.Date[:10]); err == nil {
			name = page.Date[:10]
		}
	}
	if name == "" {
		name = page.ID
	}

	used[name]++
	if n := used[name]; n > 1 {
		return fmt.Sprintf("%s-%d.md", name, n)
	}
	return name + ".md"
}

// watermarkLabel renders the previous watermark for the summary line.
func watermarkLabel(p *message.Printer, watermark string) string {
	if watermark == "" {
		return p.Sprintf("none (full backup)")
	}
	return p.Sprintf("since %s", watermark)
}
