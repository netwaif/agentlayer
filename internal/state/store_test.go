package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s := newTestStore(t)
	a := &Agent{ID: "claude-ai-1-3", Kind: "claude", State: StateWorking, UpdatedAt: t0, StateSince: t0}
	if err := s.Save(a); err != nil {
		t.Fatal(err)
	}
	back, err := s.Load("claude-ai-1-3")
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != a.ID || back.State != a.State {
		t.Errorf("round-trip 불일치: %+v", back)
	}
}

func TestLoadMissing(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Load("없음"); err == nil {
		t.Error("없는 ID는 에러여야 함")
	}
}

func TestListSortsByPriorityThenSince(t *testing.T) {
	s := newTestStore(t)
	mk := func(id string, st AgentState, since time.Time) {
		if err := s.Save(&Agent{ID: id, State: st, StateSince: since, UpdatedAt: since}); err != nil {
			t.Fatal(err)
		}
	}
	mk("w1", StateWorking, t0)
	mk("wait-late", StateWaiting, t0.Add(time.Minute))
	mk("wait-early", StateWaiting, t0)
	mk("done", StateDoneUnread, t0)
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, a := range got {
		ids = append(ids, a.ID)
	}
	want := []string{"wait-early", "wait-late", "done", "w1"}
	if fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Errorf("정렬 = %v, want %v", ids, want)
	}
}

func TestListGroupsByKindFirst(t *testing.T) {
	// 3사 정보가 섞이지 않게 종류별 그룹 우선, 그룹 안에서 상태 우선순위.
	s := newTestStore(t)
	mk := func(id, kind string, st AgentState) {
		if err := s.Save(&Agent{ID: id, Kind: kind, State: st, StateSince: t0, UpdatedAt: t0}); err != nil {
			t.Fatal(err)
		}
	}
	mk("g-wait", "gemini", StateWaiting)
	mk("c-work", "claude", StateWorking)
	mk("x-done", "codex", StateDoneUnread)
	mk("c-done", "claude", StateDoneUnread)
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, a := range got {
		ids = append(ids, a.ID)
	}
	want := []string{"c-done", "c-work", "x-done", "g-wait"}
	if fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Errorf("종류 그룹 정렬 = %v, want %v", ids, want)
	}
}

func TestListSkipsCorruptFiles(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save(&Agent{ID: "ok", State: StateIdle, UpdatedAt: t0, StateSince: t0}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "agents", "broken.json"), []byte("{잘림"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.List()
	if err != nil {
		t.Fatalf("깨진 파일이 있어도 List는 성공해야 함: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ok" {
		t.Errorf("정상 레코드만 반환해야 함: %+v", got)
	}
}

func TestMarkRead(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save(&Agent{ID: "d", State: StateDoneUnread, UpdatedAt: t0, StateSince: t0}); err != nil {
		t.Fatal(err)
	}
	later := t0.Add(time.Minute)
	if err := s.MarkRead("d", later); err != nil {
		t.Fatal(err)
	}
	a, _ := s.Load("d")
	if a.State != StateIdle {
		t.Errorf("DONE_UNREAD → IDLE 이어야 함: %s", a.State)
	}
	// DONE_UNREAD가 아니면 no-op
	if err := s.Save(&Agent{ID: "w", State: StateWorking, UpdatedAt: t0, StateSince: t0}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRead("w", later); err != nil {
		t.Fatal(err)
	}
	w, _ := s.Load("w")
	if w.State != StateWorking {
		t.Errorf("WORKING은 MarkRead로 안 바뀜: %s", w.State)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save(&Agent{ID: "x", State: StateDead, UpdatedAt: t0, StateSince: t0}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("x"); err == nil {
		t.Error("삭제 후 Load는 실패해야 함")
	}
}

func TestConcurrentSaveIntegrity(t *testing.T) {
	s := newTestStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			a := &Agent{ID: "same", Kind: "claude", State: StateWorking,
				Task: fmt.Sprintf("작업-%d", n), UpdatedAt: t0, StateSince: t0}
			if err := s.Save(a); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	a, err := s.Load("same")
	if err != nil {
		t.Fatalf("동시 쓰기 후 파일이 온전해야 함: %v", err)
	}
	if a.ID != "same" || a.Task == "" {
		t.Errorf("완전한 레코드여야 함: %+v", a)
	}
}

func TestDefaultDirEnvOverride(t *testing.T) {
	t.Setenv("AGENTLAYER_STATE_DIR", "/tmp/custom-state")
	if got := DefaultDir(); got != "/tmp/custom-state" {
		t.Errorf("env 오버라이드 = %s", got)
	}
	t.Setenv("AGENTLAYER_STATE_DIR", "")
	home, _ := os.UserHomeDir()
	if got := DefaultDir(); got != filepath.Join(home, ".local", "state", "agentlayer") {
		t.Errorf("기본 경로 = %s", got)
	}
}
