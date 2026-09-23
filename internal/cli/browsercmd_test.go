package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/go-rod/rod"
	"github.com/netwaif/agentlayer/internal/browser"
	"github.com/netwaif/agentlayer/internal/state"
)

// 플래그는 인자 위치와 무관하게 인식돼야 한다 — Go flag는 첫 비플래그
// 인자에서 멈추므로 `shot <url> --send`가 --send를 조용히 삼키면 안 된다.
func TestParseShotArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    shotOpts
		wantErr bool
	}{
		{"url 뒤 --send", []string{"https://example.com", "--send"}, shotOpts{URL: "https://example.com", Send: true}, false},
		{"--send 뒤 url", []string{"--send", "https://example.com"}, shotOpts{URL: "https://example.com", Send: true}, false},
		{"인자 없음", nil, shotOpts{}, false},
		{"url만", []string{"https://example.com"}, shotOpts{URL: "https://example.com"}, false},
		{"--notify", []string{"--notify", "https://example.com"}, shotOpts{URL: "https://example.com", Notify: true}, false},
		{"--send --notify 동시", []string{"https://example.com", "--send", "--notify"}, shotOpts{URL: "https://example.com", Send: true, Notify: true}, false},
		{"잉여 인자는 에러", []string{"a", "b"}, shotOpts{}, true},
		{"모르는 플래그는 에러", []string{"--bogus"}, shotOpts{}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseShotArgs(c.args)
			if c.wantErr {
				if err == nil {
					t.Fatalf("에러여야 함: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestRunBrowserOpenRequiresURL(t *testing.T) {
	var out bytes.Buffer
	err := RunBrowser(&out, []string{"open"})
	if err == nil || !strings.Contains(err.Error(), "사용법") {
		t.Fatalf("url 없으면 사용법 에러, got %v", err)
	}
}

// errors는 위치 인자가 없다 — --send만 인식하고 잉여 인자는 에러.
func TestParseErrorsArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		send    bool
		wantErr bool
	}{
		{"인자 없음", nil, false, false},
		{"--send", []string{"--send"}, true, false},
		{"--reload", []string{"--reload"}, false, false},
		{"잉여 인자는 에러", []string{"뭔가"}, false, true},
		{"잉여 인자 뒤 --send도 에러", []string{"뭔가", "--send"}, false, true},
		{"모르는 플래그는 에러", []string{"--bogus"}, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o, err := parseErrorsArgs(c.args)
			if c.wantErr {
				if err == nil {
					t.Fatalf("에러여야 함: %+v", o)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if o.Send != c.send {
				t.Errorf("got send=%v, want %v", o.Send, c.send)
			}
			if o.Reload != (len(c.args) > 0 && c.args[0] == "--reload") {
				t.Errorf("reload 파싱: %+v", o)
			}
		})
	}
}

// preview 인자는 포트 목록 — 각각 정수여야 하고 범위(1~65535)를 벗어나면 에러.
func TestParsePreviewArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		ports   []int
		wantErr bool
	}{
		{"인자 없음", nil, nil, false},
		{"포트 하나", []string{"3000"}, []int{3000}, false},
		{"포트 여럿", []string{"3000", "5173"}, []int{3000, 5173}, false},
		{"비정수는 에러", []string{"abc"}, nil, true},
		{"섞여도 에러", []string{"3000", "abc"}, nil, true},
		{"0은 에러", []string{"0"}, nil, true},
		{"범위 초과는 에러", []string{"70000"}, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ports, err := parsePreviewArgs(c.args)
			if c.wantErr {
				if err == nil {
					t.Fatalf("에러여야 함: %v", ports)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(ports) != len(c.ports) {
				t.Fatalf("got %v, want %v", ports, c.ports)
			}
			for i := range ports {
				if ports[i] != c.ports[i] {
					t.Errorf("got %v, want %v", ports, c.ports)
				}
			}
		})
	}
}

func chooseCands() []*state.Agent {
	return []*state.Agent{
		{ID: "claude-1", Kind: "claude", Tmux: state.TmuxRef{Session: "dev", PaneID: "%1"}},
		{ID: "codex-2", Kind: "codex", Tmux: state.TmuxRef{Session: "fix", PaneID: "%2"}},
	}
}

// 후보 1명이면 입력을 묻지 않고 즉시 그 후보를 돌려준다.
func TestChooseAgentSingle(t *testing.T) {
	var out bytes.Buffer
	got, err := chooseAgent(chooseCands()[:1], strings.NewReader(""), &out)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "claude-1" {
		t.Errorf("단일 후보 즉시 반환이어야: %v", got.ID)
	}
	if out.Len() != 0 {
		t.Errorf("단일 후보에 목록 출력 금지: %q", out.String())
	}
}

// 후보 복수면 번호 목록(kind·세션명)을 띄우고 stdin 번호로 선택한다 —
// 무통보 cands[0] 전송 금지 (스펙 라우팅 규칙: 후보 복수면 선택).
func TestChooseAgentMultiple(t *testing.T) {
	var out bytes.Buffer
	got, err := chooseAgent(chooseCands(), strings.NewReader("2\n"), &out)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "codex-2" {
		t.Errorf("2번 선택은 codex-2여야: %v", got.ID)
	}
	for _, want := range []string{"1)", "2)", "claude", "dev", "codex", "fix"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("목록에 %q 없음: %q", want, out.String())
		}
	}
}

// 비정수·범위 밖·빈 입력은 에러.
func TestChooseAgentInvalidInput(t *testing.T) {
	for _, in := range []string{"abc\n", "0\n", "3\n", ""} {
		var out bytes.Buffer
		if _, err := chooseAgent(chooseCands(), strings.NewReader(in), &out); err == nil {
			t.Errorf("입력 %q는 에러여야 함", in)
		}
	}
}

// 미지 서브커맨드는 명확한 에러로 알린다 — Task 6~8이 case를 추가해도
// default 분기의 문구는 유지돼야 한다.
func TestRunBrowserUnknownSub(t *testing.T) {
	var buf bytes.Buffer
	err := RunBrowser(&buf, []string{"없는명령"})
	if err == nil || !strings.Contains(err.Error(), "없는명령") {
		t.Fatalf("미지 서브커맨드는 에러: %v", err)
	}
	if !strings.Contains(err.Error(), "help") {
		t.Errorf("에러에 help 안내가 없다: %v", err)
	}
}

func TestMCPCommands(t *testing.T) {
	lines := MCPCommands(9333)
	want := []string{
		"claude mcp add --scope user chrome-devtools -- npx chrome-devtools-mcp@latest --browserUrl=http://127.0.0.1:9333",
		"codex mcp add chrome-devtools -- npx chrome-devtools-mcp@latest --browserUrl=http://127.0.0.1:9333",
		"gemini mcp add --scope user chrome-devtools npx -- chrome-devtools-mcp@latest --browserUrl=http://127.0.0.1:9333",
	}
	if len(lines) != len(want) {
		t.Fatalf("줄 수 %d, want %d: %v", len(lines), len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("[%d]\n got %q\nwant %q", i, lines[i], want[i])
		}
	}
}

func TestRunBrowserMCPPrintsCommands(t *testing.T) {
	var out bytes.Buffer
	if err := RunBrowser(&out, []string{"mcp"}); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"claude mcp add", "codex mcp add", "gemini mcp add"} {
		if !strings.Contains(out.String(), tool) {
			t.Errorf("%q 누락:\n%s", tool, out.String())
		}
	}
}

