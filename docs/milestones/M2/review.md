# M2 리뷰 (migrate + 변환 파이프라인)

기간: 2026-08-21 (1일)
상태: 완료. 실제 이관 64건 성공(경고 0), 재실행 멱등성(전건 중복 스킵) 검증 완료

## 산출물

- `pipeline.go`: 변환 정책의 유일한 출처 (D10-변환 규칙의 위치). dailyDateRules(DN_060102 → 060102 → created_date), platinumMapping(Type=Todo, Status=Done), buildPipeline, 커스텀 Transformer nfcTitle(제목 NFC 정규화)
- `migrate.go`: FR-2 전체 흐름 조립. 워커 풀 5개, --dry-run 플래그, 실행 내 중복 키 선점(claim), effectiveDate 게이트, state 워터마크
- `internal/transform`: Transformer 파이프라인과 내장 4종 (표준 라이브러리만 의존)
- `internal/markdown`: 마크다운 -> 노션 블록 (heading/todo/bullet/paragraph, 2,000자 rune 청킹)
- `internal/notion`: blocks.go(블록 타입·생성자·청킹), pages.go(중복 쿼리, 페이지 생성 100블록 분할, append)
- `internal/config`: DateRule/MappingEntry 타입, ValidateMapping(지원 타입 화이트리스트)·ValidateDateRules, KnownFields 엄격 파싱
- `internal/vault`: CollectNotes(재귀 수집, ScanFrontmatterKeys가 재사용)
- init 검증자 전환 (D5-설정 역할 분리): 코드 규칙을 실제 스키마·노트와 대조
- 태스크 문서: task-007 ~ task-011 (docs/milestones/M2/tasks/, 변경 이력 포함)

## 주요 설계 변경 (마일스톤 중 사용자 결정)

- **D10-변환 규칙의 위치**: 날짜 파생·속성 매핑 규칙을 base.config.yaml에서 코드(pipeline.go)로 이동. 설정은 환경 정보만 담고, 엄격 파싱이 낡은 규칙 키를 거부한다. task-007의 원래 방향(설정 기반 규칙)이 반전된 결정이며, 상세는 task-007 문서의 변경 이력 참조

## 코드리뷰 결과

- **스타일**: docs/review-checklist.md 18항목 전 항목 통과, 위반 0건
- **정확성**: 지적 9건 + 부수 2건, 사용자 결정으로 전건 수정

| # | 심각도 | 내용 | 수정 |
|---|--------|------|------|
| 1 | High | Obsidian_URI 공백이 `+`로 인코딩되어 링크 깨짐 | 공백을 `%20`으로 치환하는 escapeURIComponent |
| 2 | Med-High | 실파일 결합 테스트의 규칙 개수 하드코딩으로 전체 테스트 실패 | 엄격 파싱 통과만 검사하도록 완화 |
| 3 | Medium | 매핑 이름 해석이 정확 일치를 무시(검증과 불일치) | 정확 일치 우선, 없을 때만 fold 조회 |
| 4 | Medium | 미지원 타입(checkbox 등) 매핑이 init 통과 후 migrate 전 건 실패 | ValidateMapping에 지원 타입 화이트리스트 |
| 5 | Medium | 중복 검사-생성 사이 워커 경합(TOCTOU) | 실행 내 키 선점(claim, mutex 집합) |
| 6 | Med-Low | base/latest 규칙 신선도 혼용 | D10으로 규칙이 코드로 이동하며 자연 해소 |
| 7 | Low | init 키 검증(대소문자 무시)과 실제 조회(정확 일치) 불일치 | 정확 일치 기준 + 대소문자 불일치 별도 경고 |
| 8 | Low | 빈 체크박스 줄(`- [ ]`)이 불릿으로 오변환 | 후행 공백 없는 마커도 to_do로 인정 |
| 9 | Low | 한글 제목 NFD로 인한 중복 검사 멱등성 위험 | nfcTitle 커스텀 Transformer (D8-변환 아키텍처의 첫 코드 플러그인) |
| 부수 | - | 부분 실패 로그 문구 중복, 실패 시 로그 경로 이중 출력 | 정리 |

## M1 이월 항목 처리

1. buildMapping 대소문자 충돌 → ValidateMapping의 중복 경고 + 리뷰 3번 수정으로 해소
2. KnownFields + MappingEntry value 필드 → 해소 (task-007)
3. init 검증자 전환 → 해소
4. 대소문자만 다른 속성 공존 시 매핑 소실 → 리뷰 3번 수정으로 해소

## 실전 검증

- dry-run: 대상 64건 전부 이관 가능, 경고 0. 이 과정에서 DN_ 접두사 없는 YYMMDD 파일명 실데이터를 발견해 날짜 규칙에 '060102' 추가
- 실제 이관 (2026-08-21 15:55): **64건 전부 성공, 실패 0, 경고 0.** state.firstRunAt/lastMigrateRunAt 기록됨
- 멱등성 재실행 (15:57): **이관 0, 중복 스킵 64** — D2-멱등성 충족
- `go test -race -count=1 ./...` 전 패키지 통과, gofmt/vet 깨끗

## 후속: task-012 인라인 서식 + 위키링크 (같은 날 사용자 검수로 추가)

- 사용자 검수에서 bold/italic이 리터럴 `*`로, [[위키링크]]가 일반 문자열로 남는 것을 발견 (D4-본문 변환 수준의 스코프 공백)
- 인라인 파서 추가: bold/italic/취소선/인라인코드 → annotations, 청킹을 블록 분할에서 원소 분할로 개선(문단이 갈라지지 않음)
- 리뷰: 스타일 18항목 전 통과, 정확성 4건(줄 경계, 코드 스팬 보호, 공백 가드, 앵커 제거) 전건 수정
- **API 제약 발견**: 본문 인라인 링크는 obsidian:// 스킴 거부(400). 위키링크는 밑줄 문서명 + 복사용 URI 텍스트로 표현 (사용자 결정)
- 재이관: 기존 64건 휴지통(로그의 페이지 ID 기준, 실패 0) 후 재실행. 1차 38성공/26실패(링크 400) → 표현 변경 후 2차에서 전건 완결

## 남은 확인 사항

- 노션에서 눈으로 확인 권장: 페이지의 Type=Todo/Status=Done 값, 본문 블록 렌더링, 그리고 Obsidian_URI 클릭 시 실제 노트가 열리는지 (URI 인코딩 수정의 최종 확인은 실제 클릭만이 가능)
- M3(backup)로 이월: logs/backup/ 로깅 구조 재사용, exclude/증분 워터마크 활용
