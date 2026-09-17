# v0.1.0 — M1 리뷰 (골격 + init 설정 부트스트랩)

기간: 2026-08-21 (1일)
상태: 완료 (사용자 승인, 실제 노션 DB·볼트 E2E 동작 확인)

## 산출물

- 단일 바이너리 `etl-worker` (init / migrate / backup 서브커맨드 라우팅, migrate·backup은 스텁)
- `internal/config`: base/latest 설정 로드·검증·원자적 저장·아카이빙, .env 로딩
- `internal/notion`: Rate Limiter(2.5 TPS, burst 3) 내장 클라이언트, 429 재시도(Retry-After, 최대 3회, 60초 클램프), API 2026-03-11 (데이터베이스/데이터 소스 분리 대응)
- `internal/vault`: 재귀 스캔 + exclude glob(조상 디렉토리 매칭, NFC/NFD 정규화), frontmatter 파서(따옴표 키·키 내 콜론 오타 대응)
- `internal/logging`: 실행별 로그 파일 (logs/migration/), 빈 로그 자동 정리
- 태스크 문서: task-001 ~ task-006 (docs/releases/v0.1.0/tasks/)
- 테스트: config 6개 함수 / notion 8개 함수 / vault 5개 함수 / logging 3개 함수, 전체 통과

## 1차 코드리뷰 (docs/review-checklist.md 18항목 + 정확성)

지적 7건, 전건 수정 완료 (사용자 승인):

| # | 심각도 | 내용 | 수정 |
|---|--------|------|------|
| 1 | High | 루프 방지 가드가 macOS 대소문자/NFD 차이에 뚫림 | os.SameFile 물리 동일성 재검사 추가 |
| 2 | Med-High | latest.config.yaml 비원자적 쓰기 | 임시 파일 + rename |
| 3 | Medium | frontmatter `---` 구분자가 줄 단위가 아님 | 줄 단위 판정으로 재작성 |
| 4 | Low | main의 os.Exit 3곳 | run(args) error 패턴, 종료 지점 1곳 |
| 5 | Low | 아카이빙 여부 미출력 | SaveLatest가 (archived, err) 반환 |
| 6 | Low | `#` 주석 줄이 키로 수집 | 주석 줄 스킵 |
| 7 | Low | 같은 초 아카이브 덮어쓰기 | O_EXCL + 접미사 |

## 중간 변경 (사용자 결정)

- **Notion API 2022-06-28 → 2026-03-11 상향**: RetrieveDatabase는 데이터 소스 참조 반환, RetrieveDataSource 신설, latest에 dataSourceId 저장, 단일 데이터 소스만 지원(0/다중이면 에러)
- **task-006**: 재귀 스캔 + exclude 목록(gitignore식) + 실행별 파일 로깅. E2E에서 발견된 월별 하위 디렉토리 문제의 해결
- **파서 버그 수정**: 실데이터의 `"docu_type:": Plan`(따옴표 안 콜론 오타 키) 대응

## 2차 코드리뷰

- **스타일**: 18항목 전 항목 통과, 위반 0건. 1차 수정의 회귀 없음
- **정확성**: 지적 5건 + 참고 1건. 사용자 결정에 따라 1·2·3·5 수정, 4·6은 M2 이월

수정 완료 (1·2·3·5):

| # | 심각도 | 내용 | 수정 |
|---|--------|------|------|
| 1 | Medium | exclude가 1단계 glob (gitignore 의미론 불일치) | 조상 디렉토리 경로 매칭 추가 + WalkDir SkipDir 가지치기 |
| 2 | Low | init 실패 시 빈 로그 누적, 로그 경로 미안내 | 무기록 시 Close가 파일 제거, 실패 시 stderr에 경로 안내 |
| 3 | Low | 짝 없는 따옴표 오타 키 오염 | 폴백 경로에서 키 양끝 따옴표 제거 |
| 5 | Low | 429 Retry-After 무상한 대기 | 60초 클램프 |

## M2 이월 항목

1. **buildMapping 대소문자 충돌 미처리** (2차 리뷰 #4): 매핑이 base 기반 검증(D5)으로 재설계되면서 함께 처리. 같은 노션 속성을 가리키는 매핑 중복과 속성 이름 충돌에 경고 필요
2. **설정 오타 감지** (2차 리뷰 #6): dateFrom/mapping 필드를 구조체에 추가한 뒤 yaml KnownFields(true) 활성화. MappingEntry에 고정값(value) 필드 추가 (D9 표현용)
3. **init의 매핑 생성 방식 전환**: 현재는 이름 자동 매칭 초안 생성(작성자), D5 확정에 따라 base 규칙 검증(검증자)으로 변경
4. **대소문자만 다른 노션 속성 공존 시 매핑 소실** (2차 스타일 리뷰 관찰): 발생 가능성 낮음, 매핑 재설계 시 경고 추가 검토

## 검증 상태

- `go build -o etl-worker .` / `gofmt -l .`(출력 없음) / `go vet ./...` / `go test -count=1 ./...` 전체 통과
- E2E: 실제 노션 DB(속성 5개)와 실볼트로 `./etl-worker init` 성공. frontmatter 키(category, docu_type) 정상 수집, 설정 변경 시 아카이빙 동작, 경고의 로그 파일 기록 확인
