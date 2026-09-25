package task

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

// 회사 루트 하나와 업무 하나(tasks/<id>/task.md·log.md)를 만든다.
func remoteFixture(t *testing.T) (stateDir, root, inbox string) {
	t.Helper()
	stateDir, root = t.TempDir(), t.TempDir()
	inbox = filepath.Join(root, "runtime", "inbox")
	dir := board.TaskDir(root, "PING-2")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "task.md"), []byte("# 연결 시험\n\n```yaml\nstatus: in_progress\nparents: []\n```\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "log.md"), []byte(""), 0o644)
	as := Assignment{TaskID: "PING-2", AgentID: RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote", Inbox: inbox,
		TaskDir: dir, AssignedAt: time.Now(), Remote: &RemoteRef{Name: "hermes-qa", Handle: "t_1"}}
	if err := Assign(stateDir, as, false); err != nil {
		t.Fatal(err)
	}
	return
}

func pendingReports(t *testing.T, inbox string) []Report {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(inbox, "pending", "*.json"))
	var out []Report
	for _, f := range files {
		b, _ := os.ReadFile(f)
		var r Report
		if err := json.Unmarshal(b, &r); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func TestApplyRemoteStatusReportsAndUpdatesBoard(t *testing.T) {
	stateDir, root, inbox := remoteFixture(t)
	as, _, _ := Load(stateDir, RemoteAgentID("hermes-qa"))
	now := time.Now()
	changed, err := ApplyRemoteStatus(stateDir, as, remote.Status{State: state.StateWaiting, Ask: "어느 파일?", Seen: 10}, "", now)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	reps := pendingReports(t, inbox)
	if len(reps) != 1 || reps[0].To != "WAITING" || reps[0].Ask != "어느 파일?" || reps[0].Session != "hermes-qa" || reps[0].Kind != "hermes" || reps[0].TaskID != "PING-2" {
		t.Fatalf("reports=%+v", reps)
	}
	tf, _ := board.ReadTaskFile(root, "PING-2")
	if tf.Status != "waiting_hermes-qa" {
		t.Errorf("status=%s", tf.Status)
	}
	if !strings.Contains(board.ReadLastLog(root, "PING-2"), "[ASK]") {
		t.Error("[ASK] 로그")
	}
	as, _, _ = Load(stateDir, RemoteAgentID("hermes-qa"))
	if as.Remote.LastState != state.StateWaiting || as.Remote.Seen != 10 {
		t.Errorf("등록 파일 갱신: %+v", as.Remote)
	}
	changed, _ = ApplyRemoteStatus(stateDir, as, remote.Status{State: state.StateWaiting, Ask: "어느 파일?", Seen: 10}, "", now)
	if changed || len(pendingReports(t, inbox)) != 1 {
		t.Error("재관측은 보고하지 않음")
	}
}

func TestApplyRemoteStatusSkipsIntermediate(t *testing.T) {
	stateDir, root, inbox := remoteFixture(t)
	as, _, _ := Load(stateDir, RemoteAgentID("hermes-qa"))
	dest := filepath.Join(root, "결과물", "PING-2", "remote")
	if _, err := ApplyRemoteStatus(stateDir, as, remote.Status{State: state.StateDoneUnread, Summary: "PONG-2", Seen: 99}, dest, time.Now()); err != nil {
		t.Fatal(err)
	}
	reps := pendingReports(t, inbox)
	if len(reps) != 1 || reps[0].To != "DONE_UNREAD" || reps[0].Task != "PONG-2" || reps[0].CWD != dest {
		t.Fatalf("reports=%+v", reps)
	}
	tf, _ := board.ReadTaskFile(root, "PING-2")
	if tf.Status != "reviewing" {
		t.Errorf("status=%s", tf.Status)
	}
}

func TestMessageAndLetterReports(t *testing.T) {
	inbox := filepath.Join(t.TempDir(), "inbox")
	r := MessageReport(inbox, "codex-live", "", strings.Repeat("가", MaxMessageRunes+5), time.Now())
	if r.To != "MESSAGE" || r.TaskID != "-" || r.From != "codex-live" || r.Session != "codex-live" || r.Kind != "message" {
		t.Errorf("%+v", r)
	}
	if n := len([]rune(r.Task)); n != MaxMessageRunes+1 {
		t.Errorf("본문 절단 %d", n)
	}
	if _, err := WriteReport(r); err != nil {
		t.Fatal(err)
	}
	l := LetterReport(inbox, remote.Letter{ID: "t_m1", From: "default", Text: "정리본", TaskID: "VIDEO-07", At: time.Unix(1790320000, 0)}, time.Now())
	if l.TaskID != "VIDEO-07" || l.From != "default" || l.Task != "정리본" || l.To != "MESSAGE" {
		t.Errorf("%+v", l)
	}
	if _, err := WriteReport(l); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Poll(inbox)
	if err != nil || !ok || got.To != "MESSAGE" {
		t.Fatalf("Poll: %+v ok=%v err=%v", got, ok, err)
	}
}

type fakeAdapter struct {
	status  remote.Status
	letters []remote.Letter
	pulled  []string
	pollErr error
}

func (f *fakeAdapter) Dispatch(context.Context, string, string, string, remote.Handle) (remote.Handle, error) {
	return "h", nil
}
func (f *fakeAdapter) Poll(context.Context, remote.Handle) (remote.Status, error) {
	return f.status, f.pollErr
}
func (f *fakeAdapter) Reply(context.Context, remote.Handle, string) error { return nil }
func (f *fakeAdapter) Pull(_ context.Context, _ remote.Handle, dest string) error {
	f.pulled = append(f.pulled, dest)
	return os.MkdirAll(dest, 0o755)
}
func (f *fakeAdapter) Mailbox(context.Context) ([]remote.Letter, error) {
	l := f.letters
	f.letters = nil
	return l, nil
}
func (f *fakeAdapter) Finish(context.Context, remote.Handle) error { return nil }
func (f *fakeAdapter) Check(context.Context) (remote.Info, error)  { return remote.Info{}, nil }

func TestPollRemotesOncePullsOnDoneAndDeliversLetters(t *testing.T) {
	stateDir, root, inbox := remoteFixture(t)
	if err := remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"}); err != nil {
		t.Fatal(err)
	}
	fa := &fakeAdapter{status: remote.Status{State: state.StateDoneUnread, Summary: "PONG-2", Seen: 5},
		letters: []remote.Letter{{ID: "m1", From: "default", Text: "편지", At: time.Now()}}}
	open := func(name string) (remote.Adapter, *remote.Remote, error) {
		r, _, _ := remote.Load(stateDir, name)
		return fa, r, nil
	}
	var warns []string
	PollRemotesOnce(context.Background(), stateDir, inbox, open, func(s string) { warns = append(warns, s) }, time.Now())
	dest := filepath.Join(root, "결과물", "PING-2", "remote")
	if len(fa.pulled) != 1 || fa.pulled[0] != dest {
		t.Errorf("done이면 결과물/<ID>/remote/로 회수: %v", fa.pulled)
	}
	reps := pendingReports(t, inbox)
	var tos []string
	for _, r := range reps {
		tos = append(tos, r.To)
	}
	if len(reps) != 2 || !strings.Contains(strings.Join(tos, ","), "DONE_UNREAD") || !strings.Contains(strings.Join(tos, ","), "MESSAGE") {
		t.Fatalf("reports=%v warns=%v", tos, warns)
	}
	PollRemotesOnce(context.Background(), stateDir, inbox, open, func(s string) { warns = append(warns, s) }, time.Now())
	if len(pendingReports(t, inbox)) != 2 {
		t.Error("변화 없으면 무음")
	}
}

