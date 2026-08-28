package ui

import (
	"fmt"
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
	m.insideTmux = true // 기존 테스트는 tmux 안 동작(점프) 기준. 밖 동작은 개별 테스트가 끈다
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

// notice는 일회성 — 다음 키 입력(커서 이동 등)에서 사라져야 한다.
// 복구 불가(sid 없음) 죽은 세션의 안내로 검증한다.
func TestNoticeClearsOnNextKey(t *testing.T) {
	m := Model{agents: []*state.Agent{
		{ID: "claude-6", Kind: "claude", State: state.StateDead,
			UpdatedAt: t0, StateSince: t0},
		{ID: "claude-9", Kind: "claude", State: state.StateWaiting,
			UpdatedAt: t0, StateSince: t0},
	}}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !strings.Contains(m.notice, "복구 불가") {
		t.Fatalf("죽은 세션 enter의 notice가 없다: %q", m.notice)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	if m.notice != "" {
		t.Errorf("커서 이동 후에도 notice 잔류: %q", m.notice)
	}
}

// tmux 밖에서 enter는 TUI를 끝내지 않고(점프 금지) attach 경로로 가야 한다.
// 남의 tmux 클라이언트(iMac iTerm2)를 전환시키는 부작용이 없어야 한다.
func TestEnterOutsideTmuxAttachesInsteadOfJump(t *testing.T) {
	m := fixtureModel(t)
	m.insideTmux = false
	m.cursor = 1 // DONE_UNREAD 행

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("enter가 아무 cmd도 안 냈다")
	}
	// 읽음 처리는 tmux 안팎 공통
	a, err := m.store.Load("claude-3")
	if err != nil {
		t.Fatal(err)
	}
	if a.State != state.StateIdle {
		t.Errorf("enter 후에도 읽음 처리가 안 됐다: %s", a.State)
	}
}

// attach에서 돌아오면(detach) TUI는 종료가 아니라 새로고침으로 살아 있어야 한다.
func TestAttachDoneRefreshesInsteadOfQuit(t *testing.T) {
	m := fixtureModel(t)
	next, cmd := m.Update(attachDoneMsg{})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("attach 복귀 후 cmd가 없다")
	}
	if msg := cmd(); msg != nil {
		if _, quit := msg.(tea.QuitMsg); quit {
			t.Error("attach 복귀가 TUI를 종료시킨다")
		}
	}
}

// 죽은 세션 enter는 안내문 대신 y/n 복구 확인으로 진입해야 한다.
func TestDeadEnterAsksResumeConfirm(t *testing.T) {
	m := fixtureModel(t)
	m.agents = []*state.Agent{{ID: "claude-20", Kind: "claude", State: state.StateDead,
		SessionID: "sid-20", CWD: "/tmp/w", Tmux: state.TmuxRef{Session: "work"},
		UpdatedAt: t0, StateSince: t0}}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.pendingCmd != "resume" {
		t.Fatalf("resume 확인 대기 상태여야 함: %q", m.pendingCmd)
	}
	if v := m.View(); !strings.Contains(v, "y 확인") || !strings.Contains(v, "복구") {
		t.Errorf("복구 확인 프롬프트가 보여야 함:\n%s", v)
	}
}

// 세션 ID가 없어 복구 불가면 기존 안내 유지 (확인 진입 금지).
func TestDeadEnterWithoutSidKeepsNotice(t *testing.T) {
	m := fixtureModel(t)
	m.agents = []*state.Agent{{ID: "claude-21", Kind: "claude", State: state.StateDead,
		UpdatedAt: t0, StateSince: t0}}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.pendingCmd != "" {
		t.Error("복구 불가 세션은 확인 대기로 가면 안 됨")
	}
	if m.notice == "" {
		t.Error("복구 불가 사유 안내가 있어야 함")
	}
}

// y 확인 시 resume 창을 만들고(tmux 안) TUI를 닫아 그 화면으로 이동한다.
func TestResumeConfirmYCreatesWindowAndQuits(t *testing.T) {
	m := fixtureModel(t)
	m.agents = []*state.Agent{{ID: "claude-20", Kind: "claude", State: state.StateDead,
		SessionID: "sid-20", CWD: "/tmp/w", Tmux: state.TmuxRef{Session: "work"},
		UpdatedAt: t0, StateSince: t0}}
	var gotSess, gotName, gotDir, gotCmd string
	m.activeSession = func() string { return "cur-sess" }
	m.spawnWindow = func(session, name, dir, cmd string) (string, error) {
		gotSess, gotName, gotDir, gotCmd = session, name, dir, cmd
		return "%99", nil
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, cmd := next.(Model).Update(key("y"))
	if gotSess != "cur-sess" || gotName != "resume-claude-20" || gotDir != "/tmp/w" ||
		!strings.Contains(gotCmd, "claude --resume sid-20") {
		t.Errorf("창 생성 인자: %q %q %q %q", gotSess, gotName, gotDir, gotCmd)
	}
	if cmd == nil {
		t.Fatal("y 후 cmd가 없다")
	}
	if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Error("tmux 안에서는 새 창으로 이동(TUI 종료)해야 함")
	}
	_ = next
}

// y 아닌 키는 취소 — pendingResume도 비워야 한다.
func TestResumeConfirmCancel(t *testing.T) {
	m := fixtureModel(t)
	m.agents = []*state.Agent{{ID: "claude-20", Kind: "claude", State: state.StateDead,
		SessionID: "sid-20", CWD: "/tmp/w", UpdatedAt: t0, StateSince: t0}}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = next.(Model).Update(key("n"))
	m = next.(Model)
	if m.pendingCmd != "" || m.pendingResume != nil {
		t.Error("취소 시 확인 상태가 남으면 안 됨")
	}
}

// resume 성공 시 원본 dead 레코드는 즉시 삭제 — status 이중 행 방지
// (restore와 같은 기준, 2026-08-27 결정).
func TestResumeDeletesDeadRecord(t *testing.T) {
	m := fixtureModel(t)
	dead := &state.Agent{ID: "claude-20", Kind: "claude", State: state.StateDead,
		SessionID: "sid-20", CWD: "/tmp/w", Tmux: state.TmuxRef{Session: "work"},
		UpdatedAt: t0, StateSince: t0}
	if err := m.store.Save(dead); err != nil {
		t.Fatal(err)
	}
	m.agents = []*state.Agent{dead}
	m.activeSession = func() string { return "cur-sess" }
	m.spawnWindow = func(session, name, dir, cmd string) (string, error) { return "%99", nil }

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = next.(Model).Update(key("y"))
	if _, err := next.(Model).store.Load("claude-20"); err == nil {
		t.Error("resume 성공 후 dead 레코드가 남아 있다")
	}
}

// 창 생성 실패 시에는 레코드를 보존해야 다시 시도할 수 있다.
func TestResumeKeepsRecordOnSpawnFailure(t *testing.T) {
	m := fixtureModel(t)
	dead := &state.Agent{ID: "claude-20", Kind: "claude", State: state.StateDead,
		SessionID: "sid-20", CWD: "/tmp/w", UpdatedAt: t0, StateSince: t0}
	if err := m.store.Save(dead); err != nil {
		t.Fatal(err)
	}
	m.agents = []*state.Agent{dead}
	m.activeSession = func() string { return "cur-sess" }
	m.spawnWindow = func(session, name, dir, cmd string) (string, error) {
		return "", fmt.Errorf("boom")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = next.(Model).Update(key("y"))
	if _, err := next.(Model).store.Load("claude-20"); err != nil {
		t.Error("창 생성 실패면 레코드를 보존해야 함")
	}
}
