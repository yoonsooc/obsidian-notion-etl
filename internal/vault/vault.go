// Package vault는 옵시디언 볼트의 마크다운 노트 스캔과
// YAML Frontmatter 파싱을 담당한다. 정규식 없이 strings 표준 함수만 사용한다.
package vault

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Note는 파싱된 마크다운 노트 한 건이다.
type Note struct {
	Filename    string            // 예: "2025-11-05.md"
	RelPath     string            // 스캔 루트 기준 상대경로 (슬래시 구분). CollectNotes가 채운다
	Frontmatter map[string]string // 키 -> 값 (값의 양끝 따옴표 제거됨)
	Body        string            // frontmatter를 제외한 본문 (TrimSpace 적용)
}

// ParseNote는 노트 내용에서 frontmatter와 본문을 분리한다.
// frontmatter가 없으면 Frontmatter는 빈 맵, Body는 전체 내용이다.
// 파싱 규칙: 첫 줄이 정확히 "---"이고 닫는 "---" 줄이 있을 때만 frontmatter로
// 취급하며(줄 단위 판정이라 값 안의 "---"나 수평선 "----"에 오동작하지 않는다),
// 그 사이 줄들에서 "key: value" 형태만 수집한다.
// 중첩 YAML과 리스트 값은 M1 범위 밖이므로 무시하되 키는 수집한다.
func ParseNote(filename, content string) Note {
	note := Note{
		Filename:    filename,
		Frontmatter: make(map[string]string),
		Body:        strings.TrimSpace(content),
	}

	lines := strings.Split(content, "\n")
	// 첫 줄이 정확히 "---"일 때만 frontmatter가 있는 문서다.
	if trimLineEnd(lines[0]) != "---" {
		return note
	}

	// 정확히 "---"인 닫는 줄을 찾는다. 없으면 frontmatter 없음으로 취급한다.
	closing := -1
	for i := 1; i < len(lines); i++ {
		if trimLineEnd(lines[i]) == "---" {
			closing = i
			break
		}
	}
	if closing < 0 {
		return note
	}

	note.Frontmatter = parseFrontmatter(strings.Join(lines[1:closing], "\n"))
	note.Body = strings.TrimSpace(strings.Join(lines[closing+1:], "\n"))
	return note
}

// trimLineEnd는 줄 끝의 캐리지 리턴(CRLF 문서 대응)과 공백을 제거한다.
func trimLineEnd(line string) string {
	return strings.TrimRight(line, "\r \t")
}

// parseFrontmatter는 frontmatter 블록에서 "key: value" 줄들을 맵으로 수집한다.
func parseFrontmatter(block string) map[string]string {
	fm := make(map[string]string)
	for line := range strings.SplitSeq(block, "\n") {
		line = strings.TrimSpace(line)
		// 빈 줄, 리스트 항목("- a"), YAML 주석("# ...")은 키가 아니므로 건너뛴다.
		if line == "" || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := splitKeyValue(line)
		if !ok || key == "" {
			continue
		}
		fm[key] = trimQuotes(value)
	}
	return fm
}

// splitKeyValue는 frontmatter 줄에서 키와 값을 분리한다.
// 키가 따옴표로 감싸인 경우(`"docu_type:": Plan`처럼 따옴표 안에 콜론이
// 있을 수 있다) 따옴표 안 전체를 키로 취급하고, 그 외에는 값의 콜론을
// 보존하기 위해("time: 10:30") 첫 콜론만 구분자로 쓴다.
// 실무 데이터에서 키 끝에 붙은 콜론은 작성 도구의 오타이므로 제거한다.
func splitKeyValue(line string) (key, value string, ok bool) {
	if quote := line[0]; quote == '"' || quote == '\'' {
		if end := strings.IndexByte(line[1:], quote); end >= 0 {
			rest := strings.TrimSpace(line[end+2:])
			if after, found := strings.CutPrefix(rest, ":"); found {
				key = strings.TrimSuffix(line[1:end+1], ":")
				return key, strings.TrimSpace(after), true
			}
		}
		// 짝이 맞지 않거나 뒤에 콜론이 없으면 일반 규칙으로 폴백한다.
	}
	k, v, found := strings.Cut(line, ":")
	if !found {
		return "", "", false
	}
	// 짝이 맞지 않는 따옴표 오타(`"created: ...`)도 키를 오염시키지 않도록
	// 키 양끝의 따옴표 문자를 전부 벗긴다.
	key = strings.TrimSuffix(strings.Trim(strings.TrimSpace(k), `"'`), ":")
	return key, strings.TrimSpace(v), true
}

