// internal/task/report.go
package task

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

// Report는 총괄 수신함에 떨어지는 이벤트 한 건. 파일은 <inbox>/pending/<id>.json.
type Report struct {
	Version int       `json:"version"`
	ID      string    `json:"id"`
	TaskID  string    `json:"task_id"`
	Session string    `json:"session"`
	Window  string    `json:"window,omitempty"`
	Kind    string    `json:"kind"`
	From    string    `json:"from"`
	To      string    `json:"to"`
	Task    string    `json:"task,omitempty"`
	Ask     string    `json:"ask,omitempty"`
	CWD     string    `json:"cwd,omitempty"`
	At      time.Time `json:"at"`
	Inbox   string    `json:"-"`
}

// ShouldReport — 총괄이 알아야 하는 것은 "멈췄다"뿐: 끝남·승인 대기·에러.
// heartbeat, 승인됨(WAITING→WORKING), 읽음(DONE→IDLE), dead(scan 몫)는 보고하지 않는다.
func ShouldReport(prev, to state.AgentState) bool {
	if prev == to {
		return false
	}
	switch to {
	case state.StateDoneUnread, state.StateWaiting, state.StateError:
		return true
	}
	return false
}

// ReportFor는 등록된 에이전트의 보고 대상 전이면 Report를 만든다.
func ReportFor(stateDir string, a *state.Agent, prev, to state.AgentState, now time.Time) (*Report, bool) {
	if !ShouldReport(prev, to) {
		return nil, false
	}
	as, ok, err := Load(stateDir, a.ID)
	if err != nil || !ok {
		return nil, false
	}
	// 낡은 등록 방지: 같은 에이전트 ID를 새 세션이 재사용했을 수 있다(재시작 등).
	// 등록 당시의 세션·pane과 지금 것이 다르면 엉뚱한 세션 앞으로 보고하지 않는다.
	if as.Session != a.Tmux.Session || as.Pane != a.Tmux.PaneID {
		return nil, false
	}
	r := &Report{Version: 1, ID: NewID(), TaskID: as.TaskID, Session: a.Tmux.Session, Window: a.Tmux.WindowName,
		Kind: a.Kind, From: string(prev), To: string(to), Task: a.Task, CWD: a.CWD, At: now, Inbox: as.Inbox}
	if to == state.StateWaiting {
		r.Ask = a.Ask
	}
	return r, true
}

// WriteReport는 <inbox>/pending/<id>.json을 원자적으로 쓴다. 디렉터리는 0700으로 만든다.
func WriteReport(r *Report) (string, error) {
	dir := filepath.Join(r.Inbox, "pending")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, r.ID+".json")
	return p, writeAtomic(p, r)
}

// NewID는 32자 hex(128비트 난수). 외부 uuid 의존 없이 충분히 유일하다.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 난수 실패는 사실상 없지만, 시각 기반으로라도 유일하게
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000"))[:16])
	}
	return hex.EncodeToString(b[:])
}
