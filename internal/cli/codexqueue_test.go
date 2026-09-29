package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/state"
)

type queueCall struct{ thread, cwd, msg string }

func stubQueue(t *testing.T, err error) *[]queueCall {
	t.Helper()
	var calls []queueCall
	old := codexQueueFn
	codexQueueFn = func(_ context.Context, thread, cwd, msg string) error {
		calls = append(calls, queueCall{thread, cwd, msg})
		return err
	}
	t.Cleanup(func() { codexQueueFn = old })
	t.Setenv("AGENTLAYER_CONFIG", t.TempDir()+"/config.json")
	oldStart, oldRoot := procStartFn, codexSessionsRootFn
	procStartFn = func(int) (time.Time, bool) { return time.Time{}, false }
	root := t.TempDir()
	codexSessionsRootFn = func() string { return root }
	t.Cleanup(func() { procStartFn, codexSessionsRootFn = oldStart, oldRoot })
	return &calls
}

func codexAgent(session, pane string, s state.AgentState, sid string) *state.Agent {
	a := mkAgent("codex", session, pane, s)
	a.SessionID = sid
	a.CWD = "/work/" + session
	return a
}

// 코덱스는 tmux 키 입력이 아니라 codex queue로 간다. 작업 중이어도 --force 없이 간다.
func TestRunSendCodexUsesQueue(t *testing.T) {
	calls := stubQueue(t, nil)
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(codexAgent("textreview-bot", "%7", state.StateIdle, "sid-1"))
	_ = st.Save(codexAgent("busy-codex", "%8", state.StateWorking, "sid-2"))
	var out bytes.Buffer
	f := &fakeSender{}
	if err := RunSend(context.Background(), &out, strings.NewReader("첫 줄\n둘째 줄\n"), st, stateDir, f, []string{"textreview-bot", "-"}); err != nil {
		t.Fatal(err)
	}
	if f.calls != 0 || len(*calls) != 1 {
		t.Fatalf("tmux 0회·큐 1회여야 함: tmux=%d queue=%d", f.calls, len(*calls))
	}
	if c := (*calls)[0]; c.thread != "sid-1" || c.cwd != "/work/textreview-bot" || c.msg != "첫 줄\n둘째 줄" {
		t.Errorf("큐 호출: %+v", c)
	}
	if !strings.Contains(out.String(), "(codex queue)") {
		t.Errorf("출력에 경로 표시: %s", out.String())
	}
	out.Reset()
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"--json", "busy-codex", "x"}); err != nil {
		t.Fatalf("작업 중 코덱스도 큐로는 보낸다: %v", err)
	}
	if f.calls != 0 || len(*calls) != 2 || !strings.Contains(out.String(), `"via":"queue"`) {
		t.Errorf("tmux=%d queue=%d out=%s", f.calls, len(*calls), out.String())
	}
}

// 큐가 실패하면 tmux 입력으로 되돌아간다. 단 tmux 관문에 걸린 상태(작업 중)면 되돌아가지 않고 오류.
func TestRunSendCodexFallsBackToTmux(t *testing.T) {
	calls := stubQueue(t, errors.New("Error: no rollout found"))
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(codexAgent("textreview-bot", "%7", state.StateIdle, "sid-1"))
	_ = st.Save(codexAgent("busy-codex", "%8", state.StateWorking, "sid-2"))
	var out bytes.Buffer
	f := &fakeSender{}
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"textreview-bot", "안녕"}); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || f.calls != 1 || f.pane != "%7" || f.text != "안녕" {
		t.Fatalf("큐 실패 뒤 tmux: queue=%d tmux=%d", len(*calls), f.calls)
	}
	if !strings.Contains(out.String(), "codex queue 실패") || strings.Contains(out.String(), "(codex queue)") {
		t.Errorf("되돌아간 사유를 알려야 한다: %s", out.String())
	}
	f = &fakeSender{}
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"busy-codex", "x"}); err == nil || f.calls != 0 {
		t.Fatalf("작업 중인데 큐 실패면 tmux로 밀어 넣지 않는다: err=%v tmux=%d", err, f.calls)
	}
}

// 세션 ID가 없거나, 설정으로 껐거나, 코덱스가 아니면 예전 tmux 경로 그대로.
func TestCanCodexQueue(t *testing.T) {
	on := &config.Config{}
	off := false
	cases := []struct {
		name string
		a    *state.Agent
		cfg  *config.Config
		want bool
	}{
		{"코덱스+sid", codexAgent("c", "%1", state.StateIdle, "s"), on, true},
		{"작업 중", codexAgent("c", "%1", state.StateWorking, "s"), on, true},
		{"승인 대기", codexAgent("c", "%1", state.StateWaiting, "s"), on, false},
		{"sid 없음", codexAgent("c", "%1", state.StateIdle, ""), on, false},
		{"죽음", codexAgent("c", "%1", state.StateDead, "s"), on, false},
		{"설정 끔", codexAgent("c", "%1", state.StateIdle, "s"), &config.Config{CodexQueue: &off}, false},
		{"claude", func() *state.Agent { a := mkAgent("claude", "c", "%1", state.StateIdle); a.SessionID = "s"; return a }(), on, false},
	}
	for _, c := range cases {
		if got := canCodexQueue(c.a, c.cfg); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestCodexQueueResult(t *testing.T) {
	if err := codexQueueResult("Queued message 01a for thread 01b.\n", nil); err != nil {
		t.Errorf("성공 출력: %v", err)
	}
	err := codexQueueResult("warn: x\nError: failed to queue session message: no rollout found for thread id 0 (code -32603)\n", nil)
	if err == nil || !strings.HasPrefix(err.Error(), "Error: failed to queue") {
		t.Errorf("종료 코드가 0이어도 Error 출력이면 실패: %v", err)
	}
	if err := codexQueueResult("", errors.New("exit status 1")); err == nil {
		t.Error("실행 오류는 실패")
	}
}

// 훅이 세션 ID를 못 남겼으면 rollout에서 찾되, 같은 폴더에 다른 코덱스가 살아 있으면 찾지 않는다.
func TestResolveCodexThread(t *testing.T) {
	stubQueue(t, nil)
	a := codexAgent("solo", "%1", state.StateIdle, "")
	a.ID, a.PID = "a", 100
	withSID := codexAgent("x", "%2", state.StateIdle, "from-hook")
	if got := ResolveCodexThread(withSID, nil); got != "from-hook" {
		t.Errorf("훅 값이 정본: %q", got)
	}
	if got := ResolveCodexThread(a, []*state.Agent{a}); got != "" {
		t.Errorf("기동 시각을 모르면 찾지 않는다: %q", got)
	}
	sib := codexAgent("solo", "%3", state.StateWorking, "")
	sib.ID, sib.CWD = "b", a.CWD
	procStartFn = func(int) (time.Time, bool) { return time.Now().Add(-time.Hour), true }
	if got := ResolveCodexThread(a, []*state.Agent{a, sib}); got != "" {
		t.Errorf("같은 폴더에 다른 코덱스가 살아 있으면 찾지 않는다: %q", got)
	}
}
