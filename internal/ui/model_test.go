package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
	"github.com/netwaif/agentlayer/internal/usage"
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
			Tmux:      state.TmuxRef{Session: "collab-bot", Window: 0, PaneID: "%7"},
			UpdatedAt: t0, StateSince: t0},
		{ID: "claude-3", Kind: "claude", Task: "핸드오프 문서 확인", State: state.StateDoneUnread,
			Tmux:      state.TmuxRef{Session: "ai", Window: 1, PaneID: "%3"},
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

func TestDefaultModelInHeader(t *testing.T) {
	m := fixtureModel(t)
	if strings.Contains(m.View(), "기본모델") {
		t.Error("기본 모델 미수집 상태에서는 표시하지 않는다")
	}
	next, _ := m.Update(ctxMsg{defModels: map[string]string{
		"claude": "claude-fable-5", "codex": "gpt-5.6-sol high", "gemini": ""}})
	v := next.(Model).View()
	for _, want := range []string{"기본모델", "Claude Fable 5", "Codex gpt-5.6-sol high", "Gemini 자동"} {
		if !strings.Contains(v, want) {
			t.Errorf("헤더에 %q 표시돼야 함:\n%s", want, v)
		}
	}
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

func TestUsageToggleAndView(t *testing.T) {
	m := fixtureModel(t)
	pct77, pct16 := 77.0, 16.0
	m.usagePay = &usage.Payload{Providers: map[string]usage.Provider{
		"claude": {OK: true, Level: "green", Action: "지금 큰 작업 돌리세요",
			Email:  "kshxxthm@gmail.com",
			Reason: "단기 한도 77% 남았어요.",
			Windows: map[string]usage.Window{
				"5h": {LeftPct: &pct77}, "7d": {LeftPct: &pct16}}},
	}}
	// 메인 뷰 헤더 요약
	v := m.View()
	if !strings.Contains(v, "5h 77%") || !strings.Contains(v, "7d 16%") {
		t.Errorf("헤더 사용량 요약 있어야 함:\n%s", v)
	}
	// u 토글 → 사용량 전용 뷰
	next, _ := m.Update(key("u"))
	uv := next.(Model).View()
	for _, want := range []string{"사용량", "Claude", "지금 큰 작업 돌리세요", "kshxxthm@gmail.com", "█", "77%"} {
		if !strings.Contains(uv, want) {
			t.Errorf("사용량 뷰에 %q 있어야 함:\n%s", want, uv)
		}
	}
	// 다시 u → 관제 화면 복귀
	back, _ := next.Update(key("u"))
	if !strings.Contains(back.(Model).View(), "SESSION") {
		t.Error("u 재입력 시 관제 화면 복귀")
	}
}

func TestCtxBadgeOnRows(t *testing.T) {
	m := fixtureModel(t)
	used := 42.0
	m.agents[0].CWD = "/Users/x/proj"
	// 에이전트 ID 키 — 같은 폴더의 다른 종류 에이전트와 오귀속되지 않게
	m.ctx = map[string]usage.CtxInfo{
		m.agents[0].ID: {Model: "Opus 5 (1M context)", UsedPct: &used, TS: t0.Add(-10 * time.Minute)},
	}
	v := m.View()
	if !strings.Contains(v, "Opus 5 (1M context)") || !strings.Contains(v, "ctx 42%") {
		t.Errorf("행에 모델·ctx%% 표시:\n%s", v)
	}
}

func TestCtxBadgeApprox(t *testing.T) {
	m := fixtureModel(t)
	used := 7.0
	m.ctx = map[string]usage.CtxInfo{
		m.agents[0].ID: {Model: "gemini-3.6-flash", UsedPct: &used, Approx: true, TS: t0},
	}
	if v := m.View(); !strings.Contains(v, "ctx ~7%") {
		t.Errorf("근사값은 ~ 접두 표시:\n%s", v)
	}
}

func TestKindDividerBetweenGroups(t *testing.T) {
	m := fixtureModel(t)
	m.agents = append(m.agents, &state.Agent{ID: "gemini-9", Kind: "gemini",
		State: state.StateIdle, Tmux: state.TmuxRef{Session: "g"},
		UpdatedAt: t0, StateSince: t0})
	v := m.View()
	if !strings.Contains(v, "── gemini") {
		t.Errorf("종류 경계에 구분선 나와야 함:\n%s", v)
	}
	// 같은 종류 사이(claude 두 행)에는 구분선 없음 — claude 그룹 시작엔 divider가 없다
	if strings.Contains(v, "── claude") {
		t.Errorf("첫 그룹 앞에는 구분선 없음:\n%s", v)
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
