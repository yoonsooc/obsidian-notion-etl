package cli

import (
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// registerKoCatalog registers the Korean translations for user-facing stdout
// messages. Keys are the exact English format strings used in code; a missing
// key falls through to English, so forgetting a translation degrades gracefully.
func registerKoCatalog() {
	ko := language.Korean
	for key, msg := range map[string]string{
		// shared
		"Details: %s\n": "상세 로그: %s\n",

		// init
		"Notion DB %q(%s): data source %q, %d properties verified\n": "노션 DB %q(%s): 데이터 소스 %q, 속성 %d개 확인\n",
		"Obsidian source (%s): collected %d frontmatter keys\n":      "옵시디언 소스(%s): frontmatter 키 %d개 수집\n",
		"Validated %d mapping rules (%d warnings), saved %s\n":       "매핑 규칙 %d건 검증(경고 %d건), %s 저장 완료\n",
		"Config changed; previous version archived to %s\n":          "설정이 변경되어 기존 설정을 %s에 아카이빙함\n",

		// migrate
		"Migration complete%s: %d targets, %d migrated, %d duplicate skips, %d date-gate skips, %d failed\n": "마이그레이션 완료%s: 대상 %d건 중 이관 %d, 중복 스킵 %d, 날짜 게이트 스킵 %d, 실패 %d\n",
		" (dry-run: no pages created)": " (dry-run: 실제 생성 없음)",

		// backup
		"Backup complete%s: %d targets, %d backed up, %d failed (watermark: %s)\n": "백업 완료%s: 대상 %d건 중 백업 %d, 실패 %d (워터마크: %s)\n",
		" (dry-run: nothing written)": " (dry-run: 실제 쓰기 없음)",
		"none (full backup)":          "없음(전체 백업)",
		"since %s":                    "%s 이후",

		// schedule
		"Automatic backup is not registered: no etl-worker backup entry in crontab (register with: backup --schedule)\n": "자동 백업 미등록: crontab에 etl-worker backup 엔트리가 없습니다 (등록: backup --schedule)\n",
		"Automatic backup is registered:\n  %s\n": "자동 백업 등록됨:\n  %s\n",
		"Already registered:\n  %s\nTo change the cron spec, run --unschedule and then --schedule=\"<cron>\"\n": "이미 등록되어 있습니다:\n  %s\n크론식을 바꾸려면 --unschedule 후 --schedule=\"<크론식>\"으로 재등록하세요\n",
		"Registered automatic backup (cron spec %q):\n  %s\n":                                                   "자동 백업을 등록했습니다 (크론식 %q):\n  %s\n",
		"Removed automatic backup (deleted line):\n  %s\n":                                                      "자동 백업을 해제했습니다 (제거된 라인):\n  %s\n",
		"No etl-worker backup entry is registered\n":                                                            "등록된 etl-worker backup 엔트리가 없습니다\n",

		// usage (shown on argument errors)
		usageText: `Usage: etl-worker <command>

Commands:
  init     base.config.yaml과 Notion DB를 검증하고 configs/latest.config.yaml을 생성
  migrate  Obsidian -> Notion 마이그레이션 (수동 1회성, --dry-run 지원)
  backup   Notion -> Obsidian 증분 백업 1회 실행 (수동. --schedule로 자동화)

자세한 사용법: etl-worker help
`,
	} {
		// SetString only fails on malformed keys, which the tests catch.
		_ = message.SetString(ko, key, msg)
	}
}
