package hookcmd

import (
	"os"
	"strings"
	"testing"

	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
)

// 테스트 프로세스는 Claude 세션 안에서 돌 수도 있다(조상에 claude) — 기본값은 "에이전트 조상 없음"으로 고정하고,
// tmux 밖 경로를 보는 테스트만 stubDetached로 좌표를 준다.
func TestMain(m *testing.M) {
	detachedSelf = func(string) (int, string, bool) { return 0, "", false }
	os.Exit(m.Run())
}

func stubDetached(t *testing.T, pid int, name string, nested bool) {
	t.Helper()
	old := detachedSelf
	detachedSelf = func(string) (int, string, bool) { return pid, name, nested }
	t.Cleanup(func() { detachedSelf = old })
}

// TMUX_PANE 없이 뜬 세션(데스크톱 앱·맨 터미널)은 에이전트 프로세스 PID 좌표로 기록한다.
func TestDetachedClaudeRecordedByProcess(t *testing.T) {
	stubDetached(t, 500, "기획서", false)
	st := newStore(t)
	if err := RunClaude(st, "stop", strings.NewReader(payload), env(""), t0); err != nil {
		t.Fatal(err)
	}
	a, err := st.Load(scan.IDForProcess("claude", 500))
	if err != nil {
		t.Fatalf("PID 레코드가 생겨야 함: %v", err)
	}
	if !a.Detached() || a.PID != 500 || a.Name != "기획서" || a.SessionID != "10ec8033-ca55" || a.State != state.StateDoneUnread {
		t.Errorf("레코드: %+v", a)
	}
	if a.Label() != "기획서" || a.Where() != "app" {
		t.Errorf("표시: %q %q", a.Label(), a.Where())
	}
	// 같은 프로세스의 다음 이벤트는 같은 레코드를 갱신한다(/clear로 세션 ID가 바뀌어도 레코드는 하나)
	next := `{"session_id":"ffffffff-0000","cwd":"/tmp/x"}`
	if err := RunClaude(st, "user-prompt-submit", strings.NewReader(next), env(""), t0.Add(1)); err != nil {
		t.Fatal(err)
	}
	got, _ := st.List()
	if len(got) != 1 || got[0].SessionID != "ffffffff-0000" || got[0].State != state.StateWorking {
		t.Errorf("레코드 %d개 %+v", len(got), got)
	}
}

// 이름 없는 세션은 세션 ID 앞 8자리가 표시 이름이다.
func TestDetachedLabelFallsBackToSessionID(t *testing.T) {
	stubDetached(t, 501, "", false)
	st := newStore(t)
	if err := RunClaude(st, "session-start", strings.NewReader(payload), env(""), t0); err != nil {
		t.Fatal(err)
	}
	a, _ := st.Load(scan.IDForProcess("claude", 501))
	if a == nil || a.Label() != "10ec8033" {
		t.Errorf("세션 ID 앞 8자리여야 함: %+v", a)
	}
}

// 앱 세션이 Bash 도구로 띄운 자식 claude(-p)는 예전 pane 규칙과 같이 무시한다.
func TestDetachedNestedClaudeIgnored(t *testing.T) {
	stubDetached(t, 500, "", true)
	st := newStore(t)
	if err := RunClaude(st, "stop", strings.NewReader(payload), env(""), t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.List(); len(got) != 0 {
		t.Errorf("중첩 claude는 기록하지 않음: %+v", got)
	}
}

// 조상에서 에이전트를 못 찾으면 예전대로 관제 대상이 아니다(레코드 없음).
func TestDetachedWithoutAgentAncestorNoop(t *testing.T) {
	st := newStore(t)
	for _, run := range []func() error{
		func() error { return RunClaude(st, "stop", strings.NewReader(payload), env(""), t0) },
		func() error {
			return RunCodex(st, []string{`{"type":"agent-turn-complete","cwd":"/tmp"}`}, env(""), t0)
		},
		func() error { return RunCodexEvent(st, "stop", strings.NewReader(`{"session_id":"c1"}`), env(""), t0) },
		func() error { return RunGemini(st, "stop", strings.NewReader(`{"cwd":"/tmp"}`), env(""), t0) },
	} {
		if err := run(); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := st.List(); len(got) != 0 {
		t.Errorf("레코드가 생기면 안 됨: %+v", got)
	}
}

// 코덱스(notify·hooks 두 경로)와 제미나이도 tmux 밖에서 PID 좌표로 기록한다. 코덱스 두 경로는 같은 레코드에 모인다.
func TestDetachedCodexAndGemini(t *testing.T) {
	stubDetached(t, 600, "", false)
	st := newStore(t)
	if err := RunCodexEvent(st, "user-prompt-submit", strings.NewReader(`{"session_id":"c-600","cwd":"/tmp/c"}`), env(""), t0); err != nil {
		t.Fatal(err)
	}
	if err := RunCodex(st, []string{`{"type":"agent-turn-complete","cwd":"/tmp/c","last-assistant-message":"끝"}`}, env(""), t0.Add(1)); err != nil {
		t.Fatal(err)
	}
	a, err := st.Load(scan.IDForProcess("codex", 600))
	if err != nil || a.SessionID != "c-600" || a.State != state.StateDoneUnread || a.PID != 600 {
		t.Errorf("코덱스: %+v %v", a, err)
	}
	if err := RunGemini(st, "after-agent", strings.NewReader(`{"session_id":"g-600","cwd":"/tmp/g"}`), env(""), t0); err != nil {
		t.Fatal(err)
	}
	if g, err := st.Load(scan.IDForProcess("gemini", 600)); err != nil || g.SessionID != "g-600" || !g.Detached() {
		t.Errorf("제미나이: %+v %v", g, err)
	}
}

// 잔류 TMUX_PANE(TMUX 없음)·별도 서버는 tmux 밖 세션으로도 잡지 않는다 — guard.go 방어 유지.
func TestStaleTmuxPaneNotTreatedAsDetached(t *testing.T) {
	stubDetached(t, 500, "", false)
	for _, tmux := range []string{"", "/private/tmp/tmux-501/other,1,0"} {
		st := newStore(t)
		if err := RunClaude(st, "stop", strings.NewReader(payload), envOn(tmux, "%3"), t0); err != nil {
			t.Fatal(err)
		}
		if got, _ := st.List(); len(got) != 0 {
			t.Errorf("TMUX=%q: 레코드가 생기면 안 됨: %+v", tmux, got)
		}
	}
}

func TestSelfAgent(t *testing.T) {
	pane := &state.Agent{ID: "claude-3", Kind: "claude", State: state.StateIdle, Tmux: state.TmuxRef{Session: "ai", PaneID: "%3"}}
	app := &state.Agent{ID: "claude-pid500", Kind: "claude", State: state.StateIdle, PID: 500, Name: "기획서"}
	agents := []*state.Agent{pane, app}
	if got := SelfAgent(env("%3"), agents); got != pane {
		t.Errorf("pane 안: %+v", got)
	}
	if got := SelfAgent(env(""), agents); got != nil {
		t.Errorf("조상 없음이면 nil: %+v", got)
	}
	stubDetached(t, 500, "", false)
	if got := SelfAgent(env(""), agents); got != app {
		t.Errorf("tmux 밖: %+v", got)
	}
	if got := SelfAgent(envOn("", "%9"), agents); got != nil {
		t.Errorf("잔류 TMUX_PANE은 nil: %+v", got)
	}
}
