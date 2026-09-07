package scan

import (
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
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

// 실측 데이터 기반 케이스: Claude Code는 pane_current_command가 버전
// 문자열("2.1.241")로 잡히고 title에 "✳"를 남긴다.
func TestDetectKind(t *testing.T) {
	cases := []struct {
		pane tmuxx.Pane
		want string
	}{
		{tmuxx.Pane{Command: "claude", Title: ""}, "claude"},
		{tmuxx.Pane{Command: "2.1.241", Title: "✳ 핸드오프 문서 확인"}, "claude"},
		{tmuxx.Pane{Command: "node", Title: "✳ collab-bot"}, "claude"},
		{tmuxx.Pane{Command: "codex", Title: "workspace | weekly 85% left"}, "codex"},
		{tmuxx.Pane{Command: "gemini", Title: ""}, "gemini"},
		{tmuxx.Pane{Command: "agy", Title: "gemini"}, "gemini"}, // Antigravity CLI
		{tmuxx.Pane{Command: "agy", Title: ""}, "gemini"},
		{tmuxx.Pane{Command: "python3.11", Title: "gwonsunhouiiMac"}, ""},
		{tmuxx.Pane{Command: "zsh", Title: ""}, ""},
		{tmuxx.Pane{Command: "2.1.241", Title: ""}, "claude"},             // 버전형 = Claude (로케일 무관 판정)
		{tmuxx.Pane{Command: "2.1.241", Title: "_ collab-bot"}, "claude"}, // LANG 없어 ✳가 _로 치환된 경우
	}
	for _, c := range cases {
		if got := DetectKind(c.pane); got != c.want {
			t.Errorf("DetectKind(%q,%q) = %q, want %q", c.pane.Command, c.pane.Title, got, c.want)
		}
	}
}

func claudePane() tmuxx.Pane {
	return tmuxx.Pane{Session: "ai", Window: 1, PaneID: "%3", Command: "2.1.241",
		Path: "/Users/soonho/ai-folder/dev/agentlayer", Title: "✳ 핸드오프 문서 확인", PanePID: 70882}
}

func TestSyncDiscoversNewAgent(t *testing.T) {
	st := newStore(t)
	if err := Sync(st, []tmuxx.Pane{claudePane()}, t0); err != nil {
		t.Fatal(err)
	}
	got, _ := st.List()
	if len(got) != 1 {
		t.Fatalf("레코드 1개여야 함: %d", len(got))
	}
	a := got[0]
	if a.Kind != "claude" || a.State != state.StateIdle ||
		a.Tmux.PaneID != "%3" || a.CWD != "/Users/soonho/ai-folder/dev/agentlayer" {
		t.Errorf("신규 레코드 불일치: %+v", a)
	}
}

func TestSyncIgnoresNonAgentPanes(t *testing.T) {
	st := newStore(t)
	shell := tmuxx.Pane{Session: "ai", Window: 0, PaneID: "%0", Command: "zsh", Path: "/tmp"}
	if err := Sync(st, []tmuxx.Pane{shell}, t0); err != nil {
		t.Fatal(err)
	}
	got, _ := st.List()
	if len(got) != 0 {
		t.Errorf("에이전트 아닌 pane은 등록 안 함: %+v", got)
	}
}

func TestSyncUpdatesCoordinatesKeepsState(t *testing.T) {
	st := newStore(t)
	if err := Sync(st, []tmuxx.Pane{claudePane()}, t0); err != nil {
		t.Fatal(err)
	}
	// hook이 상태를 WORKING으로 올린 뒤라고 가정
	got, _ := st.List()
	got[0].Transition(state.StateWorking, t0.Add(time.Minute))
	if err := st.Save(got[0]); err != nil {
		t.Fatal(err)
	}
	// 창 번호가 바뀐 같은 pane
	moved := claudePane()
	moved.Window = 2
	moved.Path = "/Users/soonho/ai-folder"
	if err := Sync(st, []tmuxx.Pane{moved}, t0.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	after, _ := st.List()
	if len(after) != 1 {
		t.Fatalf("같은 pane은 레코드 하나 유지: %d", len(after))
	}
	if after[0].Tmux.Window != 2 || after[0].CWD != "/Users/soonho/ai-folder" {
		t.Errorf("좌표·cwd 갱신돼야 함: %+v", after[0])
	}
	if after[0].State != state.StateWorking {
		t.Errorf("스캐너는 hook이 만든 상태를 덮지 않아야 함: %s", after[0].State)
	}
}

func TestSyncMarksVanishedDead(t *testing.T) {
	st := newStore(t)
	if err := Sync(st, []tmuxx.Pane{claudePane()}, t0); err != nil {
		t.Fatal(err)
	}
	if err := Sync(st, nil, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, _ := st.List()
	if len(got) != 1 || got[0].State != state.StateDead {
		t.Errorf("사라진 pane은 DEAD: %+v", got)
	}
}

func TestSyncPurgesOldDead(t *testing.T) {
	st := newStore(t)
	if err := Sync(st, []tmuxx.Pane{claudePane()}, t0); err != nil {
		t.Fatal(err)
	}
	if err := Sync(st, nil, t0.Add(time.Minute)); err != nil { // DEAD 전환
		t.Fatal(err)
	}
	if err := Sync(st, nil, t0.Add(25*time.Hour)); err != nil { // 24h 경과
		t.Fatal(err)
	}
	got, _ := st.List()
	if len(got) != 0 {
		t.Errorf("24h 지난 DEAD는 정리돼야 함: %+v", got)
	}
}

func TestSyncPurgesDeadSupersededByRevivedSession(t *testing.T) {
	// 재부팅 후 브리지 LaunchAgent가 restore를 거치지 않고 같은 세션명·cwd로
	// 새 pane을 띄우면(코덱스 브리지 실사례), 옛 DEAD 레코드는 이중 행으로
	// 남는다 — 같은 kind·세션·cwd의 살아있는 pane이 있으면 즉시 정리해야 함.
	st := newStore(t)
	dead := &state.Agent{ID: "codex-4", Kind: "codex", State: state.StateDead,
		Tmux:      state.TmuxRef{Session: "codex-live", Window: 0, PaneID: "%4"},
		CWD:       "/Users/soonho/ai-folder/codex-discord-workspace",
		UpdatedAt: t0, StateSince: t0}
	if err := st.Save(dead); err != nil {
		t.Fatal(err)
	}
	revived := tmuxx.Pane{Session: "codex-live", Window: 0, PaneID: "%9",
		Command: "codex", Path: "/Users/soonho/ai-folder/codex-discord-workspace"}
	if err := Sync(st, []tmuxx.Pane{revived}, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, _ := st.List()
	if len(got) != 1 {
		t.Fatalf("살아있는 레코드 1개만 남아야 함: %d개 %+v", len(got), got)
	}
	if got[0].ID != "codex-9" || got[0].State == state.StateDead {
		t.Errorf("옛 DEAD는 정리되고 새 레코드만: %+v", got[0])
	}
}

func TestSyncKeepsDeadWithoutLiveReplacement(t *testing.T) {
	// 대체 pane이 없는 DEAD는 기존대로 24h 보존 (restore 대상)
	st := newStore(t)
	dead := &state.Agent{ID: "codex-4", Kind: "codex", State: state.StateDead,
		Tmux:      state.TmuxRef{Session: "codex-live", Window: 0, PaneID: "%4"},
		CWD:       "/Users/soonho/ai-folder/codex-discord-workspace",
		UpdatedAt: t0, StateSince: t0}
	if err := st.Save(dead); err != nil {
		t.Fatal(err)
	}
	// 다른 세션의 다른 kind pane만 존재
	if err := Sync(st, []tmuxx.Pane{claudePane()}, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, _ := st.List()
	if len(got) != 2 {
		t.Fatalf("무관한 pane으로 DEAD가 지워지면 안 됨: %d개 %+v", len(got), got)
	}
}

func TestSyncMatchesHookCreatedRecordBySessionCoords(t *testing.T) {
	// hook이 tmux 좌표 기반 ID로 먼저 레코드를 만든 경우 스캐너가 중복 생성하면 안 됨
	st := newStore(t)
	pre := &state.Agent{ID: AgentID("claude", claudePane()), Kind: "claude",
		State: state.StateWorking, Tmux: state.TmuxRef{Session: "ai", Window: 1, PaneID: "%3"},
		UpdatedAt: t0, StateSince: t0}
	if err := st.Save(pre); err != nil {
		t.Fatal(err)
	}
	if err := Sync(st, []tmuxx.Pane{claudePane()}, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, _ := st.List()
	if len(got) != 1 {
		t.Errorf("중복 생성 금지: %d개", len(got))
	}
}

// WSL2 실측(2026-09-07): npm으로 깐 codex는 pane_current_command가 `node`(래퍼)이고
// 진짜 codex 바이너리는 그 자식이다. gemini-cli는 node 자신이 에이전트(인자에 경로).
func TestKindFromArgs(t *testing.T) {
	cases := []struct{ args, want string }{
		{"node /home/netwa/.nvm/versions/node/v22.23.2/bin/codex", "codex"},
		{"/home/netwa/.nvm/versions/node/v22.23.2/lib/node_modules/@openai/codex/bin/../vendor/x86_64-unknown-linux-musl/codex/codex", "codex"},
		{"node /home/netwa/.nvm/versions/node/v22.23.2/bin/gemini", "gemini"},
		{"node /usr/lib/node_modules/@google/gemini-cli/dist/index.js", "gemini"},
		{"/home/netwa/.local/bin/agy", "gemini"},
		{"node /home/x/lib/node_modules/@anthropic-ai/claude-code/cli.js", "claude"},
		{"node server.ts", ""},
		{"-zsh", ""},
		{"python3 codex-runner.py", ""}, // basename이 정확히 일치할 때만
	}
	for _, c := range cases {
		if got := KindFromArgs(c.args); got != c.want {
			t.Errorf("KindFromArgs(%q) = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestDescendantKind(t *testing.T) {
	pt := ParseProcTable(`
 10999 10000 -bash
 11086 10999 node /home/netwa/.nvm/versions/node/v22.23.2/bin/codex
 11095 11086 /home/netwa/.nvm/.../@openai/codex/vendor/x86_64-unknown-linux-musl/codex/codex
 12000 11999 node /opt/app/server.js
 12001 12000 sh -c codex exec
 12002 12001 codex exec
 13000 12999 node /home/netwa/.nvm/versions/node/v22.23.2/bin/gemini
`)
	if got := pt.DescendantKind(11086); got != "codex" {
		t.Errorf("node 래퍼(codex) = %q, want codex", got)
	}
	// pane_pid가 셸이고 그 자식이 래퍼인 경우(셸에서 codex를 친 pane)
	if got := pt.DescendantKind(10999); got != "codex" {
		t.Errorf("셸 → node 래퍼(codex) = %q, want codex", got)
	}
	if got := pt.DescendantKind(13000); got != "gemini" {
		t.Errorf("node 래퍼(gemini) = %q, want gemini", got)
	}
	// 자식까지만 본다 — 앱 서버가 셸을 거쳐 띄운 codex(손자)는 안 잡는다
	if got := pt.DescendantKind(12000); got != "" {
		t.Errorf("무관한 node 서버 = %q, want \"\"", got)
	}
	if got := pt.DescendantKind(0); got != "" {
		t.Errorf("pid 0 = %q", got)
	}
}

func TestSyncResolvesNodeWrapperViaProcTable(t *testing.T) {
	orig := loadProcTable
	loadProcTable = func() ProcTable {
		return ParseProcTable("11086 1 node /home/netwa/.nvm/versions/node/v22/bin/codex\n11095 11086 /x/@openai/codex/vendor/codex/codex\n")
	}
	defer func() { loadProcTable = orig }()
	st := newStore(t)
	pane := tmuxx.Pane{Session: "t2", Window: 0, PaneID: "%7", Command: "node", Path: "/home/netwa/work/t2", PanePID: 11086}
	if err := Sync(st, []tmuxx.Pane{pane}, t0); err != nil {
		t.Fatal(err)
	}
	a, err := st.Load("codex-7")
	if err != nil || a == nil {
		t.Fatalf("codex-7 미발견: %v", err)
	}
	if a.Kind != "codex" || a.Tmux.Session != "t2" || a.PID != 11086 || a.State != state.StateIdle {
		t.Errorf("잘못된 레코드: %+v", a)
	}
}
