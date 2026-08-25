package hookcmd

import (
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
)

var t0 = time.Date(2026, 8, 25, 12, 0, 0, 0, time.FixedZone("KST", 9*3600))

func newStore(t *testing.T) *state.Store {
	t.Helper()
	s, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func env(pane string) func(string) string {
	return func(k string) string {
		if k == "TMUX_PANE" {
			return pane
		}
		return ""
	}
}

const payload = `{"session_id":"10ec8033-ca55","cwd":"/Users/soonho/ai-folder/dev/agentlayer","hook_event_name":"Stop","unknown_field":123}`

func TestEventTransitions(t *testing.T) {
	cases := []struct {
		event string
		want  state.AgentState
	}{
		{"post-tool-use", state.StateWorking},
		{"session-start", state.StateWorking},
		{"notification", state.StateWaiting},
		{"stop", state.StateDoneUnread},
	}
	for _, c := range cases {
		st := newStore(t)
		if err := RunClaude(st, c.event, strings.NewReader(payload), env("%3"), t0); err != nil {
			t.Fatalf("%s: %v", c.event, err)
		}
		id := scan.IDForPane("claude", "%3")
		a, err := st.Load(id)
		if err != nil {
			t.Fatalf("%s: 레코드 생성돼야 함: %v", c.event, err)
		}
		if a.State != c.want {
			t.Errorf("%s → %s, want %s", c.event, a.State, c.want)
		}
		if a.SessionID != "10ec8033-ca55" {
			t.Errorf("session_id 기록돼야 함: %q", a.SessionID)
		}
		if a.CWD != "/Users/soonho/ai-folder/dev/agentlayer" {
			t.Errorf("cwd 기록돼야 함: %q", a.CWD)
		}
	}
}

func TestNotificationMessageBecomesTask(t *testing.T) {
	st := newStore(t)
	in := `{"session_id":"s1","message":"Bash 명령 실행 승인이 필요합니다"}`
	if err := RunClaude(st, "notification", strings.NewReader(in), env("%3"), t0); err != nil {
		t.Fatal(err)
	}
	a, _ := st.Load(scan.IDForPane("claude", "%3"))
	if a.Task != "Bash 명령 실행 승인이 필요합니다" {
		t.Errorf("notification message가 Task로: %q", a.Task)
	}
	// 후속 stop은 Task를 지우지 않는다
	if err := RunClaude(st, "stop", strings.NewReader(`{"session_id":"s1"}`), env("%3"), t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	a, _ = st.Load(scan.IDForPane("claude", "%3"))
	if a.Task == "" {
		t.Error("기존 Task 유지돼야 함")
	}
}

func TestEmptyStdinTolerated(t *testing.T) {
	st := newStore(t)
	if err := RunClaude(st, "stop", strings.NewReader(""), env("%5"), t0); err != nil {
		t.Fatalf("빈 stdin 허용: %v", err)
	}
	if _, err := st.Load(scan.IDForPane("claude", "%5")); err != nil {
		t.Error("빈 stdin이어도 pane 기반 레코드는 생성")
	}
}

func TestOutsideTmuxNoop(t *testing.T) {
	st := newStore(t)
	if err := RunClaude(st, "stop", strings.NewReader(payload), env(""), t0); err != nil {
		t.Fatalf("tmux 밖에서는 조용히 no-op: %v", err)
	}
	got, _ := st.List()
	if len(got) != 0 {
		t.Errorf("tmux 밖 이벤트는 레코드를 만들지 않음: %+v", got)
	}
}

func TestUnknownEventIgnored(t *testing.T) {
	st := newStore(t)
	if err := RunClaude(st, "mystery", strings.NewReader(payload), env("%3"), t0); err != nil {
		t.Fatalf("모르는 이벤트도 에러 없이 무시: %v", err)
	}
}

func TestExistingStateAndCoordsPreserved(t *testing.T) {
	// 스캐너가 이미 좌표를 채운 레코드에 hook이 상태만 얹는다
	st := newStore(t)
	pre := &state.Agent{ID: scan.IDForPane("claude", "%3"), Kind: "claude",
		State: state.StateIdle,
		Tmux:  state.TmuxRef{Session: "ai", Window: 1, PaneID: "%3"},
		Task:  "핸드오프 문서 확인", UpdatedAt: t0, StateSince: t0}
	if err := st.Save(pre); err != nil {
		t.Fatal(err)
	}
	if err := RunClaude(st, "post-tool-use", strings.NewReader(`{"session_id":"s1"}`), env("%3"), t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	a, _ := st.Load(pre.ID)
	if a.Tmux.Session != "ai" || a.Task != "핸드오프 문서 확인" {
		t.Errorf("좌표·Task 보존돼야 함: %+v", a)
	}
	if a.State != state.StateWorking {
		t.Errorf("상태는 WORKING으로: %s", a.State)
	}
}
