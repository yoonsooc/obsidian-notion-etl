# task-001: 프로젝트 골격

담당: 메인 세션 (선행 작업, 병렬 불가)
상태: 완료

## 목표

go.mod 생성, 서브커맨드 라우팅, 기존 초안 코드 제거. 이후 태스크(002~004)가 병렬로 붙을 수 있는 토대를 만든다.

## 작업 내용

1. `go mod init github.com/yoonsooc/obsidian-notion-etl` (Go 1.26 로컬, go 지시자는 1.21 유지)
2. 의존성 추가: `golang.org/x/time/rate`, `gopkg.in/yaml.v3`
3. `main.go`: `os.Args[1]` 기반 서브커맨드 라우팅 (`init` / `migrate` / `backup`). 알 수 없는 커맨드나 인자 없음이면 usage 출력 후 exit 1. migrate/backup은 M1에서는 "not implemented" 안내만.
4. 기존 `to-notion/`, `to-obsidian/` 디렉토리 삭제 (git rm). 초안 로직은 git 히스토리에 보존되며 M2에서 internal 패키지로 재구현한다. 초안 코드는 문자열 리터럴 안에 실제 개행이 들어 있어 컴파일도 되지 않는 상태였다.
5. `.env` 로더: `internal/config`에 둔다 (task-002 계약 참조). main에서는 `config.LoadNotionToken(".env")` 호출만.

## 완료 기준

- `go build -o etl-worker .` 성공 (main 패키지가 여러 파일이 될 수 있으므로 빌드 대상은 `main.go`가 아니라 `.`으로 한다. PRD 3장의 빌드 명령은 이후 갱신)
- `./etl-worker` 실행 시 usage 출력
