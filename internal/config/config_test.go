package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// newValidBase builds a temp vault layout and a matching valid BaseConfig.
func newValidBase(t *testing.T) BaseConfig {
	t.Helper()
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, "Daily"), 0o755); err != nil {
		t.Fatalf("소스 디렉토리 생성 실패: %v", err)
	}

	var cfg BaseConfig
	cfg.Obsidian.Vault.ToNotion = VaultTarget{
		Name:          "TestVault",
		Path:          vault,
		Target:        "Daily",
		EffectiveDate: "2025-11-01",
	}
	cfg.Obsidian.Vault.FromNotion = VaultTarget{
		Name:   "TestVault",
		Path:   vault,
		Target: "Daily-NotionBackup",
	}
	cfg.Notion.DB.URL = "https://www.notion.so/abc123?v=def456"
	cfg.Notion.DB.Name = "Platinum"
	return cfg
}

// writeBaseYAML saves cfg as a YAML file and returns its path.
func writeBaseYAML(t *testing.T, cfg BaseConfig) string {
	t.Helper()
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("테스트 설정 마샬링 실패: %v", err)
	}
	path := filepath.Join(t.TempDir(), "base.config.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("테스트 설정 파일 쓰기 실패: %v", err)
	}
	return path
}

// TestLoadBaseSameDirGuard verifies the loop guard catches paths that differ
// as strings but resolve to the same directory (symlink; the os.SameFile check
// also covers macOS case-insensitivity and NFC/NFD normalization).
func TestLoadBaseSameDirGuard(t *testing.T) {
	cfg := newValidBase(t)
	srcDir := filepath.Join(cfg.Obsidian.Vault.ToNotion.Path, cfg.Obsidian.Vault.ToNotion.Target)
	linkDir := filepath.Join(cfg.Obsidian.Vault.FromNotion.Path, "Daily-Link")
	if err := os.Symlink(srcDir, linkDir); err != nil {
		t.Fatalf("심볼릭 링크 생성 실패: %v", err)
	}
	cfg.Obsidian.Vault.FromNotion.Target = "Daily-Link"

	_, err := LoadBase(writeBaseYAML(t, cfg))
	if err == nil {
		t.Fatal("같은 디렉토리를 가리키는 설정이 가드를 통과함")
	}
	if !strings.Contains(err.Error(), "루프 방지") {
		t.Errorf("루프 방지 에러를 기대했지만 %v를 받음", err)
	}
}

