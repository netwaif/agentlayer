package remote

import (
	"context"
	"fmt"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

// Handle은 실행기 쪽 식별자(Hermes = 카드 ID).
type Handle = string

// Status는 Poll 결과. State는 agentlayer 상태 값 그대로. Seen은 마지막 관측 이벤트 시각(초) — 같은 상태의
// 재관측을 걸러 낸다.
type Status struct {
	State   state.AgentState
	Summary string
	Ask     string
	Error   string
	Seen    int64
}

// Letter는 실행기 쪽에서 총괄 앞으로 온 편지. Mailbox가 돌려준 편지는 수신 확인된 것이다.
type Letter struct {
	ID     string
	From   string
	Text   string
	TaskID string
	At     time.Time
}

// Info는 Check 결과.
type Info struct {
	Version   string
	ProfileOK bool
	RoundTrip time.Duration
	Detail    string
}

// Adapter는 send·task watch·task done이 실행기에 대해 아는 전부다.
type Adapter interface {
	Dispatch(ctx context.Context, taskID, title, body string, parent Handle) (Handle, error)
	Poll(ctx context.Context, h Handle) (Status, error)
	Reply(ctx context.Context, h Handle, text string) error
	Pull(ctx context.Context, h Handle, destDir string) error
	Mailbox(ctx context.Context) ([]Letter, error)
	Finish(ctx context.Context, h Handle) error
	Check(ctx context.Context) (Info, error)
}

const (
	TimeoutQuery    = 30 * time.Second
	TimeoutDispatch = 60 * time.Second
	TimeoutPull     = 5 * time.Minute
	MaxPullBytes    = 50 << 20
)

// Open은 등록에 맞는 어댑터를 만든다.
func Open(r Remote, stateDir string) (Adapter, error) {
	switch r.Kind {
	case "hermes":
		return &Hermes{R: SSHRunner{Host: r.SSH, Exec: r.Exec, ControlDir: Dir(stateDir)}, Profile: r.Profile, Board: r.Board,
			WorkspaceRoot: r.WorkspaceRoot, MailboxAssignee: r.Mailbox, MaxRuntime: r.MaxRuntime, Now: time.Now}, nil
	}
	return nil, fmt.Errorf("알 수 없는 kind: %q", r.Kind)
}
