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