func TestParsePickArgsAgent(t *testing.T) {
	id, once, err := parsePickArgs([]string{"--agent", "claude-7"})
	if err != nil || id != "claude-7" || once {
		t.Fatalf("got %q %v %v", id, once, err)
	}
	if id, once, err := parsePickArgs(nil); err != nil || id != "" || once {
		t.Fatalf("인자 없음: %q %v %v", id, once, err)
	}
	if _, _, err := parsePickArgs([]string{"extra"}); err == nil {
		t.Error("잉여 인자는 에러")
	}
	if _, once, err := parsePickArgs([]string{"--once"}); err != nil || !once {
		t.Errorf("--once: %v %v", once, err)
	}
	if _, _, err := parsePickArgs([]string{"--once", "--agent", "x"}); err == nil {
		t.Error("--once와 --agent 동시 지정은 에러")
	}
}

func TestParseShotArgsAgent(t *testing.T) {
	o, err := parseShotArgs([]string{"--send", "--agent", "codex-2"})
	if err != nil || o.Agent != "codex-2" || !o.Send {
		t.Fatalf("got %+v %v", o, err)
	}
}

func TestFilterAgentByID(t *testing.T) {
	agents := []*state.Agent{{ID: "a"}, {ID: "b"}}
	got, err := FilterAgentByID(agents, "b")
	if err != nil || len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := FilterAgentByID(agents, "zzz"); err == nil || !strings.Contains(err.Error(), "zzz") {
		t.Errorf("없는 ID는 ID를 품은 에러: %v", err)
	}
}

