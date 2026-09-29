package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

// 훅이 하는 순서 그대로: 보드 반영 → 보고 만들기 → 쓰기(+질문 기억).
func hookTransition(t *testing.T, stateDir string, a *state.Agent, prev, to state.AgentState) bool {
	t.Helper()
	if _, err := ApplyTransition(stateDir, a, prev, to, time.Now()); err != nil {
		t.Fatal(err)
	}
	rep, ok := ReportFor(stateDir, a, prev, to, time.Now())
	if !ok {
		return false
	}
	if _, err := WriteReportFor(stateDir, a.ID, rep); err != nil {
		t.Fatal(err)
	}
	return true
}

func localFixture(t *testing.T) (stateDir, root, inbox string, a *state.Agent) {
	t.Helper()
	stateDir, root = t.TempDir(), t.TempDir()
	inbox = filepath.Join(root, "runtime", "inbox")
	dir := board.TaskDir(root, "T-1")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "task.md"), []byte("# 제목\n\n```yaml\nstatus: in_progress\nparents: []\n```\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "log.md"), []byte(""), 0o644)
	if err := Assign(stateDir, Assignment{TaskID: "T-1", AgentID: "claude-%16", Session: "lab-bot", Pane: "%16", Inbox: inbox, TaskDir: dir}, false); err != nil {
		t.Fatal(err)
	}
	a = &state.Agent{ID: "claude-%16", Kind: "claude", Tmux: state.TmuxRef{Session: "lab-bot", PaneID: "%16"}}
	return
}

// 로컬 훅 경로: WAITING에 머문 채 질문만 바뀌면 새 보고가 가고, 같은 질문의 반복 알림은 가지 않는다.
func TestLocalAskChangeWhileWaitingReportsOnce(t *testing.T) {
	stateDir, root, inbox, a := localFixture(t)
	a.Ask = "Bash 실행을 승인할까요?"
	if !hookTransition(t, stateDir, a, state.StateWorking, state.StateWaiting) {
		t.Fatal("WORKING→WAITING은 보고")
	}
	if hookTransition(t, stateDir, a, state.StateWaiting, state.StateWaiting) {
		t.Error("같은 질문의 반복 알림은 보고하지 않음")
	}
	a.Ask = "파일을 덮어쓸까요?"
	if !hookTransition(t, stateDir, a, state.StateWaiting, state.StateWaiting) {
		t.Fatal("질문이 바뀌면 같은 WAITING이어도 보고")
	}
	if hookTransition(t, stateDir, a, state.StateWaiting, state.StateWaiting) {
		t.Error("바뀐 질문도 한 번만")
	}
	a.Ask = ""
	if hookTransition(t, stateDir, a, state.StateWaiting, state.StateWaiting) {
		t.Error("질문이 비면(문구 없는 알림) 보고하지 않음")
	}
	reps := pendingReports(t, inbox)
	asks := map[string]int{}
	for _, r := range reps {
		if r.To != "WAITING" {
			t.Errorf("to=%s", r.To)
		}
		asks[r.Ask]++
	}
	if len(reps) != 2 || asks["Bash 실행을 승인할까요?"] != 1 || asks["파일을 덮어쓸까요?"] != 1 {
		t.Fatalf("보고는 질문마다 한 건: %+v", reps)
	}
	b, _ := os.ReadFile(filepath.Join(board.TaskDir(root, "T-1"), "log.md"))
	if strings.Count(string(b), "[ASK]") != 2 || !strings.Contains(string(b), "파일을 덮어쓸까요?") {
		t.Errorf("log.md에도 질문마다 [ASK] 한 줄: %s", b)
	}
	// 다른 상태의 heartbeat는 여전히 무음
	if hookTransition(t, stateDir, a, state.StateWorking, state.StateWorking) {
		t.Error("WORKING heartbeat는 보고하지 않음")
	}
}

