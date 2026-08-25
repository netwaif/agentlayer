package cli

import (
	"testing"

	"github.com/netwaif/agentlayer/internal/state"
)

func mkAgent(kind, session, pane string, st state.AgentState) *state.Agent {
	return &state.Agent{ID: kind + "-" + pane, Kind: kind, State: st,
		Tmux: state.TmuxRef{Session: session, PaneID: pane}}
}

func TestTargetsSelection(t *testing.T) {
	agents := []*state.Agent{
		mkAgent("claude", "collab-bot", "%1", state.StateIdle),
		mkAgent("codex", "codex-live", "%2", state.StateIdle),
		mkAgent("gemini", "gem", "%3", state.StateIdle),      // 제외: 미지원
		mkAgent("claude", "dead-bot", "%4", state.StateDead), // 제외: 죽음
		mkAgent("claude", "ai", "%5", state.StateWorking),    // 자기 자신
		mkAgent("claude", "zzukumi-bot", "%6", state.StateIdle),
	}
	got := Targets(agents, "%5", []string{"zzukumi-bot"})
	if len(got) != 2 {
		t.Fatalf("대상 2개(collab-bot, codex-live): %d", len(got))
	}
	names := got[0].Tmux.Session + "," + got[1].Tmux.Session
	if names != "collab-bot,codex-live" && names != "codex-live,collab-bot" {
		t.Errorf("대상: %s", names)
	}
}

func TestTargetsIncludesCodex(t *testing.T) {
	agents := []*state.Agent{mkAgent("codex", "codex-live", "%2", state.StateDoneUnread)}
	if got := Targets(agents, "", nil); len(got) != 1 {
		t.Error("codex도 대상 (loadout이 AGENTS.md로도 설치됨)")
	}
}

func TestParseAllFlags(t *testing.T) {
	o, rest, err := ParseAllFlags("broadcast", []string{"--yes", "--except", "a,b", "안녕하세요"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !o.yes || len(o.except) != 2 || len(rest) != 1 || rest[0] != "안녕하세요" {
		t.Errorf("파싱: %+v rest=%v", o, rest)
	}
}
