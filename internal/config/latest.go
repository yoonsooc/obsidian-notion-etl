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

// Property is one Notion database property (column).
type Property struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

// State records pipeline run history.
type State struct {
	FirstRunAt       string `yaml:"firstRunAt"`
	LastMigrateRunAt string `yaml:"lastMigrateRunAt"`
	LastBackupRunAt  string `yaml:"lastBackupRunAt"`
}

// LatestConfig models latest.config.yaml.
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
	Plugin   string         `yaml:"plugin,omitempty"` // transform plugin that produced this snapshot
	Mapping  []MappingEntry `yaml:"mapping"`
	DateFrom []DateRule     `yaml:"dateFrom,omitempty"`
	State    State          `yaml:"state"`
}

// LoadLatest reads latest.config.yaml; a missing file returns (nil, nil).
func LoadLatest(path string) (*LatestConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read latest config: %w", err)
	}

	var cfg LatestConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse latest config: %w", err)
	}
	return &cfg, nil
}

// SaveLatest writes cfg to path and reports whether the previous file was
// archived. If an existing file differs outside State, it is first copied to
// backupDir as "2006-01-02-150405.config.yaml" (suffixing on name collision).
// now feeds both the archive name and cfg.GeneratedAt. The write is atomic
// (temp file + rename), so a mid-write failure leaves the old file intact.
func SaveLatest(path, backupDir string, cfg *LatestConfig, now time.Time) (archived bool, err error) {
	if cfg == nil {
		return false, errors.New("latest config to save is nil")
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
		return archived, fmt.Errorf("marshal latest config: %w", err)
	}
	if err := writeFileAtomic(path, data); err != nil {
		return archived, fmt.Errorf("save latest config: %w", err)
	}
	return archived, nil
}

// writeFileAtomic writes to a temp file in the same directory and renames it
// over path, so a failed write never corrupts the existing file. The parent
// directory is created if missing: configs/ is gitignored, so a fresh
// checkout (or a relocated runtime home) starts without it.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		if writeErr != nil {
			return fmt.Errorf("write temp file: %w", writeErr)
		}
		return fmt.Errorf("close temp file: %w", closeErr)
	}

	// CreateTemp uses 0600; match the usual 0644.
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("set temp file permissions: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
}

// differsExceptState reports whether a and b differ outside State and
// GeneratedAt (run history and save timestamp should not trigger archiving);
// it compares YAML marshalings of copies with those fields zeroed.
func differsExceptState(a, b *LatestConfig) (bool, error) {
	ca, cb := *a, *b
	ca.State, cb.State = State{}, State{}
	ca.GeneratedAt, cb.GeneratedAt = "", ""

	da, err := yaml.Marshal(&ca)
	if err != nil {
		return false, fmt.Errorf("marshal config for comparison: %w", err)
	}
	db, err := yaml.Marshal(&cb)
	if err != nil {
		return false, fmt.Errorf("marshal config for comparison: %w", err)
	}
	return !bytes.Equal(da, db), nil
}

// archiveFile copies path into backupDir as "2006-01-02-150405.config.yaml",
// appending "-2", "-3", ... when the same second already has an archive so
// prior history is never overwritten.
func archiveFile(path, backupDir string, now time.Time) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read archive source: %w", err)
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}

	// More collisions than this within one second is abnormal; bail out.
	const maxSuffix = 1000
	base := now.Format("2006-01-02-150405")
	for i := 1; i <= maxSuffix; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		dst := filepath.Join(backupDir, name+".config.yaml")

		// O_EXCL fails if the file exists, preventing overwrites.
		f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return fmt.Errorf("create archive file: %w", err)
		}

		_, writeErr := f.Write(data)
		closeErr := f.Close()
		if writeErr != nil {
			return fmt.Errorf("write archive file: %w", writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close archive file: %w", closeErr)
		}
		return nil
	}
	return fmt.Errorf("archive filename collisions exceeded %d: %s", maxSuffix, base)
}
