# PRD: obsidian-notion-etl

작성일: 2026-08-20
상태: 개발 착수 전 확정본 (v1)

---

## 1. 개요

Obsidian(로컬 Markdown 볼트)과 Notion(클라우드 데이터베이스) 사이의 **단방향 비동기 ETL CLI 도구**를 Go로 구현한다.

두 가지 작업을 수행한다.

1. **Migration (Obsidian → Notion, 수동 1회성):** 과거의 데일리/다이어리 노트를 노션 DB(`Platinum`)로 이관한다. 이후 데일리 노트는 노션에서 관리한다.
2. **Backup ETL (Notion → Obsidian, 주기 실행):** 노션 DB에서 작성/수정된 문서를 로컬 볼트의 백업 디렉토리로 주기적으로 긁어온다. 목적은 분석과 백업이다.

양방향 실시간 동기화가 아니므로 충돌(Conflict) 해결 로직은 만들지 않는다. 이관된 데이터는 목적지에서 읽기 전용으로 취급한다.

### 1.1 비목표 (Non-Goals)

- 실시간 또는 양방향 동기화
- 이미지, 첨부파일, 임베드 등 미디어 블록의 이관
- 노션 페이지 간 관계(Relation), 데이터베이스 중첩 처리
- 삭제 전파 (한쪽에서 지워도 반대쪽에는 반영하지 않는다)

---

## 2. 확정된 설계 결정 (Decision Log)

