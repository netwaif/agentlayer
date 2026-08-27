package hookcmd

import (
	"encoding/json"
	"time"

	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
)

// codexPayload는 codex notify가 argv로 넘기는 JSON 중 우리가 쓰는 필드.
type codexPayload struct {
	Type string `json:"type"`
	CWD  string `json:"cwd"`
}

// RunCodex는 codex config.toml의 notify 프로그램 호출을 받는다.
// codex는 JSON 하나를 마지막 인자로 넘긴다. notify는 codex의 자식
// 프로세스라 TMUX_PANE을 상속하므로 pane 식별이 그대로 된다.
func RunCodex(st *state.Store, args []string, env func(string) string, now time.Time) error {
	pane := hookPane(env)
	if pane == "" {
		return nil
	}
	var p codexPayload
	if len(args) > 0 {
		_ = json.Unmarshal([]byte(args[len(args)-1]), &p)
	}
	var to state.AgentState
	switch p.Type {
	case "agent-turn-complete":
		to = state.StateDoneUnread
	default:
		return nil // 미래 이벤트는 조용히 무시
	}
	id := scan.IDForPane("codex", pane)
	a, err := st.Load(id)
	if err != nil {
		a = &state.Agent{ID: id, Kind: "codex", State: state.StateIdle,
			Tmux: state.TmuxRef{PaneID: pane}, UpdatedAt: now, StateSince: now}
	}
	if p.CWD != "" {
		a.CWD = p.CWD
	}
	prev := a.State
	a.Transition(to, now)
	if err := st.Save(a); err != nil {
		return err
	}
	if onTransition != nil {
		onTransition(a, prev, to)
	}
	return nil
}
