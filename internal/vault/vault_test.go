package vault

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseNote(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantFM   map[string]string
		wantBody string
	}{
		{
			name:     "frontmatter 없는 문서는 빈 맵과 전체 본문",
			content:  "# 제목\n\n본문입니다.\n",
			wantFM:   map[string]string{},
			wantBody: "# 제목\n\n본문입니다.",
		},
		{
			name:     "정상 frontmatter",
			content:  "---\ntitle: 일기\ndate: 2025-11-05\n---\n본문\n",
			wantFM:   map[string]string{"title": "일기", "date": "2025-11-05"},
			wantBody: "본문",
		},
		{
			name:     "닫는 구분자가 없으면 frontmatter 없음으로 취급",
			content:  "---\ntitle: 일기\n본문처럼 이어짐\n",
			wantFM:   map[string]string{},
			wantBody: "---\ntitle: 일기\n본문처럼 이어짐",
		},
		{
			name:     "본문의 수평선(---)에서 본문이 잘리지 않음",
			content:  "---\ntitle: 일기\n---\n앞부분\n\n---\n\n뒷부분\n",
			wantFM:   map[string]string{"title": "일기"},
			wantBody: "앞부분\n\n---\n\n뒷부분",
		},
		{
			name:     "값에 콜론이 포함된 경우 첫 콜론만 구분자",
			content:  "---\ntime: 10:30\nurl: https://example.com\n---\n본문",
			wantFM:   map[string]string{"time": "10:30", "url": "https://example.com"},
			wantBody: "본문",
		},
		{
			name:     "값 양끝의 작은/큰따옴표 제거",
			content:  "---\ntitle: \"따옴표 제목\"\nalias: '별칭'\n---\n본문",
			wantFM:   map[string]string{"title": "따옴표 제목", "alias": "별칭"},
			wantBody: "본문",
		},
		{
			name:     "리스트 값은 키만 수집하고 리스트 항목 줄은 무시",
			content:  "---\ntags:\n- a\n- b\ntitle: 일기\n---\n본문",
			wantFM:   map[string]string{"tags": "", "title": "일기"},
			wantBody: "본문",
		},
		{
			name:     "빈 문서",
			content:  "",
			wantFM:   map[string]string{},
			wantBody: "",
		},
		{
			name:     "값 안의 ---에서 frontmatter가 잘리지 않음",
			content:  "---\ntitle: my---note\ntags: work\n---\n본문",
			wantFM:   map[string]string{"title": "my---note", "tags": "work"},
			wantBody: "본문",
		},
		{
			name:     "frontmatter 없이 수평선으로 시작하는 문서는 본문 보존",
			content:  "----\nNote: remember this\n----\n본문",
			wantFM:   map[string]string{},
			wantBody: "----\nNote: remember this\n----\n본문",
		},
		{
			name:     "구분자는 정확히 --- 줄이어야 함(---abc는 구분자 아님)",
			content:  "---abc\ntitle: 일기\n---\n본문",
			wantFM:   map[string]string{},
			wantBody: "---abc\ntitle: 일기\n---\n본문",
		},
		{
			name:     "YAML 주석 줄은 키로 수집하지 않음",
			content:  "---\n# draft: true\ntitle: 일기\n---\n본문",
			wantFM:   map[string]string{"title": "일기"},
			wantBody: "본문",
		},
		{
			name:     "CRLF 줄바꿈 문서도 구분자 인식",
			content:  "---\r\ntitle: 일기\r\n---\r\n본문",
			wantFM:   map[string]string{"title": "일기"},
			wantBody: "본문",
		},
		{
			name:     "따옴표로 감싼 키도 따옴표를 벗겨 수집",
			content:  "---\n\"docu_type\": \"Plan\"\n'category': work\n---\n본문",
			wantFM:   map[string]string{"docu_type": "Plan", "category": "work"},
			wantBody: "본문",
		},
		{
			// 실제 볼트에서 관측된 형태: 따옴표 안에 콜론이 포함된 키.
			name:     "따옴표 안에 콜론이 포함된 키(실데이터 오타 형태)",
			content:  "---\n\"docu_type:\": Plan\ncategory:\n  - daily-note\n---\n본문",
			wantFM:   map[string]string{"docu_type": "Plan", "category": ""},
			wantBody: "본문",
		},
		{
			name:     "짝 없는 따옴표 오타 키도 따옴표 없이 수집",
			content:  "---\n\"created: 2025-11-01\n---\n본문",
			wantFM:   map[string]string{"created": "2025-11-01"},
			wantBody: "본문",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseNote("test.md", tt.content)
			if got.Filename != "test.md" {
				t.Errorf("Filename = %q, want %q", got.Filename, "test.md")
			}
			if !reflect.DeepEqual(got.Frontmatter, tt.wantFM) {
				t.Errorf("Frontmatter = %#v, want %#v", got.Frontmatter, tt.wantFM)
			}
			if got.Body != tt.wantBody {
				t.Errorf("Body = %q, want %q", got.Body, tt.wantBody)
			}
		})
	}
}

