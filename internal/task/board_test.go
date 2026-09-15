// internal/task/board_test.go
package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

// linkedAgent는 회사 tasks/LAB-1에 등록된 에이전트를 하나 만든다. ask가 ""면 Ask 없는(=idle
// echo 같은) 전이를 재현하고, 아니면 그 문구를 Ask로 채운다.
func linkedAgent(t *testing.T, ask string) (stateDir, root string, a *state.Agent) {
	t.Helper()
	stateDir, root = t.TempDir(), t.TempDir()
	dir := filepath.Join(root, "tasks", "LAB-1")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "task.md"), []byte("# LAB-1\n```yaml\nstatus: in_progress\n```\n"), 0o644)
	a = &state.Agent{ID: "claude-%1", Kind: "claude", Task: "OK 답하기", Ask: ask,
		Tmux: state.TmuxRef{Session: "collab-bot", WindowName: "t123456", PaneID: "%1"}}
	if err := Assign(stateDir, Assignment{TaskID: "LAB-1", AgentID: a.ID, Session: "collab-bot", Window: "t123456",
		Pane: "%1", Inbox: filepath.Join(root, "runtime", "inbox"), TaskDir: dir, AssignedAt: time.Now()}, false); err != nil {
		t.Fatal(err)
	}
	return
}

func readTask(t *testing.T, root string) (string, string) {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "LAB-1", "task.md"))
	l, _ := os.ReadFile(filepath.Join(root, "tasks", "LAB-1", "log.md"))
	return string(b), string(l)
}

func TestApplyTransitionTable(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		prev, to    state.AgentState
		wantStatus  string // "" = 변경 없음(in_progress 유지)
		wantLog     string // "" = 기록 없음
		wantApplied bool
	}{
		{state.StateWorking, state.StateWaiting, "waiting_collab-bot", "[ASK] 폴더 밖 읽어도 될까요?", true},
		{state.StateWaiting, state.StateWorking, "in_progress", "", true},
		{state.StateIdle, state.StateWorking, "in_progress", "", true},
		{state.StateWorking, state.StateDoneUnread, "reviewing", "[REPORT] DONE: 폴더 밖 읽어도 될까요?", true},
		{state.StateWorking, state.StateError, "", "[ERROR] 폴더 밖 읽어도 될까요?", true},
		{state.StateWorking, state.StateWorking, "", "", false}, // heartbeat
		{state.StateDoneUnread, state.StateIdle, "", "", false}, // 읽음
	}
	for _, c := range cases {
		stateDir, root, a := linkedAgent(t, "폴더 밖 읽어도 될까요?")
		applied, err := ApplyTransition(stateDir, a, c.prev, c.to, now)
		if err != nil {
			t.Fatalf("%s→%s: %v", c.prev, c.to, err)
		}
		if applied != c.wantApplied {
			t.Errorf("%s→%s applied=%v want %v", c.prev, c.to, applied, c.wantApplied)
		}
		task, lg := readTask(t, root)
		wantStatus := c.wantStatus
		if wantStatus == "" {
			wantStatus = "in_progress"
		}
		if !strings.Contains(task, "status: "+wantStatus) {
			t.Errorf("%s→%s status:\n%s", c.prev, c.to, task)
		}
		if c.wantLog == "" && lg != "" {
			t.Errorf("%s→%s 기록이 있으면 안 됨: %s", c.prev, c.to, lg)
		}
		if c.wantLog != "" && !strings.Contains(lg, c.wantLog) {
			t.Errorf("%s→%s log:\n%s", c.prev, c.to, lg)
		}
	}
}

func TestApplyTransitionWaitWithoutAskLogsIdleEcho(t *testing.T) {
	stateDir, root, a := linkedAgent(t, "")
	applied, err := ApplyTransition(stateDir, a, state.StateWorking, state.StateWaiting, time.Now())
	if err != nil || !applied {
		t.Fatalf("applied=%v err=%v", applied, err)
	}
	task, lg := readTask(t, root)
	if !strings.Contains(task, "status: waiting_collab-bot") {
		t.Errorf("status:\n%s", task)
	}
	if !strings.Contains(lg, "[ASK] 입력 대기(승인창 아님)") {
		t.Errorf("log:\n%s", lg)
	}
}

