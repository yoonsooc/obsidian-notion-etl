// Package config loads, validates, saves, and archives base.config.yaml /
// latest.config.yaml, and reads the Notion token from a .env file.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// VaultTarget points at one directory inside an Obsidian vault.
// Exclude lists glob patterns (matched against both relative paths and
// base names) for files to skip during the recursive scan.
type VaultTarget struct {
	Name          string   `yaml:"name"`
	Path          string   `yaml:"path"`
	Target        string   `yaml:"target"`
	EffectiveDate string   `yaml:"effectiveDate,omitempty"`
	Exclude       []string `yaml:"exclude,omitempty"`
}

// BaseConfig models base.config.yaml.
type BaseConfig struct {
	Obsidian struct {
		Vault struct {
			ToNotion   VaultTarget `yaml:"toNotion"`
			FromNotion VaultTarget `yaml:"fromNotion"`
		} `yaml:"vault"`
	} `yaml:"obsidian"`
	Notion struct {
		DB struct {
			URL  string `yaml:"url"`
			Name string `yaml:"name"`
		} `yaml:"db"`
	} `yaml:"notion"`
}

// LoadBase reads and validates base.config.yaml. Required fields must be
// non-empty, the source and backup directories must differ (sync-loop guard),
// and the source directory must exist; the backup directory is created if
// missing. effectiveDate, when set, must use the 2006-01-02 layout.
func LoadBase(path string) (*BaseConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("base 설정 파일 읽기 실패: %w", err)
	}

	cfg, err := decodeBase(data)
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// decodeBase parses base config YAML strictly (KnownFields) so unknown or
// misspelled keys fail instead of being silently dropped. No path validation.
func decodeBase(data []byte) (*BaseConfig, error) {
	var cfg BaseConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("base 설정 파싱 실패: %w", err)
	}
	return &cfg, nil
}

// SourceDir returns the cleaned join of the toNotion path and target.
func (c *BaseConfig) SourceDir() string {
	return filepath.Clean(filepath.Join(c.Obsidian.Vault.ToNotion.Path, c.Obsidian.Vault.ToNotion.Target))
}

// BackupDir returns the cleaned join of the fromNotion path and target.
func (c *BaseConfig) BackupDir() string {
	return filepath.Clean(filepath.Join(c.Obsidian.Vault.FromNotion.Path, c.Obsidian.Vault.FromNotion.Target))
}

// validate checks required fields and path constraints; as a side effect it
// creates the fromNotion backup directory.
func (c *BaseConfig) validate() error {
	required := []struct {
		field string
		value string
	}{
		{field: "obsidian.vault.toNotion.name", value: c.Obsidian.Vault.ToNotion.Name},
		{field: "obsidian.vault.toNotion.path", value: c.Obsidian.Vault.ToNotion.Path},
		{field: "obsidian.vault.toNotion.target", value: c.Obsidian.Vault.ToNotion.Target},
		{field: "obsidian.vault.fromNotion.name", value: c.Obsidian.Vault.FromNotion.Name},
		{field: "obsidian.vault.fromNotion.path", value: c.Obsidian.Vault.FromNotion.Path},
		{field: "obsidian.vault.fromNotion.target", value: c.Obsidian.Vault.FromNotion.Target},
		{field: "notion.db.url", value: c.Notion.DB.URL},
		{field: "notion.db.name", value: c.Notion.DB.Name},
	}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			return fmt.Errorf("필수 설정 %s 값이 비어 있음", r.field)
		}
	}

	targets := []struct {
		field string
		value VaultTarget
	}{
		{field: "obsidian.vault.toNotion", value: c.Obsidian.Vault.ToNotion},
		{field: "obsidian.vault.fromNotion", value: c.Obsidian.Vault.FromNotion},
	}
	for _, t := range targets {
		for _, pattern := range t.value.Exclude {
			if _, err := path.Match(pattern, "x"); err != nil {
				return fmt.Errorf("%s.exclude 패턴 %q이 잘못됨: %w", t.field, pattern, err)
			}
		}
		if t.value.EffectiveDate == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", t.value.EffectiveDate); err != nil {
			return fmt.Errorf("%s.effectiveDate는 2006-01-02 형식이어야 함: %w", t.field, err)
		}
	}

	srcDir := c.SourceDir()
	backupDir := c.BackupDir()
	if srcDir == backupDir {
		return fmt.Errorf("toNotion 소스 경로와 fromNotion 백업 경로가 같음(루프 방지): %s", srcDir)
	}

	srcInfo, err := os.Stat(srcDir)
	if err != nil {
		return fmt.Errorf("toNotion 소스 디렉토리 확인 실패: %w", err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("toNotion 소스 경로가 디렉토리가 아님: %s", srcDir)
	}

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("fromNotion 백업 디렉토리 생성 실패: %w", err)
	}

	// macOS filesystems are case-insensitive and Unicode-normalization
	// (NFC/NFD) insensitive, so differing strings can name the same physical
	// directory. Re-check with os.SameFile, which also catches symlink aliases.
	backupInfo, err := os.Stat(backupDir)
	if err != nil {
		return fmt.Errorf("fromNotion 백업 디렉토리 확인 실패: %w", err)
	}
	if os.SameFile(srcInfo, backupInfo) {
		return fmt.Errorf("toNotion 소스 경로와 fromNotion 백업 경로가 같은 디렉토리를 가리킴(루프 방지): %s", srcDir)
	}
	return nil
}
