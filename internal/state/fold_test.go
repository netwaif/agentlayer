package state

import (
	"testing"
	"time"
)

var f0 = time.Date(2026, 9, 13, 12, 0, 0, 0, time.FixedZone("KST", 9*3600))

func TestIsThreadWindow(t *testing.T) {
	cases := map[string]bool{
		"t552990": true, "t999002": true,
		"t55299": false, "t5529901": false, "agentlayer": false, "": false, "T552990": false,
	}
	for name, want := range cases {
		if got := IsThreadWindow(name); got != want {
			t.Errorf("IsThreadWindow(%q) = %v, want %v", name, got, want)
		}
	}
}

func bot(id, sess, win string, st AgentState, since time.Time) *Agent {
	return &Agent{ID: id, Kind: "claude", State: st, Task: "task " + id,
		Tmux:      TmuxRef{Session: sess, Window: 0, PaneID: "%" + id[len(id)-1:], WindowName: win},
		CWD:       "/bots/" + sess,
		UpdatedAt: since, StateSince: since}
}

// 실측(2026-09-13): dev-claudecode 세션의 메인(창 0)과 스레드 창 t552990 둘 다 레코드 →
// 표시에서는 봇당 한 행이어야 한다.
func TestFoldMergesThreadIntoMain(t *testing.T) {
	main := bot("claude-33", "dev-claudecode", "dev-claudecode", StateIdle, f0.Add(-time.Hour))
	thr := bot("claude-35", "dev-claudecode", "t552990", StateWorking, f0.Add(-time.Minute))
	other := bot("claude-40", "ai", "agentlayer", StateWorking, f0)
	// List 순서: WORK가 idle보다 위
	got := Fold([]*Agent{thr, other, main})
	if len(got) != 2 {
		t.Fatalf("2행이어야 함: %d", len(got))
	}
	// 대표는 급한 쪽(WORK인 스레드) — 상태·TASK·점프 대상이 그 pane
	r := got[0]
	if r.ID != "claude-35" || r.State != StateWorking || r.Threads != 1 {
		t.Errorf("대표 불일치: %+v", r)
	}
	if r.ThreadBadge() != "스레드 1" {
		t.Errorf("배지: %q", r.ThreadBadge())
	}
	if got[1].ID != "claude-40" || got[1].Threads != 0 || got[1].ThreadBadge() != "" {
		t.Errorf("무관한 행은 그대로: %+v", got[1])
	}
	// 원본은 건드리지 않는다(레코드 정본 보호)
	if thr.Threads != 0 || main.Threads != 0 {
		t.Errorf("원본 변경 금지: %d %d", thr.Threads, main.Threads)
	}
}

func TestFoldTieGoesToMain(t *testing.T) {
	main := bot("claude-33", "dev-claudecode", "dev-claudecode", StateIdle, f0.Add(-time.Hour))
	thr := bot("claude-35", "dev-claudecode", "t552990", StateIdle, f0)
	got := Fold([]*Agent{thr, main})
	if len(got) != 1 || got[0].ID != "claude-33" || got[0].Threads != 1 {
		t.Errorf("동률이면 메인이 대표: %+v", got)
	}
}

func TestFoldCountsMultipleThreads(t *testing.T) {
	main := bot("claude-33", "dev-claudecode", "dev-claudecode", StateIdle, f0)
	t1 := bot("claude-35", "dev-claudecode", "t552990", StateIdle, f0)
	t2 := bot("claude-36", "dev-claudecode", "t552991", StateWaiting, f0)
	got := Fold([]*Agent{t2, main, t1})
	if len(got) != 1 || got[0].ID != "claude-36" || got[0].Threads != 2 || got[0].ThreadBadge() != "스레드 2" {
		t.Errorf("스레드 2개 접힘: %+v", got)
	}
}

func TestFoldLeavesUserWindowsAlone(t *testing.T) {
	// 같은 세션의 이름 있는 창들(사용자 worktree 작업 등)은 스레드가 아니므로 접지 않는다
	a := bot("claude-1", "ai", "agentlayer", StateWorking, f0)
	b := bot("claude-2", "ai", "hero-bold", StateIdle, f0)
	got := Fold([]*Agent{a, b})
	if len(got) != 2 {
		t.Errorf("스레드 창 없으면 그대로: %d", len(got))
	}
}

func TestFoldThreadWithoutMainStandsWithBadge(t *testing.T) {
	thr := bot("claude-35", "dev-claudecode", "t552990", StateWorking, f0)
	got := Fold([]*Agent{thr})
	if len(got) != 1 || got[0].Threads != 0 || got[0].ThreadBadge() != "스레드 t552990" {
		t.Errorf("메인 없는 스레드는 창 이름으로 구분: %+v", got)
	}
}

func TestFoldSkipsDeadAndOtherKinds(t *testing.T) {
	main := bot("claude-33", "dev-claudecode", "dev-claudecode", StateIdle, f0)
	deadThr := bot("claude-35", "dev-claudecode", "t552990", StateDead, f0)
	codexThr := bot("codex-9", "dev-claudecode", "t552991", StateWorking, f0)
	codexThr.Kind = "codex"
	got := Fold([]*Agent{main, deadThr, codexThr})
	if len(got) != 3 {
		t.Fatalf("dead·다른 kind는 접지 않음: %d", len(got))
	}
	if got[0].Threads != 0 {
		t.Errorf("메인에 스레드 수 붙지 않아야: %+v", got[0])
	}
}

func TestFoldAttachesThreadsToSameCWDMain(t *testing.T) {
	m1 := bot("claude-1", "bots", "main-a", StateIdle, f0)
	m2 := bot("claude-2", "bots", "main-b", StateIdle, f0)
	m2.CWD = "/bots/other"
	thr := bot("claude-3", "bots", "t000001", StateIdle, f0)
	thr.CWD = "/bots/other"
	got := Fold([]*Agent{m1, m2, thr})
	if len(got) != 2 || got[0].Threads != 0 || got[1].ID != "claude-2" || got[1].Threads != 1 {
		t.Errorf("같은 폴더 메인에 붙어야: %+v %+v", got[0], got[1])
	}
}
