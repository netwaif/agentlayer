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
	plan := PlanRestore(agents, envExists(func(string) bool { return false }), RestoreOpts{Resume: false})
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
	plan := PlanRestore(agents, envExists(func(name string) bool { return name == "ai" }), RestoreOpts{Resume: false})
	if len(plan.Items) != 1 || plan.Items[0].NewSession {
		t.Fatalf("기존 세션엔 window 추가: %+v", plan)
	}
}

// 죽지 않은 레코드는 복원 대상이 아니다.
func TestPlanRestoreSkipsAlive(t *testing.T) {
	dir := t.TempDir()
	a := deadAgent("claude-1", "claude", "ai", 0, dir)
	a.State = state.StateWorking
	plan := PlanRestore([]*state.Agent{a}, envExists(func(string) bool { return false }), RestoreOpts{Resume: false})
	if len(plan.Items) != 0 {
		t.Fatalf("살아 있는 레코드는 제외: %+v", plan)
	}
}

// CWD가 사라진 레코드는 건너뛰고 사유를 남긴다.
func TestPlanRestoreSkipsMissingCWD(t *testing.T) {
	agents := []*state.Agent{deadAgent("claude-1", "claude", "ai", 0, "/no/such/dir-xyz")}
	plan := PlanRestore(agents, envExists(func(string) bool { return false }), RestoreOpts{Resume: false})
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
	plan := PlanRestore(agents, envExists(func(string) bool { return false }), RestoreOpts{Resume: false})
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
	tm := tmuxx.Tmux{Args: []string{"-f", "/dev/null", "-L", sock}}
	t.Cleanup(func() { exec.Command("tmux", "-f", "/dev/null", "-L", sock, "kill-server").Run() })
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

// restore <id> 선택: 지정한 죽은 레코드만 통과하고, 없는 ID·살아 있는 ID는
// 사유를 남긴다 — 명시 지정을 조용히 거르면 사용자가 원인을 모른다.
func TestFilterByIDs(t *testing.T) {
	dir := t.TempDir()
	dead := deadAgent("claude-1", "claude", "ai", 0, dir)
	alive := deadAgent("claude-2", "claude", "ai", 1, dir)
	alive.State = state.StateWorking
	agents := []*state.Agent{dead, alive}

	picked, skipped := FilterByIDs(agents, []string{"claude-1", "claude-2", "claude-9"})
	if len(picked) != 1 || picked[0].ID != "claude-1" {
		t.Fatalf("죽은 claude-1만 통과해야 함: %+v", picked)
	}
	if len(skipped) != 2 {
		t.Fatalf("사유 2건 기대: %v", skipped)
	}
	if !strings.Contains(skipped[0], "claude-2") || !strings.Contains(skipped[0], "살아") {
		t.Errorf("살아 있는 ID 사유: %v", skipped[0])
	}
	if !strings.Contains(skipped[1], "claude-9") {
		t.Errorf("없는 ID 사유: %v", skipped[1])
	}
}

// restore <id>는 지정한 레코드만 계획에 올린다 (dry-run으로 검증).
func TestRunRestoreSelectsIDs(t *testing.T) {
	st, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, a := range []*state.Agent{
		deadAgent("claude-1", "claude", "one", 0, dir),
		deadAgent("claude-2", "claude", "two", 0, dir),
	} {
		if err := st.Save(a); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	tm := tmuxx.Tmux{Args: []string{"-f", "/dev/null", "-L", fmt.Sprintf("al-sel-%d", os.Getpid())}}
	if err := RunRestore(&buf, st, tm, []string{"--dry-run", "claude-2"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "two") || strings.Contains(out, "one") {
		t.Fatalf("claude-2(two)만 계획돼야 함:\n%s", out)
	}
}

// --resume이면 대화 부활 명령(claude --resume <sid>)을 쓴다.
func TestPlanRestoreResume(t *testing.T) {
	dir := t.TempDir()
	agents := []*state.Agent{deadAgent("claude-1", "claude", "ai", 0, dir)}
	plan := PlanRestore(agents, envExists(func(string) bool { return false }), RestoreOpts{Resume: true})
	if len(plan.Items) != 1 || plan.Items[0].Cmd != "claude --resume sid-claude-1" {
		t.Fatalf("resume 명령 기대: %+v", plan.Items)
	}
}

// resume 불가(세션 ID 없음)면 새 기동으로 폴백하고 사유를 남긴다.
func TestPlanRestoreResumeFallback(t *testing.T) {
	dir := t.TempDir()
	a := deadAgent("claude-1", "claude", "ai", 0, dir)
	a.SessionID = ""
	plan := PlanRestore([]*state.Agent{a}, envExists(func(string) bool { return false }), RestoreOpts{Resume: true})
	if len(plan.Items) != 1 || plan.Items[0].Cmd != "claude" {
		t.Fatalf("resume 불가 시 새 기동 폴백: %+v", plan.Items)
	}
	if len(plan.Skipped) != 1 {
		t.Fatalf("폴백 사유 기록: %v", plan.Skipped)
	}
}

// envExists는 세션 존재 여부만 주입하는 테스트용 환경.
func envExists(f func(string) bool) RestoreEnv {
	return RestoreEnv{SessionExists: f}
}

// 같은 세션·같은 폴더에 이미 pane이 있으면(밖에서 기동됐거나 기동 중) 복원하지
// 않는다 — 봇이 bot-up.sh 락 대기 중이라 명령이 아직 bash인 동안 restore가
// 돌면 옛 DEAD 레코드로 같은 세션에 window를 하나 더 만들어 봇이 두 개씩
// 떴다(2026-09-03 재부팅 실측).
func TestPlanRestoreSkipsOccupiedSlot(t *testing.T) {
	dir := t.TempDir()
	agents := []*state.Agent{deadAgent("claude-1", "claude", "claude-discord", 0, dir)}
	env := RestoreEnv{
		SessionExists: func(string) bool { return true },
		PaneAt:        func(session, cwd string) bool { return session == "claude-discord" && cwd == dir },
	}
	plan := PlanRestore(agents, env, RestoreOpts{})
	if len(plan.Items) != 0 {
		t.Fatalf("같은 자리에 pane이 있으면 복원 제외: %+v", plan.Items)
	}
	if len(plan.Skipped) != 1 || !strings.Contains(plan.Skipped[0], "같은 자리") {
		t.Fatalf("건너뛴 사유 기록: %v", plan.Skipped)
	}
	// ID 명시로도 물리 충돌은 못 넘는다
	plan = PlanRestore(agents, env, RestoreOpts{Explicit: true})
	if len(plan.Items) != 0 {
		t.Fatalf("명시 지정이어도 같은 자리 pane은 제외: %+v", plan.Items)
	}
}

// tmux 세션을 띄우는 LaunchAgent가 있는 세션(봇)은 부팅 시 launchd가 살리므로
// restore 대상이 아니다 — restore가 먼저 세션을 만들면 launchd의 new-session이
// duplicate로 실패해 봇이 영영 안 뜬다. ID를 명시하면 강제 가능.
func TestPlanRestoreSkipsLaunchAgentSession(t *testing.T) {
	dir := t.TempDir()
	agents := []*state.Agent{deadAgent("claude-1", "claude", "collab-bot", 0, dir)}
	env := RestoreEnv{
		SessionExists: func(string) bool { return false },
		LaunchAgents: func(session string) []string {
			if session == "collab-bot" {
				return []string{"com.folder-bot.collab"}
			}
			return nil
		},
	}
	plan := PlanRestore(agents, env, RestoreOpts{})
	if len(plan.Items) != 0 {
		t.Fatalf("LaunchAgent 관할 세션은 복원 제외: %+v", plan.Items)
	}
	if len(plan.Skipped) != 1 || !strings.Contains(plan.Skipped[0], "com.folder-bot.collab") {
		t.Fatalf("사유에 plist 라벨: %v", plan.Skipped)
	}
	plan = PlanRestore(agents, env, RestoreOpts{Explicit: true})
	if len(plan.Items) != 1 {
		t.Fatalf("ID 명시면 LaunchAgent 세션도 복원: %+v", plan)
	}
}

// dry-run 출력에 레코드 ID가 보여야 `restore <id>`로 골라 살릴 수 있다 —
// 이전엔 세션·폴더만 찍혀 전부 아니면 전무로 보였다(실사용 피드백).
func TestRunRestoreDryRunShowsIDsAndHint(t *testing.T) {
	st, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, a := range []*state.Agent{
		deadAgent("claude-1", "claude", "one", 0, dir),
		deadAgent("codex-2", "codex", "two", 0, dir),
	} {
		if err := st.Save(a); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	tm := tmuxx.Tmux{Args: []string{"-f", "/dev/null", "-L", fmt.Sprintf("al-dry-%d", os.Getpid())}}
	if err := RunRestore(&buf, st, tm, []string{"--dry-run"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"[claude-1]", "[codex-2]", "agentlayer restore <id>"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run 출력에 %q 기대:\n%s", want, out)
		}
	}
}

// 통합: 세션에 같은 폴더의 pane(셸)이 이미 있으면 window를 더 만들지 않고
// 레코드도 남긴다(에이전트가 뜨면 Sync가 정리).
func TestRunRestoreSkipsOccupiedPane(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux 없음")
	}
	sock := fmt.Sprintf("al-occ-%d", os.Getpid())
	tm := tmuxx.Tmux{Args: []string{"-f", "/dev/null", "-L", sock}}
	t.Cleanup(func() { exec.Command("tmux", "-f", "/dev/null", "-L", sock, "kill-server").Run() })
	dir := t.TempDir()
	if _, err := tm.NewSession("lab", dir); err != nil {
		t.Fatal(err)
	}
	st, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(deadAgent("claude-1", "claude", "lab", 0, dir)); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := RunRestore(&buf, st, tm, nil); err != nil {
		t.Fatal(err)
	}
	panes, err := tm.ListPanes()
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 1 {
		t.Fatalf("window가 늘면 안 됨 (pane %d개):\n%s", len(panes), buf.String())
	}
	if _, err := st.Load("claude-1"); err != nil {
		t.Error("건너뛴 레코드는 남아야 함 (Sync가 정리)")
	}
}
