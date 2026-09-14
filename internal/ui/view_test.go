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

// SESSION 열은 가장 긴 세션 문구("이름 (스레드 N)")에 맞춰 넓어져 DIR 열이 밀리지 않는다.
// 고정 20칸이던 때는 "search-youtube-bot (스레드 1)"이 DIR을 8칸 밀어 행마다 열이 어긋났다.
func TestSessionColumnWidensForThreadBadge(t *testing.T) {
	m := fixtureModel(t)
	m.width, m.height = 200, 30
	m.agents[0].Tmux.Session = "search-youtube-bot"
	m.agents[0].Threads = 1
	m.agents[0].CWD = "/Users/soonho/a"
	m.agents[1].CWD = "/Users/soonho/b"
	m.cursor = 1 // 0번(긴 문구)은 일반 행, 1번은 선택 행 — 둘 다 같은 열에 DIR이 와야 한다

	if got, want := m.sessionColWidth(), ansi.StringWidth("search-youtube-bot (스레드 1)"); got != want {
		t.Fatalf("sessionColWidth = %d, want %d", got, want)
	}
	var cols []int
	for _, ln := range strings.Split(m.View(), "\n") {
		plain := ansi.Strip(ln)
		if i := strings.Index(plain, "~/a"); i >= 0 {
			cols = append(cols, ansi.StringWidth(plain[:i]))
		}
		if i := strings.Index(plain, "~/b"); i >= 0 {
			cols = append(cols, ansi.StringWidth(plain[:i]))
		}
	}
	if len(cols) != 2 || cols[0] != cols[1] {
		t.Fatalf("DIR 열 시작 위치가 행마다 달라야 안 됨: %v", cols)
	}

	m.agents[0].Tmux.Session = strings.Repeat("x", 60)
	if got := m.sessionColWidth(); got != 36 {
		t.Fatalf("상한 36칸이어야 함, got %d", got)
	}
}
