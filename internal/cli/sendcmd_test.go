package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/netwaif/agentlayer/internal/state"
)

type fakeSender struct {
	pane, text string
	calls      int
	fail       bool
}

func (f *fakeSender) SendText(paneID, text string) error {
	f.calls++
	f.pane, f.text = paneID, text
	if f.fail {
		return errString("boom")
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }

func withWindow(a *state.Agent, w string) *state.Agent { a.Tmux.WindowName = w; return a }

func TestResolveTarget(t *testing.T) {
	agents := []*state.Agent{
		mkAgent("claude", "search-youtube-bot", "%1", state.StateIdle),
		withWindow(mkAgent("claude", "search-youtube-bot", "%16", state.StateIdle), "t170966"),
		mkAgent("claude", "dead-bot", "%4", state.StateDead),
		mkAgent("codex", "codex-live", "%2", state.StateIdle),
	}
	if a, err := ResolveTarget(agents, "codex-live"); err != nil || a.Tmux.PaneID != "%2" {
		t.Fatalf("세션만: %v %v", a, err)
	}
	if a, err := ResolveTarget(agents, "search-youtube-bot:t170966"); err != nil || a.Tmux.PaneID != "%16" {
		t.Fatalf("세션:창: %v %v", a, err)
	}
	if _, err := ResolveTarget(agents, "search-youtube-bot"); err == nil || !strings.Contains(err.Error(), "둘 이상") {
		t.Fatalf("같은 세션에 pane 둘 → 창 명시 요구: %v", err)
	}
	if _, err := ResolveTarget(agents, "없는세션"); err == nil {
		t.Error("없는 세션은 오류")
	}
	if a, err := ResolveTarget(agents, "dead-bot"); err != nil || a.State != state.StateDead {
		t.Fatalf("dead도 해석은 됨(게이트가 거부): %v %v", a, err)
	}
}

func TestSendGate(t *testing.T) {
	cases := []struct {
		s     state.AgentState
		force bool
		ok    bool
	}{
		{state.StateIdle, false, true}, {state.StateDoneUnread, false, true},
		{state.StateWorking, false, false}, {state.StateWorking, true, true},
		{state.StateWaiting, false, false}, {state.StateWaiting, true, true},
		{state.StateDead, true, false}, {state.StateError, true, false},
	}
	for _, c := range cases {
		if ok, _ := SendGate(c.s, c.force); ok != c.ok {
			t.Errorf("%s force=%v: %v", c.s, c.force, ok)
		}
	}
}

func TestParseSendFlags(t *testing.T) {
	o, rest, err := ParseSendFlags([]string{"--force", "--json", "sess", "안녕"})
	if err != nil || !o.Force || !o.JSON || len(rest) != 2 {
		t.Fatalf("%+v %v %v", o, rest, err)
	}
	if _, _, err := ParseSendFlags([]string{"--nope"}); err == nil {
		t.Error("모르는 플래그는 오류")
	}
	if o, rest, err := ParseSendFlags([]string{"bot", "please", "--force", "here"}); err != nil || o.Force || len(rest) != 4 ||
		rest[0] != "bot" || rest[1] != "please" || rest[2] != "--force" || rest[3] != "here" {
		t.Errorf("첫 위치 인자 뒤 --force는 본문: %+v %v %v", o, rest, err)
	}
	if o, rest, err := ParseSendFlags([]string{"--json", "bot", "--force"}); err != nil || !o.JSON || o.Force ||
		len(rest) != 2 || rest[0] != "bot" || rest[1] != "--force" {
		t.Errorf("첫 위치 인자 뒤 --force는 본문(플래그 선행): %+v %v %v", o, rest, err)
	}
}

func TestSanitizeMessage(t *testing.T) {
	if got := SanitizeMessage("첫 줄\r\n둘째 줄\r\n"); got != "첫 줄\n둘째 줄\n" {
		t.Errorf("CRLF → LF: %q", got)
	}
	if got := SanitizeMessage("경고\x1b[31m빨강\x1b[0m"); got != "경고[31m빨강[0m" {
		t.Errorf("이스케이프 문자 제거: %q", got)
	}
	if got := SanitizeMessage("탭\t개행\n유지"); got != "탭\t개행\n유지" {
		t.Errorf("탭·개행은 보존: %q", got)
	}
}

func TestRunSendRejectsOversizedBody(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	_ = st.Save(mkAgent("claude", "collab-bot", "%1", state.StateIdle))
	var out bytes.Buffer
	big := strings.Repeat("a", 65537)
	err := RunSend(&out, strings.NewReader(big), st, &fakeSender{}, []string{"collab-bot", "-"})
	if err == nil || !strings.Contains(err.Error(), "너무 깁니다") {
		t.Fatalf("64KiB 초과는 거부: %v", err)
	}
}

func TestRunSendDeliversAndGates(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	_ = st.Save(mkAgent("claude", "collab-bot", "%1", state.StateIdle))
	_ = st.Save(mkAgent("claude", "busy-bot", "%2", state.StateWorking))
	var out bytes.Buffer
	f := &fakeSender{}
	if err := RunSend(&out, nil, st, f, []string{"collab-bot", "주제", "3개"}); err != nil {
		t.Fatal(err)
	}
	if f.pane != "%1" || f.text != "주제 3개" {
		t.Errorf("전송: %q %q", f.pane, f.text)
	}
	out.Reset()
	f = &fakeSender{}
	err := RunSend(&out, nil, st, f, []string{"busy-bot", "x"})
	if err == nil || f.calls != 0 {
		t.Fatalf("WORKING은 거부(전송 0회): err=%v calls=%d", err, f.calls)
	}
	if err := RunSend(&out, nil, st, f, []string{"--force", "busy-bot", "x"}); err != nil || f.calls != 1 {
		t.Fatalf("--force면 전송: %v %d", err, f.calls)
	}
	// stdin 본문
	f = &fakeSender{}
	if err := RunSend(&out, strings.NewReader("첫 줄\n둘째 줄\n"), st, f, []string{"collab-bot", "-"}); err != nil {
		t.Fatal(err)
	}
	if f.text != "첫 줄\n둘째 줄" {
		t.Errorf("stdin 본문(끝 개행 제거): %q", f.text)
	}
	// --json
	out.Reset()
	if err := RunSend(&out, nil, st, &fakeSender{}, []string{"--json", "collab-bot", "x"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"sent":true`) || !strings.Contains(out.String(), `"pane":"%1"`) {
		t.Errorf("json 출력: %s", out.String())
	}
	if err := RunSend(&out, nil, st, &fakeSender{fail: true}, []string{"collab-bot", "x"}); err == nil {
		t.Error("전송 실패는 에러")
	}
	if err := RunSend(&out, nil, st, f, []string{"collab-bot"}); err == nil {
		t.Error("메시지 없으면 오류")
	}
}
