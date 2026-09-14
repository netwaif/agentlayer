// internal/task/report_test.go
package task

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

func TestShouldReportTable(t *testing.T) {
	cases := []struct {
		prev, to state.AgentState
		want     bool
	}{
		{state.StateWorking, state.StateDoneUnread, true},
		{state.StateWorking, state.StateWaiting, true},
		{state.StateWorking, state.StateError, true},
		{state.StateWorking, state.StateWorking, false}, // heartbeat
		{state.StateWaiting, state.StateWorking, false}, // 승인됨 — 총괄이 알 필요 없음
		{state.StateDoneUnread, state.StateIdle, false},
		{state.StateIdle, state.StateDead, false}, // dead는 scan 몫, v1 범위 밖
		{state.StateDoneUnread, state.StateDoneUnread, false},
	}
	for _, c := range cases {
		if got := ShouldReport(c.prev, c.to); got != c.want {
			t.Errorf("%s→%s = %v, want %v", c.prev, c.to, got, c.want)
		}
	}
}

func TestReportForUnassignedIsSilent(t *testing.T) {
	a := &state.Agent{ID: "claude-%1", Kind: "claude", Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}}
	if _, ok := ReportFor(t.TempDir(), a, state.StateWorking, state.StateDoneUnread, time.Now()); ok {
		t.Error("등록 안 된 세션은 보고하지 않음")
	}
}

func TestReportForAssignedAndWrite(t *testing.T) {
	stateDir, inbox := t.TempDir(), filepath.Join(t.TempDir(), "inbox")
	if err := Assign(stateDir, Assignment{TaskID: "T-1", AgentID: "claude-%16", Session: "search-youtube-bot",
		Window: "t170966", Pane: "%16", Inbox: inbox}, false); err != nil {
		t.Fatal(err)
	}
	a := &state.Agent{ID: "claude-%16", Kind: "claude", Task: "주제 3개 정리", Ask: "Claude needs your permission",
		CWD: "/tmp/w", Tmux: state.TmuxRef{Session: "search-youtube-bot", WindowName: "t170966", PaneID: "%16"}}
	now := time.Date(2026, 9, 14, 15, 7, 12, 0, time.UTC)
	r, ok := ReportFor(stateDir, a, state.StateWorking, state.StateWaiting, now)
	if !ok {
		t.Fatal("등록된 세션의 WAITING 전이는 보고")
	}
	if r.TaskID != "T-1" || r.To != "WAITING" || r.From != "WORKING" || r.Ask == "" || r.Inbox != inbox || len(r.ID) != 32 {
		t.Fatalf("보고 내용: %+v", r)
	}
	p, err := WriteReport(r)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(p) != filepath.Join(inbox, "pending") || filepath.Base(p) != r.ID+".json" {
		t.Errorf("경로: %s", p)
	}
	b, _ := os.ReadFile(p)
	var back Report
	if err := json.Unmarshal(b, &back); err != nil || back.Version != 1 || back.Task != "주제 3개 정리" || !back.At.Equal(now) {
		t.Fatalf("파일 내용: %s err=%v", b, err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("파일 권한 0600이어야 함: %v", fi.Mode().Perm())
	}
	if _, ok := ReportFor(stateDir, a, state.StateWorking, state.StateWorking, now); ok {
		t.Error("heartbeat는 보고 안 함")
	}
}

func TestNewIDIsHex32Unique(t *testing.T) {
	a, b := NewID(), NewID()
	if len(a) != 32 || a == b {
		t.Errorf("id: %s %s", a, b)
	}
	for _, c := range a {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			t.Fatalf("hex 아님: %s", a)
		}
	}
}
