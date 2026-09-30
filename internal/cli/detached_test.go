package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

// 테스트 프로세스는 Claude 세션 안에서 돌 수도 있다(조상에 claude). 기본값은 "Claude 프로세스 없음"으로 고정하고,
// tmux 밖 경로를 보는 테스트만 stubSelfProcess로 PID를 준다.
func TestMain(m *testing.M) {
	selfProcessFn = func() int { return 0 }
	os.Exit(m.Run())
}

func stubSelfProcess(t *testing.T, pid int) {
	t.Helper()
	old := selfProcessFn
	selfProcessFn = func() int { return pid }
	t.Cleanup(func() { selfProcessFn = old })
}

// mkApp — tmux 밖(데스크톱 앱) 세션 레코드.
func mkApp(kind, name, sid string, pid int, st state.AgentState) *state.Agent {
	return &state.Agent{ID: kind + "-pid" + strconv.Itoa(pid), Kind: kind, State: st, PID: pid, Name: name, SessionID: sid, CWD: "/tmp/app"}
}

func TestProcessInboxAndAgentInbox(t *testing.T) {
	if got := ProcessInbox("/s", 4242); got != "/s/inboxes/pid4242" {
		t.Errorf("got %q", got)
	}
	if got := ProcessInbox("/s", 0); got != "" {
		t.Errorf("pid 0은 빈 값: %q", got)
	}
	if got := AgentInbox("/s", mkAgent("claude", "ai", "%12", state.StateIdle)); got != "/s/inboxes/p12" {
		t.Errorf("pane 세션: %q", got)
	}
	if got := AgentInbox("/s", mkApp("claude", "", "sid", 4242, state.StateIdle)); got != "/s/inboxes/pid4242" {
		t.Errorf("앱 세션: %q", got)
	}
	if got := AgentInbox("/s", nil); got != "" {
		t.Errorf("nil: %q", got)
	}
}

// send 대상: tmux 세션 이름이 먼저, 없으면 -n 이름 정확 일치나 세션 ID 접두(4자 이상). 모호하면 후보를 보이고 거부.
func TestResolveTargetByNameOrSessionID(t *testing.T) {
	plan := mkApp("claude", "기획서", "10ec8033-ca55-4c1e-9f1a-000000000001", 500, state.StateIdle)
	review := mkApp("claude", "리뷰", "10ec9999-0000-4c1e-9f1a-000000000002", 501, state.StateWorking)
	deadTwin := mkApp("claude", "기획서", "abcdef00-0000-4c1e-9f1a-000000000003", 502, state.StateDead)
	pane := mkAgent("claude", "collab-bot", "%1", state.StateIdle)
	pane.SessionID = "77777777-1111-4c1e-9f1a-000000000004"
	agents := []*state.Agent{pane, plan, review, deadTwin}

	if a, err := ResolveTarget(agents, "collab-bot"); err != nil || a != pane {
		t.Fatalf("tmux 세션 이름 우선: %v %v", a, err)
	}
	if a, err := ResolveTarget(agents, "기획서"); err != nil || a != plan {
		t.Fatalf("-n 이름(산 것 우선): %v %v", a, err)
	}
	if a, err := ResolveTarget(agents, "10ec9999"); err != nil || a != review {
		t.Fatalf("세션 ID 접두: %v %v", a, err)
	}
	if a, err := ResolveTarget(agents, "77777777-1111"); err != nil || a != pane {
		t.Fatalf("pane 세션도 세션 ID로: %v %v", a, err)
	}
	_, err := ResolveTarget(agents, "10ec")
	if err == nil || !strings.Contains(err.Error(), "둘 이상") || !strings.Contains(err.Error(), "기획서 (app, 세션 10ec8033)") || !strings.Contains(err.Error(), "리뷰 (app, 세션 10ec9999)") {
		t.Fatalf("모호하면 후보를 보이고 거부: %v", err)
	}
	if _, err := ResolveTarget(agents, "10e"); err == nil || strings.Contains(err.Error(), "둘 이상") {
		t.Fatalf("3자 접두는 안 찾는다: %v", err)
	}
	if _, err := ResolveTarget(agents, "기획서:t123456"); err == nil {
		t.Fatal("창 지정은 tmux 세션에만")
	}
	if a, err := ResolveTarget(agents, "abcdef00"); err != nil || a != deadTwin {
		t.Fatalf("죽은 것만 맞으면 그것(게이트가 거부): %v %v", a, err)
	}
}

// 채널 서버가 뜬 앱 세션에는 이름·세션 ID로 찾아 채널로 보낸다. 서버는 자기 Claude 프로세스 PID 수신함을 쥔다.
func TestRunSendToDetachedSessionViaChannel(t *testing.T) {
	t.Setenv("AGENTLAYER_CONFIG", t.TempDir()+"/config.json")
	stubSelfProcess(t, 4242)
	stateDir := t.TempDir()
	lines, stop := startSelfServer(t, stateDir, "")
	defer stop()
	st, _ := state.NewStore(stateDir)
	app := mkApp("claude", "기획서", "10ec8033-ca55-4c1e-9f1a-000000000001", 4242, state.StateWorking)
	_ = st.Save(app)
	f := &fakeSender{}
	for _, target := range []string{"기획서", "10ec8033"} {
		var out bytes.Buffer
		if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"--json", target, "지시 본문"}); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		var res map[string]any
		_ = json.Unmarshal(out.Bytes(), &res)
		if f.calls != 0 || res["via"] != "channel" || res["session"] != "기획서" || res["where"] != "app" || res["session_id"] != app.SessionID {
			t.Fatalf("%s: tmux 0회·채널 경로·앱 표시여야 함: tmux=%d %s", target, f.calls, out.String())
		}
		select {
		case l := <-lines:
			if !strings.Contains(l, `"event":"SEND"`) || !strings.Contains(l, "지시 본문") {
				t.Errorf("알림: %s", l)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("세션이 지시를 받지 못함")
		}
	}
	// 사람이 읽는 출력도 앱 표시
	var out bytes.Buffer
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"기획서", "둘째"}); err != nil {
		t.Fatal(err)
	}
	<-lines
	if !strings.Contains(out.String(), "전송 완료 → 기획서 app [WORKING] (채널)") {
		t.Errorf("출력: %s", out.String())
	}
}

