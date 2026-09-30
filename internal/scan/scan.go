// Package scan은 tmux pane 현실과 상태 저장소를 동기화한다.
// 스캐너는 발견·좌표 갱신·소실 처리만 하고, 의미 상태(WORKING 등)는
// hook의 영역이므로 절대 덮어쓰지 않는다.
package scan

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
)

// DEAD 레코드를 보존하는 기간. 그 뒤에는 저장소에서 정리한다.
const deadRetention = 24 * time.Hour

// versionRe: Claude Code는 프로세스 이름을 자기 버전("2.1.241")으로 바꾼다.
var versionRe = regexp.MustCompile(`^\d+(\.\d+)+$`)

// DetectKind는 pane이 어떤 에이전트인지 판정한다.
// 화면 내용 스크래핑은 하지 않는다 — command와 title 메타데이터만 본다.
// 판정은 로케일 무관해야 한다: LANG 없는 환경(LaunchAgent)에서 tmux가
// 제목의 비ASCII를 치환하므로, ✳ 제목 신호는 보조로만 쓴다.
func DetectKind(p tmuxx.Pane) string {
	cmd := strings.ToLower(p.Command)
	switch {
	case strings.HasPrefix(cmd, "claude"):
		return "claude"
	case strings.HasPrefix(cmd, "codex"):
		return "codex"
	case strings.HasPrefix(cmd, "gemini"), strings.HasPrefix(cmd, "agy"):
		// agy = Antigravity CLI (Gemini 계열) — 같은 gemini kind로 관제한다
		return "gemini"
	case versionRe.MatchString(cmd):
		// 버전 형식 command = Claude Code (프로세스명을 버전으로 바꿈)
		return "claude"
	case strings.Contains(p.Title, "✳"):
		return "claude"
	}
	return ""
}

// AgentID는 결정적 ID. hook은 TMUX_PANE만 알므로 pane ID 기반으로 만든다.
// tmux 서버가 재시작되면 pane ID가 재사용될 수 있으나, 소실 레코드는
// DEAD로 정리되므로 관제 목적에는 충분하다.
func AgentID(kind string, p tmuxx.Pane) string {
	return IDForPane(kind, p.PaneID)
}

// IDForPane은 hook 경로(TMUX_PANE 환경변수)와 스캐너가 공유하는 ID 규칙.
func IDForPane(kind, paneID string) string {
	return fmt.Sprintf("%s-%s", kind, strings.TrimPrefix(paneID, "%"))
}

// IDForProcess는 tmux 밖 세션(데스크톱 앱·맨 터미널)의 ID 규칙 — 좌표가 pane 대신 에이전트 프로세스 PID다.
//
// 세션 ID가 아니라 PID를 키로 쓰는 이유: 훅과 채널 서버(Claude가 띄운 MCP 자식)가 서로 조율 없이 같은 주소를
// 만들 수 있는 값은 "자기를 띄운 에이전트 프로세스"뿐이다. 세션 ID는 훅만 알고, /clear 때 바뀌지만 프로세스와
// MCP 서버는 그대로이며, 코덱스 notify 경로는 세션 ID를 아예 주지 않는다. 세션 ID·이름은 레코드 필드로 남겨
// send 대상 해석에 쓴다. PID 재사용(재부팅)은 pane 번호 재사용과 같은 방식으로 다룬다(SyncDetached·purgeStale).
func IDForProcess(kind string, pid int) string {
	return fmt.Sprintf("%s-pid%d", kind, pid)
}

// Sync는 pane 목록을 정본 저장소에 반영한다.
func Sync(st *state.Store, panes []tmuxx.Pane, now time.Time) error {
	existing, err := st.List()
	if err != nil {
		return err
	}
	byID := make(map[string]*state.Agent, len(existing))
	for _, a := range existing {
		byID[a.ID] = a
	}

	// kind|세션|cwd 키 — restore 없이 밖에서(예: 브리지 LaunchAgent) 같은 자리에
	// 되살린 세션을 식별해, 옛 DEAD 레코드의 이중 행을 정리하는 데 쓴다.
	liveSlot := func(kind, session, cwd string) string {
		return kind + "|" + session + "|" + cwd
	}
	alive := make(map[string]bool)
	occupied := make(map[string]bool)
	liveSession := make(map[string]bool) // kind|세션 — 스레드 창 정리 근거
	var procs ProcTable                  // 래퍼 pane이 있을 때만 한 번 읽는다
	for _, p := range panes {
		kind := DetectKind(p)
		if kind == "" && IsWrapperCommand(p.Command) {
			// npm으로 깐 codex·gemini-cli: pane 전면이 node 래퍼 → 프로세스 표로 2차 판정
			if procs == nil {
				procs = loadProcTable()
			}
			kind = procs.DescendantKind(p.PanePID)
		}
		if kind == "" {
			continue
		}
		id := AgentID(kind, p)
		alive[id] = true
		occupied[liveSlot(kind, p.Session, p.Path)] = true
		liveSession[kind+"|"+p.Session] = true
		a, ok := byID[id]
		if !ok {
			a = &state.Agent{ID: id, Kind: kind, State: state.StateIdle,
				UpdatedAt: now, StateSince: now}
		}
		// 좌표·환경은 항상 현실을 따른다. 의미 상태는 건드리지 않는다.
		a.Tmux = state.TmuxRef{Session: p.Session, Window: p.Window, WindowName: p.WindowName, PaneID: p.PaneID}
		a.CWD = p.Path
		a.PID = p.PanePID
		if a.State == state.StateDead {
			// 같은 pane ID가 되살아났다(재사용 포함) — 새 관찰로 취급
			a.Transition(state.StateIdle, now)
		}
		if err := st.Save(a); err != nil {
			return err
		}
	}

	for _, a := range existing {
		if alive[a.ID] || a.Detached() {
			continue // pane 없는 레코드는 pane 목록으로 판정할 수 없다 — SyncDetached가 프로세스 표로 본다
		}
		switch {
		case a.State == state.StateDead && occupied[liveSlot(a.Kind, a.Tmux.Session, a.CWD)]:
			// 같은 자리에 산 pane이 있다 — 세션이 밖에서 부활함. 이중 행 즉시 정리
			if err := st.Delete(a.ID); err != nil {
				return err
			}
		case a.IsThread() && liveSession[a.Kind+"|"+a.Tmux.Session]:
			// 봇 스레드 창이 닫혔고 봇 본체는 살아 있다 — 스레드는 일회성이니 DEAD를
			// 거치지 않고 바로 정리. cwd가 메인과 달라도(컨테이너) 같은 kind·세션의 산 pane이 근거다.
			if err := st.Delete(a.ID); err != nil {
				return err
			}
		case a.State == state.StateDead && now.Sub(a.StateSince) > deadRetention:
			if err := st.Delete(a.ID); err != nil {
				return err
			}
		case a.State != state.StateDead:
			a.Transition(state.StateDead, now)
			if err := st.Save(a); err != nil {
				return err
			}
		}
	}
	return nil
}

