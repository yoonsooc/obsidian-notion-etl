# task-005: init 커맨드 통합

담당: 메인 세션 (task-002~004 완료 후)
상태: 완료 (실제 토큰 기반 E2E 확인은 사용자와 함께 진행 예정)

## 목표

PRD FR-1의 init 흐름을 main.go에서 조립한다.

## 흐름

1. `config.LoadNotionToken(".env")`
2. `config.LoadBase("base.config.yaml")` (경로 검증, 루프 가드 포함)
3. `notion.ExtractDatabaseID(base.Notion.DB.URL)`
4. `notion.NewClient(token).RetrieveDatabase(ctx, id)` — 401/404 시 원인 안내 후 종료
5. `vault.ScanFrontmatterKeys(base.SourceDir())`
6. 매핑 초안 생성: frontmatter 키와 노션 속성 이름을 대소문자 무시 매칭.
   title/date/url 타입 속성은 파일명에서 파생되므로 매핑 대상에서 제외.
   매칭 실패 키는 stderr로 안내.
7. `config.LoadLatest` → 기존 State 보존 → `config.SaveLatest(path, backupDir, cfg, time.Now())`
8. 요약 출력: DB 이름/속성 수, 수집된 키 수, 매핑 결과, 아카이빙 여부

## 완료 기준

- `go build -o etl-worker .` 후 `./etl-worker init` 실행이 (토큰이 유효하다는 전제에서) configs/latest.config.yaml을 생성
- 실제 E2E 확인은 .env에 NOTION_TOKEN 입력 후 사용자와 함께 진행
