package hookcmd

import (
	"os"

	"github.com/netwaif/agentlayer/internal/scan"
)

// NestedClaude는 pid의 조상 사슬에 claude 프로세스가 둘 이상이면 true — 직원 세션이 Bash 도구로 띄운
// 자식 claude(예: `claude -p`)의 훅은 같은 TMUX_PANE으로 들어와 부모 업무ID로 DONE_UNREAD를 보고한다
// (WAKE-PATH-RESEARCH-3 실측 4회). 표에 없는 pid·사슬 끊김은 false(훅을 막지 않는다).
func NestedClaude(pt scan.ProcTable, pid int) bool {
	n := 0
	for p := pid; p > 1; {
		e, ok := pt[p]
		if !ok || e.PPID == p {
			return false
		}
		if scan.KindFromArgs(e.Args) == "claude" {
			n++
			if n >= 2 {
				return true
			}
		}
		p = e.PPID
	}
	return false
}

// nestedCheck는 RunClaude가 부른다(ps 한 번, 수십 ms). 테스트가 바꿔 끼운다.
var nestedCheck = func() bool { return NestedClaude(scan.LoadProcTable(), os.Getppid()) }
