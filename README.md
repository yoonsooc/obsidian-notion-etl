# obsidian-notion-etl

옵시디언 볼트(로컬 Markdown)와 노션 데이터베이스 사이의 **단방향 비동기 ETL CLI**입니다.
실시간 양방향 동기화가 아니라, 정해진 방향으로 데이터를 읽고 변환해 적재하는 배치 워커입니다.
이관된 데이터는 목적지에서 읽기 전용으로 취급하며, 충돌 해결 로직은 두지 않습니다.

| 명령 | 방향 | 용도 | 상태 |
|------|------|------|------|
| `init` | - | 설정·스키마 검증 및 스냅샷 생성 | 완료 |
| `migrate` | Obsidian → Notion | 과거 노트 일괄 이관 (수동, 멱등) | 완료 |
| `backup` | Notion → Obsidian | 노션 문서의 주기적 증분 백업 | 완료 |

## 요구사항

- macOS (Apple Silicon 기준. NFD 파일명 처리와 cron/TCC 안내가 macOS 전제입니다)
- Go 1.27+ (구버전 툴체인은 `GOTOOLCHAIN=auto`인 경우 빌드 시 1.27을 자동 다운로드)
- [Notion 개발자 토큰](https://www.notion.so/developers/tokens), 그리고 대상 데이터베이스에 연결(Connection)
- Notion API version [`2026-03-11`](https://developers.notion.com/guides/get-started/upgrade-guide-2026-03-11) 기준
  - 데이터베이스/데이터 소스 분리 구조
  - 단일 데이터 소스 DB만 지원

## 설치와  빌드

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

`base.config.example.yaml`을 복사해 작성합니다. **환경 정보만** 담습니다(파일 변환 규칙은 코드로 다룹니다. 아래 참조).

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
    fromNotion:                    # backup의 목적지. 소스와 반드시 분리(동일·중첩 모두 init이 거부)
      name: 'MyVault'
      path: '/path/to/vault'
      target: 'Daily-NotionBackup'
notion:
  db:
    url: 'https://www.notion.so/<32자리ID>?v=...'  # DB URL (ID 자동 추출)
    name: 'MyDatabase'
```

`exclude` 매칭 규칙:
- 패턴은 target 기준 상대경로, 파일명, 그리고 **Root 디렉토리 경로**에 적용됩니다. `templates` 하나로 `templates/2025/note.md`까지 제외됩니다 (gitignore와 유사).
- macOS의 한글 파일명(NFD)과 설정의 NFC 표기는 자동으로 정규화되어 매칭됩니다.
- 설정 파싱은 strict 모드라서 키의 오타는 로드 시점에 에러로 잡힙니다.

### 3. 변환 규칙 — `plugin/` (코드)

날짜 파생과 속성 매핑 규칙은 설정 파일이 아니라 **코드**에 있습니다. 
설정이 길어지는 것을 피하고, 규칙을 타입 검사와 테스트 아래에 두기 위한 선택입니다.

사용자 정의 변환 정책은 `pipeline.Plugin` 인터페이스를 구현한 **플러그인**으로 작성합니다 (예: [plugin/platinum.go](plugin/platinum.go)).
플러그인은 `init()`에서 스스로 레지스트리에 등록되고, `base.config.yaml`의 `pipeline.plugin` 키로 사용할 플러그인을 선택합니다 (Quartz의 플러그인 설정과 유사한 방식이며, 하나만 등록된 경우 생략 가능).

플러그인 코드를 작성하지 않아도 내장 **`default` 플러그인**으로 동작합니다. `pipeline.plugin: 'default'`(또는 등록된 플러그인이 없을 때 생략)를 지정하면, `base.config.yaml`의 `pipeline.dateFrom`/`pipeline.mapping`에 yaml로 정의한 규칙을 사용합니다. 규칙을 생략하면 일반 관례(파일명 `YYYY-MM-DD` → frontmatter `date` → `created`, 매핑 없음)로 폴백하며, 규칙 형식은 `base.config.example.yaml`의 주석을 참고하세요. 이 yaml 규칙은 default 플러그인 전용이라서 사용자 플러그인과 함께 지정하면 에러가 됩니다.
공통부 코드는 플러그인 패키지에 의존하지 않으며, 접합부는 `internal/pipeline`의 레지스트리와 체인 조립기 한 곳입니다.
규칙 변경은 플러그인 파일 수정 후 리빌드로 이뤄지며, `init`이 실행 시점에 실제 노션 스키마 vs 옵시디언 노트와 대조해 검증합니다.

```go
// pipeline.Plugin 인터페이스: 모든 메서드는 선언적(데이터 반환)이며 실패하지 않음
type Plugin interface {
    Name() string                        // base.config.yaml의 pipeline.plugin으로 선택되는 이름
    DateRules() []config.DateRule        // 날짜 파생 체인: 위에서부터 시도, 처음 성공한 값 사용
    Mapping() []config.MappingEntry      // 속성 매핑: value는 고정값 주입, Frontmatter+Values는 키 매핑과 값 변환
    Transformers() []transform.Transformer // 커스텀 변환 단계 (예: 제목 NFC 정규화)
}

// 플러그인 등록: 파일 하나가 정책 하나, init에서 자가 등록
func init() { pipeline.Register(platinum{}) }
```

- `FileLayout`은 Go 시간 레이아웃 문법입니다 (`06`=년, `01`=월, `02`=일, 그 외 문자는 리터럴).
- 매핑 가능한 노션 속성 타입: `select`, `status`, `rich_text`. title/date/url 타입은 파이프라인이 파일명에서 자동 파생하므로 매핑할 수 없습니다.
- 설정으로 표현할 수 없는 커스텀 변환은 `transform.Transformer` 인터페이스를 구현해 플러그인의 `Transformers()`로 반환합니다. 반환된 단계는 제목 파생 이후, URI/속성 매핑 이전에 체인에 삽입됩니다.
- 플러그인의 규칙 오류는 접합부로 전파되지 않습니다. 규칙 데이터는 `init` 검증에서, 노트 단위 변환 에러는 `transform.Run`에서 격리(로그 후 해당 노트 스킵)됩니다.


## 사용법

### 1. `init` — 검증과 스냅샷

```bash
./etl-worker init
```

- 볼트 경로/디렉토리(소스·백업 분리 포함), 노션 DB 접근(토큰), 데이터 소스 단일성, 선택된 플러그인의 변환 규칙을 실제 스키마와 노트를 대조해 검증합니다.
- 결과를 `configs/latest.config.yaml`에 스냅샷으로 저장합니다. 이 파일은 수동 편집 대상이 아니며, 내용이 바뀌면 이전본이 `configs/backups/`에 자동 아카이빙됩니다.

### 2. `migrate` — Obsidian → Notion 이관

```bash
./etl-worker migrate --dry-run   # 실제 생성 없이 대상·변환 결과를 로그로 확인
./etl-worker migrate             # 실제 이관
```

- **멱등**: 페이지 생성 전 제목(파일명 어간) 기준으로 노션을 조회해 중복을 건너뜁니다. 제목은 노트 유형(Daily/Weekly/Monthly) 간에도 유일해서, Weekly 시작일이 Daily 날짜와 겹쳐도 안전합니다. 재실행해도 안전하고, 부분 실패 후 재실행하면 실패분만 다시 이관됩니다.
- **생성 전용(갱신 없음)**: migrate는 기존 페이지를 수정하지 않습니다. 노트 본문이 바뀌었거나 변환 규칙을 바꿔 소급 적용하고 싶으면, 노션에서 해당 페이지를 삭제(휴지통, 30일 내 복구 가능)한 뒤 migrate를 재실행하세요. 멱등이라 삭제한 페이지만 다시 만들어집니다.
- **노션 쪽 편집 규칙**: 페이지 **제목(Name)은 노션에서 바꾸지 마세요**. 제목이 노트의 정체성 키라서, 바꾸면 다음 migrate가 같은 노트를 새 페이지로 오인해 중복이 생기고 backup 파일명도 갈라집니다. 본문과 다른 속성 편집은 자유이며 backup이 그대로 미러링합니다.
- 워커 5개가 병렬 처리하지만 실제 처리량은 내장 Rate Limiter(2.5 req/s)가 결정합니다.
- 항목 단위 경고·진행 기록은 `logs/migration/<실행시각>.log`에 남고, 실행 요약과 로그 경로가 stdout에 출력됩니다.

### 3. `backup` — Notion → Obsidian 증분 백업

```bash
./etl-worker backup --dry-run   # 실제 쓰기 없이 대상만 로그로 확인
./etl-worker backup             # 실제 백업 (cron 등록용 명령도 동일)
```

- **증분**: `last_edited_time >= state.lastBackupRunAt` 워터마크 필터로 변경분만 조회합니다 (워터마크가 없으면 전체 백업). 워터마크는 실행 시작 시각으로, **전건 성공 시에만** 갱신되므로 부분 실패분은 다음 실행에서 재시도됩니다.
- **덮어쓰기**: 백업 디렉토리(`fromNotion.target`)는 노션의 미러라서 같은 파일은 무조건 덮어씁니다. 소스 디렉토리와의 분리는 init이 검증합니다.
- **파일명**: 제목(= 원본 옵시디언 파일명, 금지 문자 `-` 치환) → 제목이 비면 Date(`YYYY-MM-DD`) → 페이지 ID 순으로 파생합니다. 제목이 양방향의 노트 정체성이라 백업이 원본 이름을 복원하며, 같은 날짜의 데일리/주간 노트도 충돌하지 않습니다. 실행 내 충돌은 `-2`, `-3` 접미사로 처리합니다.
- **frontmatter**: `notion_id`, `notion_last_edited`, `source: notion`을 기록합니다.
- **삭제 비전파**: 노션에서 삭제한 페이지의 백업 파일은 남습니다 (삭제 전파는 비목표). 필요하면 수동으로 지우세요.
- 항목 단위 경고·진행 기록은 `logs/backup/<실행시각>.log`에 남습니다.

역변환 규칙 (블록 → 마크다운):

| 노션 블록 | 마크다운 |
|-----------|----------|
| heading_1/2/3 | `# ` / `## ` / `### ` |
| to_do | `- [ ] ` / `- [x] ` |
| bulleted_list_item | `- ` |
| quote | `> ` 접두 줄들 |
| callout | `> [!type] ...` (아이콘 이모지를 타입으로 역매핑, 미지의 이모지는 note) |
| paragraph | 일반 문단 (빈 문단은 빈 줄) |
| 그 외 타입 | 텍스트 추출 가능하면 문단으로 폴백, 불가하면 건너뛰고 로그 |

- 인라인 서식(annotations)은 복원하지 않고 plain text만 씁니다. migrate의 위키링크 변형(밑줄+URI 텍스트)도 되돌리지 않으므로 왕복은 손실이 있습니다 (백업/분석 목적으로 수용).
- 중첩 블록은 내려가지 않고 최상위 텍스트만 백업하며, 중첩이 있는 블록은 로그로 알립니다.

## 본문 변환 규칙 (migrate)

### Block (줄 단위)

| 마크다운 | 노션 블록 |
|----------|-----------|
| `# ` / `## ` / `### ` | heading_1/2/3 (h4 이상은 h3 Fallback) |
| `- [ ] ` / `- [x] ` | to_do (체크 상태 유지, 빈 체크박스 줄도 유지) |
| `- ` / `* ` | bulleted_list_item (중첩은 평탄화) |
| `> [!type] 제목` + `> 본문` | callout (타입별 이모지 아이콘, 미지의 타입은 📝. 접힘 마커 `+`/`-`는 무시) |
| `> ` 연속 줄 | quote (한 그룹이 한 블록) |
| 그 외 연속 줄 | 빈 줄 경계로 묶인 paragraph |
| `---` 수평선 | 생략 (문단 경계로만 작동) |

### Inline

| 마크다운 | 노션 |
|----------|------|
| `**bold**`, `*italic*`, `~~취소선~~`, `` `코드` `` | annotations (중첩 지원, 짝 없는 마커는 리터럴 유지) |
| `[[문서명]]`, `[[문서명\|별칭]]` | **밑줄 문서명 + 복사용 `obsidian://` URI 텍스트 병기** |

- 위키링크가 클릭 링크가 아닌 이유: 노션 API가 본문 인라인 링크(link.url)의 `obsidian://` 스킴을 거부합니다. url **속성**(Obsidian_URI 컬럼)은 커스텀 스킴을 허용하므로 페이지 단위 원본 링크는 속성으로 제공됩니다 (노션 앱에서 직접 클릭은 안 되고, 복사해 브라우저에서 열면 옵시디언이 실행됩니다).
- 위키링크는 줄을 넘지 않으며, 헤딩/블록 앵커(`#`, `^`)는 URI 대상에서 제거됩니다.
- 텍스트는 노션 제약(rich_text 원소당 2,000자, 블록당 원소 100개)에 맞춰 자동 분할됩니다. 긴 문단도 블록이 갈라지지 않고 하나의 문단으로 유지됩니다.

## 스케줄링 (cron)과 운영 홈

무인 실행은 저장소가 아니라 **운영 홈**(TCC 비보호 경로, 예: `~/etl-worker/`)에서 합니다.
저장소를 `~/Documents` 아래에 두는 경우, cron이 그 안의 바이너리·설정에 접근하려면 전체
디스크 접근 권한이 필요해지기 때문입니다 (아래 TCC 항목 참조). 운영 홈에는 바이너리와
런타임 파일만 둡니다:

```bash
mkdir -p ~/etl-worker && cd <저장소> && go build -o ~/etl-worker/etl-worker .   # 배포(재배포 동일)
# 최초 1회: .env, base.config.yaml을 운영 홈에 두고 ~/etl-worker에서 ./etl-worker init
( crontab -l 2>/dev/null; echo '0 * * * * cd ~/etl-worker && ./etl-worker backup >> logs/cron.log 2>&1' ) | crontab -
crontab -l   # 확인 / 해제는 crontab -e
```

- 운영 홈 구성: `etl-worker`(바이너리), `.env`, `base.config.yaml`, `configs/`(워터마크 포함 스냅샷), `logs/`. **실(非 dry-run) 실행은 항상 운영 홈에서** 합니다.
- 개발 환경: 설정 경로가 전부 작업 디렉토리 기준 상대 경로라서, 저장소에 `.env`와 `base.config.yaml` 사본을 두고 `init`을 실행하면 개발용 스냅샷(configs/)이 로컬에 생겨 테스트가 가능합니다. 다만 개발 쪽에서는 `go test`와 `--dry-run`까지만 사용하세요. 실 실행을 개발 쪽에서 하면 같은 노션 DB·백업 디렉토리를 건드리면서 워터마크만 두 곳으로 갈립니다 (dry-run은 상태를 쓰지 않아 안전).
- 실행 요약(stdout)은 `logs/cron.log`에 누적되고, 항목 단위 상세는 실행별 `logs/backup/<실행시각>.log`에 남습니다. 변경분이 없는 실행은 상세 로그를 만들지 않으므로(빈 로그 자동 삭제) 로그가 시간당 쌓이지 않습니다.
- **macOS 권한(TCC)**: cron은 동의창을 띄울 수 없어, TCC 보호 폴더(`~/Documents` 등)의 파일에 닿는 순간 에러 없이 **무한히 멈춥니다** (증상: cron.log 0바이트, CPU 0%의 etl-worker 프로세스가 매시 누적. 바이너리 exec 단계의 dyld에서도 걸림). 운영 홈을 비보호 경로에 두면 이 문제가 원천 차단되며, CloudStorage(Google Drive) 볼트 쓰기는 cron에서 별도 허가 없이 동작함을 실측으로 확인했습니다. 운영 홈 분리가 불가능한 배치에서만 시스템 설정 → 개인정보 보호 및 보안 → 전체 디스크 접근 권한에 `/usr/sbin/cron`을 추가하세요.
- **동시 실행**: cron backup이 도는 중에 수동 migrate를 실행하면 프로세스별 Limiter가 독립이라 순간적으로 노션 한도를 넘을 수 있습니다. 429 재시도가 흡수하지만, 대량 이관은 정각을 피하는 것이 깔끔합니다.
- **잠자기 한계**: Mac이 잠자면 그 시각의 cron은 건너뜁니다. 워터마크 증분이라 다음 실행이 밀린 변경분을 따라잡으므로 기능상 문제는 없습니다(PRD §7). 스케줄러는 이식성을 위해 cron 유지로 확정했으며, launchd(macOS 전용)는 cron을 쓸 수 없는 배치에서만 대안으로 고려합니다.

## 설계 노트

- **Rate Limit**: 노션 3 req/s 제한에 대해 보수적으로 2.5 req/s(burst 3)로 고정. 모든 API 호출이 단일 Limiter를 통과하며 우회 경로가 없습니다. 429는 Retry-After를 존중해 최대 3회 재시도(대기 상한 60초).
- **페이지 생성**: children 100블록 초과분은 append API로 분할 전송. 부분 실패(생성 후 append 실패) 시 페이지 ID를 로그에 남겨 수동 정리를 돕습니다.
- **설정 저장**: latest.config.yaml은 임시 파일 + rename의 원자적 쓰기로 저장되어 중간 실패에도 손상되지 않습니다.
- **macOS 경로**: 소스/백업 디렉토리 동일성은 문자열 비교가 아니라 `os.SameFile`로 검사합니다 (대소문자 무시·NFD·심볼릭 링크 우회 방지).

## 프로젝트 구조

```
main.go                          # 엔트리포인트: 서브커맨드 라우팅, plugin 패키지 blank import 등록
plugin/                          # 사용자 정의 플러그인 (platinum.go 등, pipeline.Plugin 구현체)
internal/
  cli/        # 서브커맨드 진입점 (init.go / migrate.go / backup.go, 실행 흐름 조립)
  config/     # 설정 로드·검증·원자적 저장·아카이빙, .env
  notion/     # API 클라이언트 (Limiter, 429 재시도, 블록/페이지)
  vault/      # 볼트 재귀 스캔, exclude 매칭, frontmatter 파싱
  transform/  # Transformer 파이프라인과 내장 변환 단계
  pipeline/   # 플러그인 접합부: Plugin 인터페이스, 레지스트리, 체인 조립
  markdown/   # 마크다운 -> 노션 블록 (블록/인라인/청킹)
  logging/    # 실행별 로그 파일 (빈 로그 자동 정리)
```

## 검토 종결 항목

한때 Todo였던 항목들의 결정 기록입니다 (미결 Todo 없음).

- **스케줄러**: cron 유지 확정 (2026-09-03). cron이 동작하는 환경에서는 이식성을 위해 cron을 쓰고, launchd(macOS 전용)는 cron을 쓸 수 없는 배치에서만 대안으로 고려합니다.
- **인용/콜아웃**: `> ` 인용 → quote 블록, `> [!type]` 콜아웃 → callout 블록(이모지 아이콘)으로 구현 완료 (2026-09-03, 역변환 포함).
- **백슬래시 이스케이프(`\*`)**: 실볼트 조사 결과 사용 0건이라 구현하지 않기로 종결 (필요해지면 재검토).
- **임베드(`![[...]]`)**: 실볼트 사용 2건. 현행 변환(`!` + 밑줄 문서명 + URI 텍스트)을 수용하는 것으로 종결.