func TestApplyTransitionUnlinkedIsNoop(t *testing.T) {
	stateDir, root, a := linkedAgent(t, "폴더 밖 읽어도 될까요?")
	as, _, _ := Load(stateDir, a.ID)
	as.TaskDir = ""
	_ = Assign(stateDir, *as, true)
	applied, err := ApplyTransition(stateDir, a, state.StateWorking, state.StateDoneUnread, time.Now())
	if applied || err != nil {
		t.Errorf("TaskDir 없으면 무동작: applied=%v err=%v", applied, err)
	}
	if _, lg := readTask(t, root); lg != "" {
		t.Error("기록이 있으면 안 됨")
	}
	other := &state.Agent{ID: "codex-%9", Tmux: state.TmuxRef{Session: "x", PaneID: "%9"}}
	if applied, _ := ApplyTransition(stateDir, other, state.StateWorking, state.StateDoneUnread, time.Now()); applied {
		t.Error("미등록 에이전트는 무동작")
	}
}

func TestApplyTransitionStaleAssignmentIsNoop(t *testing.T) {
	stateDir, _, a := linkedAgent(t, "폴더 밖 읽어도 될까요?")
	a.Tmux.PaneID = "%77" // 재사용된 ID — 등록 당시 pane과 다름
	if applied, _ := ApplyTransition(stateDir, a, state.StateWorking, state.StateDoneUnread, time.Now()); applied {
		t.Error("세션·pane 불일치면 무동작")
	}
}

func writeCompanyTask(t *testing.T, root, id, status, parents string) {
	t.Helper()
	dir := filepath.Join(root, "tasks", id)
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "task.md"),
		[]byte("# "+id+" 제목\n```yaml\nstatus: "+status+"\nparents: "+parents+"\n```\n"), 0o644)
}

func TestMarkDoneEmitsReadyOnlyWhenAllParentsDone(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "runtime", "inbox")
	writeCompanyTask(t, root, "A", "reviewing", "[]")
	writeCompanyTask(t, root, "B", "in_progress", "[]")
	writeCompanyTask(t, root, "C", "pending", "[A]")     // A만 부모 → READY
	writeCompanyTask(t, root, "D", "pending", "[A, B]")  // B 미완 → 안 씀
	writeCompanyTask(t, root, "E", "in_progress", "[A]") // 이미 진행 중 → 안 씀
	now := time.Now()
	ready, err := MarkDone(root, "A", inbox, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 || ready[0] != "C" {
		t.Errorf("ready = %v, want [C]", ready)
	}
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "A", "task.md"))
	if !strings.Contains(string(b), "status: done") {
		t.Errorf("A status:\n%s", b)
	}
	lg, _ := os.ReadFile(filepath.Join(root, "tasks", "A", "log.md"))
	if !strings.Contains(string(lg), "[COMPLETE]") {
		t.Errorf("A log:\n%s", lg)
	}
	rep, ok, err := Poll(inbox)
	if err != nil || !ok {
		t.Fatalf("READY 이벤트 없음: ok=%v err=%v", ok, err)
	}
	if rep.Kind != "board" || rep.To != "READY" || rep.From != "pending" || rep.TaskID != "C" ||
		rep.Task != "C 제목" || rep.TaskDir != filepath.Join(root, "tasks", "C") {
		t.Errorf("rep = %+v", rep)
	}
	if _, ok, _ := Poll(inbox); ok {
		t.Error("이벤트는 하나여야 함")
	}
	// B까지 끝나면 D가 READY
	ready, _ = MarkDone(root, "B", inbox, now)
	if len(ready) != 1 || ready[0] != "D" {
		t.Errorf("ready = %v, want [D]", ready)
	}
}

func TestMarkDoneWithoutTaskFileFails(t *testing.T) {
	if _, err := MarkDone(t.TempDir(), "NOPE", "", time.Now()); err == nil {
		t.Error("task.md 없으면 에러")
	}
}

// 이미 done인 업무에 MarkDone을 다시 걸어도(재시도·중복 호출) [COMPLETE]가 두 번 쌓이거나
// 이미 배정 가능해진 자식에게 READY가 다시 나가면 안 된다.
func TestMarkDoneIsIdempotent(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "runtime", "inbox")
	writeCompanyTask(t, root, "A", "reviewing", "[]")
	writeCompanyTask(t, root, "C", "pending", "[A]")
	now := time.Now()
	if _, err := MarkDone(root, "A", inbox, now); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := Poll(inbox); err != nil || !ok {
		t.Fatalf("첫 MarkDone은 READY를 내야 함: ok=%v err=%v", ok, err)
	}
	ready, err := MarkDone(root, "A", inbox, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 0 {
		t.Errorf("이미 done이면 ready는 비어야 함: %v", ready)
	}
	lg, _ := os.ReadFile(filepath.Join(root, "tasks", "A", "log.md"))
	if strings.Count(string(lg), "[COMPLETE]") != 1 {
		t.Errorf("[COMPLETE]는 한 번만 기록돼야 함:\n%s", lg)
	}
	if _, ok, _ := Poll(inbox); ok {
		t.Error("두 번째 MarkDone은 READY를 다시 쓰면 안 됨")
	}
}