func TestLoadBase(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(cfg *BaseConfig)
		wantErr string // substring expected in the error; empty means success
	}{
		{
			name:   "유효한 설정",
			mutate: func(cfg *BaseConfig) {},
		},
		{
			name:    "notion.db.url 누락",
			mutate:  func(cfg *BaseConfig) { cfg.Notion.DB.URL = "" },
			wantErr: "notion.db.url",
		},
		{
			name:    "toNotion.name 공백만",
			mutate:  func(cfg *BaseConfig) { cfg.Obsidian.Vault.ToNotion.Name = "   " },
			wantErr: "toNotion.name",
		},
		{
			name:    "fromNotion.target 누락",
			mutate:  func(cfg *BaseConfig) { cfg.Obsidian.Vault.FromNotion.Target = "" },
			wantErr: "fromNotion.target",
		},
		{
			name: "effectiveDate 형식 오류",
			mutate: func(cfg *BaseConfig) {
				cfg.Obsidian.Vault.ToNotion.EffectiveDate = "2025/11/01"
			},
			wantErr: "effectiveDate",
		},
		{
			name: "소스와 백업 경로 동일",
			mutate: func(cfg *BaseConfig) {
				cfg.Obsidian.Vault.FromNotion.Target = cfg.Obsidian.Vault.ToNotion.Target
			},
			wantErr: "루프 방지",
		},
		{
			name: "소스와 백업 경로가 Clean 후 동일",
			mutate: func(cfg *BaseConfig) {
				cfg.Obsidian.Vault.FromNotion.Target = "./" + cfg.Obsidian.Vault.ToNotion.Target + "/."
			},
			wantErr: "루프 방지",
		},
		{
			name: "백업 디렉토리가 소스 내부",
			mutate: func(cfg *BaseConfig) {
				cfg.Obsidian.Vault.FromNotion.Target = cfg.Obsidian.Vault.ToNotion.Target + "/NotionBackup"
			},
			wantErr: "재이관 루프",
		},
		{
			name: "소스 디렉토리가 백업 내부",
			mutate: func(cfg *BaseConfig) {
				// Backup at the vault root contains the source subdirectory.
				cfg.Obsidian.Vault.FromNotion.Target = "."
			},
			wantErr: "미러 덮어쓰기",
		},
		{
			name: "소스 디렉토리 없음",
			mutate: func(cfg *BaseConfig) {
				cfg.Obsidian.Vault.ToNotion.Target = "no-such-dir"
			},
			wantErr: "소스 디렉토리",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newValidBase(t)
			tt.mutate(&cfg)
			path := writeBaseYAML(t, cfg)

			got, err := LoadBase(path)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("에러를 기대했지만 nil을 받음")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("에러 메시지에 %q가 없음: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadBase 실패: %v", err)
			}
			wantSrc := filepath.Join(cfg.Obsidian.Vault.ToNotion.Path, cfg.Obsidian.Vault.ToNotion.Target)
			if got.SourceDir() != wantSrc {
				t.Errorf("SourceDir() = %q, want %q", got.SourceDir(), wantSrc)
			}
			wantBackup := filepath.Join(cfg.Obsidian.Vault.FromNotion.Path, cfg.Obsidian.Vault.FromNotion.Target)
			if got.BackupDir() != wantBackup {
				t.Errorf("BackupDir() = %q, want %q", got.BackupDir(), wantBackup)
			}
			info, statErr := os.Stat(got.BackupDir())
			if statErr != nil || !info.IsDir() {
				t.Errorf("백업 디렉토리가 생성되지 않음: %v", statErr)
			}
		})
	}
}

func TestLoadBaseFileErrors(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T) string
	}{
		{
			name: "파일 없음",
			prepare: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.yaml")
			},
		},
		{
			name: "yaml 파싱 불가",
			prepare: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "broken.yaml")
				if err := os.WriteFile(path, []byte("obsidian: [broken"), 0o644); err != nil {
					t.Fatalf("파일 쓰기 실패: %v", err)
				}
				return path
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := LoadBase(tt.prepare(t)); err == nil {
				t.Fatal("에러를 기대했지만 nil을 받음")
			}
		})
	}
}

// newLatest builds a LatestConfig fixture.
func newLatest() *LatestConfig {
	cfg := &LatestConfig{
		GeneratedAt: "",
		Mapping: []MappingEntry{
			{Frontmatter: "date", NotionProperty: "Date"},
		},
		State: State{
			FirstRunAt: "2026-01-01T00:00:00Z",
		},
	}
	cfg.Obsidian.FrontmatterKeys = []string{"date", "tags"}
	cfg.Notion.DatabaseID = "b8842240012340b9bff4a51f2a0fd668"
	cfg.Notion.Properties = []Property{
		{Name: "Date", Type: "date"},
	}
	return cfg
}

// countBackups counts files in backupDir.
func countBackups(t *testing.T, backupDir string) int {
	t.Helper()
	entries, err := os.ReadDir(backupDir)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("백업 디렉토리 읽기 실패: %v", err)
	}
	return len(entries)
}

