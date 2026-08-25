package starter

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTask(t *testing.T, root, name, status string, age time.Duration) {
	t.Helper()
	dir := filepath.Join(root, "tasks", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "# " + name + "\n\n## 메타\n```yaml\nstatus: " + status + "\ncreated: 2026-08-25\n```\n\n## Goal\n테스트\n"
	p := filepath.Join(dir, "task.md")
	if err := os.WriteFile(p, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	mt := time.Now().Add(-age)
	os.Chtimes(p, mt, mt)
}

func TestActiveTasks(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "done-task", "done", time.Minute)
	writeTask(t, root, "running", "in_progress", 2*time.Minute)
	writeTask(t, root, "waiting", "waiting_approval", time.Minute)
	writeTask(t, root, "review", "reviewing", 3*time.Minute)

	got := ActiveTasks(root)
	if len(got) != 3 {
		t.Fatalf("활성 3개: %+v", got)
	}
	// mtime 역순: waiting(1m) → running(2m) → review(3m)
	if got[0].Name != "waiting" || got[2].Name != "review" {
		t.Errorf("정렬: %+v", got)
	}
	if got[0].Status != "waiting_approval" {
		t.Errorf("status: %+v", got[0])
	}
}

func TestActiveTasksMissingRoot(t *testing.T) {
	if got := ActiveTasks(""); got != nil {
		t.Error("빈 root는 nil")
	}
	if got := ActiveTasks("/없는/경로"); got != nil {
		t.Error("없는 경로는 nil")
	}
}

func TestReadStatusBrokenFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tasks", "broken")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "task.md"), []byte("yaml 블록 없음"), 0o644)
	if got := ActiveTasks(root); len(got) != 0 {
		t.Errorf("unknown status는 비활성: %+v", got)
	}
}
