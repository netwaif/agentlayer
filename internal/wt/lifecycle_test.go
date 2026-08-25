package wt

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewCreatesWorktreeAndMeta(t *testing.T) {
	repo := fixtureRepo(t)
	stateDir := t.TempDir()
	m, err := New(stateDir, NewOptions{Task: "auth-api", Repo: repo, NoWindow: true})
	if err != nil {
		t.Fatal(err)
	}
	if m.Base != "main" || m.Branch != "agent/auth-api" || m.Agent != "claude" {
		t.Errorf("meta: %+v", m)
	}
	if _, err := os.Stat(filepath.Join(m.Path, "a.txt")); err != nil {
		t.Error("worktree에 파일 있어야 함")
	}
	if !BranchExists(repo, "agent/auth-api") {
		t.Error("브랜치 생성")
	}
	// 같은 태스크 재생성 거부
	if _, err := New(stateDir, NewOptions{Task: "auth-api", Repo: repo, NoWindow: true}); err == nil {
		t.Error("중복 태스크 거부")
	}
}

func TestNewRejectsBadInput(t *testing.T) {
	stateDir := t.TempDir()
	if _, err := New(stateDir, NewOptions{Task: "", Repo: t.TempDir(), NoWindow: true}); err == nil {
		t.Error("빈 태스크 거부")
	}
	if _, err := New(stateDir, NewOptions{Task: "x", Repo: t.TempDir(), NoWindow: true}); err == nil {
		t.Error("git 아닌 폴더 거부")
	}
	repo := fixtureRepo(t)
	if _, err := New(stateDir, NewOptions{Task: "x", Repo: repo, Agent: "gpt9", NoWindow: true}); err == nil {
		t.Error("미지원 에이전트 거부")
	}
}

func TestCleanRefusesDirtyAndUnmerged(t *testing.T) {
	repo := fixtureRepo(t)
	stateDir := t.TempDir()
	m, err := New(stateDir, NewOptions{Task: "t1", Repo: repo, NoWindow: true})
	if err != nil {
		t.Fatal(err)
	}
	// dirty → 거부
	os.WriteFile(filepath.Join(m.Path, "wip.txt"), []byte("작업중\n"), 0o644)
	err = Clean(stateDir, "t1")
	var refusal *CleanRefusal
	if r, ok := err.(*CleanRefusal); !ok {
		t.Fatalf("CleanRefusal이어야 함: %v", err)
	} else {
		refusal = r
	}
	if len(refusal.Dirty) != 1 {
		t.Errorf("dirty 1건: %+v", refusal)
	}
	if _, err := os.Stat(m.Path); err != nil {
		t.Fatal("거부 시 worktree 보존")
	}
	// 커밋 → 미병합 거부
	gitRun(t, m.Path, "add", ".")
	gitRun(t, m.Path, "commit", "-m", "wip")
	err = Clean(stateDir, "t1")
	if r, ok := err.(*CleanRefusal); !ok || r.Unmerged != 1 {
		t.Fatalf("미병합 거부: %v", err)
	}
	// 병합 → 정리 성공
	if err := Merge(m.Repo, m.Base, m.Branch); err != nil {
		t.Fatal(err)
	}
	if err := Clean(stateDir, "t1"); err != nil {
		t.Fatalf("깨끗+병합 후 정리: %v", err)
	}
	if _, err := os.Stat(m.Path); err == nil {
		t.Error("worktree 제거돼야 함")
	}
	if _, err := LoadMeta(stateDir, "t1"); err == nil {
		t.Error("메타 제거돼야 함")
	}
}

func TestMergeGuideConfirmFlow(t *testing.T) {
	repo := fixtureRepo(t)
	stateDir := t.TempDir()
	m, _ := New(stateDir, NewOptions{Task: "t2", Repo: repo, NoWindow: true})
	os.WriteFile(filepath.Join(m.Path, "f.txt"), []byte("x\n"), 0o644)
	gitRun(t, m.Path, "add", ".")
	gitRun(t, m.Path, "commit", "-m", "work")

	// n → merge 안 함
	var buf bytes.Buffer
	if err := MergeGuide(&buf, stateDir, "t2", func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	if n, _ := Unmerged(repo, "main", "agent/t2"); n != 1 {
		t.Error("거절 시 merge 안 됨")
	}
	if !strings.Contains(buf.String(), "git -C") || !strings.Contains(buf.String(), "--no-ff") {
		t.Errorf("명령 안내 포함:\n%s", buf.String())
	}
	// y → merge 실행
	buf.Reset()
	if err := MergeGuide(&buf, stateDir, "t2", func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	if n, _ := Unmerged(repo, "main", "agent/t2"); n != 0 {
		t.Error("승인 시 merge 완료")
	}
}
