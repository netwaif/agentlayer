package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/usage"
)

// 코덱스에는 tmux 키 입력 대신 `codex queue`로 보낸다(2026-09-29 실측, codex 0.157.1).
//   - 떠 있는 TUI 세션에 그대로 들어가 사용자가 친 것처럼 처리된다.
//   - 작업 중이면 현재 턴 뒤에 처리된다 — tmux 경로의 --force가 필요 없다. 승인 대기는 예전대로 거부한다.
//   - 여러 줄 본문이 그대로 간다. 붙여넣기 감지·제출 확인 같은 화면 의존이 없다.
//   - 없는 세션이면 "Error: … no rollout found"로 끝난다.
// 세션 ID는 훅이 채운 값(a.SessionID)만 쓴다 — 폴더로 추측하면 같은 폴더의 다른 세션(스레드 세션)에 들어간다.

const codexQueueTimeout = 15 * time.Second

// codexQueueFn은 큐 전송 주입점 — 테스트가 바꿔 끼운다.
var codexQueueFn = execCodexQueue

// codexBin은 codex 실행 파일을 찾는다. 훅·LaunchAgent처럼 PATH가 최소인 환경에서도 찾도록 흔한 위치를 본다.
func codexBin() string {
	if p, err := exec.LookPath("codex"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{"/opt/homebrew/bin/codex", "/usr/local/bin/codex",
		filepath.Join(home, ".local/bin/codex"), filepath.Join(home, ".npm-global/bin/codex")} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func execCodexQueue(ctx context.Context, thread, cwd, message string) error {
	bin := codexBin()
	if bin == "" {
		return errors.New("codex 명령을 찾지 못함")
	}
	c, cancel := context.WithTimeout(ctx, codexQueueTimeout)
	defer cancel()
	cmd := exec.CommandContext(c, bin, "queue", "--thread", thread, "--message", message)
	if cwd != "" {
		cmd.Dir = cwd
	}
	out, err := cmd.CombinedOutput()
	return codexQueueResult(string(out), err)
}

// codexQueueResult는 출력으로 성패를 가른다 — 종료 코드만 믿지 않는다(실패해도 0으로 끝나는 판이 있다).
func codexQueueResult(out string, runErr error) error {
	if runErr == nil && strings.Contains(out, "Queued message") {
		return nil
	}
	msg := strings.TrimSpace(out)
	if i := strings.Index(msg, "Error:"); i >= 0 {
		msg = msg[i:]
	}
	if r := []rune(msg); len(r) > 200 {
		msg = string(r[:200]) + "…"
	}
	if msg == "" && runErr != nil {
		msg = runErr.Error()
	}
	if msg == "" {
		msg = "응답 없음"
	}
	return errors.New(msg)
}

// procStartFn은 pid의 기동 시각(`ps -o lstart=`). 테스트가 바꿔 끼운다.
var procStartFn = func(pid int) (time.Time, bool) {
	if pid <= 0 {
		return time.Time{}, false
	}
	out, err := exec.Command("ps", "-o", "lstart=", "-p", fmt.Sprint(pid)).Output()
	if err != nil {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("Mon Jan _2 15:04:05 2006", strings.TrimSpace(string(out)), time.Local)
	return t, err == nil
}

// codexSessionsRootFn — rollout 폴더. 테스트가 바꿔 끼운다.
var codexSessionsRootFn = usage.CodexSessionsRoot

// ResolveCodexThread는 큐로 보낼 세션 ID를 정한다. 훅이 남긴 값이 정본이고, 없을 때만 rollout에서 찾는다.
// 찾는 조건은 엄격하다: 같은 폴더에 산 코덱스가 하나뿐이고(스레드 세션과 섞이지 않게), 그 폴더의 가장 최근
// 세션이 이 프로세스가 뜬 뒤에 만들어졌을 때만. 어긋나면 빈 값 — tmux 입력으로 간다.
func ResolveCodexThread(a *state.Agent, agents []*state.Agent) string {
	if a == nil || a.Kind != "codex" {
		return ""
	}
	if a.SessionID != "" {
		return a.SessionID
	}
	for _, o := range agents {
		if o != nil && o.ID != a.ID && o.Kind == "codex" && o.CWD == a.CWD && o.State != state.StateDead {
			return ""
		}
	}
	started, ok := procStartFn(a.PID)
	if !ok || a.CWD == "" {
		return ""
	}
	return usage.CodexSessionSince(codexSessionsRootFn(), a.CWD, started.Add(-5*time.Second))
}

// canCodexQueue — 큐로 보낼 수 있는 대상인가. 죽은 세션·세션 ID 없는 세션·설정으로 끈 경우는 tmux 경로.
func canCodexQueue(a *state.Agent, cfg *config.Config) bool {
	if a == nil || a.Kind != "codex" || a.SessionID == "" || !cfg.CodexQueueEnabled() {
		return false
	}
	// 승인 대기(WAIT)는 뺀다 — 큐에 넣은 글은 턴이 끝나야 처리되는데 승인창은 턴을 붙들고 있어
	// 메시지가 조용히 쌓이기만 한다. 예전처럼 거부해 총괄이 승인 뒤 다시 보내게 한다.
	switch a.State {
	case state.StateIdle, state.StateDoneUnread, state.StateWorking:
		return true
	}
	return false
}

// Delivery는 전송 한 건의 부가 정보 — 채널 경로가 쓴다.
type Delivery struct {
	StateDir string // 직원 수신함(inboxes/)의 뿌리. 비면 채널 경로를 쓰지 않는다
	From     string // 보낸 세션 이름
	TaskID   string // 업무ID(없으면 "")
}

// canClaudeChannel — 채널로 보낼 수 있는 대상인가: Claude이고, 그 세션 수신함(pane 또는 PID)의 채널 서버가 살아 있고, 본문이 상한 안.
// 승인 대기(WAIT)는 코덱스 큐와 같은 이유로 뺀다.
func canClaudeChannel(a *state.Agent, cfg *config.Config, d Delivery, message string) bool {
	if a == nil || a.Kind != "claude" || d.StateDir == "" || !cfg.ClaudeChannelEnabled() || len(message) > maxChannelDirective {
		return false
	}
	switch a.State {
	case state.StateIdle, state.StateDoneUnread, state.StateWorking:
		return ChannelLive(AgentInbox(d.StateDir, a))
	}
	return false
}

// CanBypassGate — tmux 관문(작업 중 거부)에 걸려도 보낼 수 있는 경로가 있는가.
func CanBypassGate(a *state.Agent, cfg *config.Config, d Delivery, message string) bool {
	return canCodexQueue(a, cfg) || canClaudeChannel(a, cfg, d, message)
}

// DeliverTo는 send 밖의 경로(browser pick·shot, wt review)가 쓰는 공통 진입점 — 전송 규칙은 한 가지다:
// 코덱스는 큐, 채널 서버가 뜬 Claude는 채널이 정본이고 tmux 키 입력은 어디서나 폴백이다(2026-09-30 사용자 결정).
// 관문(작업 중 거부)은 두지 않는다 — 이 경로들은 예전에도 상태와 무관하게 pane에 쳤다. agents는 발신 세션 이름을 찾는 데만 쓴다(nil 가능).
func DeliverTo(ctx context.Context, stateDir string, agents []*state.Agent, a *state.Agent, tm TextSender, message string) (via, warn string, err error) {
	d := Delivery{StateDir: stateDir, From: senderName(agents)}
	return deliver(ctx, a, config.Load(), tm, message, true, d)
}

// errNoFallback — tmux 밖 세션(데스크톱 앱)에는 키 입력 폴백이 없다. 채널·큐가 안 되면 여기서 끝난다.
var errNoFallback = errors.New("tmux 밖 세션(app)이라 키 입력 폴백이 없습니다 — 채널 서버(claude mcp add -s local agentlayer -- agentlayer channel serve --self)나 codex 큐가 필요합니다")

// deliver는 에이전트 하나에 메시지를 넣는다. 코덱스는 큐, 채널 서버가 뜬 Claude는 채널을 먼저 쓰고,
// 실패하면 tmux 키 입력으로 되돌아간다. via는 "queue"·"channel"·"tmux". warn은 되돌아갔을 때의 사유(없으면 "").
// tmuxOK가 false면(작업 중인데 --force 없음) 실패 시 tmux로 되돌아가지 않고 오류를 낸다.
// tmux 밖 세션(pane 없음)은 폴백이 없다 — 채널·큐가 실패하거나 둘 다 불가능하면 오류로 끝난다.
func deliver(ctx context.Context, a *state.Agent, cfg *config.Config, tm TextSender, message string, tmuxOK bool, d Delivery) (via, warn string, err error) {
	tmuxOK = tmuxOK && !a.Detached()
	if canClaudeChannel(a, cfg, d, message) {
		cerr := SendViaChannel(AgentInbox(d.StateDir, a), d.From, d.TaskID, message, time.Now())
		if cerr == nil {
			return "channel", "", nil
		}
		warn = "채널 전송 실패(" + cerr.Error() + ") — tmux 입력으로 보냄"
		if !tmuxOK {
			return "", "", fmt.Errorf("채널 전송 실패: %v", cerr)
		}
	}
	if canCodexQueue(a, cfg) {
		qerr := codexQueueFn(ctx, a.SessionID, a.CWD, message)
		if qerr == nil {
			return "queue", "", nil
		}
		warn = "codex queue 실패(" + qerr.Error() + ") — tmux 입력으로 보냄"
		if !tmuxOK {
			return "", "", fmt.Errorf("codex queue 실패: %v", qerr)
		}
	}
	if a.Detached() {
		return "", "", errNoFallback
	}
	if err := tm.SendText(a.Tmux.PaneID, message); err != nil {
		return "", warn, err
	}
	return "tmux", warn, nil
}
