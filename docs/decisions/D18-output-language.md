# D18: 출력 언어 정책 — 에러·로그 영어 정본, stdout만 지역화

날짜: 2026-09-16
상태: 채택

## 맥락

실행 결과(stdout 요약)와 로그가 fmt 기반 한글 고정이었다. 도구가 공개된 이상 기본
표면은 영어가 바람직하고, 사용자는 한국어 stdout을 저장 가능한 설정으로 선택하고
싶어 했다. 로그와 에러에는 에러 값이 %v로 섞여 들어가므로, 로그만 영어화하는 것은
불가능하고 에러 값의 영문화가 전제된다.

## 결정

- **에러 값(93건)·로그 메시지(18건)·변환 경고(4건)는 영어 단일 정본**으로 전환한다.
  Go 관례(소문자 시작, 마침표 없음)를 따르고, 이후에도 이 층위에 한국어를 넣지 않는다.
- **stdout 사용자 대면 메시지(요약·안내·usage)만 지역화**한다. 메커니즘은 이미
  의존성에 있는 `golang.org/x/text/message`이며, 영어 원문이 카탈로그 키이고 한국어는
  수동 등록(SetString)한다. 등록은 sync.Once로 수행해 init() 금지 컨벤션을 지킨다.
  카탈로그에 없는 키는 영어로 폴백하므로 번역 누락은 기능 저하가 아니다.
- **언어 선택 우선순위: `ETL_LANG` 환경변수 > base.config.yaml `lang` 키 > 영어.**
  base.config.yaml은 런타임 파일이라 빌드 없이 수정된다(설정 저장 요구 충족).
  미지원 값은 에러가 아니라 영어 폴백이다(표시 취향이 실행을 막으면 안 됨).
- **cron 경로는 영어 고정**: `--schedule`이 생성하는 crontab 라인에 `ETL_LANG=en`을
  포함한다. cron의 stdout은 cron.log로 가는 로그이므로 설정이 ko여도 영어가 유지된다.
- 스케줄 관리 플래그(--schedule 등)는 설정을 로드하지 않으므로 ETL_LANG만 따른다.
- help는 기존 `--lang` 플래그가 최우선이고, 없으면 ETL_LANG, 없으면 영어다.

## 근거와 대안

- 에러·로그 영어 단일화: grep·모니터링·`errors.Is` 계열 매칭이 언어에 흔들리지 않고,
  오픈소스 관례와 일치한다 (셀프 리뷰 가이드 Q6의 확정).
- go-i18n 등 서드파티 대신 x/text/message: 신규 의존성 없음, 표준 관례, 문자열 규모
  (약 18키)에 충분. gotext 추출 도구는 규모상 불필요해 수동 카탈로그로 유지.
- base.config.yaml lang의 cron 부작용(한글 cron.log)은 설정 우선순위(env > config)와
  cron 라인의 ETL_LANG=en 고정 조합으로 해소했다.

## 영향

- 전 패키지 문자열 스윕(에러 93·로그 18·경고 4)과 한글 단언 테스트 갱신.
- internal/cli에 lang.go(NewPrinter, FprintUsage)·catalog_ko.go 신설. usage는 cli로
  이동해 main과의 문자열 중복(드리프트)을 제거.
- config.BaseConfig.Lang 추가(en/ko 외 값은 로드 시 에러 — strict 철학), cronLine에
  ETL_LANG=en. 상세: [releases/v1.2.0.md](../releases/v1.2.0.md).