func TestSaveLatest(t *testing.T) {
	now := time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC)
	later := now.Add(time.Hour)

	tests := []struct {
		name        string
		second      func(cfg *LatestConfig) // mutation before a second save; nil saves once
		wantBackups int
	}{
		{
			name:        "최초 저장은 백업 없음",
			second:      nil,
			wantBackups: 0,
		},
		{
			name:        "동일 내용 재저장은 백업 없음",
			second:      func(cfg *LatestConfig) {},
			wantBackups: 0,
		},
		{
			name: "State만 변경되면 백업 없음",
			second: func(cfg *LatestConfig) {
				cfg.State.LastMigrateRunAt = "2026-08-21T11:00:00Z"
			},
			wantBackups: 0,
		},
		{
			name: "매핑 변경 시 백업 생성",
			second: func(cfg *LatestConfig) {
				cfg.Mapping = append(cfg.Mapping, MappingEntry{Frontmatter: "tags", NotionProperty: "Tags"})
			},
			wantBackups: 1,
		},
		{
			name: "노션 속성 변경 시 백업 생성",
			second: func(cfg *LatestConfig) {
				cfg.Notion.Properties[0].Type = "rich_text"
			},
			wantBackups: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "latest.config.yaml")
			backupDir := filepath.Join(dir, "backups")

			first := newLatest()
			archived, err := SaveLatest(path, backupDir, first, now)
			if err != nil {
				t.Fatalf("최초 SaveLatest 실패: %v", err)
			}
			if archived {
				t.Error("최초 저장에서 archived = true")
			}
			if first.GeneratedAt != now.Format(time.RFC3339) {
				t.Errorf("GeneratedAt = %q, want %q", first.GeneratedAt, now.Format(time.RFC3339))
			}

			if tt.second != nil {
				updated := newLatest()
				tt.second(updated)
				archived, err := SaveLatest(path, backupDir, updated, later)
				if err != nil {
					t.Fatalf("두 번째 SaveLatest 실패: %v", err)
				}
				if wantArchived := tt.wantBackups > 0; archived != wantArchived {
					t.Errorf("archived = %v, want %v", archived, wantArchived)
				}
			}

			if got := countBackups(t, backupDir); got != tt.wantBackups {
				t.Errorf("백업 파일 개수 = %d, want %d", got, tt.wantBackups)
			}
			if tt.wantBackups == 1 {
				wantName := later.Format("2006-01-02-150405") + ".config.yaml"
				if _, err := os.Stat(filepath.Join(backupDir, wantName)); err != nil {
					t.Errorf("아카이브 파일 %q이 없음: %v", wantName, err)
				}
			}
		})
	}
}

// TestSaveLatestArchiveSameSecond verifies that two archives within the same
// second are preserved via name suffixes instead of overwriting.
func TestSaveLatestArchiveSameSecond(t *testing.T) {
	now := time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "latest.config.yaml")
	backupDir := filepath.Join(dir, "backups")

	if _, err := SaveLatest(path, backupDir, newLatest(), now); err != nil {
		t.Fatalf("최초 SaveLatest 실패: %v", err)
	}

	// Trigger two archives at the same timestamp with distinct changes.
	for i, id := range []string{"changed-1", "changed-2"} {
		updated := newLatest()
		updated.Notion.DatabaseID = id
		archived, err := SaveLatest(path, backupDir, updated, now)
		if err != nil {
			t.Fatalf("%d번째 변경 저장 실패: %v", i+2, err)
		}
		if !archived {
			t.Fatalf("%d번째 변경 저장에서 archived = false", i+2)
		}
	}

	base := now.Format("2006-01-02-150405")
	for _, name := range []string{base + ".config.yaml", base + "-2.config.yaml"} {
		if _, err := os.Stat(filepath.Join(backupDir, name)); err != nil {
			t.Errorf("아카이브 %q이 없음: %v", name, err)
		}
	}
	if got := countBackups(t, backupDir); got != 2 {
		t.Errorf("백업 파일 개수 = %d, want 2", got)
	}
}

func TestSaveLatestArchiveKeepsOldContent(t *testing.T) {
	now := time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "latest.config.yaml")
	backupDir := filepath.Join(dir, "backups")

	if _, err := SaveLatest(path, backupDir, newLatest(), now); err != nil {
		t.Fatalf("최초 SaveLatest 실패: %v", err)
	}
	firstData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("최초 저장본 읽기 실패: %v", err)
	}

	updated := newLatest()
	updated.Notion.DatabaseID = "changed-db-id"
	later := now.Add(time.Hour)
	if _, err := SaveLatest(path, backupDir, updated, later); err != nil {
		t.Fatalf("두 번째 SaveLatest 실패: %v", err)
	}

	archived, err := os.ReadFile(filepath.Join(backupDir, later.Format("2006-01-02-150405")+".config.yaml"))
	if err != nil {
		t.Fatalf("아카이브 읽기 실패: %v", err)
	}
	if string(archived) != string(firstData) {
		t.Error("아카이브 내용이 이전 저장본과 다름")
	}
}

