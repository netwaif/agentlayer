package wt

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/tmuxx"
	"github.com/netwaif/agentlayer/internal/usage"
)

// 에이전트 종류 → 실행 명령. 실행은 pane 안에서 사용자의 셸 환경으로 한다.
var agentCommand = map[string]string{
	"claude": "claude",
	"codex":  "codex",
	"gemini": "gemini",
}

// commandFor는 실제 기동 명령을 결정한다. gemini는 환경에 따라 agy/gemini가
// 갈린다 (restore의 freshCommand와 같은 규칙 — usage.GeminiCommand 공유).
func commandFor(agent string) string {
	if agent == "gemini" {
		return usage.GeminiCommand()
	}
	return agentCommand[agent]
}

// NewOptions는 wt new의 입력.
type NewOptions struct {
	Task     string
	Repo     string // repo 안 아무 경로나 — RepoRoot로 정규화
	Base     string // 비면 현재 HEAD 브랜치
	Agent    string // claude(기본) | codex | gemini
	TestCmd  string
	Tmux     tmuxx.Tmux
	NoWindow bool // tmux window 생성 생략 (테스트·수동 모드)
}

// New는 태스크 하나를 시작한다:
// 메타 기록 → worktree+branch 생성 → tmux window(-c worktree) → 에이전트 실행.
// 중간 실패 시 만든 것을 되돌린다 (기록이 먼저라 잔해 추적이 항상 가능).
func New(stateDir string, o NewOptions) (*Meta, error) {
	if o.Task == "" {
		return nil, fmt.Errorf("태스크 이름이 필요합니다")
	}
	if o.Agent == "" {
		o.Agent = "claude"
	}
	if _, ok := agentCommand[o.Agent]; !ok {
		return nil, fmt.Errorf("지원하지 않는 에이전트: %s", o.Agent)
	}
	repo, err := RepoRoot(o.Repo)
	if err != nil {
		return nil, fmt.Errorf("git 저장소가 아닙니다: %s", o.Repo)
	}
	base := o.Base
	if base == "" {
		if base, err = git(repo, "branch", "--show-current"); err != nil || base == "" {
			return nil, fmt.Errorf("base 브랜치를 알 수 없습니다 — --base로 지정하세요")
		}
	}
	branch := "agent/" + o.Task
	if BranchExists(repo, branch) {
		return nil, fmt.Errorf("브랜치 %s가 이미 있습니다 — 다른 태스크 이름을 쓰세요", branch)
	}
	if _, err := LoadMeta(stateDir, o.Task); err == nil {
		return nil, fmt.Errorf("태스크 %q가 이미 있습니다", o.Task)
	}
	path := filepath.Join(repo, ".agentlayer", "worktrees", o.Task)

	m := &Meta{Task: o.Task, Repo: repo, Base: base, Branch: branch, Path: path,
		Agent: o.Agent, TestCmd: o.TestCmd, CreatedAt: time.Now()}
	// 기록이 파일시스템 조작보다 먼저다.
	if err := SaveMeta(stateDir, m); err != nil {
		return nil, err
	}
	if err := AddWorktree(repo, base, branch, path); err != nil {
		_ = DeleteMeta(stateDir, o.Task)
		return nil, err
	}
	if !o.NoWindow {
		if err := openWindow(o.Tmux, o.Task, path, commandFor(o.Agent)); err != nil {
			// worktree는 남긴다 — 사용자가 수동으로 쓸 수 있고, 잔해는 wt list에 보인다
			return m, fmt.Errorf("worktree는 만들었지만 tmux window 생성 실패: %w", err)
		}
	}
	return m, nil
}

// openWindow는 현재 세션에 태스크 window를 만들고 에이전트를 실행한다.
func openWindow(tm tmuxx.Tmux, task, path, command string) error {
	if !tmuxx.InsideTmux() {
		return fmt.Errorf("tmux 밖입니다 — tmux 안에서 실행하세요")
	}
	return tm.NewWindow(task, path, command)
}