func TestPollRemotesOnceUnreachableIsQuietThenReportsOnce(t *testing.T) {
	stateDir, _, inbox := remoteFixture(t)
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	fa := &fakeAdapter{pollErr: os.ErrDeadlineExceeded}
	open := func(name string) (remote.Adapter, *remote.Remote, error) {
		r, _, _ := remote.Load(stateDir, name)
		return fa, r, nil
	}
	var warns []string
	warn := func(s string) { warns = append(warns, s) }
	t0 := time.Now()
	PollRemotesOnce(context.Background(), stateDir, inbox, open, warn, t0)
	PollRemotesOnce(context.Background(), stateDir, inbox, open, warn, t0.Add(1*time.Minute))
	if len(pendingReports(t, inbox)) != 0 || len(warns) != 2 {
		t.Fatalf("5분 전에는 stderr만: reports=%d warns=%d", len(pendingReports(t, inbox)), len(warns))
	}
	PollRemotesOnce(context.Background(), stateDir, inbox, open, warn, t0.Add(6*time.Minute))
	PollRemotesOnce(context.Background(), stateDir, inbox, open, warn, t0.Add(7*time.Minute))
	reps := pendingReports(t, inbox)
	if len(reps) != 1 || reps[0].To != "ERROR" || !strings.Contains(reps[0].Task, "원격 연결 실패") {
		t.Fatalf("5분 넘으면 ERROR 한 번: %+v", reps)
	}
}
