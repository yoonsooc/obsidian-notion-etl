# task-012: 인라인 서식 + 위키링크 변환 (M2 후속)

담당: 메인 세션
상태: 완료 (2026-08-21, 재이관까지 완료: 64건 전건 성공)

> 변경 이력:
> 1. 코드리뷰(스타일 18항목 전 통과 / 정확성 4건) 반영: 위키링크 줄 경계 거부,
>    코드 스팬 보호(betweenStyled의 백틱 홀수 거부), bold/취소선/삼중 마커에도
>    공백·탭 가드 적용, 위키링크의 헤딩(#)/블록(^) 앵커를 URI 대상에서 제거.
> 2. **위키링크 표현 변경 (API 제약 발견)**: 노션 API가 본문 인라인 링크의
>    link.url에서 obsidian:// 스킴을 거부함을 실이관 실패(400 Invalid URL for
>    link)와 최소 재현으로 확인. url 속성(Obsidian_URI 컬럼)은 커스텀 스킴 허용,
>    본문 일반 텍스트도 허용. 사용자 결정으로 "밑줄 문서명 + 복사용 URI 일반
>    텍스트 병기"로 변경 (Annotations에 underline 추가).
> 3. 재이관 경과: 1차(링크 방식) 38 성공/26 실패(전부 위키링크 포함 노트, 부분
>    생성 없음) -> 표현 변경 후 2차에서 26건 생성 + 38건 중복 스킵으로 완결.
배경: 실이관 후 사용자 검수에서 발견 (2026-08-21). bold(**)/italic(*)이 리터럴로
남고, [[위키링크]]가 일반 문자열로 이관됨 (DN_260813 등). D4-본문 변환 수준이
블록 매핑까지만 정의한 스코프 공백.

## 사용자 결정

1. 인라인 서식: **bold**, *italic*, ~~취소선~~, `인라인코드` 4종을 노션
   rich_text annotations로 변환
2. [[위키링크]]: 옵시디언 URI(obsidian://open?vault=...&file=문서명) 인라인
   링크가 달린 텍스트로 변환. [[문서명|별칭]]은 별칭을 표시 텍스트로
3. 기존 64건: 이관 로그의 페이지 ID 기준으로 휴지통(in_trash) 처리 후 재이관

## 설계

- internal/notion/blocks.go: RichText에 Annotations(bold/italic/strikethrough/code)와
  Text.Link 추가. rich text 배열을 받는 생성자(NewParagraphRich 등)와
  PlainText 헬퍼 추가. 기존 문자열 생성자는 유지(위임)
- internal/markdown/inline.go: 정규식 없는 인라인 파서.
  - 우선순위: [[링크]] > `코드`(내부 서식 없음) > ~~취소선~~ > **bold** > *italic*
  - 짝이 없는 마커는 리터럴 유지. *italic*은 내부가 공백으로 시작/끝나면 리터럴
    (2*3=6 같은 오탐 방지). 중첩 허용(**굵은 *기울임***)
  - 위키링크 URI의 공백은 %20 인코딩 (transform의 escapeURIComponent와 동일 규칙)
- 청킹 재설계: rich_text 원소당 2,000자(rune) 분할 + 블록당 rich_text 원소
  100개 제한을 지키며 블록 분할. 분할 시 블록 타입과 서식 유지
- ToBlocks(body, vaultName string) 시그니처 변경 (위키링크 URI에 볼트 이름 필요)
- 재이관: 이관 로그(2026-08-21-155554.log)의 페이지 ID 64건을 PATCH로
  in_trash 처리(일회성 스크래치 스크립트, Limiter 준수) 후 migrate 재실행

## 완료 기준

- 인라인 파서 테이블 테스트 (서식 4종, 위키링크/별칭, 중첩, 짝 없음, 오탐 방지)
- 전체 테스트/vet/gofmt 통과, 코드리뷰 후 재이관 실행
