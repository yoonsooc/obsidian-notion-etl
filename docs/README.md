# 개발 문서

**[CHANGELOG.md](CHANGELOG.md)에서 시작하세요** — 버전별로 무엇이 바뀌었는지와
해당 버전의 상세 기록으로 가는 링크가 있습니다.

## 구성

| 위치 | 내용 |
|------|------|
| [CHANGELOG.md](CHANGELOG.md) | 버전별 변경 요약 (진입점, git 태그와 일치) |
| [releases/](releases/) | 버전별 과정 기록. 버전 디렉토리 하위에 리뷰 문서와 기능별 task 문서 (초기 버전은 M1~M4 마일스톤 명칭으로 작성된 기록을 버전으로 재명명한 것) |
| [PRD.md](PRD.md) | **아카이브(동결)**: v1.0.0 프로토타입의 요구사항과 결정 로그 D1~D16 |
| [decisions/](decisions/) | v1.0.0 이후의 설계 결정 (ADR, D17부터 번호 연속) |
| [diagrams.md](diagrams.md) | 전체 구조와 init/migrate/backup 흐름 다이어그램 (Mermaid) |
| [review-checklist.md](review-checklist.md) | 코드리뷰 기준: Uber Go 스타일 가이드 18항목 + 프로젝트 도메인 어휘 항목 |

## 관리 정책 (2026-09-03)

- 릴리즈마다 CHANGELOG에 요약을 추가하고, 과정 기록은 `releases/v<버전>/`
  디렉토리(리뷰 문서 + 기능별 task 문서)로 남깁니다.
- 설계 결정은 `decisions/`에 ADR 한 건씩(D17~), 기록은 추가만 하고 다시 쓰지 않습니다.
- 문서 속 D번호는 결정 참조입니다: D1~D16은 PRD, D17부터는 decisions/.
