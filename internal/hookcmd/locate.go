package hookcmd

import (
	"os"

	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
)

// location은 훅이 기록할 에이전트의 좌표 — tmux pane이면 pane, tmux 밖(데스크톱 앱·맨 터미널)이면 프로세스 PID.
type location struct {
	id   string
	pane string // tmux pane ID("%3"). tmux 밖이면 빈 값
	pid  int    // tmux 밖 세션의 에이전트 프로세스 PID
	name string // tmux 밖 세션의 `-n` 이름(있을 때)
	// nested — tmux 밖 claude가 다른 claude의 자식(Bash 도구로 띄운 claude -p 등)인가. pane 경로는 nestedCheck가 본다.
	nested bool
}

// locate는 훅의 좌표를 정한다.
//   - TMUX_PANE이 기본 tmux 서버의 것이면 pane 좌표(예전 그대로).
//   - TMUX_PANE이 있는데 기본 서버가 아니거나 TMUX가 비었으면(별도 -L 서버·서버 사망 뒤 잔류) 예전대로 무시한다 —
//     guard.go의 방어를 그대로 둔다. 이런 세션은 tmux 밖 세션으로 다시 잡지 않는다.
//   - TMUX_PANE이 아예 없으면 tmux 밖 세션이다. 훅을 띄운 에이전트 프로세스(조상 사슬에서 가장 가까운 kind)를 PID 좌표로 쓴다.
//     못 찾으면(명령행에서 종류를 못 읽는 배포) 관제 대상이 아니다.
func locate(kind string, env func(string) string) (location, bool) {
	if pane := hookPane(env); pane != "" {
		return location{id: scan.IDForPane(kind, pane), pane: pane}, true
	}
	if env("TMUX_PANE") != "" {
		return location{}, false
	}
	pid, name, nested := detachedSelf(kind)
	if pid <= 0 {
		return location{}, false
	}
	return location{id: scan.IDForProcess(kind, pid), pid: pid, name: name, nested: nested}, true
}

// apply는 tmux 밖 세션의 프로세스 좌표를 레코드에 반영한다(pane 세션은 건드리지 않는다).
func (l location) apply(a *state.Agent) {
	if l.pane != "" {
		return
	}
	a.PID = l.pid
	if l.name != "" {
		a.Name = l.name
	}
}

// detachedSelf는 tmux 밖 훅의 좌표 — 이 훅을 띄운 kind 에이전트 프로세스의 PID와 `-n` 이름, claude면 중첩 여부.
// ps 한 번(수십 ms). 테스트가 바꿔 끼운다.
var detachedSelf = func(kind string) (pid int, name string, nested bool) {
	pt := scan.LoadProcTable()
	me := os.Getppid()
	pid, k, args, ok := scan.FindAgentProcess(pt, me, kind)
	if !ok {
		return 0, "", false
	}
	if k == "claude" {
		nested = NestedClaude(pt, me)
	}
	return pid, scan.NameFromArgs(args), nested
}

// SelfAgent는 이 프로세스를 띄운 세션의 레코드 — send·task message가 "나는 누구인가"를 훅과 같은 규칙으로 판정한다.
// pane 안이면 그 pane의 레코드, tmux 밖이면 조상 에이전트 프로세스의 PID 레코드. 없으면 nil.
func SelfAgent(env func(string) string, agents []*state.Agent) *state.Agent {
	if pane := hookPane(env); pane != "" {
		for _, a := range agents {
			if a != nil && a.Tmux.PaneID == pane && a.State != state.StateDead {
				return a
			}
		}
		return nil
	}
	if env("TMUX_PANE") != "" {
		return nil
	}
	pid, _, _ := detachedSelf("")
	if pid <= 0 {
		return nil
	}
	for _, a := range agents {
		if a != nil && a.Detached() && a.PID == pid {
			return a
		}
	}
	return nil
}
