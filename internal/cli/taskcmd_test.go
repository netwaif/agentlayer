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