// pick 대기 중 터미널에서 esc·q·Ctrl-C면 관제탑으로 돌아간다.
func TestQuitKey(t *testing.T) {
	for _, b := range []byte{0x1b, 'q', 0x03} {
		if !quitKey(b) {
			t.Errorf("%#x는 종료 키", b)
		}
	}
	for _, b := range []byte{'a', '\n', ' '} {
		if quitKey(b) {
			t.Errorf("%#x는 종료 키 아님", b)
		}
	}
}

func TestIsMCPToolCall(t *testing.T) {
	if !IsMCPToolCall([]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_pages"}}` + "\n")) {
		t.Error("tools/call 인식")
	}
	for _, l := range []string{`{"jsonrpc":"2.0","id":1,"method":"initialize"}`, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, `not json`} {
		if IsMCPToolCall([]byte(l)) {
			t.Errorf("%s 는 도구 호출 아님", l)
		}
	}
}

// TestToolCallName — Fix round 1: new_page만 골라 osascript를 부르는 근거이므로
// tools/call+click, tools/call+new_page, tools/list, 깨진 JSON 네 경우를 표로 확인한다.
func TestToolCallName(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{"click", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"click","arguments":{"pageId":1}}}`, "click"},
		{"new_page", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"new_page","arguments":{}}}`, "new_page"},
		{"tools/list", `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`, ""},
		{"garbage", `not json`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := toolCallName([]byte(c.line)); got != c.want {
				t.Errorf("toolCallName(%q) = %q, want %q", c.line, got, c.want)
			}
		})
	}
}

// TestFxSignalerSyncTabsReconnectsAfterFailure — Fix round 2 (a): 죽은 연결 감지는
// syncTabs()가 실제로 실패했을 때만 일어난다 — 핫 패스(연결이 멀쩡한 보통 호출)에
// 판정용 CDP 왕복을 추가로 태우지 않는다. f.sync를 주입해 실제 Chrome 없이 검증한다.
func TestFxSignalerSyncTabsReconnectsAfterFailure(t *testing.T) {
	connectCalls := 0
	f := newFxSignaler(false, func() (*rod.Browser, error) {
		connectCalls++
		return &rod.Browser{}, nil
	})
	syncCalls := 0
	f.sync = func(*rod.Browser, browser.Control, string, string, string) ([]browser.TabRequest, error) {
		syncCalls++
		if syncCalls == 1 {
			return nil, errors.New("연결 죽음")
		}
		return nil, nil
	}
	if reqs := f.syncTabs(browser.Control{}, "", ""); reqs != nil {
		t.Fatalf("첫 호출은 실패 주입 — nil이어야 함: %v", reqs)
	}
	if connectCalls != 1 {
		t.Fatalf("첫 연결: connectCalls=%d", connectCalls)
	}
	f.syncTabs(browser.Control{}, "", "")
	if connectCalls != 2 {
		t.Fatalf("실패로 버려진 연결이 다음 호출에서 재연결돼야 함: connectCalls=%d", connectCalls)
	}
}

