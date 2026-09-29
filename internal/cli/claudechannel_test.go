package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

func TestPaneInbox(t *testing.T) {
	if got := PaneInbox("/s", "%12"); got != "/s/inboxes/p12" {
		t.Errorf("got %q", got)
	}
	for _, bad := range []string{"", "%", "%1a", "../x"} {
		if got := PaneInbox("/s", bad); got != "" {
			t.Errorf("%q → %q (빈 값이어야 함)", bad, got)
		}
	}
}

// startSelfServer는 pane 전용 수신함을 쥔 채널 서버를 띄우고, 세션이 받는 알림 줄을 돌려주는 채널을 준다.
func startSelfServer(t *testing.T, stateDir, pane string) (lines chan string, stop func()) {
	t.Helper()
	t.Setenv("TMUX_PANE", pane)
	st, _ := state.NewStore(stateDir)
	inr, inw := io.Pipe()
	outr, outw := io.Pipe()
	done := make(chan struct{})
	go func() {
		_ = RunChannel(context.Background(), inr, outw, io.Discard, st, stateDir, "1.11.0", []string{"serve", "--self", "--interval", "10ms"})
		outw.Close()
		close(done)
	}()
	go inw.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"))
	lines = make(chan string, 16)
	go func() {
		br := bufio.NewReader(outr)
		for {
			l, err := br.ReadString('\n')
			if l != "" {
				lines <- l
			}
			if err != nil {
				return
			}
		}
	}()
	<-lines // initialize 응답
	deadline := time.Now().Add(3 * time.Second)
	for !ChannelLive(PaneInbox(stateDir, pane)) {
		if time.Now().After(deadline) {
			t.Fatal("채널 서버가 수신함을 잡지 못함")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return lines, func() { inw.Close(); <-done }
}

// 채널 서버가 뜬 Claude 세션에는 tmux 키 입력이 아니라 채널로 간다. 작업 중이어도 --force 없이 간다.
func TestRunSendClaudeUsesChannel(t *testing.T) {
	t.Setenv("AGENTLAYER_CONFIG", t.TempDir()+"/config.json")
	stateDir := t.TempDir()
	lines, stop := startSelfServer(t, stateDir, "%41")
	defer stop()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(mkAgent("claude", "collab-bot", "%41", state.StateWorking))
	_ = st.Save(mkAgent("claude", "plain-bot", "%42", state.StateIdle))
	var out bytes.Buffer
	f := &fakeSender{}
	t.Setenv("TMUX_PANE", "%99")
	if err := RunSend(context.Background(), &out, strings.NewReader("첫 줄\n둘째 줄\n"), st, stateDir, f, []string{"--json", "collab-bot", "-"}); err != nil {
		t.Fatal(err)
	}
	if f.calls != 0 || !strings.Contains(out.String(), `"via":"channel"`) {
		t.Fatalf("tmux 0회·채널 경로여야 함: tmux=%d out=%s", f.calls, out.String())
	}
	select {
	case l := <-lines:
		var m struct {
			Method string `json:"method"`
			Params struct {
				Content string            `json:"content"`
				Meta    map[string]string `json:"meta"`
			} `json:"params"`
		}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		if m.Method != "notifications/claude/channel" || m.Params.Content != "첫 줄\n둘째 줄" || m.Params.Meta["event"] != "SEND" || m.Params.Meta["from"] != "user" {
			t.Errorf("알림: %s", l)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("세션이 지시를 받지 못함")
	}
	// 채널 서버가 없는 세션은 예전대로 tmux
	out.Reset()
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"--json", "plain-bot", "x"}); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 || !strings.Contains(out.String(), `"via":"tmux"`) {
		t.Errorf("서버 없는 세션은 tmux: tmux=%d out=%s", f.calls, out.String())
	}
}

// 서버가 집어 가지 않으면 편지를 회수하고 tmux 입력으로 되돌아간다 — 두 번 전달되지 않는다.
func TestSendViaChannelReclaimsWhenNotConsumed(t *testing.T) {
	old := channelDeliverWait
	channelDeliverWait = 100 * time.Millisecond
	defer func() { channelDeliverWait = old }()
	inbox := filepath.Join(t.TempDir(), "p7")
	if err := SendViaChannel(inbox, "총괄", "T-1", "본문", time.Now()); err != errNotConsumed {
		t.Fatalf("집어 가지 않으면 오류: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(inbox, "pending", "*.json")); len(left) != 0 {
		t.Fatalf("편지를 회수해야 한다: %v", left)
	}
}

// 기동 전부터 있던 지시는 옛 세션 앞으로 온 것 — 전달하지 않고 치운다(pane ID 재사용).
func TestSelfServerPurgesStaleDirectives(t *testing.T) {
	stateDir := t.TempDir()
	inbox := PaneInbox(stateDir, "%51")
	r := DirectiveReport(inbox, "총괄", "", "낡은 지시", time.Now())
	if err := os.MkdirAll(filepath.Join(inbox, "pending"), 0o700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r)
	if err := os.WriteFile(filepath.Join(inbox, "pending", r.ID+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	lines, stop := startSelfServer(t, stateDir, "%51")
	defer stop()
	select {
	case l := <-lines:
		t.Fatalf("낡은 지시가 전달됨: %s", l)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := os.Stat(filepath.Join(inbox, "quarantine", "stale-"+r.ID+".json")); err != nil {
		t.Errorf("quarantine으로 치워야 한다: %v", err)
	}
}

// tmux 밖에서 뜬 --self 서버는 수신함 없이 핸드셰이크만 받는다.
func TestSelfServerWithoutPane(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	var out, errb bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	if err := RunChannel(context.Background(), in, &out, &errb, st, stateDir, "1.11.0", []string{"serve", "--self"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":1`) || !strings.Contains(errb.String(), "수신함 없이") {
		t.Errorf("out=%s err=%s", out.String(), errb.String())
	}
	if _, err := os.Stat(filepath.Join(stateDir, "inboxes")); err == nil {
		t.Error("수신함을 만들면 안 된다")
	}
}
