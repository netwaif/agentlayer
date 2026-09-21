package hookcmd

import (
	"strings"
	"testing"

	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
)

func TestRunCodexTurnComplete(t *testing.T) {
	st := newStore(t)
	payload := `{"type":"agent-turn-complete","turn-id":"x","cwd":"/Users/x/proj"}`
	if err := RunCodex(st, []string{payload}, env("%9"), t0); err != nil {
		t.Fatal(err)
	}
	a, err := st.Load(scan.IDForPane("codex", "%9"))
	if err != nil {
		t.Fatal(err)
	}
	if a.State != state.StateDoneUnread || a.Kind != "codex" || a.CWD != "/Users/x/proj" {
		t.Errorf("turn-complete → DONE_UNREAD: %+v", a)
	}
}

func TestRunCodexUnknownTypeIgnored(t *testing.T) {
	st := newStore(t)
	if err := RunCodex(st, []string{`{"type":"mystery"}`}, env("%9"), t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.List(); len(got) != 0 {
		t.Error("모르는 이벤트는 무시")
	}
}

func TestRunCodexOutsideTmux(t *testing.T) {
	st := newStore(t)
	if err := RunCodex(st, []string{`{"type":"agent-turn-complete"}`}, env(""), t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.List(); len(got) != 0 {
		t.Error("tmux 밖 no-op")
	}
}

func TestRunCodexEmptyArgs(t *testing.T) {
	st := newStore(t)
	if err := RunCodex(st, nil, env("%9"), t0); err != nil {
		t.Fatal(err)
	}
}

func TestRunCodexEventTransitions(t *testing.T) {
	st := newStore(t)
	in := func(s string) *strings.Reader { return strings.NewReader(s) }
	body := `{"session_id":"s1","cwd":"/Users/x/proj","hook_event_name":"UserPromptSubmit"}`
	if err := RunCodexEvent(st, "user-prompt-submit", in(body), env("%9"), t0); err != nil {
		t.Fatal(err)
	}
	a, _ := st.Load(scan.IDForPane("codex", "%9"))
	if a == nil || a.State != state.StateWorking || a.CWD != "/Users/x/proj" || a.SessionID != "s1" {
		t.Fatalf("user-prompt-submit → WORK: %+v", a)
	}
	if err := RunCodexEvent(st, "permission-request", in(`{}`), env("%9"), t0); err != nil {
		t.Fatal(err)
	}
	if a, _ = st.Load(a.ID); a.State != state.StateWaiting {
		t.Errorf("permission-request → WAIT: %v", a.State)
	}
	if err := RunCodexEvent(st, "stop", in(`{}`), env("%9"), t0); err != nil {
		t.Fatal(err)
	}
	if a, _ = st.Load(a.ID); a.State != state.StateDoneUnread {
		t.Errorf("stop → DONE_UNREAD: %v", a.State)
	}
	if err := RunCodexEvent(st, "mystery", in(`{}`), env("%9"), t0); err != nil {
		t.Fatal(err)
	}
	if a, _ = st.Load(a.ID); a.State != state.StateDoneUnread {
		t.Errorf("모르는 이벤트는 상태를 안 바꿈: %v", a.State)
	}
}

// codex notify(agent-turn-complete)의 last-assistant-message가 Task(최근 작업)에 실린다 — 총괄 수신함
// [REPORT] DONE: 요약이 "(요약 없음)"으로 비던 문제(WSL2 실기 2026-09-21).
func TestRunCodexTurnCompleteFillsTaskFromLastMessage(t *testing.T) {
	st := newStore(t)
	payload := `{"type":"agent-turn-complete","turn-id":"x","cwd":"/p","last-assistant-message":"\n  OK  \n두 번째 줄"}`
	if err := RunCodex(st, []string{payload}, env("%9"), t0); err != nil {
		t.Fatal(err)
	}
	a, _ := st.Load(scan.IDForPane("codex", "%9"))
	if a.Task != "OK" {
		t.Errorf("Task는 첫 줄 요약이어야 함: %q", a.Task)
	}
}

func TestRunCodexEventStopFillsTaskFromLastMessage(t *testing.T) {
	st := newStore(t)
	body := `{"session_id":"s1","cwd":"/p","hook_event_name":"Stop","last_assistant_message":"DONE"}`
	if err := RunCodexEvent(st, "stop", strings.NewReader(body), env("%9"), t0); err != nil {
		t.Fatal(err)
	}
	a, _ := st.Load(scan.IDForPane("codex", "%9"))
	if a.Task != "DONE" {
		t.Errorf("Task: %q", a.Task)
	}
}