| # | 항목 | 결정 | 근거 |
|---|------|------|------|
| D1 | 바이너리 구조 | **단일 바이너리 + 서브커맨드** (`etl-worker init` / `migrate` / `backup`) | 설정 로더, Notion 클라이언트, Rate Limiter를 internal 패키지로 공유. cron 등록 단순화 |
| D2 | 마이그레이션 멱등성 | **생성 전 노션 조회 후 스킵.** Date 속성으로 DB를 쿼리하여 이미 존재하면 건너뜀 | 로컬 상태 파일이 유실되어도 중복이 생기지 않음. 파일당 API 1회 추가는 1회성 작업이므로 허용. 조회 키는 D14로 제목 기준 전환 |
| D3 | 백업 목적지 | **마이그레이션 소스와 분리된 별도 백업 디렉토리** | 원본 훼손 방지, 백업본이 다시 마이그레이션 대상이 되는 루프 차단 |
| D4 | 본문 변환 수준 | **기본 마크다운 블록 매핑** (heading 1~3, bullet list, todo, paragraph). 양방향 모두 적용 | 가독성 확보. 표/이미지/코드블록 등 그 외 타입은 v1 범위에서 제외하고 plain paragraph로 폴백 |
| D5 | 설정 역할 분리 | **변환 규칙(dateFrom, mapping 등)은 base.config.yaml(사람 작성)에 두고, latest.config.yaml은 init이 검증한 스냅샷+상태만 담는 순수 생성물**로 유지 | latest를 수동 수정하면 init 재실행 시 덮어써지는 충돌 제거 (2026-08-21) |
| D6 | 대상 선정과 날짜 파생의 분리 | 선정은 재귀 스캔+exclude만으로 결정. 날짜는 `dateFrom` 우선순위 체인(파일명 레이아웃 → frontmatter 키)으로 파생하며, **실패 시 Date를 비운 채 이관**하고 로그에 남긴다. effectiveDate 필터는 날짜 파생에 성공한 파일에만 적용 | 실제 볼트의 파일명이 `DN_{YYMMDD}` 등 혼재. 파일명 형식이 선정 필터를 겸하면 exclude 원칙과 충돌 (2026-08-21) |
| D7 | 노션 페이지 제목 | **Name(title)은 파일명(확장자 제외)으로 통일** | 날짜 없는 파일도 대상에 포함되므로 날짜 문자열 제목은 성립하지 않음 (2026-08-21) |
| D8 | 변환 아키텍처 | **Transformer 인터페이스 기반 파이프라인.** 문서마다 등록 순서대로 한 번씩 통과. 선언적 설정(mapping, dateFrom)은 내장 Transformer의 조립 입력이고, 커스텀 로직은 같은 인터페이스의 코드 Transformer로 추가 | 단순 규칙은 설정에, 복잡한 규칙은 코드에. 동적 플러그인 로딩은 하지 않음 (2026-08-21) |
| D9 | 실제 매핑 규칙 | Type은 고정값 `Todo`, Status는 고정값 `Done`. `docu_type`(Plan/Project/Research/Study)과 `category`는 노션으로 보내지 않음 | docu_type 전 값이 Todo로 수렴하므로 값 변환 테이블 불필요 (2026-08-21) |
| D10 | 변환 규칙의 위치 | **날짜 파생(dateFrom)과 속성 매핑(mapping) 규칙은 설정 파일이 아니라 코드(`pipeline.go`)에 둔다.** base.config.yaml은 환경 정보(경로, DB, exclude)만 담는다. 규칙 변경은 코드 수정 + 리빌드 | 설정이 길어지는 것을 피하려는 사용자 결정. 코드에 있으면 base/latest 간 신선도 불일치 문제도 소멸 (2026-08-21). 위치와 계약은 D11로 재정리, 설정 기반 규칙은 D12로 opt-in 복원 |
| D11 | 플러그인 아키텍처 | **변환 정책을 `pipeline.Plugin` 인터페이스 구현체(`plugin/` 패키지, 예: platinum.go)로 분리하고, 접합부를 `internal/pipeline`(레지스트리 Register/Select + 체인 조립 BuildChain) 한 곳으로 일원화한다.** 공통부(init/migrate)는 플러그인 패키지에 의존하지 않으며 유일한 참조는 main의 blank import(자가 등록). 사용할 플러그인은 base.config.yaml의 `pipeline.plugin`으로 선택한다 | pipeline.go에 커스텀 비즈니스 로직과 범용 기능이 혼재하던 문제 해소. D10의 "규칙은 코드로"는 유지하되 위치와 계약을 정리. 인터페이스는 선언적(규칙 데이터만 반환, 실패 없음)이라 플러그인 에러가 접합부로 전파되지 않고, 규칙 오류는 init 검증에서, 노트 단위 에러는 transform.Run에서 격리된다 (2026-08-28) |
| D12 | 내장 default 플러그인 | **사용자 플러그인 코드가 없어도 동작하는 내장 default 플러그인을 둔다.** 규칙은 base.config.yaml의 `pipeline.dateFrom`/`pipeline.mapping`(yaml, 코드 수정 없이 편집)에서 오고, 없으면 일반 관례로 폴백(파일명 `2006-01-02` → frontmatter `date` → `created`, 매핑 없음). yaml 규칙은 default 전용이라 사용자 플러그인과 병용하면 에러이고, `default`는 예약어다. 위치는 plugin/이 아닌 internal(순환 참조, 등록-설정 로드 시점 불일치, 프레임워크 기능이라는 의미론) | 초기 상태(플러그인 전무)에서도 도구가 기능해야 함. latest의 dateFrom/mapping은 검증 후 기록용 출력이고 base가 유일한 규칙 입력이라는 역할 구분 유지 (2026-08-29) |
| D13 | 도메인 어휘 규칙 | **타입/필드의 어휘는 입출력 위치가 아니라 데이터가 속한 도메인을 따르고, 흐름의 방향은 패키지/함수 이름이 표현한다.** 산출물 타입은 `Draft` 접미사(migrate: Note → PageDraft, backup: Page → NoteDraft 예정). 단일 언어 패키지 불변식: vault에 Notion 어휘, notion에 Obsidian 어휘 금지(혼용은 변환 패키지에서만). 상세 glossary는 CLAUDE.md, grep 검증은 리뷰 체크리스트 19번 | 두 도메인 용어가 여러 곳에 쓰이는 가독성 문제. 입출력 위치 기준 규칙은 방향이 반대인 backup에서 깨지므로 소속 기준으로 확정 (2026-08-29) |
| D14 | 노트 정체성은 제목 | **중복 검사와 백업 파일명 모두 제목(Name) 기준으로 통일한다.** 제목은 파일명 어간이라 노트 유형 간 유일하고, 기존 이관분의 제목도 원본 파일명이라 재실행 멱등성이 유지되며, 백업이 원본 옵시디언 이름을 복원한다. ExistsByDate API는 제거, 백업 파일명은 제목 → Date → 페이지 ID 순 폴백 | D2의 Date 우선 검사와 Date 기준 백업 파일명은 "1일 1노트" 전제였는데, D15로 Weekly/Monthly가 시작 날짜를 갖게 되면 같은 Date의 노트가 여럿 생겨(주간 시작일 = 그 날의 데일리) 중복 오인 스킵과 백업 파일명 `-2` 접미사 충돌이 발생함. 셀프 리뷰 가이드 Q4의 현실화 (2026-09-01) |
| D15 | 이관 범위 확장 | **migrate 대상을 Daily에서 Work 전체(Daily/Weekly/Monthly)로 확장한다.** 날짜 파생은 platinum 체인 확장으로 해결: Monthly(`MG_202603`)는 레이아웃 `MG_200601` 선언(일 미지정 → 1일), Weekly(`WG_260513-0517`, 구분자 `-`/`~`, 끝 4/6자리 혼재)는 범위 접미사를 레이아웃으로 표현할 수 없어 커스텀 Transformer(weeklyStartDate)로 시작일 파생하며, 주간 노트는 created_date 폴백보다 파일명이 정본이라 덮어쓴다. 유형별 별도 플러그인은 두지 않음(실행당 플러그인 1개 구조, 체인이 유형 디스패처) | Weekly/Monthly도 노션 관리·백업 대상에 포함. 이에 따라 백업 디렉토리를 소스 밖(`100. Inbox/Daily-NotionBackup`)으로 이동하고 워터마크를 리셋해 전체 미러를 재수행. **백업-소스 중첩은 설정 검증이 조상 경로 SameFile 비교로 양방향 차단**(백업⊂소스: 재이관 루프, 소스⊂백업: 미러 덮어쓰기 위험) (2026-09-01) |
| D16 | 인용·콜아웃 매핑과 인라인 확장 종결 | **`> ` 인용은 quote 블록, `> [!type]` 콜아웃은 callout 블록(타입별 이모지 아이콘)으로 양방향 매핑한다** (D4의 블록 집합 확장). 노션 에디터의 `>`는 토글이지만 API로는 quote/callout 생성이 가능함을 이용. 백슬래시 이스케이프(`\*`)는 실볼트 사용 0건으로 미구현 종결, 임베드(`![[...]]`)는 2건으로 현행 변환(`!`+밑줄+URI) 수용 종결. 스케줄러는 이식성을 위해 cron 유지 확정(launchd는 cron 불가 배치의 대안) | 실볼트 조사 기반 결정: 인용 180줄·콜아웃 14건(5유형)은 실사용, 이스케이프·임베드는 근거 부족. README Todo 소거 목표 (2026-09-03) |

