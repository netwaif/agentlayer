// internal/task/report_test.go
package task

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	now := time.Date(2026, 9, 14, 15, 7, 12, 0, time.UTC)
	a := &state.Agent{ID: "claude-%16", Kind: "claude", Task: "주제 3개 정리", Ask: "Claude needs your permission",
		CWD: "/tmp/w", Tmux: state.TmuxRef{Session: "search-youtube-bot", WindowName: "t170966", PaneID: "%16"},
		LastSendAt: now.Add(-time.Minute)}
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

// 에이전트 ID가 재사용됐지만(재시작 등) tmux 세션·pane이 등록 당시와 다르면
// 낡은 등록으로 간주해 보고하지 않는다 — 엉뚱한 세션 앞으로 보고가 새지 않게.
func TestReportForStaleAssignmentIsSilent(t *testing.T) {
	stateDir, inbox := t.TempDir(), filepath.Join(t.TempDir(), "inbox")
	if err := Assign(stateDir, Assignment{TaskID: "T-1", AgentID: "claude-%16", Session: "old-bot",
		Window: "", Pane: "%16", Inbox: inbox}, false); err != nil {
		t.Fatal(err)
	}
	// 같은 에이전트 ID지만 세션이 바뀐 상태(pane 번호는 우연히 같음)
	a := &state.Agent{ID: "claude-%16", Kind: "claude", LastSendAt: time.Now(),
		Tmux: state.TmuxRef{Session: "new-bot", PaneID: "%16"}}
	if _, ok := ReportFor(stateDir, a, state.StateWorking, state.StateDoneUnread, time.Now()); ok {
		t.Error("세션이 바뀐 낡은 등록은 보고하지 않아야 함")
	}
}

func TestReportCarriesTaskDir(t *testing.T) {
	stateDir, root, a := linkedAgent(t, "폴더 밖 읽어도 될까요?")
	rep, ok := ReportFor(stateDir, a, state.StateWorking, state.StateDoneUnread, time.Now())
	if !ok || rep.TaskDir != filepath.Join(root, "tasks", "LAB-1") {
		t.Errorf("rep=%+v ok=%v", rep, ok)
	}
	b, _ := json.Marshal(rep)
	if !strings.Contains(string(b), `"task_dir"`) {
		t.Errorf("JSON에 task_dir 없음: %s", b)
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

// 총괄이 시킨 턴만 보고한다 — send 기록이 없으면 조용하고, send 뒤 첫 DONE이 그 send를 소비하면 사용자가 직접 친
// 턴의 DONE·WAITING은 다음 send까지 보고하지 않는다(사용자가 등록 세션과 대화할 때마다 총괄이 깨어나던 것).
func TestReportOnlyForTurnsStartedBySend(t *testing.T) {
	stateDir, inbox := t.TempDir(), filepath.Join(t.TempDir(), "inbox")
	if err := Assign(stateDir, Assignment{TaskID: "T-2", AgentID: "claude-%7", Session: "s", Pane: "%7", Inbox: inbox}, false); err != nil {
		t.Fatal(err)
	}
	a := &state.Agent{ID: "claude-%7", Kind: "claude", Task: "답", Tmux: state.TmuxRef{Session: "s", PaneID: "%7"}}
	t0 := time.Date(2026, 10, 3, 14, 0, 0, 0, time.UTC)
	if _, ok := ReportFor(stateDir, a, state.StateWorking, state.StateDoneUnread, t0); ok {
		t.Fatal("send 기록 없는 세션의 DONE은 보고 안 함(사용자 직접 대화)")
	}
	a.LastSendAt = t0.Add(time.Second) // 총괄 send
	r, ok := ReportFor(stateDir, a, state.StateWorking, state.StateWaiting, t0.Add(5*time.Second))
	if !ok {
		t.Fatal("send 뒤 승인 대기는 보고")
	}
	if _, err := WriteReportFor(stateDir, a.ID, r); err != nil {
		t.Fatal(err)
	}
	r, ok = ReportFor(stateDir, a, state.StateWorking, state.StateDoneUnread, t0.Add(10*time.Second))
	if !ok {
		t.Fatal("WAITING 보고는 send를 소비하지 않는다 — 승인 뒤 DONE도 보고")
	}
	if _, err := WriteReportFor(stateDir, a.ID, r); err != nil {
		t.Fatal(err)
	}
	as, _, _ := Load(stateDir, a.ID)
	if !as.DoneSendAt.Equal(a.LastSendAt) {
		t.Fatalf("DONE 보고가 send 시각을 기억해야 함: %v", as.DoneSendAt)
	}
	if _, ok := ReportFor(stateDir, a, state.StateWorking, state.StateDoneUnread, t0.Add(20*time.Second)); ok {
		t.Error("같은 send에 대한 두 번째 DONE(사용자 턴)은 보고 안 함")
	}
	if _, ok := ReportFor(stateDir, a, state.StateWorking, state.StateWaiting, t0.Add(21*time.Second)); ok {
		t.Error("DONE 뒤 사용자 턴의 승인 대기도 보고 안 함")
	}
	a.LastSendAt = t0.Add(30 * time.Second) // 다음 send
	if _, ok := ReportFor(stateDir, a, state.StateWorking, state.StateDoneUnread, t0.Add(35*time.Second)); !ok {
		t.Error("새 send 뒤 DONE은 다시 보고")
	}
	if files, _ := filepath.Glob(filepath.Join(inbox, "pending", "*.json")); len(files) != 2 {
		t.Errorf("보고 파일은 WAITING·DONE 2건이어야 함: %d", len(files))
	}
}
