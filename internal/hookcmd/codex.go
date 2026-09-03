package hookcmd

import (
	"encoding/json"
	"io"
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

// codexHookPayload는 codex hooks(~/.codex/hooks.json) stdin JSON 중 쓰는 필드.
// 필드명은 Claude Code hook과 같다(session_id·cwd·hook_event_name).
type codexHookPayload struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
}

// RunCodexEvent는 `agentlayer hook codex --event <event>`의 본체 — codex hooks 경로.
// notify(agent-turn-complete)는 턴 완료만 알려서 codex가 일하는 동안에도 DONE으로
// 보였다(2026-09-03 촬영 실측). hooks가 WORK·WAIT를 채운다. 훅은 codex의 자식이라
// TMUX_PANE을 상속하고, stdout을 비워 두면 codex 동작에 개입하지 않는다.
func RunCodexEvent(st *state.Store, event string, stdin io.Reader, env func(string) string, now time.Time) error {
	pane := hookPane(env)
	if pane == "" {
		return nil
	}
	var p codexHookPayload
	if b, err := io.ReadAll(stdin); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &p)
	}
	var to state.AgentState
	switch event {
	case "user-prompt-submit", "post-tool-use":
		to = state.StateWorking
	case "session-start":
		to = state.StateIdle // 떴다고 일하는 게 아니다 — claude와 같은 규칙
	case "permission-request":
		to = state.StateWaiting
	case "stop":
		to = state.StateDoneUnread
	default:
		return nil
	}
	id := scan.IDForPane("codex", pane)
	a, err := st.Load(id)
	if err != nil {
		a = &state.Agent{ID: id, Kind: "codex", State: state.StateIdle,
			Tmux: state.TmuxRef{PaneID: pane}, UpdatedAt: now, StateSince: now}
	}
	if p.SessionID != "" {
		a.SessionID = p.SessionID
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