// CleanRefusal은 정리를 거부한 이유.
type CleanRefusal struct {
	Dirty    []string
	Unmerged int
}

func (r *CleanRefusal) Error() string {
	var parts []string
	if len(r.Dirty) > 0 {
		parts = append(parts, fmt.Sprintf("미커밋·untracked %d건", len(r.Dirty)))
	}
	if r.Unmerged > 0 {
		parts = append(parts, fmt.Sprintf("미병합 커밋 %d개", r.Unmerged))
	}
	return "정리 거부: " + strings.Join(parts, ", ") + " — 커밋·병합 후 다시 시도하세요"
}

// Clean은 보존 우선 정리: 미커밋·미병합이 하나라도 있으면 CleanRefusal을
// 돌려주고 아무것도 지우지 않는다.
func Clean(stateDir, task string) error {
	m, err := LoadMeta(stateDir, task)
	if err != nil {
		return err
	}
	refusal := &CleanRefusal{}
	if d, err := Dirty(m.Path); err == nil {
		refusal.Dirty = d
	}
	if n, err := Unmerged(m.Repo, m.Base, m.Branch); err == nil {
		refusal.Unmerged = n
	}
	if len(refusal.Dirty) > 0 || refusal.Unmerged > 0 {
		return refusal
	}
	if err := RemoveWorktree(m.Repo, m.Path); err != nil {
		return err
	}
	if err := DeleteBranch(m.Repo, m.Branch); err != nil {
		return err
	}
	return DeleteMeta(stateDir, task)
}

// MergeGuide는 merge 전 검사 요약과 실행할 명령을 보여주고,
// confirm이 "y"를 돌려줄 때만 실제 merge를 실행한다. 자동 merge는 없다.
func MergeGuide(w io.Writer, stateDir, task string, confirm func() bool) error {
	m, err := LoadMeta(stateDir, task)
	if err != nil {
		return err
	}
	dirty, _ := Dirty(m.Path)
	unmerged, _ := Unmerged(m.Repo, m.Base, m.Branch)

	fmt.Fprintf(w, "태스크 %s — %s ← %s\n", m.Task, m.Base, m.Branch)
	if m.TestPass != nil {
		if *m.TestPass {
			fmt.Fprintf(w, "  테스트: ✔ 통과 (%s)\n", m.TestAt.Format("15:04"))
		} else {
			fmt.Fprintf(w, "  테스트: ✖ 실패 (%s) — merge 전 확인 필요\n", m.TestAt.Format("15:04"))
		}
	} else {
		fmt.Fprintln(w, "  테스트: 실행 안 함 (wt test로 실행 가능)")
	}
	if len(dirty) > 0 {
		fmt.Fprintf(w, "  ⚠ worktree에 미커밋 %d건 — merge 대상에서 빠집니다:\n", len(dirty))
		for _, d := range dirty {
			fmt.Fprintln(w, "    ", d)
		}
	}
	if unmerged == 0 {
		fmt.Fprintln(w, "  병합할 커밋이 없습니다.")
		return nil
	}
	fmt.Fprintf(w, "  병합할 커밋: %d개\n\n실행할 명령:\n", unmerged)
	fmt.Fprintf(w, "  git -C %s checkout %s\n", m.Repo, m.Base)
	fmt.Fprintf(w, "  git -C %s merge --no-ff %s\n\n", m.Repo, m.Branch)
	fmt.Fprint(w, "진행할까요? [y/N] ")
	if !confirm() {
		fmt.Fprintln(w, "취소했습니다.")
		return nil
	}
	if err := Merge(m.Repo, m.Base, m.Branch); err != nil {
		return fmt.Errorf("merge 실패 — 저장소에서 직접 해결하세요: %w", err)
	}
	fmt.Fprintln(w, "병합 완료. 정리는 wt clean으로.")
	return nil
}
