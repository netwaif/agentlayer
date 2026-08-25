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
