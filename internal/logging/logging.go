// Package logging은 수동 명령(init/migrate/backup)의 항목 단위 경고를
// 실행별 로그 파일로 남기는 얇은 로거를 제공한다. 치명적 에러는 여기로 오지 않고
// main의 stderr 출력으로 처리된다 (PRD 6.2).
package logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// Logger는 하나의 명령 실행에 대한 로그 파일 기록기다.
// log.Logger가 직렬화를 보장하고 wrote는 원자적이므로 여러 고루틴에서
// 동시에 사용해도 안전하다 (migrate 워커 풀에서 공유).
type Logger struct {
	file  *os.File
	l     *log.Logger
	path  string
	wrote atomic.Bool
}

// New는 logs/<name>/ 아래에 실행 시각 기반 로그 파일을 만들어 Logger를 돌려준다.
// 예: New("migration", now) -> logs/migration/2026-08-21-143000.log
func New(name string, now time.Time) (*Logger, error) {
	dir := filepath.Join("logs", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("로그 디렉토리 생성 실패: %w", err)
	}

	path := filepath.Join(dir, now.Format("2006-01-02-150405")+".log")
	// 같은 초에 재실행해도 이전 로그를 덮지 않도록 append 모드로 연다.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("로그 파일 생성 실패: %w", err)
	}

	return &Logger{
		file: file,
		l:    log.New(file, "", log.LstdFlags),
		path: path,
	}, nil
}

// Warnf는 항목 단위 경고를 로그 파일에 기록한다.
func (lg *Logger) Warnf(format string, args ...any) {
	lg.wrote.Store(true)
	lg.l.Printf("WARN "+format, args...)
}

// Infof는 항목 단위 정보(진행 기록)를 로그 파일에 기록한다.
func (lg *Logger) Infof(format string, args ...any) {
	lg.wrote.Store(true)
	lg.l.Printf("INFO "+format, args...)
}

// Path는 로그 파일 경로를 돌려준다 (실행 요약에서 안내용).
func (lg *Logger) Path() string {
	return lg.path
}

// Wrote는 이 실행에서 로그가 한 건이라도 기록됐는지 돌려준다.
func (lg *Logger) Wrote() bool {
	return lg.wrote.Load()
}

// Close는 로그 파일을 닫는다. 아무것도 기록되지 않았으면 빈 파일이
// 누적되지 않도록 파일을 제거한다(제거 실패는 무시하는 최선 노력).
func (lg *Logger) Close() error {
	if err := lg.file.Close(); err != nil {
		return fmt.Errorf("로그 파일 닫기 실패: %w", err)
	}
	if !lg.wrote.Load() {
		_ = os.Remove(lg.path)
	}
	return nil
}

// Write는 io.Writer를 구현해, 로거를 경고 출력 대상으로 주입할 수 있게 한다.
// 한 번의 Write 호출을 한 줄의 WARN 레코드로 기록한다.
func (lg *Logger) Write(p []byte) (int, error) {
	lg.wrote.Store(true)
	lg.l.Print("WARN " + string(p))
	return len(p), nil
}