// TestFxSignalerSyncTabsKeepsConnectionOnSuccess — sync가 성공하면 재연결하지 않는다
// (불필요한 connect 호출 방지 확인).
func TestFxSignalerSyncTabsKeepsConnectionOnSuccess(t *testing.T) {
	connectCalls := 0
	f := newFxSignaler(false, func() (*rod.Browser, error) {
		connectCalls++
		return &rod.Browser{}, nil
	})
	f.sync = func(*rod.Browser, browser.Control, string, string, string) ([]browser.TabRequest, error) {
		return nil, nil
	}
	f.syncTabs(browser.Control{}, "", "")
	f.syncTabs(browser.Control{}, "", "")
	if connectCalls != 1 {
		t.Fatalf("성공하면 재연결 없음: connectCalls=%d", connectCalls)
	}
}

// TestFxSignalerDisabledStillTracksToolNames — 게이트 리뷰 1차: OnClientLine/OnServerLine의
// "도구명 추적은 항상, FX 신호는 enabled일 때만" 분리를 검증한다. browser_fx가 꺼져도
// trim.go가 쓸 도구명은 End에서 나와야 하고, 그 경로는 connect()(신호 송출)를 부르면 안 된다.
func TestFxSignalerDisabledStillTracksToolNames(t *testing.T) {
	waitForCall := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"wait_for","arguments":{}}}` + "\n")
	waitForReply := []byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[]}}` + "\n")

	t.Run("disabled: 신호 없이 도구명만", func(t *testing.T) {
		connectCalls := 0
		f := newFxSignaler(false, func() (*rod.Browser, error) {
			connectCalls++
			return nil, errors.New("연결 안 함")
		})
		f.OnClientLine(waitForCall)
		tool := f.OnServerLine(waitForReply)
		if tool != "wait_for" {
			t.Fatalf("도구명 반환: %q", tool)
		}
		if connectCalls != 0 {
			t.Fatalf("enabled=false면 connect(신호 송출)를 부르면 안 됨: connectCalls=%d", connectCalls)
		}
	})

	t.Run("enabled: 장전 뒤 Flush로 송출, 도구명은 그대로", func(t *testing.T) {
		connectCalls := 0
		f := newFxSignaler(true, func() (*rod.Browser, error) {
			connectCalls++
			return nil, errors.New("연결 실패") // 실제 브라우저 없이 signal()의 실패-삼킴 경로만 탐
		})
		f.OnClientLine(waitForCall)
		if connectCalls != 0 {
			t.Fatalf("OnClientLine은 장전만 한다(게이트 미러 왕복에 실어 보낸다): connectCalls=%d", connectCalls)
		}
		f.Flush() // 게이트가 미러를 건너뛴 경우의 대체 경로
		if connectCalls == 0 {
			t.Fatal("Flush는 장전된 신호를 송출해야 함(connect 호출)")
		}
		tool := f.OnServerLine(waitForReply)
		if tool != "wait_for" {
			t.Fatalf("도구명 반환: %q", tool)
		}
	})
}

