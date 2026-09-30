package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/state"
)

// mkApp — tmux 밖(데스크톱 앱) 세션 레코드.
func mkApp(kind, name, sid string, pid int, st state.AgentState) *state.Agent {
	return &state.Agent{ID: kind + "-pid" + strconv.Itoa(pid), Kind: kind, State: st, PID: pid, Name: name, SessionID: sid, CWD: "/tmp/app"}
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

// 앱 Claude 세션은 tmux 폴백이 없고 채널 수신함도 아직 없다 — 오류로 끝나고 키 입력을 시도하지 않는다.
// (idle이라 tmux 관문은 통과하는 상태에서도 그렇다.) 사람이 읽는 출력의 대상 표시도 본다.
func TestRunSendToDetachedClaudeFails(t *testing.T) {
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
	// 앱 세션은 pane 수신함이 없으니 채널 후보도 아니다
	if canClaudeChannel(mkApp("claude", "기획서", "10ec8033-ca55", 4243, state.StateIdle), config.Load(), Delivery{StateDir: stateDir}, "x") {
		t.Error("앱 Claude 세션은 아직 채널 대상이 아니다")
	}
}

// 앱 세션도 사람이 읽는 send 출력·JSON에 app 위치와 세션 ID가 실린다(코덱스 큐 경로로 확인).
func TestRunSendDetachedOutputShowsApp(t *testing.T) {
	t.Setenv("AGENTLAYER_CONFIG", t.TempDir()+"/config.json")
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(mkApp("codex", "리뷰", "c0dex-thread-0002", 4246, state.StateIdle))
	old := codexQueueFn
	codexQueueFn = func(context.Context, string, string, string) error { return nil }
	defer func() { codexQueueFn = old }()
	f := &fakeSender{}
	var out bytes.Buffer
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"리뷰", "하나"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "전송 완료 → 리뷰 app [IDLE] (codex queue)") {
		t.Errorf("출력: %s", out.String())
	}
	out.Reset()
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"--json", "리뷰", "둘"}); err != nil {
		t.Fatal(err)
	}
	var res map[string]any
	_ = json.Unmarshal(out.Bytes(), &res)
	if f.calls != 0 || res["where"] != "app" || res["session"] != "리뷰" || res["session_id"] != "c0dex-thread-0002" {
		t.Errorf("JSON: %s tmux=%d", out.String(), f.calls)
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
