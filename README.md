# obsidian-notion-etl

옵시디언 볼트(로컬 Markdown)와 노션 데이터베이스 사이의 **단방향 비동기 ETL CLI**입니다.
실시간 양방향 동기화가 아니라, 정해진 방향으로 데이터를 읽고 변환해 적재하는 배치 워커입니다.
이관된 데이터는 목적지에서 읽기 전용으로 취급하며, 충돌 해결 로직은 두지 않습니다.

| 명령 | 방향 | 용도 | 상태 |
|------|------|------|------|
| `init` | - | 설정·스키마 검증 및 스냅샷 생성 | 완료 |
| `migrate` | Obsidian → Notion | 과거 노트 일괄 이관 (수동, 멱등) | 완료 |
| `backup` | Notion → Obsidian | 노션 문서의 주기적 로컬 백업 | **Todo** |

## 요구사항

- Go 1.24+ (개발은 1.26 기준)
- [Notion Integration](https://www.notion.so/my-integrations) 토큰, 그리고 대상 데이터베이스에 해당 Integration 연결(Connection)
- Notion API 버전 `2026-03-11` 기준 (데이터베이스/데이터 소스 분리 구조). 단일 데이터 소스 DB만 지원

## 설치와 빌드

```bash
go build -o etl-worker .
```

## 설정

### 1. `.env`

```
NOTION_TOKEN=secret_xxx
```

토큰은 코드나 yaml에 넣지 않습니다. `.env.example`을 복사해 사용하세요.

### 2. `base.config.yaml` — 환경 정보

`base.config.example.yaml`을 복사해 작성합니다. **환경 정보만** 담습니다(변환 규칙은 코드에 있습니다. 아래 참조).

```yaml
obsidian:
  vault:
    toNotion:
      name: 'MyVault'              # 볼트 이름 (obsidian:// URI 생성에 사용)
      path: '/path/to/vault'       # 볼트 루트 절대경로
      target: 'Daily'              # 이관 소스 디렉토리 (하위 디렉토리 포함 재귀 스캔)
      effectiveDate: '2025-01-01'  # 이 날짜 이후(포함)의 노트만 이관. 날짜 미상 노트는 통과
      exclude:                     # 스캔 제외 glob 패턴 (제외 대상만 지정)
        - 'templates'
        - '메모.md'
    fromNotion:                    # backup(Todo)의 목적지. 소스와 반드시 다른 디렉토리
      name: 'MyVault'
      path: '/path/to/vault'
      target: 'Daily-NotionBackup'
notion:
  db:
    url: 'https://www.notion.so/<32자리ID>?v=...'  # DB URL (ID 자동 추출)
    name: 'MyDatabase'
```

`exclude` 매칭 규칙:
- 패턴은 target 기준 상대경로, 파일명, 그리고 **조상 디렉토리 경로**에 적용됩니다. `templates` 하나로 `templates/2025/note.md`까지 제외됩니다 (gitignore와 유사).
- macOS의 한글 파일명(NFD)과 설정의 NFC 표기는 자동으로 정규화되어 매칭됩니다.
- 설정 파싱은 엄격 모드라서 오타 키는 로드 시점에 에러로 잡힙니다.

### 3. 변환 규칙 — `pipeline.go` (코드)

날짜 파생과 속성 매핑 규칙은 설정 파일이 아니라 **코드**에 있습니다. 설정이 길어지는 것을 피하고, 규칙을 타입 검사와 테스트 아래에 두기 위한 선택입니다. 규칙 변경은 `pipeline.go` 수정 후 리빌드로 이뤄지며, `init`이 실행 시점에 실제 노션 스키마·노트와 대조해 검증합니다.

```go
// 날짜 파생 체인: 위에서부터 시도, 처음 성공한 값 사용
func dailyDateRules() []config.DateRule {
    return []config.DateRule{
        {FileLayout: "DN_060102"},        // 파일명 DN_251101.md -> 2025-11-01
        {FileLayout: "060102"},           // 파일명 251101.md
        {FrontmatterKey: "created_date"}, // frontmatter 폴백
    }
}

// 속성 매핑: value는 고정값 주입, Frontmatter+Values는 키 매핑과 값 변환
func platinumMapping() []config.MappingEntry {
    return []config.MappingEntry{
        {NotionProperty: "Type", Value: "Todo"},
        {NotionProperty: "Status", Value: "Done"},
    }
}
```

- `FileLayout`은 Go 시간 레이아웃 문법입니다 (`06`=년, `01`=월, `02`=일, 그 외 문자는 리터럴).
- 매핑 가능한 노션 속성 타입: `select`, `status`, `rich_text`. title/date/url 타입은 파이프라인이 파일명에서 자동 파생하므로 매핑할 수 없습니다.
- 설정으로 표현할 수 없는 커스텀 변환은 `transform.Transformer` 인터페이스를 구현해 `buildPipeline`의 체인에 추가합니다 (예: 제목 NFC 정규화 단계).

## 사용법

### 1. `init` — 검증과 스냅샷

```bash
./etl-worker init
```

- 볼트 경로/디렉토리, 노션 DB 접근(토큰·연결), 데이터 소스 단일성, 코드에 정의된 변환 규칙을 실제 스키마·노트와 대조해 검증합니다.
- 결과를 `configs/latest.config.yaml`에 스냅샷으로 저장합니다. 이 파일은 수동 편집 대상이 아니며, 내용이 바뀌면 이전본이 `configs/backups/`에 자동 아카이빙됩니다.

### 2. `migrate` — Obsidian → Notion 이관

```bash
./etl-worker migrate --dry-run   # 실제 생성 없이 대상·변환 결과를 로그로 확인
./etl-worker migrate             # 실제 이관
```

- **멱등**: 페이지 생성 전 Date(없으면 제목) 기준으로 노션을 조회해 중복을 건너뜁니다. 재실행해도 안전하고, 부분 실패 후 재실행하면 실패분만 다시 이관됩니다.
- 워커 5개가 병렬 처리하지만 실제 처리량은 내장 Rate Limiter(2.5 req/s)가 결정합니다.
- 항목 단위 경고·진행 기록은 `logs/migration/<실행시각>.log`에 남고, 실행 요약과 로그 경로가 stdout에 출력됩니다.

### 3. `backup` — Notion → Obsidian (Todo)

노션에서 작성된 문서를 `fromNotion.target`으로 증분 백업하는 기능입니다. 미구현이며 계획은 다음과 같습니다:
- `last_edited_time` 워터마크 기반 증분 조회, 백업 디렉토리에 무조건 덮어쓰기(노션이 source of truth)
- 노션 블록 → 마크다운 역변환, frontmatter에 노션 메타데이터 기록
- cron 등록으로 주기 실행, 로그는 `logs/backup/`

## 본문 변환 규칙 (migrate)

블록 (줄 단위):

| 마크다운 | 노션 블록 |
|----------|-----------|
| `# ` / `## ` / `### ` | heading_1/2/3 (h4 이상은 h3 폴백) |
| `- [ ] ` / `- [x] ` | to_do (체크 상태 유지, 빈 체크박스 줄도 유지) |
| `- ` / `* ` | bulleted_list_item (중첩은 평탄화) |
| 그 외 연속 줄 | 빈 줄 경계로 묶인 paragraph |
| `---` 수평선 | 생략 (문단 경계로만 작동) |

인라인:

| 마크다운 | 노션 |
|----------|------|
| `**bold**`, `*italic*`, `~~취소선~~`, `` `코드` `` | annotations (중첩 지원, 짝 없는 마커는 리터럴 유지) |
| `[[문서명]]`, `[[문서명\|별칭]]` | **밑줄 문서명 + 복사용 `obsidian://` URI 텍스트 병기** |

- 위키링크가 클릭 링크가 아닌 이유: 노션 API가 본문 인라인 링크(link.url)의 `obsidian://` 스킴을 거부합니다. url **속성**(Obsidian_URI 컬럼)은 커스텀 스킴을 허용하므로 페이지 단위 원본 링크는 속성으로 제공됩니다 (노션 앱에서 직접 클릭은 안 되고, 복사해 브라우저에서 열면 옵시디언이 실행됩니다).
- 위키링크는 줄을 넘지 않으며, 헤딩/블록 앵커(`#`, `^`)는 URI 대상에서 제거됩니다.
- 텍스트는 노션 제약(rich_text 원소당 2,000자, 블록당 원소 100개)에 맞춰 자동 분할됩니다. 긴 문단도 블록이 갈라지지 않고 하나의 문단으로 유지됩니다.

## 설계 노트

- **Rate Limit**: 노션 3 req/s 제한에 대해 보수적으로 2.5 req/s(burst 3)로 고정. 모든 API 호출이 단일 Limiter를 통과하며 우회 경로가 없습니다. 429는 Retry-After를 존중해 최대 3회 재시도(대기 상한 60초).
- **페이지 생성**: children 100블록 초과분은 append API로 분할 전송. 부분 실패(생성 후 append 실패) 시 페이지 ID를 로그에 남겨 수동 정리를 돕습니다.
- **설정 저장**: latest.config.yaml은 임시 파일 + rename의 원자적 쓰기로 저장되어 중간 실패에도 손상되지 않습니다.
- **macOS 경로**: 소스/백업 디렉토리 동일성은 문자열 비교가 아니라 `os.SameFile`로 검사합니다 (대소문자 무시·NFD·심볼릭 링크 우회 방지).

## 프로젝트 구조

```
main.go / init.go / migrate.go   # CLI 서브커맨드
pipeline.go                      # 변환 정책 (날짜 파생, 매핑, 커스텀 Transformer)
internal/
  config/     # 설정 로드·검증·원자적 저장·아카이빙, .env
  notion/     # API 클라이언트 (Limiter, 429 재시도, 블록/페이지)
  vault/      # 볼트 재귀 스캔, exclude 매칭, frontmatter 파싱
  transform/  # Transformer 파이프라인과 내장 변환 단계
  markdown/   # 마크다운 -> 노션 블록 (블록/인라인/청킹)
  logging/    # 실행별 로그 파일 (빈 로그 자동 정리)
```

## Todo

- [ ] `backup`: Notion → Obsidian 증분 백업 (워터마크, 블록 → 마크다운 역변환, cron)
- [ ] 마이그레이션 인라인 변환 확장 검토: 백슬래시 이스케이프(`\*`), 임베드(`![[...]]`) 표기
- [ ] macOS 잠자기와 cron의 관계 검토 (launchd 전환)
