package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/hookcmd"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

// TextSender는 pane에 지시를 넣는 최소 인터페이스 — tmuxx.Tmux가 만족하고 테스트는 페이크.
type TextSender interface {
	SendText(paneID, text string) error
}

// minSessionIDPrefix — 세션 ID 접두로 대상을 찾을 때 요구하는 최소 길이. 짧은 접두가 우연히 맞는 일을 막는다.
const minSessionIDPrefix = 4

// ResolveTarget은 대상 지정을 에이전트 하나로 해석한다. 순서:
//  1. "<세션>" 또는 "<세션>:<창이름>" — tmux 세션 이름(예전 규칙). 창 이름은 folder-bot 스레드 창(t+6자리)을 가리키는 용도.
//     같은 세션에 pane이 둘 이상이면 창 명시를 요구한다.
//  2. tmux 세션에 없으면 `-n` 이름(정확히 일치)이나 세션 ID(앞자리 접두, 4자 이상) — tmux 밖(앱) 세션이 주 대상이지만
//     pane 세션도 세션 ID로 찾을 수 있다. 산 레코드를 죽은 레코드보다 먼저 본다. 후보가 둘 이상이면 보여 주고 거부한다.
func ResolveTarget(agents []*state.Agent, spec string) (*state.Agent, error) {
	session, window := spec, ""
	if i := strings.LastIndex(spec, ":"); i > 0 {
		session, window = spec[:i], spec[i+1:]
	}
	var found []*state.Agent
	for _, a := range agents {
		if a.Tmux.Session != session {
			continue
		}
		if window != "" && a.Tmux.WindowName != window {
			continue
		}
		found = append(found, a)
	}
	if len(found) == 0 && window == "" {
		found = matchByNameOrSessionID(agents, spec)
		if len(found) > 1 {
			names := make([]string, 0, len(found))
			for _, a := range found {
				names = append(names, TargetLabel(a))
			}
			return nil, fmt.Errorf("%q에 맞는 세션이 둘 이상입니다 — 더 긴 세션 ID나 이름을 쓰세요: %s", spec, strings.Join(names, ", "))
		}
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("세션 %q을 찾지 못했습니다 ('agentlayer status'로 이름 확인)", spec)
	case 1:
		return found[0], nil
	}
	names := make([]string, 0, len(found))
	for _, a := range found {
		names = append(names, fmt.Sprintf("%s:%s(%s)", a.Tmux.Session, a.Tmux.WindowName, a.Tmux.PaneID))
	}
	return nil, fmt.Errorf("세션 %q에 pane이 둘 이상입니다 — 창을 명시하세요: %s", session, strings.Join(names, ", "))
}

// matchByNameOrSessionID — `-n` 이름 정확 일치 또는 세션 ID 접두 일치. 산 레코드가 하나라도 있으면 죽은 것은 뺀다.
func matchByNameOrSessionID(agents []*state.Agent, spec string) []*state.Agent {
	var live, dead []*state.Agent
	for _, a := range agents {
		byName := a.Name != "" && a.Name == spec
		byID := len(spec) >= minSessionIDPrefix && a.SessionID != "" && strings.HasPrefix(a.SessionID, spec)
		if !byName && !byID {
			continue
		}
		if a.State == state.StateDead {
			dead = append(dead, a)
		} else {
			live = append(live, a)
		}
	}
	if len(live) > 0 {
		return live
	}
	return dead
}

// TargetLabel은 후보 목록·전송 결과에 쓰는 한 줄 주소 — pane 세션은 "세션:창(%pane)", tmux 밖 세션은 "이름 (app, 세션 <앞 8자리>)".
func TargetLabel(a *state.Agent) string {
	if !a.Detached() {
		return fmt.Sprintf("%s:%s(%s)", a.Tmux.Session, a.Tmux.WindowName, a.Tmux.PaneID)
	}
	sid := state.ShortSessionID(a.SessionID)
	if sid == "" {
		sid = "?"
	}
	return fmt.Sprintf("%s (app, 세션 %s)", a.Label(), sid)
}

