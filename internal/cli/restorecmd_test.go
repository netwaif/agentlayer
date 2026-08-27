package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
)

var rt0 = time.Date(2026, 8, 27, 12, 0, 0, 0, time.FixedZone("KST", 9*3600))

func deadAgent(id, kind, session string, window int, cwd string) *state.Agent {
	return &state.Agent{ID: id, Kind: kind, State: state.StateDead,
		Tmux: state.TmuxRef{Session: session, Window: window, PaneID: "%9"},
		CWD:  cwd, SessionID: "sid-" + id, StateSince: rt0, UpdatedAt: rt0}
}

// 죽은 레코드는 세션이 없으면 새 세션으로, 같은 세션의 다음 레코드는
// 새 window로 계획된다.
func TestPlanRestoreGroupsBySession(t *testing.T) {
	dir := t.TempDir()
	agents := []*state.Agent{
		deadAgent("claude-1", "claude", "ai", 0, dir),
		deadAgent("claude-2", "claude", "ai", 1, dir),
	}
	plan := PlanRestore(agents, func(string) bool { return false }, false)
	if len(plan.Items) != 2 {
		t.Fatalf("계획 %d건, 2건 기대: %+v", len(plan.Items), plan)
	}
	if !plan.Items[0].NewSession {
		t.Error("첫 항목은 새 세션이어야 함")
	}
	if plan.Items[1].NewSession {
		t.Error("같은 세션의 둘째 항목은 새 window여야 함")
	}
	if plan.Items[0].Cmd != "claude" {
		t.Errorf("기본은 새 CLI 기동: %q", plan.Items[0].Cmd)
	}
}

// 이미 살아 있는 tmux 세션에는 세션을 새로 만들지 않고 window만 추가한다.
func TestPlanRestoreExistingSessionGetsWindow(t *testing.T) {
	dir := t.TempDir()
	agents := []*state.Agent{deadAgent("claude-1", "claude", "ai", 0, dir)}
	plan := PlanRestore(agents, func(name string) bool { return name == "ai" }, false)
	if len(plan.Items) != 1 || plan.Items[0].NewSession {
		t.Fatalf("기존 세션엔 window 추가: %+v", plan)
	}
}

// 죽지 않은 레코드는 복원 대상이 아니다.
func TestPlanRestoreSkipsAlive(t *testing.T) {
	dir := t.TempDir()
	a := deadAgent("claude-1", "claude", "ai", 0, dir)
	a.State = state.StateWorking
	plan := PlanRestore([]*state.Agent{a}, func(string) bool { return false }, false)
	if len(plan.Items) != 0 {
		t.Fatalf("살아 있는 레코드는 제외: %+v", plan)
	}
}

// CWD가 사라진 레코드는 건너뛰고 사유를 남긴다.
func TestPlanRestoreSkipsMissingCWD(t *testing.T) {
	agents := []*state.Agent{deadAgent("claude-1", "claude", "ai", 0, "/no/such/dir-xyz")}
	plan := PlanRestore(agents, func(string) bool { return false }, false)
	if len(plan.Items) != 0 {
		t.Fatalf("사라진 폴더는 제외: %+v", plan)
	}
	if len(plan.Skipped) != 1 || !strings.Contains(plan.Skipped[0], "claude-1") {
		t.Fatalf("건너뛴 사유 기록: %v", plan.Skipped)
	}
}

// 같은 (세션, window)의 중복 레코드(분할 pane 등)는 하나만 계획한다.
func TestPlanRestoreDedupsWindow(t *testing.T) {
	dir := t.TempDir()
	agents := []*state.Agent{
		deadAgent("claude-1", "claude", "ai", 0, dir),
		deadAgent("claude-2", "claude", "ai", 0, dir),
	}
	plan := PlanRestore(agents, func(string) bool { return false }, false)
	if len(plan.Items) != 1 {
		t.Fatalf("중복 window는 1건만: %+v", plan.Items)
	}
}

// 통합: 복원에 성공하면 원본 죽은 레코드를 지운다 — status에 옛 dead 행이
// 새 행과 나란히 남아 헷갈리지 않게 (실사용 피드백).
func TestRunRestoreRemovesDeadRecord(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux 없음")
	}
	sock := fmt.Sprintf("al-restore-%d", os.Getpid())
	tm := tmuxx.Tmux{Args: []string{"-L", sock}}
	t.Cleanup(func() { exec.Command("tmux", "-L", sock, "kill-server").Run() })
	st, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := deadAgent("claude-1", "claude", "lab", 0, t.TempDir())
	if err := st.Save(a); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := RunRestore(&buf, st, tm, nil); err != nil {
		t.Fatal(err)
	}
	if !tm.HasSession("lab") {
		t.Error("세션이 생성돼야 함")
	}
	if _, err := st.Load("claude-1"); err == nil {
		t.Error("복원된 원본 죽은 레코드는 삭제돼야 함")
	}
}

// --resume이면 대화 부활 명령(claude --resume <sid>)을 쓴다.
func TestPlanRestoreResume(t *testing.T) {
	dir := t.TempDir()
	agents := []*state.Agent{deadAgent("claude-1", "claude", "ai", 0, dir)}
	plan := PlanRestore(agents, func(string) bool { return false }, true)
	if len(plan.Items) != 1 || plan.Items[0].Cmd != "claude --resume sid-claude-1" {
		t.Fatalf("resume 명령 기대: %+v", plan.Items)
	}
}

// resume 불가(세션 ID 없음)면 새 기동으로 폴백하고 사유를 남긴다.
func TestPlanRestoreResumeFallback(t *testing.T) {
	dir := t.TempDir()
	a := deadAgent("claude-1", "claude", "ai", 0, dir)
	a.SessionID = ""
	plan := PlanRestore([]*state.Agent{a}, func(string) bool { return false }, true)
	if len(plan.Items) != 1 || plan.Items[0].Cmd != "claude" {
		t.Fatalf("resume 불가 시 새 기동 폴백: %+v", plan.Items)
	}
	if len(plan.Skipped) != 1 {
		t.Fatalf("폴백 사유 기록: %v", plan.Skipped)
	}
}