// trimQuotes는 값 양끝의 짝이 맞는 작은/큰따옴표를 한 겹 제거한다.
func trimQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) ||
		(strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`)) {
		return s[1 : len(s)-1]
	}
	return s
}

// CollectNotes는 dir 하위의 *.md 파일들을 재귀적으로 읽어 파싱된 노트
// 목록을 돌려준다. exclude의 glob 패턴에 걸리는 파일은 gitignore처럼
// 대상에서 제외한다 (Excluded 참조). 개별 파일 읽기 실패는 warn에 경고를
// 남기고 건너뛴다 (PRD 6.2 Continue 정책). 결과는 상대경로 오름차순이다.
func CollectNotes(dir string, exclude []string, warn io.Writer) ([]Note, error) {
	var notes []Note

	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// 루트 자체를 읽지 못하면 스캔이 무의미하므로 에러로 중단한다.
			if p == dir {
				return err
			}
			fmt.Fprintf(warn, "vault: skip %s: %v\n", p, err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			rel = d.Name()
		}

		// 제외 대상 디렉토리는 하위 전체를 가지치기해 탐색 비용을 줄인다.
		if d.IsDir() {
			if p != dir && Excluded(rel, exclude) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		if Excluded(rel, exclude) {
			return nil
		}

		content, readErr := os.ReadFile(p)
		if readErr != nil {
			fmt.Fprintf(warn, "vault: skip %s: %v\n", p, readErr)
			return nil
		}
		note := ParseNote(d.Name(), string(content))
		note.RelPath = filepath.ToSlash(rel)
		notes = append(notes, note)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("scan dir %s: %w", dir, walkErr)
	}
	// WalkDir는 사전식 순회지만 명시적으로 정렬해 결정적 순서를 보장한다.
	sort.Slice(notes, func(i, j int) bool { return notes[i].RelPath < notes[j].RelPath })
	return notes, nil
}

// ScanFrontmatterKeys는 dir 하위의 *.md 파일들을 재귀적으로 읽어
// 등장하는 frontmatter 키의 중복 제거·정렬된 목록을 돌려준다.
// 대상 선정 규칙은 CollectNotes와 같다.
func ScanFrontmatterKeys(dir string, exclude []string, warn io.Writer) ([]string, error) {
	notes, err := CollectNotes(dir, exclude, warn)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	for _, note := range notes {
		for key := range note.Frontmatter {
			seen[key] = struct{}{}
		}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

// Excluded는 대상 디렉토리 기준 상대경로가 exclude glob 패턴 중 하나에
// 걸리는지 판정한다. gitignore처럼 동작하도록 패턴을 세 종류의 후보에
// 적용한다: 상대경로 전체, 파일명(basename), 그리고 모든 조상 디렉토리 경로.
// 조상 매칭 덕분에 "templates"나 "templates/*" 패턴이 templates/2025/note.md
// 같은 깊은 하위 파일도 제외한다 (path.Match의 *는 /를 넘지 못하기 때문에
// 경로 전체 매칭만으로는 1단계밖에 걸리지 않는다).
// macOS 디스크의 한글 파일명은 NFD로 저장되므로, 설정 파일의 NFC 패턴과
// 맞도록 양쪽을 NFC로 정규화한 뒤 비교한다.
// 패턴 문법 오류는 설정 로드 단계에서 검증되므로 여기서는 무시한다.
func Excluded(relPath string, exclude []string) bool {
	rel := norm.NFC.String(filepath.ToSlash(relPath))

	// 후보: 상대경로 전체, 파일명, 조상 디렉토리 경로들
	// (예: templates/2025/note.md -> [전체, note.md, templates, templates/2025]).
	segments := strings.Split(rel, "/")
	candidates := make([]string, 0, len(segments)+1)
	candidates = append(candidates, rel, path.Base(rel))
	for i := 1; i < len(segments); i++ {
		candidates = append(candidates, strings.Join(segments[:i], "/"))
	}

	for _, pattern := range exclude {
		pat := strings.TrimSuffix(norm.NFC.String(pattern), "/")
		for _, candidate := range candidates {
			if ok, err := path.Match(pat, candidate); err == nil && ok {
				return true
			}
		}
	}
	return false
}