// 승인 뒤 다시 같은 질문으로 멈추면 그것은 새 전이다 — 질문이 같아도 보고한다.
func TestLocalSameAskAfterResumeStillReports(t *testing.T) {
	stateDir, _, inbox, a := localFixture(t)
	a.Ask = "Bash 실행을 승인할까요?"
	hookTransition(t, stateDir, a, state.StateWorking, state.StateWaiting)
	hookTransition(t, stateDir, a, state.StateWaiting, state.StateWorking)
	if !hookTransition(t, stateDir, a, state.StateWorking, state.StateWaiting) {
		t.Error("WORKING→WAITING 전이는 질문이 같아도 보고")
	}
	if n := len(pendingReports(t, inbox)); n != 2 {
		t.Errorf("reports=%d", n)
	}
}

// 원격 폴링 경로: blocked인 채 질문만 바뀐 카드.
func TestRemoteAskChangeWhileWaitingReportsOnce(t *testing.T) {
	stateDir, root, inbox := remoteFixture(t)
	as, _, _ := Load(stateDir, RemoteAgentID("hermes-qa"))
	now := time.Now()
	apply := func(s remote.Status) bool {
		t.Helper()
		changed, err := ApplyRemoteStatus(stateDir, as, s, "", now)
		if err != nil {
			t.Fatal(err)
		}
		return changed
	}
	apply(remote.Status{State: state.StateWaiting, Ask: "어느 파일?", Seen: 10})
	if n := len(pendingReports(t, inbox)); n != 1 {
		t.Fatalf("첫 WAITING 보고: %d", n)
	}
	// 같은 질문 — 이벤트 시각만 바뀜(댓글 등)
	apply(remote.Status{State: state.StateWaiting, Ask: "어느 파일?", Seen: 11})
	apply(remote.Status{State: state.StateWaiting, Ask: "어느 파일?", Seen: 11})
	if n := len(pendingReports(t, inbox)); n != 1 {
		t.Fatalf("같은 질문은 다시 보고하지 않음: %d", n)
	}
	// 질문이 바뀜
	if !apply(remote.Status{State: state.StateWaiting, Ask: "어느 폴더?", Seen: 12}) {
		t.Error("질문 변화는 변화다")
	}
	apply(remote.Status{State: state.StateWaiting, Ask: "어느 폴더?", Seen: 12})
	// Seen을 주지 않는 어댑터(exec, 항상 0)도 질문 변화는 잡는다
	apply(remote.Status{State: state.StateWaiting, Ask: "어느 브랜치?", Seen: 12})
	apply(remote.Status{State: state.StateWaiting, Ask: "어느 브랜치?", Seen: 12})
	reps := pendingReports(t, inbox)
	asks := map[string]int{}
	for _, r := range reps {
		asks[r.Ask]++
	}
	if len(reps) != 3 || asks["어느 파일?"] != 1 || asks["어느 폴더?"] != 1 || asks["어느 브랜치?"] != 1 {
		t.Fatalf("질문마다 한 건: %+v", reps)
	}
	b, _ := os.ReadFile(filepath.Join(board.TaskDir(root, "PING-2"), "log.md"))
	if strings.Count(string(b), "[ASK]") != 3 {
		t.Errorf("log.md [ASK] 3줄: %s", b)
	}
	cur, _, _ := Load(stateDir, RemoteAgentID("hermes-qa"))
	if cur.LastAsk != "어느 브랜치?" || cur.Remote.LastState != state.StateWaiting {
		t.Errorf("등록 파일: %+v %+v", cur, cur.Remote)
	}
}

// 질문 기억이 없는 옛 등록(업그레이드 전에 이미 WAITING을 보고함)은 변화 없는 재관측으로 중복 보고하지 않는다.
func TestRemoteLegacyWaitingWithoutLastAskIsQuiet(t *testing.T) {
	stateDir, _, inbox := remoteFixture(t)
	as, _, _ := Load(stateDir, RemoteAgentID("hermes-qa"))
	as.Remote.LastState, as.Remote.Seen = state.StateWaiting, 10
	Save(stateDir, *as)
	if changed, _ := ApplyRemoteStatus(stateDir, as, remote.Status{State: state.StateWaiting, Ask: "어느 파일?", Seen: 10}, "", time.Now()); changed {
		t.Error("변화 없음")
	}
	if n := len(pendingReports(t, inbox)); n != 0 {
		t.Errorf("reports=%d", n)
	}
}
