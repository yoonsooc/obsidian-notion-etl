// Command etl-worker runs one-way data transfers between a local
// Obsidian vault and a Notion database.
package main

import (
	"errors"
	"fmt"
	"os"
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
		return runInit()
	case "migrate":
		return runMigrate(args[1:])
	case "backup":
		return errors.New("backup: not implemented yet (M3)")
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
  backup   Notion -> Obsidian 백업 (cron 주기 실행 또는 수동)
`)
}
