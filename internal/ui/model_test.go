package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
)

var t0 = time.Date(2026, 8, 25, 12, 0, 0, 0, time.FixedZone("KST", 9*3600))

func fixtureModel(t *testing.T) Model {
	t.Helper()
	st, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	agents := []*state.Agent{
		{ID: "claude-7", Kind: "claude", Task: "승인 대기", State: state.StateWaiting,
			Tmux: state.TmuxRef{Session: "collab-bot", Window: 0, PaneID: "%7"},
			UpdatedAt: t0, StateSince: t0},
		{ID: "claude-3", Kind: "claude", Task: "핸드오프 문서 확인", State: state.StateDoneUnread,
			Tmux: state.TmuxRef{Session: "ai", Window: 1, PaneID: "%3"},
			UpdatedAt: t0, StateSince: t0},
	}
	for _, a := range agents {
		if err := st.Save(a); err != nil {
			t.Fatal(err)
		}
	}
	m := New(st, tmuxx.Tmux{})
	m.agents = agents
	m.now = t0
	return m
}

func key(s string) tea.KeyMsg {
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestCursorBounds(t *testing.T) {
	m := fixtureModel(t)
	// k는 0 아래로 못 감
	next, _ := m.Update(key("k"))
	if next.(Model).cursor != 0 {
		t.Error("커서 상한")
	}
	// j 두 번 → 마지막 행에서 멈춤
	next, _ = m.Update(key("j"))
	next, _ = next.Update(key("j"))
	if next.(Model).cursor != 1 {
		t.Errorf("커서 하한: %d", next.(Model).cursor)
	}
}

func TestEnterMarksReadAndJumps(t *testing.T) {
	m := fixtureModel(t)
	m.cursor = 1 // DONE_UNREAD 행
	next, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("enter는 점프 Cmd를 반환해야 함")
	}
	a, err := next.(Model).store.Load("claude-3")
	if err != nil {
		t.Fatal(err)
	}
	if a.State != state.StateIdle {
		t.Errorf("enter 시 읽음 처리: %s", a.State)
	}
}

func TestMarkReadOnlyKey(t *testing.T) {
	m := fixtureModel(t)
	m.cursor = 1
	next, _ := m.Update(key("o"))
	a, _ := next.(Model).store.Load("claude-3")
	if a.State != state.StateIdle {
		t.Errorf("o 키 읽음 처리: %s", a.State)
	}
}

func TestRefreshReplacesAgents(t *testing.T) {
	m := fixtureModel(t)
	m.cursor = 1
	next, _ := m.Update(refreshMsg{agents: m.agents[:1], now: t0.Add(time.Minute)})
	nm := next.(Model)
	if len(nm.agents) != 1 {
		t.Errorf("agents 교체: %d", len(nm.agents))
	}
	if nm.cursor != 0 {
		t.Errorf("커서는 범위 안으로 보정: %d", nm.cursor)
	}
}

func TestQuitKeys(t *testing.T) {
	m := fixtureModel(t)
	for _, k := range []string{"q"} {
		_, cmd := m.Update(key(k))
		if cmd == nil {
			t.Errorf("%s는 Quit Cmd 반환해야 함", k)
		}
	}
}

func TestViewContainsCoreTokens(t *testing.T) {
	m := fixtureModel(t)
	v := m.View()
	for _, want := range []string{"AgentLayer", "WAIT", "DONE", "collab-bot", "핸드오프 문서 확인", "j/k"} {
		if !strings.Contains(v, want) {
			t.Errorf("View에 %q 포함돼야 함", want)
		}
	}
}
