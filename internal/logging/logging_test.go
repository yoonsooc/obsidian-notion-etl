package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// chdir changes the working directory for the test (Logger uses relative logs/).
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("현재 디렉토리 확인 실패: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("디렉토리 이동 실패: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("디렉토리 복원 실패: %v", err)
		}
	})
}

func TestLoggerRemovesEmptyFileOnClose(t *testing.T) {
	chdir(t, t.TempDir())
	now := time.Date(2026, 8, 21, 15, 0, 0, 0, time.UTC)

	lg, err := New("migration", now)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	path := lg.Path()
	if err := lg.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("기록 없는 로그 파일 %s가 제거되지 않음 (err=%v)", path, err)
	}
}

func TestLoggerKeepsFileWithRecords(t *testing.T) {
	chdir(t, t.TempDir())
	now := time.Date(2026, 8, 21, 15, 0, 0, 0, time.UTC)

	lg, err := New("migration", now)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	lg.Warnf("테스트 경고 %d", 1)
	if !lg.Wrote() {
		t.Error("Warnf 후 Wrote() = false")
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	data, err := os.ReadFile(lg.Path())
	if err != nil {
		t.Fatalf("로그 파일 읽기 실패: %v", err)
	}
	if !strings.Contains(string(data), "WARN 테스트 경고 1") {
		t.Errorf("로그 내용에 경고가 없음: %q", string(data))
	}
	if want := filepath.Join("logs", "migration", "2026-08-21-150000.log"); lg.Path() != want {
		t.Errorf("Path() = %q, want %q", lg.Path(), want)
	}
}

func TestLoggerWriteMarksWrote(t *testing.T) {
	chdir(t, t.TempDir())
	lg, err := New("migration", time.Date(2026, 8, 21, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := lg.Write([]byte("io.Writer 경유 경고\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if !lg.Wrote() {
		t.Error("Write 후 Wrote() = false")
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Stat(lg.Path()); err != nil {
		t.Errorf("기록 있는 로그 파일이 제거됨: %v", err)
	}
}
