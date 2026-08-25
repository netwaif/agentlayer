// Package hookcmd는 에이전트 CLI의 hook 이벤트를 받아 상태를 갱신한다.
// hook은 에이전트의 정상 동작을 절대 방해하면 안 되므로, 이 경로의
// 실패는 조용히 무시되거나 stderr 한 줄로 끝난다.
package hookcmd

import (
	"encoding/json"
	"io"
	"time"

	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
)

// claudePayload는 Claude Code hook stdin JSON 중 우리가 쓰는 필드만.
// 나머지 필드는 무시한다(관대한 파싱).
type claudePayload struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	Message   string `json:"message"` // Notification 이벤트의 안내 문구
}

// RunClaude는 `agentlayer hook claude --event <event>`의 본체.
// env는 os.Getenv 주입점(테스트용), now도 주입한다.
func RunClaude(st *state.Store, event string, stdin io.Reader, env func(string) string, now time.Time) error {
	pane := env("TMUX_PANE")
	if pane == "" {
		return nil // tmux 밖 세션은 관제 대상이 아니다
	}

	var p claudePayload
	if b, err := io.ReadAll(stdin); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &p) // 파싱 실패는 무시 — pane 정보만으로도 기록한다
	}

	var to state.AgentState
	switch event {
	case "post-tool-use", "session-start":
		to = state.StateWorking
	case "notification":
		to = state.StateWaiting
	case "stop":
		to = state.StateDoneUnread
	default:
		return nil // 모르는 이벤트는 미래 호환을 위해 조용히 무시
	}

	id := scan.IDForPane("claude", pane)
	a, err := st.Load(id)
	if err != nil {
		a = &state.Agent{ID: id, Kind: "claude", State: state.StateIdle,
			Tmux:      state.TmuxRef{PaneID: pane}, // 세션·창은 다음 Sync가 채운다
			UpdatedAt: now, StateSince: now}
	}
	if p.SessionID != "" {
		a.SessionID = p.SessionID
	}
	if p.CWD != "" {
		a.CWD = p.CWD
	}
	if p.Message != "" {
		a.Task = p.Message
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

// onTransition은 상태 전이 후크(알림 발화 지점). main이 주입한다.
// heartbeat(같은 상태)를 걸러내는 건 알림 쪽 책임이다.
var onTransition func(a *state.Agent, prev, to state.AgentState)

// SetTransitionHook은 전이 콜백을 등록한다.
func SetTransitionHook(fn func(a *state.Agent, prev, to state.AgentState)) {
	onTransition = fn
}
