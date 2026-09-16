package cli

import (
	"strings"
	"testing"
)

const (
	legacyEntry = `0 * * * * cd /Users/u/etl-worker && ./etl-worker backup >> logs/cron.log 2>&1`
	quotedEntry = `0 * * * * cd "/Users/u/etl-worker" && "/Users/u/etl-worker/etl-worker" backup >> logs/cron.log 2>&1`
	otherEntry  = `30 2 * * * /usr/local/bin/other-job run`
)

func TestHasBackupEntry(t *testing.T) {
	tests := []struct {
		name     string
		crontab  string
		wantLine string
		wantOK   bool
	}{
		{name: "빈 crontab", crontab: "", wantOK: false},
		{name: "다른 엔트리만", crontab: otherEntry + "\n", wantOK: false},
		{name: "수기 등록 레거시 라인", crontab: legacyEntry + "\n", wantLine: legacyEntry, wantOK: true},
		{name: "이 도구가 쓰는 인용 부호 라인", crontab: quotedEntry + "\n", wantLine: quotedEntry, wantOK: true},
		{name: "주석 처리된 라인은 무시", crontab: "# " + legacyEntry + "\n", wantOK: false},
		{name: "다른 엔트리와 공존", crontab: otherEntry + "\n" + quotedEntry + "\n", wantLine: quotedEntry, wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, ok := hasBackupEntry(tt.crontab)
			if ok != tt.wantOK || line != tt.wantLine {
				t.Errorf("hasBackupEntry() = (%q, %v), want (%q, %v)", line, ok, tt.wantLine, tt.wantOK)
			}
		})
	}
}

func TestAddBackupEntry(t *testing.T) {
	entry := cronLine(defaultCronSpec, "/Users/u/etl-worker", "/Users/u/etl-worker/etl-worker")

	if got := addBackupEntry("", entry); got != entry+"\n" {
		t.Errorf("빈 crontab 추가 = %q", got)
	}
	// Existing content without a trailing newline still yields one entry per line.
	got := addBackupEntry(otherEntry, entry)
	want := otherEntry + "\n" + entry + "\n"
	if got != want {
		t.Errorf("기존 엔트리 뒤 추가 = %q, want %q", got, want)
	}
	// The generated line must be detected by our own matcher (self round trip).
	if _, ok := hasBackupEntry(got); !ok {
		t.Error("cronLine 결과를 hasBackupEntry가 감지하지 못함")
	}
}

func TestRemoveBackupEntry(t *testing.T) {
	tests := []struct {
		name        string
		crontab     string
		want        string
		wantRemoved bool
	}{
		{name: "다른 엔트리 보존", crontab: otherEntry + "\n" + quotedEntry + "\n", want: otherEntry + "\n", wantRemoved: true},
		{name: "우리 엔트리만 있으면 빈 crontab", crontab: legacyEntry + "\n", want: "", wantRemoved: true},
		{name: "엔트리 없음", crontab: otherEntry + "\n", want: otherEntry + "\n", wantRemoved: false},
		{name: "레거시·신규 중복도 모두 제거", crontab: legacyEntry + "\n" + otherEntry + "\n" + quotedEntry + "\n", want: otherEntry + "\n", wantRemoved: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, removed := removeBackupEntry(tt.crontab)
			if got != tt.want || removed != tt.wantRemoved {
				t.Errorf("removeBackupEntry() = (%q, %v), want (%q, %v)", got, removed, tt.want, tt.wantRemoved)
			}
		})
	}
}

func TestCronLine(t *testing.T) {
	line := cronLine("30 2 * * *", "/ops/공백 있는 경로", "/ops/etl-worker")
	if !strings.HasPrefix(line, "30 2 * * * ") {
		t.Errorf("지정한 크론식으로 시작하지 않음: %q", line)
	}
	for _, part := range []string{`cd "/ops/공백 있는 경로"`, `"/ops/etl-worker" backup`, ">> logs/cron.log 2>&1"} {
		if !strings.Contains(line, part) {
			t.Errorf("cronLine에 %q 누락: %q", part, line)
		}
	}
}

func TestValidateCronSpec(t *testing.T) {
	valid := []string{defaultCronSpec, "30 2 * * *", "*/15 * * * *", "0 9-18 * * 1-5", "0 0 1,15 * *"}
	for _, spec := range valid {
		if err := validateCronSpec(spec); err != nil {
			t.Errorf("validateCronSpec(%q) = %v, want nil", spec, err)
		}
	}
	invalid := []string{"", "0 * * *", "0 * * * * *", "0 * * * mon", "매시 * * * *", "0;rm -rf * * * *"}
	for _, spec := range invalid {
		if err := validateCronSpec(spec); err == nil {
			t.Errorf("validateCronSpec(%q) = nil, want error", spec)
		}
	}
}