이 결정에 따라 `base.config.yaml`의 `fromNotion.target`을 별도 디렉토리(예: `100. Inbox/Work/Daily-NotionBackup`)로 변경해야 한다. 또한 기존 `to-notion/`, `to-obsidian/`의 main 패키지는 단일 바이너리 구조로 재편한다.

---

## 3. 프로젝트 구조

```
obsidian-notion-etl/
├── main.go                  # 엔트리포인트. 서브커맨드 라우팅, plugin 패키지 blank import (D11)
├── go.mod
├── plugin/                  # 사용자 정의 플러그인 (pipeline.Plugin 구현체, 예: platinum.go) (D11)
├── internal/
│   ├── cli/                 # 서브커맨드 진입점 (init/migrate/backup, 실행 흐름 조립)
│   ├── config/              # base/latest 설정 로드, 검증, 백업 아카이빙
│   ├── notion/              # Notion API 클라이언트 (Rate Limiter 내장, 타입 Struct 정의)
│   ├── markdown/            # Markdown <-> Notion Block 상호 변환, 청킹
│   ├── transform/           # Transformer 인터페이스와 내장 변환 단계(날짜 파생, 제목, 속성 매핑) (D8)
│   ├── pipeline/            # 플러그인 접합부: Plugin 인터페이스, 레지스트리, 내장 default, 체인 조립 (D11, D12)
│   ├── logging/             # 실행별 로그 파일 (logs/<command>/<실행시각>.log)
│   └── vault/               # 옵시디언 볼트 파일 스캔/읽기/쓰기, Frontmatter 파싱
├── base.config.example.yaml # 커밋되는 설정 템플릿
├── base.config.yaml         # 사용자가 직접 작성하는 기본 설정 (gitignore 대상)
├── .env.example             # 커밋되는 환경변수 템플릿
├── configs/
│   ├── latest.config.yaml   # init이 생성하는 세부 설정 + 실행 상태
│   └── backups/             # 설정 변경 시 날짜 붙여 아카이빙
├── logs/                    # 실행 로그
├── .env                     # NOTION_TOKEN (gitignore 대상)
└── PRD.md
```

