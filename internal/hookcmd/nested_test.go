package hookcmd

import (
	"testing"

	"github.com/netwaif/agentlayer/internal/scan"
)

// 실측 사슬(2026-09-27): hook ← sh ← claude ← zsh ← tmux. 자식 세션은 hook ← sh ← claude(자식) ← zsh -c(Bash 도구) ← claude(부모).
const procFixture = `1 0 /sbin/launchd
1193 1 /usr/local/bin/tmux new-session -d -s company-bot
62376 1193 -zsh
62539 62376 claude --resume fe4078c8
20700 62539 /bin/sh -c agentlayer hook claude --event stop
20655 62539 /bin/zsh -c source /Users/x/.claude/shell-snapshots/snap.sh
70000 20655 claude -p hi
70001 70000 /bin/sh -c agentlayer hook claude --event stop
`

func TestNestedClaudeSingleAncestor(t *testing.T) {
	pt := scan.ParseProcTable(procFixture)
	if NestedClaude(pt, 20700) {
		t.Error("부모 세션의 훅(claude 조상 1개)은 자식이 아니다 — 재시작으로 session_id가 바뀌어도 보고는 계속돼야 한다")
	}
}

func TestNestedClaudeChildSession(t *testing.T) {
	pt := scan.ParseProcTable(procFixture)
	if !NestedClaude(pt, 70001) {
		t.Error("Bash 도구로 띄운 자식 claude의 훅은 자식으로 판정돼야 한다")
	}
}

func TestNestedClaudeUnknownPidIsNotNested(t *testing.T) {
	pt := scan.ParseProcTable(procFixture)
	if NestedClaude(pt, 99999) {
		t.Error("표에 없는 pid는 판정 불가 → 자식 아님(훅을 막지 않는다)")
	}
}
