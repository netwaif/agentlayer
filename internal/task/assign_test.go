// internal/task/assign_test.go
package task

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sample(id, agent string) Assignment {
	return Assignment{TaskID: id, AgentID: agent, Session: "search-youtube-bot", Window: "t170966",
		Pane: "%16", Inbox: "/tmp/inbox", AssignedAt: time.Date(2026, 9, 14, 15, 0, 0, 0, time.UTC)}
}

func TestValidID(t *testing.T) {
	for _, ok := range []string{"VIDEO-07", "a.b_c", "x"} {
		if !ValidID(ok) {
			t.Errorf("%q는 유효해야 함", ok)
		}
	}
	for _, bad := range []string{"", "한글", "a b", "a/b", string(make([]byte, 65))} {
		if ValidID(bad) {
			t.Errorf("%q는 거부돼야 함", bad)
		}
	}
}

func TestAssignLoadListDone(t *testing.T) {
	dir := t.TempDir()
	if err := Assign(dir, sample("T-1", "claude-%16"), false); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(dir, "claude-%16")
	if err != nil || !ok || got.TaskID != "T-1" || got.Window != "t170966" {
		t.Fatalf("Load: %+v ok=%v err=%v", got, ok, err)
	}
	if _, ok, _ := Load(dir, "없음"); ok {
		t.Error("없는 에이전트는 ok=false")
	}
	if err := Assign(dir, sample("T-2", "claude-%16"), false); !errors.Is(err, ErrAlreadyAssigned) {
		t.Fatalf("중복 배정은 ErrAlreadyAssigned: %v", err)
	}
	if err := Assign(dir, sample("T-2", "claude-%16"), true); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if err := Assign(dir, sample("A-0", "codex-%2"), false); err != nil {
		t.Fatal(err)
	}
	list, err := List(dir)
	if err != nil || len(list) != 2 || list[0].TaskID != "A-0" || list[1].TaskID != "T-2" {
		t.Fatalf("List 정렬(TaskID 오름차순): %+v err=%v", list, err)
	}
	if done, err := Done(dir, "T-2"); err != nil || !done {
		t.Fatalf("Done: %v %v", done, err)
	}
	if done, _ := Done(dir, "T-2"); done {
		t.Error("두 번째 Done은 false")
	}
	if _, err := os.Stat(filepath.Join(Dir(dir), "claude-%16.json")); !os.IsNotExist(err) {
		t.Error("Done 뒤 파일이 남아 있음")
	}
}

func TestAssignRejectsBadID(t *testing.T) {
	if err := Assign(t.TempDir(), sample("bad id", "x"), false); err == nil {
		t.Error("잘못된 업무ID는 거부")
	}
}

func TestListEmptyDir(t *testing.T) {
	list, err := List(t.TempDir())
	if err != nil || len(list) != 0 {
		t.Fatalf("tasks/ 없음 → 빈 목록: %v %v", list, err)
	}
}

func TestAssignAndLoadRejectPathTraversalAgentID(t *testing.T) {
	dir := t.TempDir()
	if err := Assign(dir, sample("T-9", "../x"), false); err == nil {
		t.Error("경로 조작 AgentID는 Assign이 거부해야 함")
	}
	if _, ok, err := Load(dir, "../x"); ok || err == nil {
		t.Errorf("경로 조작 AgentID는 Load가 거부해야 함: ok=%v err=%v", ok, err)
	}
}