func TestScanFrontmatterKeys(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"a.md":     "---\ntitle: A\ndate: 2025-11-05\n---\n본문",
		"b.md":     "---\ntitle: B\ntags:\n- x\n---\n본문",
		"plain.md": "frontmatter 없는 본문",
		"skip.txt": "---\nignored: yes\n---\nmd 파일이 아니므로 무시",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("테스트 파일 생성 실패: %v", err)
		}
	}

	// 하위 디렉토리의 md 파일도 재귀적으로 스캔 대상에 포함되어야 한다.
	subdir := filepath.Join(dir, "2026-08")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("하위 디렉토리 생성 실패: %v", err)
	}
	nested := "---\nnested: yes\n---\n본문"
	if err := os.WriteFile(filepath.Join(subdir, "2026-08-21.md"), []byte(nested), 0o644); err != nil {
		t.Fatalf("테스트 파일 생성 실패: %v", err)
	}

	got, err := ScanFrontmatterKeys(dir, nil, io.Discard)
	if err != nil {
		t.Fatalf("ScanFrontmatterKeys() error = %v", err)
	}
	want := []string{"date", "nested", "tags", "title"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ScanFrontmatterKeys() = %v, want %v", got, want)
	}

	// exclude 패턴은 파일명과 상대경로 양쪽에 적용된다.
	got, err = ScanFrontmatterKeys(dir, []string{"a.md", "2026-08/*"}, io.Discard)
	if err != nil {
		t.Fatalf("ScanFrontmatterKeys(exclude) error = %v", err)
	}
	want = []string{"tags", "title"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ScanFrontmatterKeys(exclude) = %v, want %v", got, want)
	}
}

func TestScanFrontmatterKeysMissingDir(t *testing.T) {
	if _, err := ScanFrontmatterKeys(filepath.Join(t.TempDir(), "없는디렉토리"), nil, io.Discard); err == nil {
		t.Error("존재하지 않는 디렉토리는 에러를 반환해야 한다")
	}
}

func TestExcluded(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		exclude []string
		want    bool
	}{
		{name: "패턴 없음", relPath: "a.md", exclude: nil, want: false},
		{name: "파일명 일치", relPath: "sub/note.md", exclude: []string{"note.md"}, want: true},
		{name: "파일명 glob", relPath: "sub/draft-1.md", exclude: []string{"draft-*.md"}, want: true},
		{name: "상대경로 glob", relPath: "templates/daily.md", exclude: []string{"templates/*"}, want: true},
		{name: "불일치", relPath: "2026-08/2026-08-21.md", exclude: []string{"templates/*", "*.txt"}, want: false},
		{name: "디렉토리명 패턴이 깊은 하위 파일 제외", relPath: "templates/2025/note.md", exclude: []string{"templates"}, want: true},
		{name: "디렉토리 glob이 깊은 하위 파일 제외", relPath: "templates/2025/note.md", exclude: []string{"templates/*"}, want: true},
		{name: "슬래시로 끝나는 디렉토리 패턴", relPath: "templates/note.md", exclude: []string{"templates/"}, want: true},
		{name: "조상 아닌 중간 이름은 불일치", relPath: "2026-08/templates.md", exclude: []string{"templates"}, want: false},
		{
			// macOS 디스크의 NFD 파일명이 설정 파일의 NFC 패턴과 매칭되어야 한다.
			name:    "NFD 파일명과 NFC 패턴",
			relPath: "하루를 시작하기 전에.md", // 코드상 NFC
			exclude: []string{"하루를 시작하기 전에.md"},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Excluded(tt.relPath, tt.exclude); got != tt.want {
				t.Errorf("Excluded(%q, %v) = %v, want %v", tt.relPath, tt.exclude, got, tt.want)
			}
		})
	}
}

// TestExcludedNFD는 실제 NFD 바이트열(디스크 상태)과 NFC 패턴의 매칭을 검증한다.
func TestExcludedNFD(t *testing.T) {
	// "하루.md"를 자모 분해(NFD)한 바이트열. macOS 파일시스템이 돌려주는 형태다.
	nfd := "\u1112\u1161\u1105\u116e.md" // NFD: ᄒ+ᅡ+ᄅ+ᅮ
	nfc := "\ud558\ub8e8.md"             // NFC: 하루
	if nfd == nfc {
		t.Fatal("테스트 전제 오류: 두 문자열이 이미 같음")
	}
	if !Excluded(nfd, []string{nfc}) {
		t.Errorf("NFD 상대경로 %q가 NFC 패턴 %q와 매칭되어야 한다", nfd, nfc)
	}
}
