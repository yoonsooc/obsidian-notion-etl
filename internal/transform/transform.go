// Package transform은 옵시디언 노트를 노션 페이지 초안으로 바꾸는
// Transformer 파이프라인을 제공한다 (PRD D8). 노트 하나가 체인을 등록
// 순서대로 한 번씩 통과하며 PageDraft가 완성된다. 이 패키지는 표준
// 라이브러리 외에 아무것도 의존하지 않으며, 조립은 상위(migrate)가 한다.
package transform

import "fmt"

// Note는 파이프라인 입력이다 (vault.Note와 필드 호환, 의존은 없음).
type Note struct {
	Filename    string            // 예: "DN_251101.md"
	RelPath     string            // target 기준 상대경로 (Obsidian URI 생성용)
	Frontmatter map[string]string // 키 -> 값
	Body        string            // frontmatter를 제외한 본문
}

// PageDraft는 파이프라인이 완성하는 노션 페이지 초안이다.
type PageDraft struct {
	Title       string            // Name(title) 속성
	Date        string            // "2006-01-02". 파생 실패 시 빈 문자열 (D6)
	ObsidianURI string            // url 속성
	Properties  map[string]string // 노션 속성 이름 -> 값 (타입 해석은 소비자 몫)
	Warnings    []string          // 파이프라인 중 발생한 항목 단위 경고 (로그용)
}

// Transformer는 파이프라인의 한 단계다. 내장 단계는 이 패키지의 생성자로
// 만들고, 커스텀 로직은 같은 인터페이스를 구현해 체인에 추가한다 (D8).
type Transformer interface {
	// Name은 로그와 에러 메시지에 쓰이는 단계 이름을 반환한다.
	Name() string
	// Transform은 노트를 읽어 초안을 채운다. error는 해당 노트 스킵 사유다.
	Transform(note Note, draft *PageDraft) error
}

// Run은 체인을 등록 순서대로 한 번씩 적용해 초안을 완성한다.
// 어느 단계가 에러를 반환하면 즉시 중단하고 단계 이름을 감싼 에러를 반환한다.
func Run(note Note, chain []Transformer) (*PageDraft, error) {
	draft := &PageDraft{Properties: make(map[string]string)}
	for _, t := range chain {
		if err := t.Transform(note, draft); err != nil {
			return nil, fmt.Errorf("%s: %w", t.Name(), err)
		}
	}
	return draft, nil
}