빌드: `go build -o etl-worker .` (서브커맨드 구현은 internal/cli 패키지에 있고, 루트 main 패키지는 라우팅만 담당한다)
실행 예: `./etl-worker init`, `./etl-worker migrate`, `./etl-worker backup`

기존 `to-notion/main.go`의 초안 코드(워커 풀, Limiter, 청킹, Frontmatter 파싱)는 internal 패키지로 흡수하고, 하드코딩된 토큰/경로/DB ID는 전부 `.env`와 설정 파일로 옮긴다.

---

## 4. 설정 설계

### 4.1 .env

```
NOTION_TOKEN=secret_xxx
```

토큰은 코드나 yaml에 절대 넣지 않는다. 실행 시작 시 `.env`를 읽고(외부 라이브러리 없이 `os.ReadFile` + `strings` 파싱), 누락 시 즉시 종료한다. 이때만 예외적으로 `log.Fatal`을 허용한다.

### 4.2 base.config.yaml (사용자 작성)

현재 파일 구조를 유지하되 `fromNotion.target`을 분리한다.

```yaml
obsidian:
  vault:
    toNotion:
      name: 'Yersona'
      path: '<볼트 절대경로>'
      target: '100. Inbox/Work'                # 마이그레이션 소스 (Daily/Weekly/Monthly 재귀 스캔, D15)
      effectiveDate: '2025-11-01'              # 이 날짜 이후(포함) 파일만 이관
      exclude:                                 # 스캔 제외 glob 패턴 (gitignore처럼 제외 대상만 지정. 파일명,
        - '하루를 시작하기 전에.md'            # 상대경로, 조상 디렉토리에 매칭. NFC/NFD 정규화 처리됨)
```

변환 규칙(날짜 파생 체인, 속성 매핑)의 출처는 D11-플러그인 아키텍처에 따라 **선택된 플러그인**이다. `pipeline.plugin` 키로 사용할 플러그인을 지정하며(생략 시 등록 1개면 그것, 없으면 내장 default), 사용자 플러그인(예: platinum)의 규칙은 코드에, 내장 default 플러그인(D12)의 규칙은 base.config.yaml의 `pipeline.dateFrom`/`pipeline.mapping`(yaml)에 둔다. 설정 파싱은 엄격 모드(KnownFields)라서 오타 키는 로드 시점에 에러로 잡힌다.

```yaml
    fromNotion:
      name: 'Yersona'
      path: '<볼트 절대경로>'
      target: '100. Inbox/Daily-NotionBackup'  # 백업 목적지 (소스 밖으로 완전 분리, 중첩 금지. D15)
notion:
  db:
    url: 'https://www.notion.so/<32자리ID>?v=...'
    name: 'Platinum'
pipeline:
  plugin: 'platinum'                                # 사용할 변환 플러그인 (D11)
# (참고) platinum 플러그인(plugin/platinum.go)의 규칙:
#   DateRules: DN_060102 -> 060102 -> MG_200601(월간, 1일로 파생) -> frontmatter 'created_date'
#   커스텀 Transformer: weeklyStartDate(WG_ 파일명에서 주간 시작일, created_date보다 우선), nfcTitle
#   Mapping: Type=Todo, Status=Done (고정값)
```

DB ID는 URL 경로의 32자리 hex 문자열에서 추출한다 (`strings` 기반 파싱, 정규식 미사용).

### 4.3 configs/latest.config.yaml (init이 생성)

`init` 서브커맨드가 양측을 검증한 뒤 생성한다. 설정과 실행 상태(워터마크)를 함께 담는다.

