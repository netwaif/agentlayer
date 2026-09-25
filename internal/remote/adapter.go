package remote

import (
	"context"
	"fmt"
	"os"
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

// DispatchRequest는 지시 한 건. Attempt는 등록 시도 구분자(같은 업무를 --replace로 다시 배정하면 바뀐다) —
// 실행기의 멱등 키에 들어가 죽은 옛 카드가 돌아오지 않게 한다. Parent는 DONE 뒤 후속 지시의 직전 handle.
type DispatchRequest struct {
	TaskID  string
	Title   string
	Body    string
	Parent  Handle
	Attempt string
}

// Adapter는 send·task watch·task done이 실행기에 대해 아는 전부다.
type Adapter interface {
	Dispatch(ctx context.Context, req DispatchRequest) (Handle, error)
	Resume(ctx context.Context, h Handle) error // 이미 만든 실행(카드)을 다시 기동만 — 기동 실패 재시도용
	Poll(ctx context.Context, h Handle) (Status, error)
	Reply(ctx context.Context, h Handle, text string) error
	Pull(ctx context.Context, h Handle, destDir string) error
	Mailbox(ctx context.Context) ([]Letter, error)
	Answer(ctx context.Context, letterID, text string) error // 편지에 답장 — 편지 카드를 답으로 닫아 보낸 쪽이 깨어나게
	Finish(ctx context.Context, h Handle) error
	Check(ctx context.Context) (Info, error)
}

const (
	TimeoutQuery    = 30 * time.Second
	TimeoutDispatch = 60 * time.Second
	TimeoutPull     = 5 * time.Minute
	MaxPullBytes    = 50 << 20
)

// ControlDir은 ssh ControlMaster 소켓 폴더. state dir 아래는 macOS Unix 소켓 경로 상한(104바이트)을 넘겨
// ssh가 "path too long"으로 죽으므로 /tmp 아래 짧은 경로를 쓴다(사용자별, 0700).
func ControlDir() string { return fmt.Sprintf("/tmp/agentlayer-ssh-%d", os.Getuid()) }

// Open은 등록에 맞는 어댑터를 만든다.
func Open(r Remote, stateDir string) (Adapter, error) {
	switch r.Kind {
	case "hermes":
		return &Hermes{R: SSHRunner{Host: r.SSH, Exec: r.Exec, ControlDir: ControlDir()}, Profile: r.Profile, Board: r.Board,
			WorkspaceRoot: r.WorkspaceRoot, MailboxAssignee: r.Mailbox, MaxRuntime: r.MaxRuntime, Now: time.Now}, nil
	case "exec":
		return &Exec{Commands: r.Commands}, nil
	}
	return nil, fmt.Errorf("알 수 없는 kind: %q", r.Kind)
}
