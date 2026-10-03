package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/netwaif/agentlayer/internal/scan"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
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
	// CWD — 기록 없는 코덱스 세션에 큐로 보낼 때의 작업 폴더(sendCodexDirect). 비면 rollout에서 찾는다.
	CWD string
	// Files — 업무 등록 없는 원격 직송(sendRemoteDirect)에 붙일 첨부 파일. 다른 경로에 주면 오류.
	Files []string
	// From — 발신자 이름을 명시(--from). 비면 senderName이 정한다(tmux 세션명 → 코덱스/별칭/claude → "user").
	From string
}

// ParseSendFlags는 --force·--json·--cwd·--file만 받고 나머지를 위치 인자로 돌려준다.
func ParseSendFlags(args []string) (SendOptions, []string, error) {
	var o SendOptions
	var rest []string // rest[0]=대상, rest[1:]=메시지 — 메시지가 시작되면 그 뒤는 플래그처럼 보여도 본문이다
	setFrom := func(v string) error {
		if !validAddressName(v) {
			return errBadFrom(v)
		}
		o.From = v
		return nil
	}
	for i := 0; i < len(args); i++ {
		if len(rest) >= 2 {
			rest = append(rest, args[i])
			continue
		}
		switch args[i] {
		case "--help", "-h":
			return o, nil, errSendUsage
		case "--force":
			o.Force = true
		case "--json":
			o.JSON = true
		case "--cwd", "--file", "--from":
			if i+1 >= len(args) {
				return o, nil, fmt.Errorf("%s 뒤에 값이 필요합니다", args[i])
			}
			switch args[i] {
			case "--cwd":
				o.CWD = args[i+1]
			case "--file":
				o.Files = append(o.Files, args[i+1])
			default:
				if err := setFrom(args[i+1]); err != nil {
					return o, nil, err
				}
			}
			i++
		default:
			switch {
			case strings.HasPrefix(args[i], "--cwd="):
				o.CWD = strings.TrimPrefix(args[i], "--cwd=")
			case strings.HasPrefix(args[i], "--from="):
				if err := setFrom(strings.TrimPrefix(args[i], "--from=")); err != nil {
					return o, nil, err
				}
			case strings.HasPrefix(args[i], "--file="):
				o.Files = append(o.Files, strings.TrimPrefix(args[i], "--file="))
			case strings.HasPrefix(args[i], "--"):
				return o, nil, fmt.Errorf("알 수 없는 플래그: %s", args[i])
			default:
				// 대상, 그 다음 메시지 첫 토큰. 대상 뒤에 온 플래그도 플래그다(README 예시
				// `send hermes-qa --file 스펙.md "…"`가 본문으로 들어가던 것 — Win11 WSL2 실측 2026-10-03).
				rest = append(rest, args[i])
			}
		}
	}
	if len(rest) == 0 {
		return o, nil, nil
	}
	return o, rest, nil
}

// errSendUsage — `send --help`/인자 부족 때의 사용법.
var errSendUsage = errors.New("사용법: agentlayer send [--force] [--json] [--cwd <폴더>] [--file <경로>]... [--from <이름>] <세션[:창]|이름|코덱스 세션 ID|원격> <메시지> (여러 줄은 '-'로 stdin; 플래그는 대상 뒤에 와도 된다)")

// errBadFrom — --from 값은 주소록 이름과 같은 제한(경로 문자·숨김 접두 금지, 128자 이내).
func errBadFrom(v string) error {
	return fmt.Errorf("--from 형식 오류: %q (경로 문자·숨김 접두 금지, 128자 이내)", v)
}

// errNoFileHere — --file은 업무 등록 없는 원격 직송에서만 받는다. 다른 경로(tmux·채널·큐·업무 등록된 원격)는 첨부를 나를 길이 없다.
var errNoFileHere = errors.New("--file은 업무 등록이 없는 원격(remotes/<이름>.json) 직송에서만 쓸 수 있습니다")

// sendFrom — 이번 전송의 발신자 이름: --from이 있으면 그 값, 없으면 senderNameIn.
func sendFrom(o SendOptions, agents []*state.Agent, stateDir string) string {
	if o.From != "" {
		return o.From
	}
	return senderNameIn(agents, stateDir)
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

// senderName은 이 명령을 부른 세션의 이름. tmux 안이면 자기 pane의 에이전트 레코드(세션명), 못 찾으면 "user"(예전 그대로).
// tmux 밖(코덱스 데스크톱 앱·앱 Claude 세션 등)이면 예전에는 늘 "user"였다 — 이제 부모 사슬의 에이전트 프로세스로 정한다:
// 코덱스면 "codex", claude면 그 세션이 `inbox open`으로 등록한 별칭(주소록에서 pid 역조회), 없으면 "claude". 그 밖은 "user".
func senderName(agents []*state.Agent) string { return senderNameIn(agents, "") }

// senderNameIn — stateDir은 별칭 역조회용(비면 역조회 없이 "claude").
func senderNameIn(agents []*state.Agent, stateDir string) string {
	pane := os.Getenv("TMUX_PANE")
	if pane != "" {
		for _, a := range agents {
			if a != nil && a.Tmux.PaneID == pane && a.State != state.StateDead {
				return a.Tmux.Session
			}
		}
		return "user"
	}
	kind, pid := senderProcessFn()
	switch kind {
	case "codex":
		return "codex"
	case "claude":
		if alias := aliasByPID(stateDir, pid); alias != "" {
			return alias
		}
		return "claude"
	}
	return "user"
}

// senderProcessFn — 부모 사슬에서 가장 가까운 에이전트 프로세스(종류·PID). 없으면 ("", 0). 테스트가 바꿔 끼운다.
var senderProcessFn = func() (string, int) {
	pt := scan.LoadProcTable()
	for p, depth := os.Getppid(), 0; p > 1 && depth < 8; depth++ {
		e, ok := pt[p]
		if !ok || e.PPID == p {
			return "", 0
		}
		if k := scan.KindFromArgs(e.Args); k != "" {
			return k, p
		}
		p = e.PPID
	}
	return "", 0
}

// aliasByPID — 주소록에서 pid로 역조회한 별칭(`inbox open --name`의 이름). 별칭 파일과 ID(al-…) 파일이 같은 내용이라
// 별칭 쪽을 우선하고, 별칭이 없으면 ID를 돌려준다. 없으면 빈 값.
func aliasByPID(stateDir string, pid int) string {
	if stateDir == "" || pid <= 0 {
		return ""
	}
	files, _ := filepath.Glob(filepath.Join(AddressesDir(stateDir), "*.json"))
	id := ""
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var a Address
		if json.Unmarshal(b, &a) != nil || a.PID != pid {
			continue
		}
		key := strings.TrimSuffix(filepath.Base(f), ".json")
		if key == a.ID {
			id = a.ID
			continue
		}
		return key
	}
	return id
}