```yaml
generatedAt: '2026-08-20T21:00:00+09:00'
obsidian:
  frontmatterKeys: [type, category, ...]   # 소스 디렉토리 파일들에서 수집된 키 목록
notion:
  databaseId: '<추출된 32자리 ID>'
  dataSourceId: '<데이터베이스 조회로 얻은 데이터 소스 ID>'  # 쿼리·페이지 생성에 사용
  properties:                              # data source retrieve API로 조회한 실제 스키마
    - { name: 'Name', type: 'title' }
    - { name: 'Date', type: 'date' }
    - { name: 'Type', type: 'select' }
    - { name: 'Obsidian_URI', type: 'url' }
mapping:                                   # 프론트매터 키 -> 노션 컬럼 매핑 (init이 초안 생성, 사용자가 수정 가능)
  - { frontmatter: 'type', notionProperty: 'Type' }
state:
  firstRunAt: ''                           # 최초 migrate 실행 시각
  lastMigrateRunAt: ''
  lastBackupRunAt: ''                      # backup 증분 기준 워터마크 (마지막으로 처리한 last_edited_time)
```

### 4.4 설정 아카이빙 규칙

`init` 실행 결과가 기존 `latest.config.yaml`과 다르면(설정 부분만 비교, state 제외), 기존 파일을 `configs/backups/YYYY-MM-DD-HHmmss.config.yaml`로 복사한 뒤 새로 쓴다. state 갱신(워터마크 업데이트)만으로는 아카이빙하지 않는다.

---

## 5. 기능 요구사항

### FR-1. `init` (설정 부트스트랩)

1. `base.config.yaml`을 로드하고 필수 필드를 검증한다.
2. 파일시스템 검증: 볼트 경로와 소스/백업 target 디렉토리 존재 확인. 백업 디렉토리가 없으면 생성한다. **소스와 백업 디렉토리는 완전히 분리되어야 한다**: 동일(문자열·SameFile)뿐 아니라 어느 한쪽이 다른 쪽 내부에 있는 중첩도 에러로 중단한다 (D15, 루프/미러 덮어쓰기 방지 가드. 조상 경로를 SameFile로 비교해 대소문자·NFD·심볼릭 링크 우회도 차단).
3. Notion 검증: DB ID를 URL에서 추출하고 `GET /v1/databases/{id}`로 데이터베이스를 조회한다. 접근 불가(401/404) 시 원인을 안내하고 중단한다. 데이터 소스가 정확히 1개가 아니면(0개 또는 다중 소스) 에러로 중단하고, `GET /v1/data_sources/{id}`로 속성 스키마를 조회한다.
4. 소스 디렉토리 하위의 `.md` 파일들을 재귀적으로 스캔하여 Frontmatter 키 목록을 수집한다. `exclude` glob 패턴에 걸리는 파일은 제외한다. macOS의 NFD 파일명과 설정의 NFC 패턴이 매칭되도록 유니코드 정규화 후 비교한다.
5. 선택된 플러그인(D11)이 선언한 변환 규칙을 검증한다 (D5-설정 역할 분리: init은 규칙의 작성자가 아니라 검증자다). 즉 매핑의 notionProperty가 실제 DB 컬럼에 존재하고 v1 지원 타입(select/status/rich_text)인지, frontmatter 키가 실제 노트들에서 발견되는지(대소문자 불일치는 별도 경고), 날짜 규칙의 fileLayout이 유효한 Go 시간 레이아웃인지 확인하고, 문제는 로그로 안내한다.
6. 검증을 통과한 규칙과 조회된 스키마를 4.4 규칙에 따라 `latest.config.yaml`에 스냅샷으로 저장한다. latest.config.yaml은 수동 수정 대상이 아니다.

### FR-2. `migrate` (Obsidian → Notion, 수동)

