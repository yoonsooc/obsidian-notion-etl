# task-007: 설정 확장 (dateFrom, mapping 규칙) + 엄격 파싱

담당: 병렬 에이전트 A
상태: 완료 (단, 아래 변경 이력 참조 — 이후 결정으로 방향이 크게 바뀜)

> **변경 이력 (2026-08-21, D10-변환 규칙의 위치 결정으로 방향 반전)**
>
> 이 태스크는 "변환 규칙을 base.config.yaml에 두고 로드·검증한다"는 전제로
> 구현됐으나, 이후 사용자 결정(D10)으로 규칙이 코드(pipeline.go)로 이동하면서
> 아래와 같이 재편됐다. 이 문서의 API 계약 중 일부는 더 이상 현재 코드와 일치하지 않는다.
>
> - **제거됨**: `VaultTarget.DateFrom`, `BaseConfig.Mapping` yaml 필드.
>   base.config.yaml은 환경 정보(경로, DB, exclude)만 담는다.
> - **유지·전용됨**: `DateRule`/`MappingEntry` 타입과 `ValidateMapping`은 그대로
>   살아남아, pipeline.go의 코드 규칙(dailyDateRules/platinumMapping)을 정의·검증하는
>   타입으로 쓰인다. `validateDateRules`는 init이 호출할 수 있게 `ValidateDateRules`로
>   export됐다. `LatestConfig.Mapping`/`LatestConfig.DateFrom`은 기록용 스냅샷으로 유지.
> - **유지·강화됨**: KnownFields 엄격 파싱은 그대로이며, 이제 낡은 설정에 남은
>   dateFrom/mapping 키를 미지 키로 거부해 D10 이행을 강제하는 역할을 겸한다.
> - **M2 리뷰 반영**: ValidateMapping에 v1 지원 타입 화이트리스트(select/status/rich_text)
>   추가 (M2 리뷰 4번-미지원 타입 통과). 실파일 결합 테스트의 규칙 개수 단언 제거
>   (M2 리뷰 2번-테스트 실패).
선행: 없음 (M1 config 패키지 위에 추가)

## 목표

base.config.yaml의 변환 규칙(dateFrom, mapping)을 정식 필드로 로드·검증하고,
설정 오타를 감지하도록 엄격 파싱을 켠다. M1 2차 리뷰 이월 항목 #2(KnownFields),
#1·#4(매핑 충돌 경고)의 해소를 포함한다.

## API 계약 (internal/config에 추가)

```go
// DateRule은 dateFrom 체인의 한 단계다. 두 필드 중 정확히 하나만 설정된다.
type DateRule struct {
    FileLayout     string `yaml:"fileLayout,omitempty"`     // Go 시간 레이아웃 (예: 'DN_060102'). 파일명(확장자 제외)에 적용
    FrontmatterKey string `yaml:"frontmatterKey,omitempty"` // frontmatter 키. 값은 2006-01-02 우선, 실패 시 RFC3339 시도
}

// VaultTarget에 필드 추가:
//   DateFrom []DateRule `yaml:"dateFrom,omitempty"`

// MappingEntry 확장 (기존 Frontmatter, NotionProperty 유지):
type MappingEntry struct {
    Frontmatter    string            `yaml:"frontmatter,omitempty"`
    NotionProperty string            `yaml:"notionProperty"`
    Value          string            `yaml:"value,omitempty"`   // 고정값. 설정 시 Frontmatter/Values/Default는 비어야 함
    Values         map[string]string `yaml:"values,omitempty"`  // frontmatter 값 -> 노션 값 변환 테이블
    Default        string            `yaml:"default,omitempty"` // Values에 없거나 frontmatter 값이 빈 경우의 폴백
}

// BaseConfig에 필드 추가:
//   Mapping []MappingEntry `yaml:"mapping,omitempty"`

// ValidateMapping은 매핑 규칙을 실제 노션 스키마와 대조한다 (D5: init은 검증자).
// 반환된 문자열들은 경고이며 warn 로그로 안내된다. 에러는 실행 중단 사유.
// 검증 내용:
//   - notionProperty가 properties에 존재 (대소문자 정확 일치 우선, 무시 일치는 경고 후 수용)
//   - Value와 Frontmatter가 동시에 설정되면 에러, 둘 다 없어도 에러
//   - 같은 notionProperty를 가리키는 엔트리가 2개 이상이면 경고 후 뒤 엔트리 무시
//   - title/date/url 타입 속성을 가리키면 에러 (파이프라인이 파일명에서 파생)
func ValidateMapping(mapping []MappingEntry, properties []Property) (warnings []string, err error)

// LoadBase의 validate에 추가:
//   - DateFrule 각 단계: 두 필드 중 정확히 하나만 설정. FileLayout은 time.Parse(layout, layout 스스로 포맷한 기준시각)으로 유효성 검사
//   - yaml 파싱을 yaml.Decoder + KnownFields(true)로 전환. 미지 키는 에러
```

## 주의

- KnownFields(true) 전환 시 기존 base.config.yaml의 모든 실제 키(exclude, dateFrom, mapping 포함)가 구조체에 있어야 한다. 실제 파일(`<저장소>/base.config.yaml`)로 로드 성공을 테스트하라 (경로 검증은 실볼트가 필요하므로 파싱 단계만 분리 테스트해도 좋다).
- LoadLatest는 KnownFields를 켜지 않는다 (구버전 latest와의 호환).
- latest.config.yaml 스냅샷에 mapping과 dateFrom도 저장되도록 LatestConfig에 필드 추가 (`Mapping []MappingEntry`는 이미 있음, `DateFrom []DateRule` 추가).

## 제약

- internal/config/ 밖 수정 금지 (docs의 이 문서 상태 줄 제외). go.mod 수정 금지, go mod tidy 금지.
- docs/review-checklist.md 18항목 준수. 테이블 기반 테스트 필수.

## 완료 기준

- gofmt -l internal/config/ 비어 있음, go vet, go test ./internal/config/ 통과