// SendGate — idle·DONE만 보낸다. WORK는 현재 턴 뒤에 처리되고 WAIT(승인창)는 입력이 승인창을
// 깨뜨리므로 --force 없이는 거부. dead·ERR는 받을 곳이 없다.
func SendGate(s state.AgentState, force bool) (bool, string) {
	switch s {
	case state.StateIdle, state.StateDoneUnread:
		return true, ""
	case state.StateWorking:
		if force {
			return true, "작업 중 — 현재 턴 뒤에 처리됩니다"
		}
		return false, "작업 중입니다 — 끝난 뒤 보내거나 --force"
	case state.StateWaiting:
		if force {
			return true, "승인 대기 중 — 입력이 승인창에 들어갑니다"
		}
		return false, "승인 대기 중입니다(입력이 승인창을 깨뜨림) — 승인 뒤 보내거나 --force"
	case state.StateDead:
		return false, "세션이 죽었습니다 ('agentlayer restore')"
	default:
		return false, "비정상 종료 상태입니다"
	}
}

type SendOptions struct {
	Force, JSON bool
}

// ParseSendFlags는 --force·--json만 받고 나머지를 위치 인자로 돌려준다.
func ParseSendFlags(args []string) (SendOptions, []string, error) {
	var o SendOptions
	i := 0
	for ; i < len(args); i++ {
		switch args[i] {
		case "--force":
			o.Force = true
		case "--json":
			o.JSON = true
		default:
			if strings.HasPrefix(args[i], "--") {
				return o, nil, fmt.Errorf("알 수 없는 플래그: %s", args[i])
			}
			return o, append([]string{}, args[i:]...), nil
		}
	}
	return o, nil, nil
}

// maxMessageBytes는 send 본문 상한(64KiB) — 훅·터미널 붙여넣기를 넘는
// 비정상 입력이 pane을 오래 막지 않게 막는다.
const maxMessageBytes = 64 * 1024

