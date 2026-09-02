# 개발 문서

이 디렉토리는 프로젝트의 **개발 과정 기록**입니다. 최외곽에는 현재를 설명하는 기준 문서를,
[milestones/](milestones/)에는 시간순 과정 기록을 둡니다.

## 기준 문서 (최외곽)

| 문서 | 내용 |
|------|------|
| [diagrams.md](diagrams.md) | 전체 구조와 init/migrate/backup 흐름 다이어그램 (Mermaid) |
| [review-checklist.md](review-checklist.md) | 코드리뷰 기준: Uber Go 스타일 가이드 18항목 + 프로젝트 도메인 어휘 항목 |
| [../PRD.md](../PRD.md) | 요구사항과 결정 로그 D1~D16 — 마일스톤 문서들이 참조하는 결정의 원본 |

## 과정 기록 — [milestones/](milestones/)

| 시기 | 문서 | 내용 |
|------|------|------|
| 2026-08-21 | [M1](milestones/M1/review.md) | 골격 + init (설정·클라이언트·볼트 스캔). 태스크 문서 6건 |
| 2026-08-21 | [M2](milestones/M2/review.md) | migrate: 변환 파이프라인, 블록/인라인 변환. 태스크 문서 7건 |
| 2026-08-27~28 | [M2.5](milestones/M2.5.md) | Go 1.27 업그레이드, 플러그인 아키텍처(D11·D12) |
| 2026-08-29 | [M2.9](milestones/M2.9.md) | 문서 정합화 (PRD D11~D13 반영) |
| 2026-08-30 | [M3](milestones/M3.md) | backup: 증분 미러, 블록 역변환 (D13 첫 적용) |
| 2026-09-01 | [M3.5](milestones/M3.5.md) | 이관 범위 Work 확장, 제목 정체성(D14·D15), 중첩 가드 |
| 2026-09-02~ | [M4](milestones/M4.md) | cron 무인 실행: TCC 진단, 운영 홈 분리, 1주 관찰 |

읽는 요령: 각 마일스톤 문서는 산출물 → 설계 결정(과 그 근거) → 검증 순으로 구성됩니다.
결정에는 D번호가 붙어 있고 원문은 PRD의 Decision Log에 있습니다. M1/M2의 tasks/에는
태스크 단위의 계획·변경 이력이 남아 있습니다.

경로·계정 등 개인 정보는 공개 전환 시 자리표시자로 마스킹했습니다.
