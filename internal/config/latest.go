package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Property는 노션 데이터베이스 컬럼(속성) 하나를 나타낸다.
type Property struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

// State는 파이프라인 실행 이력을 나타낸다.
type State struct {
	FirstRunAt       string `yaml:"firstRunAt"`
	LastMigrateRunAt string `yaml:"lastMigrateRunAt"`
	LastBackupRunAt  string `yaml:"lastBackupRunAt"`
}

// LatestConfig는 latest.config.yaml의 구조를 나타낸다.
type LatestConfig struct {
	GeneratedAt string `yaml:"generatedAt"`
	Obsidian    struct {
		FrontmatterKeys []string `yaml:"frontmatterKeys"`
	} `yaml:"obsidian"`
	Notion struct {
		DatabaseID   string     `yaml:"databaseId"`
		DataSourceID string     `yaml:"dataSourceId"`
		Properties   []Property `yaml:"properties"`
	} `yaml:"notion"`
	Mapping  []MappingEntry `yaml:"mapping"`
	DateFrom []DateRule     `yaml:"dateFrom,omitempty"`
	State    State          `yaml:"state"`
}

// LoadLatest는 latest.config.yaml을 읽는다. 파일이 없으면 (nil, nil)을 반환한다.
func LoadLatest(path string) (*LatestConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("latest 설정 파일 읽기 실패: %w", err)
	}

	var cfg LatestConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("latest 설정 파싱 실패: %w", err)
	}
	return &cfg, nil
}

// SaveLatest는 cfg를 path에 저장하고, 아카이빙이 일어났는지를 돌려준다.
// 기존 파일이 있고 State를 제외한 부분이 다르면, 저장 전에 기존 파일을
// backupDir/"2006-01-02-150405.config.yaml"로 복사한다(이름이 겹치면 접미사를
// 붙여 기존 아카이브를 보존한다). now는 아카이브 파일명과 GeneratedAt에
// 사용한다. 저장 시 cfg.GeneratedAt이 now 기준으로 갱신되며, 쓰기는 임시 파일
// 후 rename으로 수행되어 중간 실패에도 기존 파일이 손상되지 않는다.
func SaveLatest(path, backupDir string, cfg *LatestConfig, now time.Time) (archived bool, err error) {
	if cfg == nil {
		return false, errors.New("저장할 latest 설정이 nil")
	}

	prev, err := LoadLatest(path)
	if err != nil {
		return false, err
	}
	if prev != nil {
		changed, err := differsExceptState(prev, cfg)
		if err != nil {
			return false, err
		}
		if changed {
			if err := archiveFile(path, backupDir, now); err != nil {
				return false, err
			}
			archived = true
		}
	}

	cfg.GeneratedAt = now.Format(time.RFC3339)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return archived, fmt.Errorf("latest 설정 마샬링 실패: %w", err)
	}
	if err := writeFileAtomic(path, data); err != nil {
		return archived, fmt.Errorf("latest 설정 저장 실패: %w", err)
	}
	return archived, nil
}

// writeFileAtomic은 같은 디렉토리의 임시 파일에 쓴 뒤 rename으로 path를
// 교체한다. 쓰기 도중 실패해도 기존 파일은 온전히 남는다.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("임시 파일 생성 실패: %w", err)
	}
	tmpPath := tmp.Name()

	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		if writeErr != nil {
			return fmt.Errorf("임시 파일 쓰기 실패: %w", writeErr)
		}
		return fmt.Errorf("임시 파일 닫기 실패: %w", closeErr)
	}

	// CreateTemp는 0600으로 만들므로 기존 관례(0644)에 맞춘다.
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("임시 파일 권한 설정 실패: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("파일 교체 실패: %w", err)
	}
	return nil
}

// differsExceptState는 두 설정이 State(실행 이력)를 제외하고 다른지 판정한다.
// GeneratedAt은 저장 시각의 기록일 뿐이므로 함께 비교에서 제외한다.
// 비교는 State와 GeneratedAt을 제로값으로 바꾼 복사본을 yaml로 마샬링해
// 바이트 단위로 수행한다.
func differsExceptState(a, b *LatestConfig) (bool, error) {
	ca, cb := *a, *b
	ca.State, cb.State = State{}, State{}
	ca.GeneratedAt, cb.GeneratedAt = "", ""

	da, err := yaml.Marshal(&ca)
	if err != nil {
		return false, fmt.Errorf("설정 비교용 마샬링 실패: %w", err)
	}
	db, err := yaml.Marshal(&cb)
	if err != nil {
		return false, fmt.Errorf("설정 비교용 마샬링 실패: %w", err)
	}
	return !bytes.Equal(da, db), nil
}

// archiveFile은 path의 기존 파일을 backupDir 아래에
// "2006-01-02-150405.config.yaml" 이름으로 복사한다. 같은 초에 이미 아카이브가
// 있으면 "-2", "-3" 접미사를 붙여 기존 이력을 덮어쓰지 않는다.
func archiveFile(path, backupDir string, now time.Time) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("아카이브 원본 읽기 실패: %w", err)
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("아카이브 디렉토리 생성 실패: %w", err)
	}

	// 같은 초 안의 재시도가 이 한도를 넘는 것은 비정상 상황이므로 에러로 끊는다.
	const maxSuffix = 1000
	base := now.Format("2006-01-02-150405")
	for i := 1; i <= maxSuffix; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		dst := filepath.Join(backupDir, name+".config.yaml")

		// O_EXCL로 기존 파일 존재 시 실패시켜 덮어쓰기를 원천 차단한다.
		f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return fmt.Errorf("아카이브 파일 생성 실패: %w", err)
		}

		_, writeErr := f.Write(data)
		closeErr := f.Close()
		if writeErr != nil {
			return fmt.Errorf("아카이브 파일 저장 실패: %w", writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("아카이브 파일 닫기 실패: %w", closeErr)
		}
		return nil
	}
	return fmt.Errorf("아카이브 파일명 충돌이 %d회를 초과함: %s", maxSuffix, base)
}