1. **대상 선정:** 소스 디렉토리 하위를 재귀 탐색하여(월별 등 하위 디렉토리 포함), `exclude` 패턴에 걸리지 않는 모든 `.md` 파일이 대상이다 (D6: 파일명 형식과 날짜는 선정에 관여하지 않는다).
2. **변환 파이프라인 (D8-변환 아키텍처):** 파싱된 노트가 Transformer 체인을 등록 순서대로 한 번씩 통과하며 노션 페이지 초안(PageDraft)이 완성된다. 날짜 파생을 포함한 모든 속성 결정이 파이프라인 안에서 일어난다. 체인은 `internal/pipeline`의 `BuildChain`이 선택된 플러그인의 규칙으로 조립한다 (D10-변환 규칙의 위치, D11-플러그인 아키텍처).
   - 날짜 파생: 날짜 규칙 체인 (fileLayout → frontmatterKey). 실패 시 Date 비움 + 로그 (D6-대상 선정과 날짜 파생의 분리)
   - 제목: `Name`(title) ← 파일명(확장자 제외) (D7-노션 페이지 제목), 커스텀 단계에서 NFC 정규화 (macOS NFD 파일명의 중복 검사 멱등성 확보)
   - `Obsidian_URI`(url) ← `obsidian://open?vault=...&file=...` (URI 컴포넌트 인코딩. 공백은 `+`가 아니라 `%20` — Obsidian은 `+`를 공백으로 해석하지 않음)
   - 속성 매핑: 매핑 규칙 적용. `value` 지정 시 고정값 주입, `frontmatter` 지정 시 키 매핑(+`values` 값 변환 테이블, `default` 폴백). 매핑에 없는 frontmatter 키는 보내지 않는다 (매핑 가능 타입: select, status, rich_text)
   - 커스텀 로직이 필요해지면 같은 Transformer 인터페이스를 구현해 플러그인의 `Transformers()`로 반환한다 (예: nfcTitle). 반환된 단계는 제목 파생 이후, URI/속성 매핑 이전에 삽입된다 (D11)
   - 본문 인라인 서식 (task-012): `**bold**`, `*italic*`, `~~취소선~~`, `` `코드` ``를 노션 annotations로 변환. `[[위키링크]]`는 **밑줄 문서명 + 복사용 obsidian:// URI 일반 텍스트 병기**로 변환한다. 인라인 클릭 링크가 아닌 이유는 노션 API가 본문 link.url의 obsidian:// 스킴을 거부하기 때문이다 (url 속성은 허용). 위키링크는 줄을 넘지 않고, 헤딩/블록 앵커(#, ^)는 URI 대상에서 제거한다
3. **effectiveDate 게이트:** 파이프라인이 끝난 뒤, 초안의 Date가 존재하고 `effectiveDate` 이전이면 스킵하고 로그를 남긴다. Date 미상 초안은 통과시킨다 (D6). 날짜 파생이 파이프라인의 책임이므로 이 필터는 선정이 아니라 변환 이후의 게이트다.
4. **중복 검사 (D2-멱등성, D14-제목 기준):** 페이지 생성 전 `POST /v1/data_sources/{id}/query`로 Name(제목) 기준 동일 페이지 존재를 확인하고, 존재하면 스킵 후 로그를 남긴다 (Date 기준 검사는 Weekly/Monthly 시작일이 데일리 날짜와 충돌해 D14로 폐기). 노션 조회에 앞서 실행 내 제목 키 선점(mutex 집합)으로, 같은 제목으로 파생된 두 노트를 워커들이 동시에 통과시키는 경합을 차단한다. 페이지 생성의 parent는 `data_source_id`를 사용한다.
5. **본문 변환 (D4):** 마크다운을 줄 단위로 파싱하여 노션 블록으로 변환한다.
   - `# / ## / ###` → heading_1/2/3 (h4 이하는 heading_3으로 폴백)
   - `- [ ]`, `- [x]` → to_do (checked 반영)
   - `-`, `*` 시작 → bulleted_list_item (v1은 중첩 리스트를 평탄화한다)
   - `> [!type]` 헤더로 시작하는 인용 그룹 → callout (타입별 이모지 아이콘, D16)
   - 그 외 `> ` 연속 줄 → quote (D16)
   - 그 외 텍스트 → paragraph
   - **모든 블록의 텍스트는 `[]rune` 기반 2,000자 청킹을 통과한다.** 초과분은 같은 타입 또는 paragraph 블록으로 이어 붙인다.