// SanitizeMessage는 전송 본문을 정제한다: CRLF→LF, `\n`·`\t` 외 제어문자(0x20
// 미만·0x7f) 제거. 길이 제한은 RunSend에서 별도로 검사한다(정제 전 바이트 수 기준).
func SanitizeMessage(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' {
			b.WriteRune(r)
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// maxLogRunes — log.md에 남기는 send 본문 상한. 넘으면 절단 + …
const maxLogRunes = 1000

// LogExcerpt는 log.md 한 줄용 발췌: 개행 ⏎, 1000자 절단, 끝에 "(n자)".
func LogExcerpt(msg string) string {
	n := len([]rune(msg))
	flat := strings.ReplaceAll(msg, "\n", "⏎")
	if r := []rune(flat); len(r) > maxLogRunes {
		flat = string(r[:maxLogRunes]) + "…"
	}
	return fmt.Sprintf("%s (%d자)", flat, n)
}

// senderName은 이 명령을 부른 세션의 이름(자기 pane, tmux 밖이면 자기 프로세스의 레코드). 못 찾으면 "user".
func senderName(agents []*state.Agent) string {
	if me := hookcmd.SelfAgent(os.Getenv, agents); me != nil {
		return me.Label()
	}
	return "user"
}

// RunSend: agentlayer send [--force] [--json] <세션[:창]> <메시지…|->
// ctx는 원격 직원 경로(ssh)까지 내려간다 — Ctrl-C가 진행 중인 ssh를 바로 끊는다.
func RunSend(ctx context.Context, w io.Writer, stdin io.Reader, st *state.Store, stateDir string, tm TextSender, args []string) error {
	o, rest, err := ParseSendFlags(args)
	if err != nil {
		return err
	}
	if len(rest) < 2 {
		return errors.New("사용법: agentlayer send [--force] [--json] <세션[:창]> <메시지> (여러 줄은 '-'로 stdin)")
	}
	message := strings.Join(rest[1:], " ")
	if message == "-" {
		if stdin == nil {
			return errors.New("stdin이 없습니다")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		message = strings.TrimRight(string(b), "\n")
	}
	if len(message) > maxMessageBytes {
		return errors.New("메시지가 너무 깁니다 (최대 64KiB)")
	}
	message = SanitizeMessage(message)
	if strings.TrimSpace(message) == "" {
		return errors.New("메시지가 비었습니다")
	}
	// 원격 직원(remotes/<이름>.json)이면 어댑터 경로 — 이름 규칙(':' 없음)에 안 맞는 "<세션>:<창>"은 그대로 기존 경로.
	if r, ok, err := remote.Load(stateDir, rest[0]); err == nil && ok {
		return sendRemote(ctx, w, st, stateDir, r, message, o, time.Now())
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	a, err := ResolveTarget(agents, rest[0])
	if err != nil {
		return err
	}
	cfg := config.Load()
	// 훅이 세션 ID를 못 남긴 코덱스는 rollout에서 찾아 채운다(전송에만 쓰고 저장하지 않는다).
	if a.Kind == "codex" && a.SessionID == "" {
		c := *a
		c.SessionID = ResolveCodexThread(a, agents)
		a = &c
	}
	ok, reason := SendGate(a.State, o.Force)
	d := Delivery{StateDir: stateDir, From: senderName(agents)}
	if as, found, _ := task.Load(stateDir, a.ID); found && as.Session == a.Tmux.Session && as.Pane == a.Tmux.PaneID {
		d.TaskID = as.TaskID
	}
	// 코덱스 큐·Claude 채널은 작업 중에도 안전하다(현재 턴 뒤에 처리) — tmux 관문에 걸려도 보낸다. 승인 대기는 제외.
	if !ok && !CanBypassGate(a, cfg, d, message) {
		return fmt.Errorf("%s(%s): %s", a.Label(), a.State, reason)
	}
	via, qwarn, err := deliver(ctx, a, cfg, tm, message, ok, d)
	if err != nil {
		return fmt.Errorf("%s 전송 실패: %w", a.Label(), err)
	}
	if via == "queue" || via == "channel" {
		reason = ""
		if a.State == state.StateWorking {
			reason = "작업 중 — 현재 턴 뒤에 처리됩니다"
		}
	}
	if qwarn != "" {
		warnOut := w
		if o.JSON {
			warnOut = os.Stderr
		}
		fmt.Fprintln(warnOut, "  ⚠ "+qwarn)
	}
	// 회사 업무가 등록된 세션이면 총괄의 지시·답변을 log.md에 남긴다([ASK] 뒤의 [SEND]가 Q&A 한 쌍).
	if as, ok, _ := task.Load(stateDir, a.ID); ok && as.TaskDir != "" && as.Session == a.Tmux.Session && as.Pane == a.Tmux.PaneID {
		root, id := as.BoardRootID()
		if err := board.AppendLog(root, id, "SEND", LogExcerpt(message), time.Now()); err != nil {
			// --json이면 w는 파서가 읽는 출력 — 경고를 섞으면 JSON이 깨진다. stderr로 보낸다.
			warnOut := w
			if o.JSON {
				warnOut = os.Stderr
			}
			fmt.Fprintln(warnOut, "  ⚠ log.md 기록 실패:", err)
		}
		if _, err := RefreshBoardFile(st, stateDir, config.Load(), time.Now()); err != nil {
			warnOut := w
			if o.JSON {
				warnOut = os.Stderr
			}
			fmt.Fprintln(warnOut, "  ⚠ 보드 갱신 실패:", err)
		}
	}
	if o.JSON {
		return json.NewEncoder(w).Encode(map[string]any{"session": a.Label(), "window": a.Tmux.WindowName,
			"pane": a.Tmux.PaneID, "session_id": a.SessionID, "where": a.Where(), "state": a.State, "sent": true, "via": via})
	}
	note := ""
	if reason != "" {
		note = "  ⚠ " + reason
	}
	switch via {
	case "queue":
		note = " (codex queue)" + note
	case "channel":
		note = " (채널)" + note
	}
	addr := a.Tmux.PaneID
	if a.Detached() {
		addr = "app"
	}
	fmt.Fprintf(w, "전송 완료 → %s %s [%s]%s\n", a.Label(), addr, a.State, note)
	return nil
}
