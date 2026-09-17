# task-011: migrate 커맨드 통합 + init 검증자 전환

담당: 메인 세션 (task-007~010 완료 후)
상태: 완료 (2026-08-21, 실볼트 dry-run 64건 경고 0건 확인. 실제 이관 실행은 리뷰 후 사용자와 진행)

> 구현 노트: --dry-run 플래그 추가 (생성·state 갱신 없이 중복 검사와 변환까지 수행).
> vault.CollectNotes 신설 (ScanFrontmatterKeys가 재사용). dateFrom에 '060102' 레이아웃
> 추가 (DN_ 접두사 없는 YYMMDD 파일명 실데이터 대응).
>
> 변경 이력 (2026-08-21, D10-변환 규칙의 위치 + M2 리뷰 반영):
> - 이 문서 2항의 "체인 조립"과 3항의 규칙 출처가 latest 스냅샷에서 pipeline.go의
>   buildPipeline(코드 규칙)으로 변경됨. latest의 mapping/dateFrom은 기록용.
> - init 검증자 전환도 base.Mapping이 아니라 pipeline.go의 커스텀 매핑 함수/
>   dailyDateRules()를 검증하는 방식으로 구현됨.
> - 중복 검사 앞에 실행 내 키 선점(claim)이 추가됨 (M2 리뷰 5번-검사·생성 경합).
> - frontmatter 키 검증이 정확 일치 기준 + 대소문자 불일치 별도 경고로 강화됨
>   (M2 리뷰 7번), 매핑 이름 해석도 정확 일치 우선으로 통일 (M2 리뷰 3번).

## 목표

PRD FR-2의 migrate 흐름을 조립하고, init을 D5(검증자)로 전환한다.

## 흐름

1. init 전환: 자동 이름 매칭(buildMapping) 제거. base의 mapping을
   config.ValidateMapping으로 검증하고 경고는 로그로, 결과를 latest에 스냅샷.
2. migrate: 설정 로드 -> latest 로드(없으면 init 먼저 하라고 에러) ->
   vault 재귀 스캔(+exclude)으로 노트 수집 -> 워커 풀 5개:
   노트 파싱 -> transform.Run(체인: DateDeriver, TitleFromFilename, ObsidianURI, PropertyMapper)
   -> effectiveDate 게이트 -> ExistsByDate/ExistsByTitle 중복 검사 -> markdown.ToBlocks
   -> CreatePage(+분할 append)
3. draft.Properties의 타입 해석: latest의 properties 스키마에서 이름으로 타입을 찾아
   notion.PropertyValue{Type, Value} 구성. Name(title), Date(date), Obsidian_URI(url)는
   draft의 전용 필드에서 채운다.
4. 완료 후 state.firstRunAt(비어 있을 때만)과 state.lastMigrateRunAt 갱신,
   처리/스킵(중복/게이트)/실패 건수와 로그 경로 요약 출력. 경고·실패는 logs/migration/.

## 완료 기준

- 실볼트 대상 dry-run 성격의 소규모 검증 후 사용자와 함께 전체 실행
- 재실행 시 전건 스킵 (멱등성)
