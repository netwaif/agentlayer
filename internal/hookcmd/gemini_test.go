package hookcmd

import (
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
)

const geminiIn = `{"conversationId":"ec33ebf9-0cba","workspacePaths":["/Users/soonho/ai-folder/dev/agentlayer"],"modelName":"Gemini 3.6 Flash (Medium)","stepIdx":5}`

func TestGeminiEventTransitions(t *testing.T) {
	cases := []struct {
		event string
		want  state.AgentState
	}{
		{"pre-invocation", state.StateWorking},
		{"post-tool-use", state.StateWorking},
		{"stop", state.StateDoneUnread},
	}
	for _, c := range cases {
		st := newStore(t)
		if err := RunGemini(st, c.event, strings.NewReader(geminiIn), env("%9"), t0); err != nil {
			t.Fatalf("%s: %v", c.event, err)
		}
		a, err := st.Load(scan.IDForPane("gemini", "%9"))
		if err != nil {
			t.Fatalf("%s: 레코드 생성돼야 함: %v", c.event, err)
		}
		if a.State != c.want {
			t.Errorf("%s → %s, want %s", c.event, a.State, c.want)
		}
		if a.SessionID != "ec33ebf9-0cba" {
			t.Errorf("conversationId 기록돼야 함: %q", a.SessionID)
		}
		if a.CWD != "/Users/soonho/ai-folder/dev/agentlayer" {
			t.Errorf("workspacePaths[0] → CWD: %q", a.CWD)
		}
		if a.Model != "Gemini 3.6 Flash (Medium)" {
			t.Errorf("modelName 기록돼야 함: %q", a.Model)
		}
	}
}

const geminiCLIIn = `{"session_id":"abc-123","cwd":"/w/proj","hook_event_name":"AfterAgent","timestamp":"2026-08-26T01:00:00Z"}`

func TestGeminiStockCLIEvents(t *testing.T) {
	cases := []struct {
		event string
		want  state.AgentState
	}{
		{"session-start", state.StateIdle},
		{"before-agent", state.StateWorking},
		{"after-tool", state.StateWorking},
		{"notification", state.StateWaiting},
		{"after-agent", state.StateDoneUnread},
	}
	for _, c := range cases {
		st := newStore(t)
		if err := RunGemini(st, c.event, strings.NewReader(geminiCLIIn), env("%9"), t0); err != nil {
			t.Fatalf("%s: %v", c.event, err)
		}
		a, err := st.Load(scan.IDForPane("gemini", "%9"))
		if err != nil {
			t.Fatalf("%s: 레코드 생성돼야 함: %v", c.event, err)
		}
		if a.State != c.want {
			t.Errorf("%s → %s, want %s", c.event, a.State, c.want)
		}
		if a.SessionID != "abc-123" {
			t.Errorf("session_id 기록돼야 함: %q", a.SessionID)
		}
		if a.CWD != "/w/proj" {
			t.Errorf("cwd 기록돼야 함: %q", a.CWD)
		}
	}
}

func TestGeminiOutsideTmuxNoop(t *testing.T) {
	st := newStore(t)
	if err := RunGemini(st, "stop", strings.NewReader(geminiIn), env(""), t0); err != nil {
		t.Fatalf("tmux 밖에서는 조용히 no-op: %v", err)
	}
	got, _ := st.List()
	if len(got) != 0 {
		t.Errorf("tmux 밖 이벤트는 레코드를 만들지 않음: %+v", got)
	}
}

func TestGeminiUnknownEventIgnored(t *testing.T) {
	st := newStore(t)
	if err := RunGemini(st, "post-invocation", strings.NewReader(geminiIn), env("%9"), t0); err != nil {
		t.Fatalf("모르는 이벤트도 에러 없이 무시: %v", err)
	}
	if _, err := st.Load(scan.IDForPane("gemini", "%9")); err == nil {
		t.Error("모르는 이벤트는 레코드를 만들지 않음")
	}
}

func TestGeminiEmptyStdinTolerated(t *testing.T) {
	st := newStore(t)
	if err := RunGemini(st, "stop", strings.NewReader(""), env("%9"), t0); err != nil {
		t.Fatalf("빈 stdin 허용: %v", err)
	}
	if _, err := st.Load(scan.IDForPane("gemini", "%9")); err != nil {
		t.Error("빈 stdin이어도 pane 기반 레코드는 생성")
	}
}

func TestGeminiWorkThenStop(t *testing.T) {
	st := newStore(t)
	if err := RunGemini(st, "pre-invocation", strings.NewReader(geminiIn), env("%9"), t0); err != nil {
		t.Fatal(err)
	}
	if err := RunGemini(st, "stop", strings.NewReader(geminiIn), env("%9"), t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	a, _ := st.Load(scan.IDForPane("gemini", "%9"))
	if a.State != state.StateDoneUnread {
		t.Errorf("작업 후 stop → DONE_UNREAD: %s", a.State)
	}
}
