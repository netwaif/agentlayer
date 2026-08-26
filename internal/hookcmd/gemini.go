package hookcmd

import (
	"encoding/json"
	"io"
	"time"

	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
)

// geminiPayload는 두 Gemini 계열 CLI의 훅 stdin JSON을 모두 받는다.
// agy(Antigravity CLI)는 camelCase(protojson), stock Gemini CLI는 snake_case.
// 겹치지 않는 키라 한 구조체로 관대하게 파싱한다.
type geminiPayload struct {
	// stock Gemini CLI (settings.json hooks)
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	// Antigravity CLI (hooks.json)
	ConversationID string   `json:"conversationId"`
	WorkspacePaths []string `json:"workspacePaths"`
	ModelName      string   `json:"modelName"`
}

// RunGemini는 `agentlayer hook gemini --event <event>`의 본체.
// 훅은 CLI의 자식 프로세스라 TMUX_PANE을 상속한다.
// 이벤트 매핑 (agy | stock Gemini CLI):
//   - pre-invocation·post-tool-use | before-agent·after-tool → WORK
//   - stop | after-agent → DONE_UNREAD
//   - session-start (stock) → IDLE (세션이 떴다고 일하는 게 아니다 — claude와 동일 원칙)
//   - notification (stock) → WAIT (승인 대기)
//
// 훅 출력 규약(stdout "{}")은 main이 담당한다 — 여기서는 상태만 만진다.
func RunGemini(st *state.Store, event string, stdin io.Reader, env func(string) string, now time.Time) error {
	pane := env("TMUX_PANE")
	if pane == "" {
		return nil // tmux 밖 세션은 관제 대상이 아니다
	}
	var p geminiPayload
	if b, err := io.ReadAll(stdin); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &p)
	}
	var to state.AgentState
	switch event {
	case "pre-invocation", "post-tool-use", "before-agent", "after-tool":
		to = state.StateWorking
	case "session-start":
		to = state.StateIdle
	case "notification":
		to = state.StateWaiting
	case "stop", "after-agent":
		to = state.StateDoneUnread
	default:
		return nil // 모르는 이벤트는 미래 호환을 위해 조용히 무시
	}
	id := scan.IDForPane("gemini", pane)
	a, err := st.Load(id)
	if err != nil {
		a = &state.Agent{ID: id, Kind: "gemini", State: state.StateIdle,
			Tmux:      state.TmuxRef{PaneID: pane}, // 세션·창은 다음 Sync가 채운다
			UpdatedAt: now, StateSince: now}
	}
	if p.ConversationID != "" {
		a.SessionID = p.ConversationID
	} else if p.SessionID != "" {
		a.SessionID = p.SessionID
	}
	if len(p.WorkspacePaths) > 0 && p.WorkspacePaths[0] != "" {
		a.CWD = p.WorkspacePaths[0]
	} else if p.CWD != "" {
		a.CWD = p.CWD
	}
	if p.ModelName != "" {
		a.Model = p.ModelName // agy 세션은 파일 소스가 없어 hook이 유일한 모델 출처
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
