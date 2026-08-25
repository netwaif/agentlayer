package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/usage"
	"github.com/netwaif/agentlayer/internal/wiring"
)

func TestRenderInfoFull(t *testing.T) {
	used := 8.0
	d := InfoData{
		Agent: &state.Agent{ID: "claude-6", Kind: "claude", State: state.StateIdle,
			Task:      "멤버십 글 정리",
			Tmux:      state.TmuxRef{Session: "zzukumi-bot", Window: 0, PaneID: "%6"},
			CWD:       "/Users/soonho/ai-folder/youtube-members/zzukumi",
			SessionID: "0b6fcbae-4f63-40c0", PID: 777,
			UpdatedAt: t0, StateSince: t0},
		Wiring: wiring.Info{BotName: "zzukumi", Engine: "claude",
			Discord: &wiring.Discord{DMPolicy: "allowlist", Channels: []wiring.Channel{
				{ID: "1533823223442182294", Label: "쭈꾸미방", AllowCount: 1}}},
			LaunchAgents: []string{"com.folder-bot.zzukumi"}},
		Ctx:    usage.CtxInfo{Model: "Opus 5 (1M context)", UsedPct: &used},
		Branch: "",
	}
	var buf bytes.Buffer
	RenderInfo(&buf, d, t0)
	out := buf.String()
	for _, want := range []string{
		"zzukumi-bot", "~/ai-folder/youtube-members/zzukumi",
		"Opus 5 (1M context)", "ctx 8%",
		"folder-bot zzukumi",
		"쭈꾸미방 (1533823223442182294)", "허용 1명", "mention 불필요",
		"com.folder-bot.zzukumi",
		"0b6fcba", "agentlayer resume claude-6",
		"멤버십 글 정리",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("카드에 %q 있어야 함:\n%s", want, out)
		}
	}
}

func TestRenderInfoMinimal(t *testing.T) {
	d := InfoData{Agent: &state.Agent{ID: "codex-1", Kind: "codex", State: state.StateWorking,
		Tmux:      state.TmuxRef{Session: "codex-live", PaneID: "%1"},
		UpdatedAt: t0, StateSince: t0}}
	var buf bytes.Buffer
	RenderInfo(&buf, d, t0)
	out := buf.String()
	if !strings.Contains(out, "연결 없음") || !strings.Contains(out, "수동 실행") ||
		!strings.Contains(out, "미기록") {
		t.Errorf("빈 소스 표기:\n%s", out)
	}
}

func TestFindAgent(t *testing.T) {
	agents := []*state.Agent{
		{ID: "claude-6", Tmux: state.TmuxRef{Session: "zzukumi-bot"}},
	}
	if FindAgent(agents, "zzukumi-bot") == nil || FindAgent(agents, "claude-6") == nil {
		t.Error("세션명·ID 둘 다로 찾아야 함")
	}
	if FindAgent(agents, "없음") != nil {
		t.Error("없으면 nil")
	}
}