// 앱 세션은 tmux 폴백이 없다 — 채널 서버가 없으면 오류로 끝나고 키 입력을 시도하지 않는다.
func TestRunSendToDetachedWithoutChannelFails(t *testing.T) {
	t.Setenv("AGENTLAYER_CONFIG", t.TempDir()+"/config.json")
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(mkApp("claude", "기획서", "10ec8033-ca55", 4243, state.StateIdle))
	f := &fakeSender{}
	var out bytes.Buffer
	err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"기획서", "지시"})
	if err == nil || !errors.Is(err, errNoFallback) || f.calls != 0 {
		t.Fatalf("폴백 없이 오류여야 함: err=%v tmux=%d", err, f.calls)
	}
	if !strings.Contains(err.Error(), "기획서 전송 실패") {
		t.Errorf("대상 이름이 보여야 함: %v", err)
	}
}

// 서버가 잠금은 쥐었는데 집어 가지 않으면(먹통) 지시를 회수하고 오류 — 앱 세션은 tmux로 되돌아갈 곳이 없다.
func TestRunSendToDetachedChannelNotConsumedFails(t *testing.T) {
	t.Setenv("AGENTLAYER_CONFIG", t.TempDir()+"/config.json")
	old := channelDeliverWait
	channelDeliverWait = 100 * time.Millisecond
	defer func() { channelDeliverWait = old }()
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(mkApp("claude", "기획서", "10ec8033-ca55", 4244, state.StateIdle))
	inbox := ProcessInbox(stateDir, 4244)
	if err := os.MkdirAll(inbox, 0o700); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(inbox, ".channel.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	f := &fakeSender{}
	var out bytes.Buffer
	err = RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"기획서", "지시"})
	if err == nil || !strings.Contains(err.Error(), "채널 전송 실패") || f.calls != 0 {
		t.Fatalf("채널 실패는 오류로 끝나야 함: err=%v tmux=%d", err, f.calls)
	}
	if left, _ := filepath.Glob(filepath.Join(inbox, "pending", "*.json")); len(left) != 0 {
		t.Errorf("지시를 회수해야 함: %v", left)
	}
}

// 앱에서 뜬 코덱스는 큐가 정본이고, 큐가 실패하면 tmux 폴백 없이 오류.
func TestRunSendToDetachedCodexQueue(t *testing.T) {
	t.Setenv("AGENTLAYER_CONFIG", t.TempDir()+"/config.json")
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(mkApp("codex", "", "c0dex-thread-0001", 4245, state.StateWorking))
	var got []string
	fail := false
	old := codexQueueFn
	codexQueueFn = func(_ context.Context, thread, cwd, msg string) error {
		got = append(got, thread+"|"+msg)
		if fail {
			return errors.New("no rollout found")
		}
		return nil
	}
	defer func() { codexQueueFn = old }()
	f := &fakeSender{}
	var out bytes.Buffer
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"--json", "c0dex-th", "하나"}); err != nil {
		t.Fatal(err)
	}
	if f.calls != 0 || !strings.Contains(out.String(), `"via":"queue"`) || len(got) != 1 || got[0] != "c0dex-thread-0001|하나" {
		t.Fatalf("큐 경로여야 함: tmux=%d out=%s got=%v", f.calls, out.String(), got)
	}
	fail = true
	err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"c0dex-th", "둘"})
	if err == nil || !strings.Contains(err.Error(), "codex queue 실패") || f.calls != 0 {
		t.Fatalf("큐 실패는 오류: err=%v tmux=%d", err, f.calls)
	}
}

// status 표는 앱 세션을 이름(또는 세션 ID 앞 8자리)과 "(app)"으로 보인다.
func TestStatusShowsDetachedSessions(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.NewStore(dir)
	_ = st.Save(mkApp("claude", "기획서", "10ec8033-ca55-4c1e", 500, state.StateWorking))
	_ = st.Save(mkApp("claude", "", "abcdef12-3456-7890", 501, state.StateIdle))
	_ = st.Save(mkAgent("claude", "collab-bot", "%1", state.StateIdle))
	var buf bytes.Buffer
	if err := Status(&buf, st, false, time.Now(), nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"기획서 (app)", "abcdef12 (app)", "collab-bot"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q가 보여야 함:\n%s", want, out)
		}
	}
}

func TestFindAgentByNameOrSessionID(t *testing.T) {
	app := mkApp("claude", "기획서", "10ec8033-ca55-4c1e", 500, state.StateIdle)
	agents := []*state.Agent{mkAgent("claude", "collab-bot", "%1", state.StateIdle), app}
	if FindAgent(agents, "기획서") != app || FindAgent(agents, "10ec8033") != app || FindAgent(agents, "claude-pid500") != app {
		t.Error("이름·세션 ID 접두로 찾아야 함")
	}
	if FindAgent(agents, "collab-bot") == nil || FindAgent(agents, "없음") != nil {
		t.Error("예전 규칙 유지")
	}
}