func TestLoadLatest(t *testing.T) {
	t.Run("파일 없으면 nil nil", func(t *testing.T) {
		got, err := LoadLatest(filepath.Join(t.TempDir(), "missing.yaml"))
		if err != nil {
			t.Fatalf("에러 없음을 기대: %v", err)
		}
		if got != nil {
			t.Fatalf("nil을 기대했지만 %+v를 받음", got)
		}
	})

	t.Run("저장 후 로드 왕복", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "latest.config.yaml")
		now := time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC)
		if _, err := SaveLatest(path, filepath.Join(dir, "backups"), newLatest(), now); err != nil {
			t.Fatalf("SaveLatest 실패: %v", err)
		}

		got, err := LoadLatest(path)
		if err != nil {
			t.Fatalf("LoadLatest 실패: %v", err)
		}
		if got == nil {
			t.Fatal("nil이 아닌 설정을 기대")
		}
		if got.Notion.DatabaseID != "b8842240012340b9bff4a51f2a0fd668" {
			t.Errorf("DatabaseID = %q", got.Notion.DatabaseID)
		}
		if got.GeneratedAt != now.Format(time.RFC3339) {
			t.Errorf("GeneratedAt = %q, want %q", got.GeneratedAt, now.Format(time.RFC3339))
		}
	})

	t.Run("yaml 파싱 불가", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "broken.yaml")
		if err := os.WriteFile(path, []byte("state: [broken"), 0o644); err != nil {
			t.Fatalf("파일 쓰기 실패: %v", err)
		}
		if _, err := LoadLatest(path); err == nil {
			t.Fatal("에러를 기대했지만 nil을 받음")
		}
	})
}

func TestLoadNotionToken(t *testing.T) {
	tests := []struct {
		name    string
		content string
		noFile  bool
		want    string
		wantErr bool
	}{
		{
			name:    "기본 형식",
			content: "NOTION_TOKEN=secret_abc123\n",
			want:    "secret_abc123",
		},
		{
			name:    "주석과 빈 줄 무시",
			content: "# comment\n\nOTHER_KEY=x\nNOTION_TOKEN=secret_abc123\n",
			want:    "secret_abc123",
		},
		{
			name:    "키와 값 주변 공백 제거",
			content: "NOTION_TOKEN =  secret_abc123  \n",
			want:    "secret_abc123",
		},
		{
			name:    "큰따옴표 값",
			content: "NOTION_TOKEN=\"secret_abc123\"\n",
			want:    "secret_abc123",
		},
		{
			name:    "작은따옴표 값",
			content: "NOTION_TOKEN='secret_abc123'\n",
			want:    "secret_abc123",
		},
		{
			name:    "값에 등호 포함",
			content: "NOTION_TOKEN=abc=def\n",
			want:    "abc=def",
		},
		{
			name:    "CRLF 줄바꿈",
			content: "NOTION_TOKEN=secret_abc123\r\nOTHER=x\r\n",
			want:    "secret_abc123",
		},
		{
			name:    "키 없음",
			content: "OTHER_KEY=x\n",
			wantErr: true,
		},
		{
			name:    "값 비어 있음",
			content: "NOTION_TOKEN=\n",
			wantErr: true,
		},
		{
			name:    "주석 처리된 키는 무시",
			content: "# NOTION_TOKEN=secret_abc123\n",
			wantErr: true,
		},
		{
			name:    "파일 없음",
			noFile:  true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if !tt.noFile {
				if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
					t.Fatalf(".env 쓰기 실패: %v", err)
				}
			}

			got, err := LoadNotionToken(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("에러를 기대했지만 값 %q를 받음", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadNotionToken 실패: %v", err)
			}
			if got != tt.want {
				t.Errorf("토큰 = %q, want %q", got, tt.want)
			}
		})
	}
}
