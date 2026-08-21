package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	// 종료 지점을 한 곳으로 모은다 (docs/review-checklist.md 3번).
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run은 서브커맨드를 해석해 실행하고 에러를 반환한다.
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