// RunSend: agentlayer send [--force] [--json] <세션[:창]> <메시지…|->
// ctx는 원격 직원 경로(ssh)까지 내려간다 — Ctrl-C가 진행 중인 ssh를 바로 끊는다.
func RunSend(ctx context.Context, w io.Writer, stdin io.Reader, st *state.Store, stateDir string, tm TextSender, args []string) error {
	o, rest, err := ParseSendFlags(args)
	if err != nil {
		return err
	}
	if len(rest) < 2 {
		return errSendUsage
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
		// 업무 등록이 없으면 예전에는 오류였다 — 그 경우에만 직송(sendRemoteDirect). 등록이 있으면 기존 경로 그대로.
		if _, has, lerr := task.Load(stateDir, task.RemoteAgentID(r.Name)); lerr == nil && !has {
			return sendRemoteDirect(ctx, w, stateDir, r, message, o, time.Now())
		}
		if len(o.Files) > 0 {
			return errNoFileHere
		}
		return sendRemote(ctx, w, st, stateDir, r, message, o, time.Now())
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	a, err := ResolveTarget(agents, rest[0])
	if err != nil {
		// 기존 해석이 실패한 뒤에만 타는 분기 — 예전에는 전부 오류였던 입력이다.
		// (1) 세션 ID 형식이면 기록 없는 코덱스 세션(데스크톱 앱)으로 보고 큐로 직송.
		if LooksLikeSessionID(rest[0]) {
			if len(o.Files) > 0 {
				return errNoFileHere
			}
			return sendCodexDirect(ctx, w, rest[0], o, message)
		}
		// (2) `inbox wait`가 등록한 주소록 이름이면 그 수신함에 편지를 넣는다.
		if addr, found, aerr := LoadAddress(stateDir, rest[0]); aerr == nil && found {
			if len(o.Files) > 0 {
				return errNoFileHere
			}
			return sendToAddress(w, stateDir, addr, sendFrom(o, agents, stateDir), message, o)
		}
		return err
	}
	if len(o.Files) > 0 {
		return errNoFileHere
	}
	cfg := config.Load()
	// 훅이 세션 ID를 못 남긴 코덱스는 rollout에서 찾아 채운다(전송에만 쓰고 저장하지 않는다).
	if a.Kind == "codex" && a.SessionID == "" {
		c := *a
		c.SessionID = ResolveCodexThread(a, agents)
		a = &c
	}
	ok, reason := SendGate(a.State, o.Force)
	d := Delivery{StateDir: stateDir, From: sendFrom(o, agents, stateDir)}
	if as, found, _ := task.Load(stateDir, a.ID); found && as.Session == a.Tmux.Session && as.Pane == a.Tmux.PaneID {
		d.TaskID = as.TaskID
	}
	// 코덱스 큐·Claude 채널은 작업 중에도 안전하다(현재 턴 뒤에 처리) — tmux 관문에 걸려도 보낸다. 승인 대기는 제외.
	if !ok && !CanBypassGate(a, cfg, d, message) {
		return fmt.Errorf("%s(%s): %s", a.Tmux.Session, a.State, reason)
	}
	via, qwarn, err := deliver(ctx, a, cfg, tm, message, ok, d)
	if err != nil {
		return fmt.Errorf("%s 전송 실패: %w", a.Tmux.Session, err)
	}
	// 지시가 들어간 시각을 세션 기록에 남긴다 — 등록된 세션의 멈춤 보고는 이 시각 이후 첫 DONE까지만 총괄에게 간다.
	// 훅이 같은 파일을 갱신하므로 방금 읽은 복사본이 아니라 다시 읽어 그 필드만 바꾼다(실패는 보고 한 번 덜 갈 뿐).
	if cur, lerr := st.Load(a.ID); lerr == nil && cur != nil {
		cur.LastSendAt = time.Now()
		_ = st.Save(cur)
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
		return json.NewEncoder(w).Encode(map[string]any{"session": a.Tmux.Session, "window": a.Tmux.WindowName,
			"pane": a.Tmux.PaneID, "state": a.State, "sent": true, "via": via})
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
	fmt.Fprintf(w, "전송 완료 → %s %s [%s]%s\n", a.Tmux.Session, a.Tmux.PaneID, a.State, note)
	return nil
}
