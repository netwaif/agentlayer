// Package state는 AgentLayer의 정본 상태 모델을 정의한다.
// 에이전트 하나 = 레코드 하나 = 파일 하나. 화면 스크래핑 없이
// hook·프로세스 신호로만 상태가 바뀐다.
package state

import "time"

// AgentState는 에이전트의 의미 상태.
type AgentState string

const (
	StateWorking    AgentState = "WORKING"     // 도구 실행·턴 진행 중 (hook heartbeat)
	StateWaiting    AgentState = "WAITING"     // 사용자 입력·승인 대기
	StateDoneUnread AgentState = "DONE_UNREAD" // 턴 종료, 사용자가 아직 안 봄
	StateIdle       AgentState = "IDLE"        // 확인 완료 또는 초기 상태
	StateError      AgentState = "ERROR"       // 비정상 종료 감지
	StateDead       AgentState = "DEAD"        // pane/프로세스 소실
)

// staleAfter가 지나도록 갱신 없는 WORKING은 hook 유실 가능성이 있다.
const staleAfter = 30 * time.Minute

// Priority는 대시보드 정렬 순서. 낮을수록 위에 온다:
// 사용자 행동이 필요한 것(WAITING) > 확인 안 한 완료 > 에러 > 진행 중 > 나머지.
func (s AgentState) Priority() int {
	switch s {
	case StateWaiting:
		return 0
	case StateDoneUnread:
		return 1
	case StateError:
		return 2
	case StateWorking:
		return 3
	case StateIdle:
		return 4
	default: // StateDead 포함
		return 5
	}
}

// TmuxRef는 에이전트가 사는 tmux 좌표.
type TmuxRef struct {
	Session string `json:"session"`
	Window  int    `json:"window"`
	PaneID  string `json:"pane_id"` // "%3" 형식, tmux 서버 수명 내 고유
}

// Agent는 관제 대상 에이전트 하나의 정본 레코드.
type Agent struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"` // claude | codex | gemini
	Task      string     `json:"task,omitempty"`
	State     AgentState `json:"state"`
	Tmux      TmuxRef    `json:"tmux"`
	CWD       string     `json:"cwd,omitempty"`
	SessionID string     `json:"session_id,omitempty"` // 비상 복구(resume)용
	PID       int        `json:"pid,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
	StateSince time.Time `json:"state_since"`
}

// Transition은 상태를 바꾸고 시각을 갱신한다.
// 같은 상태로의 재진입은 heartbeat로 취급해 StateSince를 유지한다.
func (a *Agent) Transition(to AgentState, now time.Time) {
	if a.State != to {
		a.State = to
		a.StateSince = now
	}
	a.UpdatedAt = now
}

// Stale은 WORKING인데 오래 갱신이 없어 hook 유실이 의심되면 true.
func (a *Agent) Stale(now time.Time) bool {
	return a.State == StateWorking && now.Sub(a.UpdatedAt) > staleAfter
}
