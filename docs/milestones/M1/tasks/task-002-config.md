# task-002: internal/config 패키지

담당: 병렬 에이전트 A
상태: 완료

> 변경 이력 (코드리뷰 반영, 2026-08-21): `SaveLatest`는 아카이빙 발생 여부를 함께
> 반환하도록 `(archived bool, err error)`로 변경됨. 루프 방지 가드는 os.SameFile
> 기반 재검사가 추가되었고, 저장은 임시 파일 + rename 방식으로 바뀜.

## 목표

base.config.yaml / latest.config.yaml 로드·검증·저장과 아카이빙, .env 로딩을 담당하는 `internal/config` 패키지를 구현한다.

## API 계약 (다른 태스크가 이 시그니처에 의존한다)

```go
package config

type VaultTarget struct {
    Name          string `yaml:"name"`
    Path          string `yaml:"path"`
    Target        string `yaml:"target"`
    EffectiveDate string `yaml:"effectiveDate,omitempty"`
}

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

type Property struct {
    Name string `yaml:"name"`
    Type string `yaml:"type"`
}

type MappingEntry struct {
    Frontmatter    string `yaml:"frontmatter"`
    NotionProperty string `yaml:"notionProperty"`
}

type State struct {
    FirstRunAt       string `yaml:"firstRunAt"`
    LastMigrateRunAt string `yaml:"lastMigrateRunAt"`
    LastBackupRunAt  string `yaml:"lastBackupRunAt"`
}

type LatestConfig struct {
    GeneratedAt string `yaml:"generatedAt"`
    Obsidian    struct {
        FrontmatterKeys []string `yaml:"frontmatterKeys"`
    } `yaml:"obsidian"`
    Notion struct {
        DatabaseID string     `yaml:"databaseId"`
        Properties []Property `yaml:"properties"`
    } `yaml:"notion"`
    Mapping []MappingEntry `yaml:"mapping"`
    State   State          `yaml:"state"`
}

// LoadBase는 base.config.yaml을 읽고 검증한다.
// 검증: 필수 필드 비어있지 않음, toNotion/fromNotion의 path+target 결합 경로가
// 서로 달라야 함(루프 방지 가드), toNotion 소스 디렉토리가 실제 존재해야 함.
// fromNotion 백업 디렉토리는 없으면 생성한다(os.MkdirAll).
// effectiveDate는 있으면 2006-01-02 형식이어야 함.
func LoadBase(path string) (*BaseConfig, error)

// SourceDir/BackupDir는 path와 target을 결합한 절대경로를 돌려주는 헬퍼.
func (c *BaseConfig) SourceDir() string
func (c *BaseConfig) BackupDir() string

// LoadLatest는 latest.config.yaml을 읽는다. 파일이 없으면 (nil, nil)을 반환한다.
func LoadLatest(path string) (*LatestConfig, error)

// SaveLatest는 cfg를 path에 저장한다. 기존 파일이 있고 State를 제외한 부분이
// 다르면, 저장 전에 기존 파일을 backupDir/"2006-01-02-150405.config.yaml"로 복사한다.
// now는 아카이브 파일명과 GeneratedAt에 사용한다(테스트 가능성 확보).
func SaveLatest(path, backupDir string, cfg *LatestConfig, now time.Time) error

// LoadNotionToken은 .env 파일에서 NOTION_TOKEN 값을 읽는다.
// 파싱은 줄 단위 KEY=VALUE, '#' 시작 줄과 빈 줄 무시, strings 표준 함수만 사용.
// 파일이 없거나 키가 없거나 값이 비어 있으면 에러.
func LoadNotionToken(envPath string) (string, error)
```

## 제약

- 외부 의존성은 `gopkg.in/yaml.v3`만 사용.
- 정규식 금지 (CLAUDE.md 규칙). `strings` 표준 함수 사용.
- docs/review-checklist.md의 18개 항목 준수 (특히: 패닉 금지, 에러 래핑 %w, 필드 태그 명시).
- 테이블 기반 단위 테스트 작성 (`config_test.go`): LoadBase 검증 실패 케이스들, SaveLatest 아카이빙 동작(변경 시에만 백업 생성, State만 바뀌면 백업 없음), LoadNotionToken 파싱.

## 완료 기준

- `go test ./internal/config/` 통과, `go vet` 통과, gofmt 적용