6. **100 블록 제한 대응:** `POST /v1/pages` 생성 시 children은 최대 100개까지만 싣고, 초과분은 `PATCH /v1/blocks/{page_id}/children`으로 100개씩 나누어 append한다. append 실패 시 해당 파일을 실패로 기록한다(부분 생성된 페이지 ID를 로그에 남겨 수동 정리를 돕는다).
7. **동시성:** 워커 풀(5 workers) + 채널 큐. 모든 API 호출(query, create, append)은 공유 Limiter를 통과한다.
8. 완료 후 `state.lastMigrateRunAt`을 갱신하고 성공/스킵/실패 건수를 요약 출력한다.

### FR-3. `backup` (Notion → Obsidian, cron 주기 실행 + 수동 실행)

cron이 호출하는 명령과 수동으로 실행하는 명령은 동일하게 `etl-worker backup`이다. 별도 데몬 모드는 없으며, 개발과 테스트 시에는 터미널에서 직접 실행한다.

1. **증분 조회:** `POST /v1/data_sources/{id}/query`에 `last_edited_time >= state.lastBackupRunAt` 필터를 적용한다 (워터마크가 비어 있으면 전체 조회). `has_more`/`next_cursor` 기반 페이지네이션(100건 단위)을 반드시 처리한다.
2. **블록 수집:** 페이지마다 `GET /v1/blocks/{page_id}/children`을 페이지네이션 포함으로 호출한다. v1은 최상위 블록만 수집하고 중첩 블록은 내려가지 않는다.
3. **역변환 (D4, D16):** heading_1/2/3 → `#`/`##`/`###`, to_do → `- [ ]`/`- [x]`, bulleted_list_item → `- `, quote → `> ` 접두 줄, callout → `> [!type] ...`(아이콘 이모지를 타입으로 역매핑), paragraph → 일반 문단. 미지원 블록 타입은 rich_text의 plain_text만 추출해 문단으로 폴백하고, 추출 불가하면 건너뛰며 로그를 남긴다.
4. **파일 쓰기 (D14):** 파일명은 제목(Name) 기준이다. 제목이 양방향의 노트 정체성이라(migrate가 파일명 어간을 제목으로 보존) 원본 옵시디언 이름이 복원되고, 같은 날짜의 데일리/주간 노트가 충돌하지 않는다. 금지 문자(`/`, `:` 등)는 `-`로 치환하고, 제목이 비면 Date(`YYYY-MM-DD`), 그것도 없으면 페이지 ID로 폴백한다. Frontmatter에 노션 메타데이터를 기록한다.

   ```yaml
   ---
   notion_id: <page id>
   notion_last_edited: <last_edited_time>
   source: notion
   ---
   ```

5. **덮어쓰기 정책 (D3):** 백업 디렉토리는 노션의 미러이므로 같은 파일명이 있으면 무조건 덮어쓴다. 별도 디렉토리이므로 원본 훼손 위험이 없다.
6. 실행 시작 시각을 기억해 두었다가, 전체 처리가 끝난 뒤에만 `state.lastBackupRunAt`을 그 시각으로 갱신한다. 중간 실패 시 워터마크를 갱신하지 않아 다음 실행에서 재시도된다 (덮어쓰기라서 재처리가 안전하다).

---

## 6. 비기능 요구사항

### 6.1 Notion API 제약 준수 (최우선)

| 제약 | 값 | 대응 |
|------|-----|------|
| Rate Limit | 3 TPS | `rate.NewLimiter(rate.Limit(2.5), 3)` 단일 인스턴스를 모든 호출이 공유. 클라이언트 생성자에 내장하여 우회 불가 구조로 만든다 |
| 429 응답 | Retry-After 헤더 | 헤더의 초만큼 대기 후 재시도, 최대 3회. 초과 시 해당 항목 실패 처리 후 다음 항목 진행 |
| 텍스트 길이 | rich_text content 2,000자 | `[]rune` 청킹 (기존 `chunkText` 로직 재사용) |
| children 개수 | 생성/append 요청당 100개 | 100개 단위 분할 append |
| 조회 결과 | 페이지당 100건 | `next_cursor` 페이지네이션 필수 |
| API 버전 | `Notion-Version: 2026-03-11` | 헤더 상수화. 2025-09-03부터 데이터베이스와 데이터 소스가 분리되어, 스키마 조회·쿼리·페이지 생성은 데이터 소스 기준으로 수행한다 (2026-08-21 결정) |
| 요청 크기 | 페이로드 상한 존재 | 청킹 + 100블록 분할로 자연 해소 |

