package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// scheduleMarker identifies this tool's backup entry in a crontab. Quotes are
// stripped before matching so both the quoted line this tool writes and a
// legacy hand-written line (cd ~/etl-worker && ./etl-worker backup ...) match.
const scheduleMarker = "etl-worker backup"

// defaultCronSpec is the schedule used when --schedule carries no value: on
// the hour, every hour.
const defaultCronSpec = "0 * * * *"

// runSchedule handles the backup schedule management flags: register the
// cron entry (spec is the 5-field time expression), remove it, or report its
// state. The command itself stays a one-shot manual run; scheduling is
// delegated to the user's crontab (D17 — no resident daemon mode, consistent
// with FR-3 and D16).
func runSchedule(mode, spec string) error {
	current, err := readCrontab()
	if err != nil {
		return err
	}
	line, registered := hasBackupEntry(current)

	switch mode {
	case "status":
		if !registered {
			fmt.Println("자동 백업 미등록: crontab에 etl-worker backup 엔트리가 없습니다 (등록: backup --schedule)")
			return nil
		}
		fmt.Printf("자동 백업 등록됨:\n  %s\n", line)
		return nil

	case "schedule":
		if err := validateCronSpec(spec); err != nil {
			return fmt.Errorf("backup --schedule: %w", err)
		}
		if registered {
			fmt.Printf("이미 등록되어 있습니다:\n  %s\n크론식을 바꾸려면 --unschedule 후 --schedule=\"<크론식>\"으로 재등록하세요\n", line)
			return nil
		}
		workDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("작업 디렉토리 확인 실패: %w", err)
		}
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("실행 파일 경로 확인 실패: %w", err)
		}
		entry := cronLine(spec, workDir, exePath)
		if err := writeCrontab(addBackupEntry(current, entry)); err != nil {
			return err
		}
		fmt.Printf("자동 백업을 등록했습니다 (크론식 %q):\n  %s\n", spec, entry)
		return nil

	case "unschedule":
		if !registered {
			fmt.Println("등록된 etl-worker backup 엔트리가 없습니다")
			return nil
		}
		next, _ := removeBackupEntry(current)
		if err := writeCrontab(next); err != nil {
			return err
		}
		fmt.Printf("자동 백업을 해제했습니다 (제거된 라인):\n  %s\n", line)
		return nil
	}
	return fmt.Errorf("알 수 없는 스케줄 모드: %s", mode)
}

// cronLine builds the crontab entry from a validated time spec. Both paths
// are absolute and quoted: cd makes the relative config/log paths resolve,
// and the executable path survives PATH-less cron environments.
func cronLine(spec, workDir, exePath string) string {
	return fmt.Sprintf(`%s cd "%s" && "%s" backup >> logs/cron.log 2>&1`, spec, workDir, exePath)
}

// validateCronSpec checks a 5-field crontab time spec (minute, hour, day of
// month, month, day of week). Only the field count and character set are
// checked; range semantics are cron's own concern.
func validateCronSpec(spec string) error {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return fmt.Errorf("크론식은 5개 필드(분 시 일 월 요일)여야 합니다: %q (예: \"0 * * * *\")", spec)
	}
	for _, field := range fields {
		for _, r := range field {
			if !strings.ContainsRune("0123456789*/,-", r) {
				return fmt.Errorf("크론식 필드 %q에 허용되지 않는 문자 %q", field, r)
			}
		}
	}
	return nil
}

// hasBackupEntry returns the first non-comment crontab line containing the
// backup entry marker.
func hasBackupEntry(crontab string) (line string, ok bool) {
	for _, l := range strings.Split(crontab, "\n") {
		if isBackupEntry(l) {
			return strings.TrimSpace(l), true
		}
	}
	return "", false
}

// isBackupEntry reports whether a single crontab line is this tool's entry.
func isBackupEntry(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	return strings.Contains(strings.ReplaceAll(trimmed, `"`, ""), scheduleMarker)
}

// addBackupEntry appends the entry, preserving existing lines and ending with
// a single trailing newline (crontab requires the last line be terminated).
func addBackupEntry(crontab, entry string) string {
	existing := strings.TrimRight(crontab, "\n")
	if existing == "" {
		return entry + "\n"
	}
	return existing + "\n" + entry + "\n"
}

// removeBackupEntry drops this tool's entry lines, keeping everything else
// as-is. An all-empty result becomes "" (installs an empty crontab).
func removeBackupEntry(crontab string) (out string, removed bool) {
	var kept []string
	for _, l := range strings.Split(strings.TrimRight(crontab, "\n"), "\n") {
		if isBackupEntry(l) {
			removed = true
			continue
		}
		kept = append(kept, l)
	}
	out = strings.Join(kept, "\n")
	if strings.TrimSpace(out) == "" {
		return "", removed
	}
	return out + "\n", removed
}

// readCrontab returns the user's crontab; a missing crontab reads as empty.
func readCrontab() (string, error) {
	output, err := exec.Command("crontab", "-l").CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "no crontab") {
			return "", nil
		}
		return "", fmt.Errorf("crontab 읽기 실패: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

// writeCrontab replaces the user's crontab. macOS TCC silently blocks crontab
// writes from non-terminal contexts, hence the hint.
func writeCrontab(content string) error {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("crontab 쓰기 실패 (macOS에서는 터미널에서 실행해야 할 수 있음): %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