// TestFxSignalerPendingRidesOnMirrorSync — 최종 리뷰 IMPORTANT 3-c: 장전된 fx "on" 값은
// 게이트의 미러 왕복(syncTabs)에 실려 나가고, 그 뒤 Flush는 아무것도 더 보내지 않는다.
func TestFxSignalerPendingRidesOnMirrorSync(t *testing.T) {
	connectCalls := 0
	f := newFxSignaler(true, func() (*rod.Browser, error) {
		connectCalls++
		return &rod.Browser{}, nil
	})
	var gotFx []string
	f.sync = func(_ *rod.Browser, _ browser.Control, _, _, fxOn string) ([]browser.TabRequest, error) {
		gotFx = append(gotFx, fxOn)
		return nil, nil
	}
	f.OnClientLine([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"click"}}` + "\n"))
	f.syncTabs(browser.Control{}, "https://x", "제목")
	if len(gotFx) != 1 || !strings.HasPrefix(gotFx[0], "on:click:") {
		t.Fatalf("미러 왕복에 on 신호가 실려야 함: %v", gotFx)
	}
	f.Flush()
	f.syncTabs(browser.Control{}, "https://x", "제목")
	if len(gotFx) != 2 || gotFx[1] != "" {
		t.Fatalf("이미 실어 보냈으면 다음 왕복엔 빈 값: %v", gotFx)
	}
	if connectCalls != 1 {
		t.Fatalf("Flush가 별도 SignalFx 왕복을 또 내면 안 됨: connectCalls=%d", connectCalls)
	}
}

// TestFxSignalerCancelDropsTracking — 게이트가 막은 호출은 서버로 안 가므로 응답도 없다.
// 추적을 안 풀면 inflight가 남아 이후 어떤 응답도 "마지막"이 되지 못해 FX가 안 꺼진다.
func TestFxSignalerCancelDropsTracking(t *testing.T) {
	blocked := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"click"}}` + "\n")
	next := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"wait_for"}}` + "\n")
	f := newFxSignaler(false, func() (*rod.Browser, error) { return nil, errors.New("없음") })
	f.OnClientLine(blocked)
	f.Cancel(blocked)
	f.OnClientLine(next)
	if tool := f.OnServerLine([]byte(`{"jsonrpc":"2.0","id":2,"result":{}}` + "\n")); tool != "wait_for" {
		t.Fatalf("도구명: %q", tool)
	}
	if n := f.tracker.Inflight(); n != 0 {
		t.Fatalf("막힌 호출의 추적이 남아 있음: inflight=%d", n)
	}
}

// TestBrowserControlReset — 최종 리뷰 CRITICAL 1의 비상구. 오버레이 버튼을 누를 수 없는
// 상황에서 사용자 소유로 잠긴 정본을 CLI로 푼다.
func TestBrowserControlReset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTLAYER_STATE_DIR", dir)
	_ = browser.SaveControl(dir, browser.Control{Owner: browser.OwnerUser, Stopped: true, Agent: "claude-%1"})
	var out strings.Builder
	if err := RunBrowser(&out, []string{"control", "reset"}); err != nil {
		t.Fatal(err)
	}
	c := browser.LoadControl(dir)
	if c.Owner != browser.OwnerIdle || c.Stopped {
		t.Fatalf("idle로 초기화돼야 함: %+v", c)
	}
	if c.LastRequestMs == 0 {
		t.Fatal("ack를 지금으로 올려야 함 — 탭에 남은 옛 클릭이 다시 잠그면 안 된다")
	}
	if !strings.Contains(out.String(), "초기화") {
		t.Fatalf("한 줄 안내: %q", out.String())
	}
	if err := RunBrowser(&out, []string{"control"}); err == nil {
		t.Fatal("reset 없이 부르면 사용법 오류")
	}
	if err := RunBrowser(&out, []string{"control", "nope"}); err == nil {
		t.Fatal("모르는 하위 명령은 오류")
	}
}

// TestSetTargetFallbackKeepsTitle — 최종 리뷰 MINOR (a): pageId 없는 호출이 같은 탭을
// 가리키면 제목을 유지한다(다른 탭의 띠가 "· " 뒤 빈칸으로 깜빡이지 않게).
func TestSetTargetFallbackKeepsTitle(t *testing.T) {
	f := newFxSignaler(false, func() (*rod.Browser, error) { return nil, errors.New("없음") })
	f.setTarget("https://a", true)
	f.setTargetTitle("A 제목")
	f.setTargetFallback("https://a", "", true)
	if url, title := f.target(); url != "https://a" || title != "A 제목" {
		t.Fatalf("같은 url이고 제목을 모르면 유지: %q %q", url, title)
	}
	// PageMap이 제목을 알면(실측 지적) 그대로 쓴다 — pageId 없는 첫 호출에서도 띠에 제목이 뜬다
	f.setTargetFallback("https://b", "B 제목", true)
	if url, title := f.target(); url != "https://b" || title != "B 제목" {
		t.Fatalf("아는 제목은 그대로 싣는다: %q %q", url, title)
	}
	f.setTargetFallback("https://c", "", true)
	if url, title := f.target(); url != "https://c" || title != "" {
		t.Fatalf("다른 url인데 제목을 모르면 비운다: %q %q", url, title)
	}
	f.setTargetFallback("", "", false)
	if url, title := f.target(); url != "" || title != "" {
		t.Fatalf("미상이면 둘 다 비운다: %q %q", url, title)
	}
}

func TestParseCookiesExport(t *testing.T) {
	cases := []struct {
		args    []string
		want    browser.ExportOptions
		wantErr bool
	}{
		{[]string{"claude.ai", "sessionKey", "--to", "~/k.txt"},
			browser.ExportOptions{Domain: "claude.ai", Name: "sessionKey", To: "~/k.txt"}, false},
		{[]string{"--to", "c.txt", "youtube.com"},
			browser.ExportOptions{Domain: "youtube.com", To: "c.txt"}, false},
		{[]string{"x.com", "--format", "json", "--to", "x.json"},
			browser.ExportOptions{Domain: "x.com", Format: browser.FormatJSONF, To: "x.json"}, false},
		{[]string{"claude.ai", "sessionKey", "--env", "~/bot/.env", "CLAUDE_SESSION_KEY"},
			browser.ExportOptions{Domain: "claude.ai", Name: "sessionKey", EnvFile: "~/bot/.env", EnvKey: "CLAUDE_SESSION_KEY"}, false},
		{[]string{"claude.ai", "sessionKey", "--env", "~/bot/.env"}, browser.ExportOptions{}, true}, // KEY 없음
		{[]string{}, browser.ExportOptions{}, true},
		{[]string{"a.com", "b", "c", "--to", "f"}, browser.ExportOptions{}, true}, // 인자 초과
	}
	for _, c := range cases {
		got, err := parseCookiesExport(c.args)
		if c.wantErr {
			if err == nil {
				t.Errorf("%v: 에러여야 함 (got %+v)", c.args, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%v: %v", c.args, err)
			continue
		}
		if got != c.want {
			t.Errorf("%v:\n got %+v\nwant %+v", c.args, got, c.want)
		}
	}
}

// 응답을 그 호출의 pageId와 짝짓는다 — "Page navigated to" 반영용(browsercmd.go popPage).
func TestFxSignalerPopPage(t *testing.T) {
	f := newFxSignaler(false, func() (*rod.Browser, error) { return nil, nil })
	f.OnClientLine([]byte(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"click","arguments":{"pageId":3,"uid":"1_2"}}}` + "\n"))
	f.OnClientLine([]byte(`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"list_pages","arguments":{}}}` + "\n"))
	if _, ok := f.popPage([]byte(`{"jsonrpc":"2.0","id":8,"result":{}}`)); ok {
		t.Fatal("pageId 없는 호출의 응답은 false")
	}
	page, ok := f.popPage([]byte(`{"jsonrpc":"2.0","id":7,"result":{}}`))
	if !ok || page != 3 {
		t.Fatalf("id 7 → pageId 3이어야 한다: %d %v", page, ok)
	}
	if _, ok := f.popPage([]byte(`{"jsonrpc":"2.0","id":7,"result":{}}`)); ok {
		t.Fatal("한 번 꺼내면 비어야 한다")
	}
}
