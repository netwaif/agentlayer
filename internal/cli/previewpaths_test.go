package cli

import (
	"testing"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/wt"
)

// hook 자동 프리뷰도 worktree 폴더에는 ⎇브랜치를 달아야 한다(B 시연의 핵심 컷).
func TestPreviewPathsAttachesWorktreeBranch(t *testing.T) {
	agents := []*state.Agent{
		{CWD: "/repo", State: state.StateIdle},
		{CWD: "/repo/.worktrees/hero", State: state.StateWorking},
		{CWD: "/dead", State: state.StateDead},
	}
	metas := []*wt.Meta{{Path: "/repo/.worktrees/hero", Branch: "agent/hero"}}
	got := PreviewPaths(agents, metas)
	if got["/repo/.worktrees/hero"] != "agent/hero" {
		t.Errorf("worktree 브랜치가 붙어야 함: %v", got)
	}
	if v, ok := got["/repo"]; !ok || v != "" {
		t.Errorf("일반 폴더는 빈 브랜치로 포함: %v", got)
	}
	if _, ok := got["/dead"]; ok {
		t.Errorf("죽은 에이전트는 제외: %v", got)
	}
}
