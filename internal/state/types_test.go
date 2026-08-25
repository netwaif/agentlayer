package state

import (
	"encoding/json"
	"testing"
	"time"
)

var t0 = time.Date(2026, 8, 25, 12, 0, 0, 0, time.FixedZone("KST", 9*3600))

func TestTransitionUpdatesStateSince(t *testing.T) {
	a := &Agent{ID: "claude-x", State: StateIdle, StateSince: t0, UpdatedAt: t0}
	later := t0.Add(5 * time.Minute)
	a.Transition(StateWorking, later)
	if a.State != StateWorking {
		t.Fatalf("State = %s, want WORKING", a.State)
	}
	if !a.StateSince.Equal(later) {
		t.Errorf("StateSince = %v, want %v", a.StateSince, later)
	}
	if !a.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", a.UpdatedAt, later)
	}
}

func TestTransitionSameStateKeepsSince(t *testing.T) {
	a := &Agent{ID: "claude-x", State: StateWorking, StateSince: t0, UpdatedAt: t0}
	later := t0.Add(2 * time.Minute)
	a.Transition(StateWorking, later)
	if !a.StateSince.Equal(t0) {
		t.Errorf("같은 상태 재진입 시 StateSince 유지되어야 함: got %v", a.StateSince)
	}
	if !a.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt은 갱신되어야 함: got %v", a.UpdatedAt)
	}
}

func TestPriorityOrder(t *testing.T) {
	order := []AgentState{StateWaiting, StateDoneUnread, StateError, StateWorking, StateIdle, StateDead}
	for i := 1; i < len(order); i++ {
		if order[i-1].Priority() >= order[i].Priority() {
			t.Errorf("%s(%d) < %s(%d) 이어야 함", order[i-1], order[i-1].Priority(), order[i], order[i].Priority())
		}
	}
}

func TestStale(t *testing.T) {
	a := &Agent{State: StateWorking, UpdatedAt: t0}
	if a.Stale(t0.Add(29 * time.Minute)) {
		t.Error("29분은 stale 아님")
	}
	if !a.Stale(t0.Add(31 * time.Minute)) {
		t.Error("31분은 stale")
	}
	idle := &Agent{State: StateIdle, UpdatedAt: t0}
	if idle.Stale(t0.Add(2 * time.Hour)) {
		t.Error("WORKING이 아니면 stale 아님")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	a := &Agent{
		ID: "claude-ai-1-3", Kind: "claude", Task: "핸드오프 문서 확인",
		State:     StateDoneUnread,
		Tmux:      TmuxRef{Session: "ai", Window: 1, PaneID: "%3"},
		CWD:       "/Users/soonho/ai-folder/dev/agentlayer",
		SessionID: "10ec8033", PID: 12345,
		UpdatedAt: t0, StateSince: t0,
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var back Agent
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.ID != a.ID || back.State != a.State || back.Tmux != a.Tmux || back.Task != a.Task {
		t.Errorf("round-trip 불일치: %+v", back)
	}
}