// SyncDetached는 tmux 밖(pane 없는) 레코드를 프로세스 표와 대조한다. 에이전트 프로세스가 사라졌거나 그 PID에
// 다른 종류의 프로세스가 앉아 있으면 레코드를 지운다 — pane 레코드와 달리 restore로 되살릴 자리가 없어 DEAD를
// 보존할 이유가 없다. 프로세스 표를 못 읽으면(빈 표) 아무것도 건드리지 않는다. 표는 대상이 있을 때만 읽는다(ps 1회).
func SyncDetached(st *state.Store, now time.Time) error {
	existing, err := st.List()
	if err != nil {
		return err
	}
	var pt ProcTable
	for _, a := range existing {
		if !a.Detached() {
			continue
		}
		if pt == nil {
			pt = loadProcTable()
			if len(pt) == 0 {
				return nil
			}
		}
		if ProcessIsAgent(pt, a.PID, a.Kind) {
			continue
		}
		if err := st.Delete(a.ID); err != nil {
			return err
		}
	}
	return nil
}

// ProcessIsAgent는 pid에 kind 에이전트가 살아 있는가. 명령행에서 종류를 못 읽는 프로세스는 산 것으로 본다 —
// 훅이 조상에서 kind를 찾지 못해 부모 PID로 기록한 경우까지 즉시 지우지 않기 위해서다.
func ProcessIsAgent(pt ProcTable, pid int, kind string) bool {
	if pid <= 0 {
		return false
	}
	p, ok := pt[pid]
	if !ok {
		return false
	}
	k := KindFromArgs(p.Args)
	return k == "" || k == kind
}

// maxAncestorDepth — 훅(agentlayer ← sh ← claude)·MCP 서버(agentlayer ← claude)·Bash 도구(agentlayer ← bash ← claude)
// 모두 서너 단계 안에 에이전트가 있다. 더 올라가면 터미널 앱·launchd까지 닿는다.
const maxAncestorDepth = 8

// FindAgentProcess는 pid에서 조상으로 올라가며 가장 가까운 kind 에이전트 프로세스를 찾는다(kind가 비면 아무 종류).
// 훅은 셸을 거쳐 뜨고 MCP 서버는 직접 뜨므로 os.Getppid()가 서로 다르다 — 둘 다 "가장 가까운 에이전트 조상"으로
// 맞춰야 같은 주소(IDForProcess)가 나온다. pid 자신도 후보다. 돌려주는 값: (에이전트 PID, 종류, 명령행).
func FindAgentProcess(pt ProcTable, pid int, kind string) (int, string, string, bool) {
	for p, depth := pid, 0; p > 1 && depth < maxAncestorDepth; depth++ {
		e, ok := pt[p]
		if !ok || e.PPID == p {
			return 0, "", "", false
		}
		if k := KindFromArgs(e.Args); k != "" && (kind == "" || k == kind) {
			return p, k, e.Args, true
		}
		p = e.PPID
	}
	return 0, "", "", false
}

// NameFromArgs는 명령행의 세션 이름(`claude -n <이름>`·`--name <이름>`·`--name=<이름>`)을 읽는다. 없으면 빈 값.
func NameFromArgs(args string) string {
	f := strings.Fields(args)
	for i, tok := range f {
		switch {
		case tok == "-n" || tok == "--name":
			if i+1 < len(f) && !strings.HasPrefix(f[i+1], "-") {
				return f[i+1]
			}
		case strings.HasPrefix(tok, "--name="):
			return strings.TrimPrefix(tok, "--name=")
		}
	}
	return ""
}
