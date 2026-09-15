package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

func TestRunTaskAssignListDone(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.NewStore(dir)
	a := mkAgent("claude", "search-youtube-bot", "%16", state.StateIdle)
	a.Tmux.WindowName = "t170966"
	_ = st.Save(a)
	inbox := filepath.Join(t.TempDir(), "inbox")
	now := time.Now()
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, dir, []string{"assign", "T-1", "search-youtube-bot:t170966", "--inbox", inbox}, now); err != nil {
		t.Fatal(err)
	}
	as, ok, _ := task.Load(dir, a.ID)
	if !ok || as.TaskID != "T-1" || as.Pane != "%16" || as.Window != "t170966" {
		t.Fatalf("등록: %+v ok=%v", as, ok)
	}
	if abs, _ := filepath.Abs(inbox); as.Inbox != abs {
		t.Errorf("inbox는 절대경로로 저장: %s", as.Inbox)
	}
	if err := RunTask(context.Background(), &out, st, dir, []string{"assign", "T-2", "search-youtube-bot:t170966", "--inbox", inbox}, now); err == nil {
		t.Error("중복 배정은 오류")
	}
	if err := RunTask(context.Background(), &out, st, dir, []string{"assign", "T-2", "search-youtube-bot:t170966", "--inbox", inbox, "--replace"}, now); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := RunTask(context.Background(), &out, st, dir, []string{"list"}, now); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "T-2") || !strings.Contains(out.String(), "search-youtube-bot") || !strings.Contains(out.String(), "idle") {
		t.Errorf("list 출력: %s", out.String())
	}
	out.Reset()
	if err := RunTask(context.Background(), &out, st, dir, []string{"list", "--json"}, now); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0]["task_id"] != "T-2" || rows[0]["state"] != "IDLE" {
		t.Errorf("list --json: %s err=%v", out.String(), err)
	}
	out.Reset()
	if err := RunTask(context.Background(), &out, st, dir, []string{"done", "T-2"}, now); err != nil {
		t.Fatal(err)
	}
	if err := RunTask(context.Background(), &out, st, dir, []string{"done", "T-2"}, now); err == nil {
		t.Error("없는 업무 done은 오류")
	}
	if err := RunTask(context.Background(), &out, st, dir, []string{"assign", "T-3", "search-youtube-bot:t170966"}, now); err == nil {
		t.Error("--inbox 없으면 오류")
	}
	if err := RunTask(context.Background(), &out, st, dir, []string{"nope"}, now); err == nil {
		t.Error("모르는 서브명령은 오류")
	}
}

// dead 세션에는 업무를 배정할 수 없다 — 받을 곳이 없는 등록을 만들지 않는다.
func TestRunTaskAssignRejectsDeadSession(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.NewStore(dir)
	_ = st.Save(mkAgent("claude", "dead-bot", "%4", state.StateDead))
	inbox := filepath.Join(t.TempDir(), "inbox")
	var out bytes.Buffer
	err := RunTask(context.Background(), &out, st, dir, []string{"assign", "T-1", "dead-bot", "--inbox", inbox}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "죽었습니다") {
		t.Fatalf("dead 세션 배정은 거부돼야 함: %v", err)
	}
	if _, ok, _ := task.Load(dir, "claude-%4"); ok {
		t.Error("거부됐으면 등록 파일이 생기면 안 됨")
	}
}

// 에이전트 레코드는 있지만(ID 재사용) 세션·pane이 등록 당시와 다르면 list가
// 그 에이전트의 실제 상태 대신 "stale"을 보여야 한다 — 엉뚱한 세션으로 오인 방지.
func TestRunTaskListMarksStaleWhenSessionPaneChanged(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.NewStore(dir)
	a := mkAgent("claude", "old-bot", "%16", state.StateIdle)
	_ = st.Save(a)
	inbox := filepath.Join(t.TempDir(), "inbox")
	now := time.Now()
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, dir, []string{"assign", "T-1", "old-bot", "--inbox", inbox}, now); err != nil {
		t.Fatal(err)
	}
	// 같은 ID의 레코드가 새 세션으로 바뀜(재시작 등)
	a.Tmux.Session = "new-bot"
	_ = st.Save(a)
	out.Reset()
	if err := RunTask(context.Background(), &out, st, dir, []string{"list", "--json"}, now); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0]["state"] != "stale" {
		t.Errorf("list --json state=stale 기대: %s err=%v", out.String(), err)
	}
	out.Reset()
	if err := RunTask(context.Background(), &out, st, dir, []string{"list"}, now); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "stale") {
		t.Errorf("표 출력에 stale 표시: %s", out.String())
	}
}

