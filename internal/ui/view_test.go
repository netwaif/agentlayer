package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// 좁은 터미널에서 어떤 줄도 화면 폭을 넘으면 안 된다 — 넘으면 터미널 래핑으로
// altscreen 화면이 깨진다 (헤더·일반 행·선택 행·미리보기 전부 대상).
func TestViewClampsToWidth(t *testing.T) {
	m := fixtureModel(t)
	m.width, m.height = 60, 24
	m.agents[0].CWD = "/Users/soonho/ai-folder/dev/very/long/path/that/overflows/narrow/terminal"
	m.agents[1].CWD = "/Users/soonho/VSCodeWorkspace/another/deeply/nested/project/folder"
	m.cursor = 0 // 선택 행(전체폭 바)과 일반 행 둘 다 렌더

	for i, ln := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(ln); w > m.width {
			t.Errorf("%d번째 줄 폭 %d > 화면 폭 %d: %q", i, w, m.width, ln)
		}
	}
}

// 에이전트가 화면 높이보다 많아도 전체 출력이 화면을 넘지 않고, 커서 행은 항상 보인다.
func TestViewScrollsListToHeight(t *testing.T) {
	m := fixtureModel(t)
	m.width, m.height = 100, 15
	base := *m.agents[0]
	m.agents = nil
	for i := 0; i < 20; i++ {
		a := base
		a.ID = fmt.Sprintf("claude-%d", i)
		a.Tmux.Session = fmt.Sprintf("sess-%d", i)
		m.agents = append(m.agents, &a)
	}
	for _, cursor := range []int{0, 10, 19} {
		m.cursor = cursor
		out := m.View()
		lines := strings.Split(out, "\n")
		if len(lines) > m.height {
			t.Errorf("cursor=%d: 출력 %d줄 > 화면 높이 %d", cursor, len(lines), m.height)
		}
		if !strings.Contains(out, "▸") {
			t.Errorf("cursor=%d: 커서 행이 화면에 없음", cursor)
		}
		if !strings.Contains(out, fmt.Sprintf("sess-%d", cursor)) {
			t.Errorf("cursor=%d: 선택 세션이 화면에 없음", cursor)
		}
	}
}

// 색 이스케이프가 섞인 줄도 표시 폭 기준으로 잘리고, 열린 스타일은 리셋으로 닫힌다.
func TestClampLinesANSI(t *testing.T) {
	in := "\x1b[31m" + strings.Repeat("가", 40) + "\x1b[0m"
	got := clampLines(in, 20)
	if w := ansi.StringWidth(got); w > 20 {
		t.Errorf("클램프 후 폭 %d > 20: %q", w, got)
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("리셋으로 끝나지 않음 (스타일 번짐 위험): %q", got)
	}
	if short := "짧은 줄"; clampLines(short, 20) != short {
		t.Errorf("폭 이내 줄이 변형됨")
	}
}
