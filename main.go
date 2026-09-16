// Command etl-worker runs one-way data transfers between a local
// Obsidian vault and a Notion database.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/yoonsooc/obsidian-notion-etl/internal/cli"

	// The only reference to user-defined plugins: importing the package runs
	// each plugin's init, which registers it into the pipeline registry.
	// Common code (internal/cli) reaches plugins solely via pipeline.Select.
	_ "github.com/yoonsooc/obsidian-notion-etl/plugin"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run dispatches the subcommand and returns its error.
func run(args []string) error {
	if len(args) < 1 {
		printUsage()
		return errors.New("명령이 필요함")
	}

	switch args[0] {
	case "init":
		return cli.RunInit()
	case "migrate":
		return cli.RunMigrate(args[1:])
	case "backup":
		return cli.RunBackup(args[1:])
	case "help", "-h", "--help":
		return runHelp(args[1:])
	default:
		printUsage()
		return fmt.Errorf("알 수 없는 명령: %s", args[0])
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `Usage: etl-worker <command>

Commands:
  init     base.config.yaml과 Notion DB를 검증하고 configs/latest.config.yaml을 생성
  migrate  Obsidian -> Notion 마이그레이션 (수동 1회성, --dry-run 지원)
  backup   Notion -> Obsidian 증분 백업 1회 실행 (수동. --schedule로 매시 자동화)

자세한 사용법: etl-worker help
`)
}

// runHelp prints the help text; English by default, Korean with --lang=ko.
func runHelp(args []string) error {
	lang := "en"
	for _, a := range args {
		value, found := strings.CutPrefix(a, "--lang=")
		if !found {
			return fmt.Errorf("help: unknown argument: %s (supported: --lang=en|ko)", a)
		}
		switch value {
		case "en", "ko":
			lang = value
		default:
			return fmt.Errorf("help: unsupported language: %s (supported: en, ko)", value)
		}
	}
	if lang == "ko" {
		fmt.Print(helpKO)
		return nil
	}
	fmt.Print(helpEN)
	return nil
}

const helpEN = `obsidian-notion-etl — one-way ETL CLI between an Obsidian vault and a Notion database

Usage:
  etl-worker <command> [flags]

Commands:
  init      Validate base.config.yaml and the Notion DB schema, then write the configs/latest.config.yaml snapshot
  migrate   One-shot Obsidian -> Notion migration (idempotent by title; existing pages are never modified)
  backup    Run one incremental Notion -> Obsidian backup (manual)
  help      Print this help (-h and --help work too)

Flags:
  migrate --dry-run       Log what would be migrated without creating pages
  backup  --dry-run       Log what would be backed up without writing files
  backup  --schedule[=<cron>]  Register automatic backup in your crontab (duplicate-safe).
                               <cron> is a 5-field crontab expression "min hour day month weekday";
                               default "0 * * * *" (hourly). e.g. --schedule="30 2 * * *"
  backup  --unschedule         Remove the automatic backup entry (other crontab lines are kept)
  backup  --status             Show whether automatic backup is registered, and the registered line
  help    --lang=<en|ko>       Help language (default: en)

Typical flow:
  ./etl-worker init                # validate config and schema
  ./etl-worker migrate --dry-run   # preview what would migrate
  ./etl-worker migrate             # migrate for real
  ./etl-worker backup --schedule   # register the hourly automatic backup

Notes:
  - Config and log paths resolve against the working directory; run real (non dry-run) commands from the operational home (e.g. ~/etl-worker).
  - Automatic-backup run summaries accumulate in logs/cron.log; per-item details go to logs/<command>/<timestamp>.log.
  - To change the schedule, run --unschedule then --schedule="<cron>" again (or edit the line with crontab -e).
`

const helpKO = `obsidian-notion-etl — 옵시디언 볼트와 노션 DB 사이의 단방향 ETL CLI

Usage:
  etl-worker <command> [flags]

Commands:
  init      base.config.yaml과 노션 DB 스키마를 검증하고 configs/latest.config.yaml 스냅샷을 생성
  migrate   Obsidian -> Notion 일괄 이관 (수동 1회성. 제목 기준 멱등, 기존 페이지는 수정하지 않음)
  backup    Notion -> Obsidian 증분 백업 1회 실행 (수동)
  help      이 도움말 출력 (-h, --help 동일)

Flags:
  migrate --dry-run       실제 생성 없이 이관 대상과 변환 결과를 로그로만 확인
  backup  --dry-run       실제 쓰기 없이 백업 대상만 로그로 확인
  backup  --schedule[=<크론식>]  자동 백업을 사용자 crontab에 등록 (중복 등록 방지).
                                 <크론식>은 "분 시 일 월 요일" 5필드 crontab 표현식이며
                                 생략 시 "0 * * * *"(매시 정각). 예: --schedule="30 2 * * *"
  backup  --unschedule           등록된 자동 백업 엔트리를 제거 (다른 crontab 항목은 보존)
  backup  --status               자동 백업 등록 여부와 등록된 라인 확인
  help    --lang=<en|ko>         도움말 언어 (기본: en)

Typical flow:
  ./etl-worker init                # 설정·스키마 검증과 스냅샷
  ./etl-worker migrate --dry-run   # 이관 대상 확인
  ./etl-worker migrate             # 실제 이관
  ./etl-worker backup --schedule   # 매시 자동 백업 등록

Notes:
  - 설정·로그 경로가 실행 디렉토리 기준이므로, 실(非 dry-run) 실행은 운영 홈(예: ~/etl-worker)에서 하세요.
  - 자동 백업의 실행 요약은 logs/cron.log에 누적되고, 항목 단위 상세는 logs/<command>/<실행시각>.log에 남습니다.
  - 자동 백업 주기를 바꾸려면 --unschedule 후 --schedule="<크론식>"으로 재등록하세요 (crontab -e 직접 수정도 가능).
`
