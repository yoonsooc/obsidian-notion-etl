# task-006: 재귀 스캔 + exclude 목록 + 파일 로깅

담당: 메인 세션
상태: 완료
배경: 사용자 E2E(init) 확인에서 발견된 개선 요청 (2026-08-21)

## 요구사항 (사용자 결정)

1. 소스 디렉토리(target)의 월별 하위 디렉토리 구조 때문에 실제 데일리 노트가
   스캔되지 않았다. target 하위 **모든 파일을 재귀적으로 읽되**, base.config.yaml의
   `exclude` glob 패턴 목록(gitignore처럼 제외 대상 지정)에 걸리는 파일만 뺀다.
2. 수동 명령(init/migrate)의 항목 단위 경고는 stderr 대신
   **logs/migration/ 아래 실행별 로그 파일**에 남긴다. 명령 실패의 치명적 에러
   한 줄은 stderr에도 유지한다. backup(M3)은 logs/backup/을 사용한다.

## 설계

- `internal/logging`: `New(dir, now)` -> logs/<dir>/2006-01-02-150405.log 생성,
  `Warnf`, `Path()`, `Close()`. 로그 파일 생성 실패 시 에러 반환(명령 중단).
- `internal/vault.ScanFrontmatterKeys(dir string, exclude []string, warn io.Writer)`:
  WalkDir 재귀 탐색. `.md`만 대상. exclude 패턴은 target 기준 상대경로(슬래시 구분)와
  파일명(basename) 양쪽에 `path.Match`로 적용, 하나라도 걸리면 제외.
- 한글 경로/파일명은 디스크에 NFD로 저장되므로, 매칭 전에 상대경로·파일명·패턴을
  전부 NFC로 정규화한다 (`golang.org/x/text/unicode/norm` 의존성 추가).
- `internal/config.VaultTarget`에 `Exclude []string` 추가. LoadBase에서
  `path.Match` 문법 검증(ErrBadPattern이면 설정 에러).
- migrate(M2)의 파일 선정도 같은 재귀 탐색 + exclude를 쓰되, 파일명 날짜 형식
  (YYYY-MM-DD.md) 필터는 유지한다 (PRD FR-2).

## 완료 기준

- 하위 디렉토리의 노트 frontmatter 키가 수집된다 (재귀).
- exclude에 넣은 한글 파일명(NFC 표기)이 NFD 디스크 파일과 매칭되어 제외된다.
- init 실행 시 경고가 logs/migration/*.log에 기록되고 stderr에는 나오지 않는다.
- gofmt / go vet / go test 전체 통과.
