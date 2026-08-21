// Package config는 base.config.yaml / latest.config.yaml의 로드·검증·저장·아카이빙과
// .env 파일 로딩을 담당한다.
package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// VaultTarget은 옵시디언 볼트 내 이관 대상 디렉토리 하나를 가리킨다.
// Exclude는 target 하위 재귀 스캔에서 제외할 파일의 glob 패턴 목록이다
// (gitignore처럼 제외 대상만 지정, 상대경로와 파일명 양쪽에 매칭).
type VaultTarget struct {
	Name          string   `yaml:"name"`
	Path          string   `yaml:"path"`
	Target        string   `yaml:"target"`
	EffectiveDate string   `yaml:"effectiveDate,omitempty"`
	Exclude       []string `yaml:"exclude,omitempty"`
}

// BaseConfig는 base.config.yaml의 구조를 나타낸다.
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

// LoadBase는 base.config.yaml을 읽고 검증한다.
// 검증 내용: 필수 필드가 비어 있지 않아야 하고, toNotion/fromNotion의
// path+target 결합 경로가 서로 달라야 하며(루프 방지 가드), toNotion 소스
// 디렉토리가 실제로 존재해야 한다. fromNotion 백업 디렉토리는 없으면 생성한다.
// effectiveDate는 값이 있으면 2006-01-02 형식이어야 한다.
func LoadBase(path string) (*BaseConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("base 설정 파일 읽기 실패: %w", err)
	}

	var cfg BaseConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("base 설정 파싱 실패: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SourceDir는 toNotion의 path와 target을 결합한 절대경로를 돌려준다.
func (c *BaseConfig) SourceDir() string {
	return filepath.Clean(filepath.Join(c.Obsidian.Vault.ToNotion.Path, c.Obsidian.Vault.ToNotion.Target))
}

// BackupDir는 fromNotion의 path와 target을 결합한 절대경로를 돌려준다.
func (c *BaseConfig) BackupDir() string {
	return filepath.Clean(filepath.Join(c.Obsidian.Vault.FromNotion.Path, c.Obsidian.Vault.FromNotion.Target))
}

// validate는 BaseConfig의 필수 값과 경로 조건을 검사한다.
// 검사 통과 시 fromNotion 백업 디렉토리를 생성하는 부수 효과가 있다.
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

	// macOS 파일시스템은 대소문자를 무시하고 유니코드 정규화(NFC/NFD)에도 무관하므로
	// 문자열이 달라도 같은 물리 디렉토리일 수 있다. 실제 파일 동일성으로 재검사한다
	// (심볼릭 링크를 통한 우회도 함께 걸러진다).
	backupInfo, err := os.Stat(backupDir)
	if err != nil {
		return fmt.Errorf("fromNotion 백업 디렉토리 확인 실패: %w", err)
	}
	if os.SameFile(srcInfo, backupInfo) {
		return fmt.Errorf("toNotion 소스 경로와 fromNotion 백업 경로가 같은 디렉토리를 가리킴(루프 방지): %s", srcDir)
	}
	return nil
}