func TestRunTaskWatchOncePrintsJSONLine(t *testing.T) {
	inbox := t.TempDir()
	id := strings.Repeat("f", 32)
	b, _ := json.Marshal(task.Report{Version: 1, ID: id, TaskID: "T-9", Session: "s", Kind: "claude",
		From: "WORKING", To: "DONE_UNREAD", At: time.Now()})
	os.MkdirAll(filepath.Join(inbox, "pending"), 0o700)
	os.WriteFile(filepath.Join(inbox, "pending", id+".json"), b, 0o600)
	var out bytes.Buffer
	st, _ := state.NewStore(t.TempDir())
	if err := RunTask(context.Background(), &out, st, t.TempDir(), []string{"watch", inbox, "--once"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(out.String())
	if strings.Count(line, "\n") != 0 || !strings.Contains(line, `"task_id":"T-9"`) || !strings.Contains(line, `"to":"DONE_UNREAD"`) {
		t.Errorf("한 줄 JSON: %q", line)
	}
}

func companyRoot(t *testing.T, id string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "tasks", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# " + id + "\n```yaml\nstatus: pending\nupdated: 2026-01-01\nparents: []\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "runtime", "inbox"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestTaskAssignLinksTaskDirAndMarksInProgress(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}})
	root := companyRoot(t, "LAB-1")
	var out bytes.Buffer
	err := RunTask(context.Background(), &out, st, stateDir,
		[]string{"assign", "LAB-1", "collab-bot", "--inbox", filepath.Join(root, "runtime", "inbox")}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	as, ok, _ := task.Load(stateDir, "claude-%1")
	if !ok || as.TaskDir != filepath.Join(root, "tasks", "LAB-1") {
		t.Errorf("TaskDir = %q", as.TaskDir)
	}
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "LAB-1", "task.md"))
	if !strings.Contains(string(b), "status: in_progress") {
		t.Errorf("status 미전이:\n%s", b)
	}
	lg, _ := os.ReadFile(filepath.Join(root, "tasks", "LAB-1", "log.md"))
	if !strings.Contains(string(lg), "[ASSIGN] collab-bot") {
		t.Errorf("log 미기록: %s", lg)
	}
}

func TestTaskAssignWithoutTaskFileWarnsButRegisters(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}})
	var out bytes.Buffer
	err := RunTask(context.Background(), &out, st, stateDir,
		[]string{"assign", "NOFILE", "collab-bot", "--inbox", t.TempDir()}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	as, ok, _ := task.Load(stateDir, "claude-%1")
	if !ok || as.TaskDir != "" {
		t.Errorf("TaskDir는 비어야 함: %q", as.TaskDir)
	}
	if !strings.Contains(out.String(), "보드에 표시되지 않음") {
		t.Errorf("경고 문구 없음: %s", out.String())
	}
}

func TestTaskAssignExplicitRoot(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}})
	root := companyRoot(t, "LAB-1")
	var out bytes.Buffer
	err := RunTask(context.Background(), &out, st, stateDir,
		[]string{"assign", "LAB-1", "collab-bot", "--inbox", t.TempDir(), "--root", root}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	as, _, _ := task.Load(stateDir, "claude-%1")
	if as.TaskDir != filepath.Join(root, "tasks", "LAB-1") {
		t.Errorf("--root 무시됨: %q", as.TaskDir)
	}
}

// [ASSIGN] 로그는 "세션[:창]" — 봇 스레드 창일 때 SessionLabel의 "(스레드 N)" 배지가 아니라
// task list와 같은 <세션[:창]> 문구를 써야 한다(설계 1.2).
func TestTaskAssignLogIncludesThreadWindow(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", WindowName: "t123456", PaneID: "%1"}})
	root := companyRoot(t, "LAB-1")
	var out bytes.Buffer
	err := RunTask(context.Background(), &out, st, stateDir,
		[]string{"assign", "LAB-1", "collab-bot:t123456", "--inbox", filepath.Join(root, "runtime", "inbox")}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	lg, _ := os.ReadFile(filepath.Join(root, "tasks", "LAB-1", "log.md"))
	if !strings.Contains(string(lg), "[ASSIGN] collab-bot:t123456") {
		t.Errorf("log에 창 이름 누락: %s", lg)
	}
}

func TestTaskDoneMarksCompanyTask(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}})
	root := companyRoot(t, "LAB-1")
	inbox := filepath.Join(root, "runtime", "inbox")
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"assign", "LAB-1", "collab-bot", "--inbox", inbox}, time.Now()); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"done", "LAB-1"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "LAB-1", "task.md"))
	if !strings.Contains(string(b), "status: done") {
		t.Errorf("status:\n%s", b)
	}
	if list, _ := task.List(stateDir); len(list) != 0 {
		t.Error("등록이 해제돼야 함")
	}
}

// task.md에 yaml status: 줄이 없으면 board.SetStatus가 ErrNoStatus로 실패한다 — 이때 task done은
// 오류를 반환해야 하고, 등록을 먼저 지우면 안 된다(재시도할 수 있어야 함).
func TestTaskDoneSetStatusFailureKeepsRegistration(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}})
	root := t.TempDir()
	dir := filepath.Join(root, "tasks", "LAB-1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// yaml 블록이 없어 SetStatus가 ErrNoStatus로 실패한다.
	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte("# LAB-1\n본문만 있고 yaml 없음\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inbox := filepath.Join(root, "runtime", "inbox")
	if err := os.MkdirAll(inbox, 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"assign", "LAB-1", "collab-bot", "--inbox", inbox}, time.Now()); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"done", "LAB-1"}, time.Now()); err == nil {
		t.Error("SetStatus 실패는 task done 오류여야 함")
	}
	list, _ := task.List(stateDir)
	if len(list) != 1 {
		t.Errorf("실패 시 등록은 남아 있어야 함: %v", list)
	}
}

func TestTaskDoneGoneNeedsRoot(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	root := companyRoot(t, "LAB-1")
	var out bytes.Buffer
	err := RunTask(context.Background(), &out, st, stateDir, []string{"done", "LAB-1"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "--root") {
		t.Errorf("등록 없으면 --root 안내: %v", err)
	}
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"done", "LAB-1", "--root", root}, time.Now()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "LAB-1", "task.md"))
	if !strings.Contains(string(b), "status: done") {
		t.Errorf("--root 경로로 done 실패:\n%s", b)
	}
}