### 6.2 에러 처리와 로깅

- Idiomatic `if err != nil`. panic은 초기 설정 단계(.env, base.config.yaml 누락)에서만 허용.
- 항목 단위 경고(매핑 제외, 파일 스킵 등)는 stderr가 아니라 **실행별 로그 파일**에 남긴다: init/migrate는 `logs/migration/<실행시각>.log`, backup은 `logs/backup/<실행시각>.log` (2026-08-21 사용자 결정). 파일 단위/페이지 단위 실패는 **건너뛰고 계속(Continue)** 한다.
- 명령 자체가 실패하는 치명적 에러 한 줄은 stderr로 출력한다 (조용한 실패 방지).
- 실행 종료 시 stdout 요약 리포트: 처리/스킵/실패 건수와 로그 파일 경로.
- HTTP 클라이언트 타임아웃 10초.

### 6.3 코드 컨벤션

- Notion 요청/응답은 명시적 Struct로 정의한다. 속성처럼 스키마가 동적인 부분에 한해서만 `map[string]interface{}`를 허용한다.
- Frontmatter와 URL 파싱에 정규식을 쓰지 않고 `strings` 표준 함수를 사용한다.
- 외부 의존성은 `golang.org/x/time/rate`, YAML 파서(`gopkg.in/yaml.v3`), 유니코드 정규화(`golang.org/x/text/unicode/norm`, 한글 NFD 파일명 매칭용)로 제한한다.

---

## 7. 스케줄링

`backup`은 macOS cron으로 등록한다 (CLAUDE.md 기준). 예시:

```
0 * * * * cd ~/etl-worker && ./etl-worker backup >> logs/cron.log 2>&1
```

Mac이 잠자기 상태면 cron이 건너뛰는 한계가 있다. 증분 워터마크 방식이라 실행이 밀려도 다음 실행에서 따라잡으므로 기능상 문제는 없다. 스케줄러는 이식성을 위해 cron 유지로 확정했고(D16), launchd(macOS 전용)는 cron을 쓸 수 없는 배치에서만 대안으로 고려한다.

---

## 8. 마일스톤

| 단계 | 내용 | 완료 기준 |
|------|------|-----------|
| M1 | 골격: go.mod, 서브커맨드 라우팅, config/env 로더, Notion 클라이언트(Limiter+재시도 내장) | `etl-worker init`이 실제 DB 스키마를 조회해 latest.config.yaml 생성 |
| M2 | `migrate`: 변환 파이프라인(internal/transform: Transformer 체인, dateFrom 날짜 파생, 매핑 규칙 검증을 init에 통합) + effectiveDate 게이트 + 중복 검사 + 블록 매핑 + 100블록 분할 | 테스트 파일 수십 건을 중복 없이 이관(Type=Todo, Status=Done 적용), 재실행 시 전건 스킵 확인 |
| M3 | `backup`: 증분 쿼리 + 역변환 + 파일 쓰기 + 워터마크 | 노션에서 수정한 페이지만 다음 실행에서 갱신됨을 확인 |
| M4 | cron 등록, 로그 로테이션 없이 일자별 파일 분리, README | 무인 주기 실행 1주일 무장애 |

---

## 9. 미해결 질문 (Open Questions)

1. **Platinum DB의 실제 컬럼 구성**은 init 실행 시 API로 확인한다. 초안 코드가 가정한 `Name/Date/Type/Obsidian_URI` 4개 컬럼과 다르면 매핑 테이블에서 조정한다.
2. 백업 파일명 충돌(같은 날짜의 노션 페이지가 2개 이상)은 v1에서 `YYYY-MM-DD-2.md` 식 접미사로 처리할지, 나중 페이지가 덮어쓸지 미정. 기본은 접미사 부여로 구현한다.

### 9.1 해소된 질문

- `effectiveDate`는 "이 날짜 이후(포함)의 문서만 이관"이 맞다고 확인되었다 (2026-08-20). 초안 코드의 필터 방향을 그대로 유지한다.

## 10. 기타 추가 구현 대상
- 에러 메시지는 어플리케이션의 일부이므로 다국어 처리 필요
- Docs로서의 README.md 개선 + 아키텍처 포함할 것
- 옵시디언 플러그인으로 추가