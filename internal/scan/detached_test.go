package scan

import (
	"testing"

	"github.com/netwaif/agentlayer/internal/state"
)

func stubProcs(t *testing.T, out string) {
	t.Helper()
	orig := loadProcTable
	loadProcTable = func() ProcTable { return ParseProcTable(out) }
	t.Cleanup(func() { loadProcTable = orig })
}

// 데스크톱 앱 세션의 프로세스 사슬: 훅은 셸을 거쳐(agentlayer ← sh ← claude), MCP 서버는 직접(agentlayer ← claude) 뜬다.
// 둘 다 가장 가까운 claude 조상으로 맞춰야 같은 주소(claude-pid500)가 나온다.
const desktopProcs = `
  1     0 /sbin/launchd
400     1 /Applications/Claude.app/Contents/MacOS/Claude
500   400 claude -n 기획서 --resume
510   500 /bin/sh -c agentlayer hook claude --event stop
511   510 agentlayer hook claude --event stop
520   500 agentlayer channel serve --self
530   500 /bin/bash -c agentlayer send 기획서 안녕
531   530 agentlayer send 기획서 안녕
600   400 node /opt/homebrew/lib/node_modules/@openai/codex/bin/codex.js
610   600 agentlayer hook codex --event stop
`

func TestFindAgentProcess(t *testing.T) {
	pt := ParseProcTable(desktopProcs)
	cases := []struct {
		from    int
		kind    string
		wantPID int
		wantOK  bool
	}{
		{511, "claude", 500, true}, // 훅(셸 경유)
		{520, "claude", 500, true}, // MCP 서버(직접)
		{531, "claude", 500, true}, // Bash 도구 안의 send
		{531, "", 500, true},       // 종류 불문
		{610, "codex", 600, true},  // npm 래퍼 코덱스
		{610, "claude", 0, false},  // 코덱스 사슬에는 claude가 없다
		{400, "claude", 0, false},  // 앱 자체는 에이전트가 아니다
		{9999, "claude", 0, false}, // 표에 없는 pid
	}
	for _, c := range cases {
		pid, _, _, ok := FindAgentProcess(pt, c.from, c.kind)
		if pid != c.wantPID || ok != c.wantOK {
			t.Errorf("FindAgentProcess(%d,%q) = %d,%v want %d,%v", c.from, c.kind, pid, ok, c.wantPID, c.wantOK)
		}
	}
	if _, _, args, _ := FindAgentProcess(pt, 511, "claude"); NameFromArgs(args) != "기획서" {
		t.Errorf("-n 이름을 읽어야 함: %q", args)
	}
}

func TestNameFromArgs(t *testing.T) {
	cases := map[string]string{
		"claude -n 기획서":              "기획서",
		"claude --name 기획서 --resume": "기획서",
		"claude --name=기획서":          "기획서",
		"claude -n":                  "",
		"claude -n --resume":         "",
		"claude --resume":            "",
	}
	for args, want := range cases {
		if got := NameFromArgs(args); got != want {
			t.Errorf("NameFromArgs(%q) = %q want %q", args, got, want)
		}
	}
}

func TestIDForProcess(t *testing.T) {
	if got := IDForProcess("claude", 500); got != "claude-pid500" {
		t.Errorf("got %q", got)
	}
}

func detachedAgent(kind string, pid int) *state.Agent {
	return &state.Agent{ID: IDForProcess(kind, pid), Kind: kind, State: state.StateIdle, PID: pid,
		SessionID: "10ec8033-ca55-4c1e-9f1a-000000000000", UpdatedAt: t0, StateSince: t0}
}

// pane 없는 레코드는 pane 목록 동기화가 죽였다고 판정하면 안 된다 — 프로세스 표가 근거다.
func TestSyncLeavesDetachedAlone(t *testing.T) {
	st := newStore(t)
	_ = st.Save(detachedAgent("claude", 500))
	if err := Sync(st, nil, t0); err != nil {
		t.Fatal(err)
	}
	a, err := st.Load("claude-pid500")
	if err != nil || a.State != state.StateIdle {
		t.Fatalf("pane 동기화가 tmux 밖 레코드를 건드림: %v %v", a, err)
	}
}

func TestSyncDetachedRemovesGoneProcess(t *testing.T) {
	stubProcs(t, desktopProcs)
	st := newStore(t)
	_ = st.Save(detachedAgent("claude", 500)) // 살아 있음
	_ = st.Save(detachedAgent("claude", 777)) // 표에 없음 → 지움
	_ = st.Save(detachedAgent("claude", 600)) // 그 PID에는 코덱스가 앉아 있다(재사용) → 지움
	_ = st.Save(detachedAgent("codex", 600))  // 코덱스 레코드는 산다
	pane := &state.Agent{ID: "claude-3", Kind: "claude", State: state.StateIdle, Tmux: state.TmuxRef{PaneID: "%3"}}
	_ = st.Save(pane) // pane 레코드는 건드리지 않는다
	if err := SyncDetached(st, t0); err != nil {
		t.Fatal(err)
	}
	got, _ := st.List()
	ids := map[string]bool{}
	for _, a := range got {
		ids[a.ID] = true
	}
	for id, want := range map[string]bool{"claude-pid500": true, "claude-pid777": false, "claude-pid600": false, "codex-pid600": true, "claude-3": true} {
		if ids[id] != want {
			t.Errorf("%s 남음=%v want %v", id, ids[id], want)
		}
	}
}

// 프로세스 표를 못 읽으면(빈 표) 지우지 않는다 — ps 실패 한 번에 관제 대상이 사라지면 안 된다.
func TestSyncDetachedKeepsAllWhenProcTableEmpty(t *testing.T) {
	stubProcs(t, "")
	st := newStore(t)
	_ = st.Save(detachedAgent("claude", 777))
	if err := SyncDetached(st, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Load("claude-pid777"); err != nil {
		t.Error("빈 표에서는 지우면 안 된다")
	}
}
