package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

// 테스트 프로세스는 Claude 세션 안에서 돌 수도 있다(조상에 claude). 기본값은 "에이전트 조상 없음"으로 고정하고,
// 발신자 판정을 보는 테스트만 stubSender로 끼운다.
func TestMain(m *testing.M) {
	senderProcessFn = func() (string, int) { return "", 0 }
	os.Exit(m.Run())
}

func stubSender(t *testing.T, kind string, pid int) {
	t.Helper()
	old := senderProcessFn
	senderProcessFn = func() (string, int) { return kind, pid }
	t.Cleanup(func() { senderProcessFn = old })
}

func TestParseSendFlagsFrom(t *testing.T) {
	o, rest, err := ParseSendFlags([]string{"--from", "코덱스-앱", "--json", "al-1", "본문"})
	if err != nil || o.From != "코덱스-앱" || !o.JSON || len(rest) != 2 || rest[0] != "al-1" {
		t.Fatalf("%+v %v %v", o, rest, err)
	}
	if o, _, err := ParseSendFlags([]string{"--from=총괄", "x", "y"}); err != nil || o.From != "총괄" {
		t.Fatalf("%+v %v", o, err)
	}
	for _, bad := range [][]string{{"--from"}, {"--from", "a/b", "x", "y"}, {"--from", ".hidden", "x", "y"}, {"--from=" + strings.Repeat("a", 129), "x", "y"}} {
		if _, _, err := ParseSendFlags(bad); err == nil {
			t.Errorf("%v: 오류여야 함", bad)
		}
	}
	// 예전 입력은 그대로
	if o, rest, err := ParseSendFlags([]string{"--force", "collab-bot", "x"}); err != nil || o.From != "" || !o.Force || rest[0] != "collab-bot" {
		t.Fatalf("%+v %v %v", o, rest, err)
	}
}

// tmux 밖 기본 발신자: 코덱스 조상이면 "codex", claude면 주소록 별칭(inbox open) → 없으면 "claude", 그 밖은 "user".
// tmux 안(TMUX_PANE)은 예전 그대로 세션명·"user".
func TestSenderNameOutsideTmux(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	stateDir := t.TempDir()
	agents := []*state.Agent{mkAgent("claude", "collab-bot", "%1", state.StateIdle)}
	if got := senderNameIn(agents, stateDir); got != "user" {
		t.Errorf("조상 없음: %q", got)
	}
	stubSender(t, "codex", 700)
	if got := senderNameIn(agents, stateDir); got != "codex" {
		t.Errorf("코덱스: %q", got)
	}
	stubSender(t, "claude", 800)
	if got := senderNameIn(agents, stateDir); got != "claude" {
		t.Errorf("별칭 없는 claude: %q", got)
	}
	if err := saveAddress(stateDir, Address{Name: "기획서", ID: "al-abc123", Keep: true, PID: 800, Inbox: AddressInbox(stateDir, 800)}); err != nil {
		t.Fatal(err)
	}
	if got := senderNameIn(agents, stateDir); got != "기획서" {
		t.Errorf("별칭 역조회: %q", got)
	}
	if got := senderNameIn(agents, ""); got != "claude" {
		t.Errorf("stateDir 없이는 역조회 없음: %q", got)
	}
	// ID 파일만 있으면(별칭 == ID) ID를 쓴다
	deleteAddress(stateDir, "기획서")
	if got := senderNameIn(agents, stateDir); got != "al-abc123" {
		t.Errorf("ID만: %q", got)
	}
	stubSender(t, "gemini", 900)
	if got := senderNameIn(agents, stateDir); got != "user" {
		t.Errorf("그 밖은 user: %q", got)
	}
	// tmux 안은 예전 그대로
	t.Setenv("TMUX_PANE", "%1")
	stubSender(t, "codex", 700)
	if got := senderNameIn(agents, stateDir); got != "collab-bot" {
		t.Errorf("tmux 안 세션명: %q", got)
	}
	t.Setenv("TMUX_PANE", "%9")
	if got := senderNameIn(agents, stateDir); got != "user" {
		t.Errorf("tmux 안인데 레코드 없음: %q", got)
	}
}

// --from은 주소록(inbox) 경로의 편지 From에 그대로 실린다 — 받는 쪽 `inbox wait`의 from: 줄. 없으면 기본 판정.
func TestSendFromToInbox(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	stubSession(t, 4300, map[int]bool{4300: true})
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	ctx := context.Background()
	done, out, _ := waitInbox(t, ctx, stateDir, "받는쪽", "--name", "받는쪽")
	var sout bytes.Buffer
	if err := RunSend(ctx, &sout, nil, st, stateDir, &fakeSender{}, []string{"--from", "코덱스-앱", "받는쪽", "회신"}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("편지를 받지 못함")
	}
	if out.String() != "from: 코덱스-앱\n\n회신\n" {
		t.Errorf("stdout: %q", out.String())
	}
	// --from 없이 코덱스 조상이면 "codex"
	stubSender(t, "codex", 700)
	done, out, _ = waitInbox(t, ctx, stateDir, "받는쪽", "--name", "받는쪽")
	if err := RunSend(ctx, &sout, nil, st, stateDir, &fakeSender{}, []string{"받는쪽", "둘"}); err != nil {
		t.Fatal(err)
	}
	<-done
	if out.String() != "from: codex\n\n둘\n" {
		t.Errorf("stdout: %q", out.String())
	}
}

// --from은 채널 경로의 meta.from에도 실린다.
func TestSendFromToChannel(t *testing.T) {
	t.Setenv("AGENTLAYER_CONFIG", t.TempDir()+"/config.json")
	stateDir := t.TempDir()
	lines, stop := startSelfServer(t, stateDir, "%61")
	defer stop()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(mkAgent("claude", "collab-bot", "%61", state.StateIdle))
	t.Setenv("TMUX_PANE", "%99")
	var out bytes.Buffer
	if err := RunSend(context.Background(), &out, nil, st, stateDir, &fakeSender{}, []string{"--json", "--from=총괄", "collab-bot", "지시"}); err != nil {
		t.Fatal(err)
	}
	select {
	case l := <-lines:
		var m struct {
			Params struct {
				Meta map[string]string `json:"meta"`
			} `json:"params"`
		}
		if err := json.Unmarshal([]byte(l), &m); err != nil || m.Params.Meta["from"] != "총괄" {
			t.Errorf("meta.from: %s", l)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("세션이 지시를 받지 못함")
	}
}

// 업무 등록 없는 원격 직송은 --from을 명시했을 때만 본문 첫 줄에 보낸이를 붙인다.
func TestSendFromToRemoteDirect(t *testing.T) {
	ad := &scriptedAdapter{handle: "card-f"}
	stubRemoteAdapter(t, ad)
	st, stateDir := newStore(t)
	if err := remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunSend(context.Background(), &out, nil, st, stateDir, nil, []string{"hermes-qa", "그대로"}); err != nil {
		t.Fatal(err)
	}
	if err := RunSend(context.Background(), &out, nil, st, stateDir, nil, []string{"--from", "코덱스-앱", "hermes-qa", "명시"}); err != nil {
		t.Fatal(err)
	}
	if len(ad.dispatched) != 2 || !strings.Contains(ad.dispatched[0], "|그대로|그대로|") || !strings.Contains(ad.dispatched[1], "|보낸이: 코덱스-앱\n명시|") {
		t.Errorf("Dispatch: %q", ad.dispatched)
	}
}
