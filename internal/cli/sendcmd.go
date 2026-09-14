package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/netwaif/agentlayer/internal/state"
)

// TextSender는 pane에 지시를 넣는 최소 인터페이스 — tmuxx.Tmux가 만족하고 테스트는 페이크.
type TextSender interface {
	SendText(paneID, text string) error
}

// ResolveTarget은 "<세션>" 또는 "<세션>:<창이름>"을 산 pane 하나로 해석한다.
// 창 이름은 folder-bot 스레드 창(t+6자리)을 가리키는 용도. 후보가 둘 이상이면 창 명시를 요구한다.
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
	var rest []string
	for _, a := range args {
		switch {
		case a == "--force":
			o.Force = true
		case a == "--json":
			o.JSON = true
		case strings.HasPrefix(a, "--") && len(rest) == 0:
			return o, nil, fmt.Errorf("알 수 없는 플래그: %s", a)
		default:
			rest = append(rest, a)
		}
	}
	return o, rest, nil
}

// RunSend: agentlayer send [--force] [--json] <세션[:창]> <메시지…|->
func RunSend(w io.Writer, stdin io.Reader, st *state.Store, tm TextSender, args []string) error {
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
	if strings.TrimSpace(message) == "" {
		return errors.New("메시지가 비었습니다")
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	a, err := ResolveTarget(agents, rest[0])
	if err != nil {
		return err
	}
	ok, reason := SendGate(a.State, o.Force)
	if !ok {
		return fmt.Errorf("%s(%s): %s", a.Tmux.Session, a.State, reason)
	}
	if err := tm.SendText(a.Tmux.PaneID, message); err != nil {
		return fmt.Errorf("%s 전송 실패: %w", a.Tmux.Session, err)
	}
	if o.JSON {
		return json.NewEncoder(w).Encode(map[string]any{"session": a.Tmux.Session, "window": a.Tmux.WindowName,
			"pane": a.Tmux.PaneID, "state": a.State, "sent": true})
	}
	note := ""
	if reason != "" {
		note = "  ⚠ " + reason
	}
	fmt.Fprintf(w, "전송 완료 → %s %s [%s]%s\n", a.Tmux.Session, a.Tmux.PaneID, a.State, note)
	return nil
}
