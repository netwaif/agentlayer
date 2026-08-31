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
		mkAgent("gemini", "gem", "%3", state.StateIdle),      // 3사 공통 — 포함
		mkAgent("claude", "dead-bot", "%4", state.StateDead), // 제외: 죽음
		mkAgent("claude", "ai", "%5", state.StateWorking),    // 자기 자신
		mkAgent("claude", "zzukumi-bot", "%6", state.StateIdle),
	}
	got := Targets(agents, "%5", []string{"zzukumi-bot"})
	if len(got) != 3 {
		t.Fatalf("대상 3개(collab-bot, codex-live, gem): %d", len(got))
	}
	names := map[string]bool{}
	for _, a := range got {
		names[a.Tmux.Session] = true
	}
	for _, want := range []string{"collab-bot", "codex-live", "gem"} {
		if !names[want] {
			t.Errorf("%s 누락", want)
		}
	}
}

// 8-26 "관제탑 기능은 3사 공통" 원칙 — gemini만 빠져 있던 초기 필터의 회귀 방지.
func TestTargetsIncludesGemini(t *testing.T) {
	agents := []*state.Agent{mkAgent("gemini", "gem", "%3", state.StateIdle)}
	if got := Targets(agents, "", nil); len(got) != 1 {
		t.Error("gemini도 일괄 지시 대상")
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
