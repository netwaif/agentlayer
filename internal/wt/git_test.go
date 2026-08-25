package wt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixtureRepo는 커밋 하나 있는 임시 git repo를 만든다.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func TestWorktreeLifecycleChecks(t *testing.T) {
	repo := fixtureRepo(t)
	wtPath := filepath.Join(t.TempDir(), "task1")

	if err := AddWorktree(repo, "main", "agent/task1", wtPath); err != nil {
		t.Fatal(err)
	}
	if !BranchExists(repo, "agent/task1") {
		t.Error("브랜치 생성돼야 함")
	}

	// 깨끗한 상태
	d, err := Dirty(wtPath)
	if err != nil || len(d) != 0 {
		t.Errorf("깨끗: %v %v", d, err)
	}
	n, err := Unmerged(repo, "main", "agent/task1")
	if err != nil || n != 0 {
		t.Errorf("미병합 0: %d %v", n, err)
	}

	// 파일 수정 → dirty + diff
	if err := os.WriteFile(filepath.Join(wtPath, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "new.txt"), []byte("신규\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, _ = Dirty(wtPath)
	if len(d) != 2 {
		t.Errorf("dirty 2건: %v", d)
	}
	diff, err := Diff(wtPath, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "changed") || !strings.Contains(diff, "untracked") || !strings.Contains(diff, "new.txt") {
		t.Errorf("diff 내용:\n%s", diff)
	}

	// dirty 상태에서 RemoveWorktree는 git이 거부해야 함 (--force 안 쓰므로)
	if err := RemoveWorktree(repo, wtPath); err == nil {
		t.Fatal("dirty worktree 제거는 거부돼야 함")
	}

	// 커밋 → unmerged 1
	gitRun(t, wtPath, "add", ".")
	gitRun(t, wtPath, "commit", "-m", "work")
	n, _ = Unmerged(repo, "main", "agent/task1")
	if n != 1 {
		t.Errorf("미병합 1: %d", n)
	}

	// merge 후 정리 가능
	if err := Merge(repo, "main", "agent/task1"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWorktree(repo, wtPath); err != nil {
		t.Fatalf("깨끗+병합됨 제거: %v", err)
	}
	if err := DeleteBranch(repo, "agent/task1"); err != nil {
		t.Fatalf("병합된 브랜치 삭제: %v", err)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
}

func TestMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Truncate(time.Second)
	m := &Meta{Task: "auth-api", Repo: "/r", Base: "main", Branch: "agent/auth-api",
		Path: "/w", Agent: "claude", TestCmd: "go test ./...", CreatedAt: now}
	if err := SaveMeta(dir, m); err != nil {
		t.Fatal(err)
	}
	back, err := LoadMeta(dir, "auth-api")
	if err != nil {
		t.Fatal(err)
	}
	if back.Branch != m.Branch || back.Agent != "claude" {
		t.Errorf("round-trip: %+v", back)
	}
	list, _ := ListMetas(dir)
	if len(list) != 1 {
		t.Errorf("list: %d", len(list))
	}
	if err := DeleteMeta(dir, "auth-api"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMeta(dir, "auth-api"); err == nil {
		t.Error("삭제 후 로드 실패해야 함")
	}
}

func TestListMetasEmpty(t *testing.T) {
	list, err := ListMetas(t.TempDir())
	if err != nil || list != nil {
		t.Errorf("빈 목록: %v %v", list, err)
	}
}
