# Changelog

버전별 변경 이력입니다. 각 버전의 상세 과정 기록은 [releases/](releases/)의 해당 문서에,
설계 결정(D번호)의 원문은 [PRD](PRD.md)의 Decision Log(D1~D16, 동결)와
[decisions/](decisions/)(D17~)에 있습니다. 버전은 git 태그와 일치합니다.

## v1.1.0 — 2026-09-16 · help 명령과 backup 스케줄 관리 ([상세](releases/v1.1.0.md))

- `etl-worker help` (-h/--help): 명령·플래그·대표 흐름을 안내하는 전역 도움말
  (영문 기본, `--lang=ko`로 한국어)
- backup 표면 재정의 (D17): 무옵션은 수동 1회 실행, 자동화는 crontab 셀프 등록
  플래그(`--schedule[=<크론식>]`/`--unschedule`/`--status`)로 관리. 크론식은 생략 시
  매시 정각이며 5필드 사전 검증. 상주 데몬은 비채택
- 등록 라인은 절대경로 인용 형태로 생성, 레거시 수기 등록 라인도 감지·관리

## v1.0.0 — 2026-09-03 · 프로토타입 완성 ([상세](releases/v1.0.0.md))

- cron 무인 주기 실행: 매시 정각 backup, macOS TCC 진단(dyld 단계 무한 대기)과
  운영 홈 분리(TCC 비보호 경로)로 전체 디스크 접근 권한 없이 동작
- 인용·콜아웃 양방향 매핑 (D16): `> ` → quote, `> [!type]` → callout(이모지 아이콘),
  역변환 포함. 전량 재이관(83건)으로 실데이터 왕복 검증
- README Todo 소거(스케줄러 cron 확정, 인라인 확장 종결), 개발 문서 공개(docs/, PRD)
- fix: fresh clone에서 configs/ 부재 시 init 실패 (SaveLatest 부모 디렉토리 생성)

## v0.5.0 — 2026-09-01 · 이관 범위 Work 확장 ([상세](releases/v0.5.0.md))

- migrate 대상을 Daily에서 Work 전체(Daily/Weekly/Monthly)로 확장 (D15).
  Monthly는 레이아웃 선언, Weekly는 커스텀 Transformer로 시작일 파생
- 노트 정체성을 제목으로 통일 (D14): 중복 검사·백업 파일명 모두 제목 기준,
  같은 날짜의 데일리/주간 충돌 해소, 백업이 원본 파일명 복원
- 백업-소스 중첩 가드: 설정 검증이 조상 경로 비교로 재이관 루프를 차단

## v0.4.0 — 2026-08-30 · backup 커맨드 ([상세](releases/v0.4.0.md))

- Notion → Obsidian 증분 백업: last_edited_time 워터마크(전건 성공 시에만 전진),
  블록 → 마크다운 역변환, 무조건 덮어쓰기 미러 (D3·D4)
- 서브커맨드를 internal/cli 패키지로 분리, main은 라우팅만

## v0.3.1 — 2026-08-29 · 문서 정합화 ([상세](releases/v0.3.1.md))

- PRD 결정 로그 D11~D13 추가, 문서를 플러그인 구조로 현행화 (코드 변경 없음.
  당시 문서가 비공개여서 대응 커밋 없음)

## v0.3.0 — 2026-08-28 · 플러그인 아키텍처 ([상세](releases/v0.3.0.md))

- 변환 정책을 Plugin 인터페이스 구현체(plugin/)로 분리, 접합부 일원화 (D11)
- 내장 default 플러그인: 플러그인 코드 없이 yaml 규칙으로 동작 (D12)
- 도메인 어휘 규칙 확립 (D13), Go 1.27 업그레이드

## v0.2.0 — 2026-08-22 · migrate 커맨드 ([상세](releases/v0.2.0/review.md))

- Obsidian → Notion 일괄 이관: Transformer 파이프라인(D8), 날짜 파생·속성 매핑,
  블록/인라인 변환(볼드·이탤릭·위키링크), 2,000자 청킹, 100블록 분할, 멱등(D2)
- 실볼트 64건 이관과 재실행 멱등성 검증. 태스크 문서 7건(tasks/)

## v0.1.0 — 2026-08-21 · 골격 + init ([상세](releases/v0.1.0/review.md))

- 단일 바이너리 + 서브커맨드 골격(D1), Rate Limiter 내장 Notion 클라이언트,
  볼트 재귀 스캔과 frontmatter 파싱, init 검증·스냅샷. 태스크 문서 6건(tasks/)
