package cli

import (
	"bytes"
	"strings"
	"testing"

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
		{"잉여 인자는 에러", []string{"뭔가"}, false, true},
		{"잉여 인자 뒤 --send도 에러", []string{"뭔가", "--send"}, false, true},
		{"모르는 플래그는 에러", []string{"--bogus"}, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			send, err := parseErrorsArgs(c.args)
			if c.wantErr {
				if err == nil {
					t.Fatalf("에러여야 함: send=%v", send)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if send != c.send {
				t.Errorf("got send=%v, want %v", send, c.send)
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
