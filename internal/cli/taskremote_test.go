package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

type finishAdapter struct {
	remote.Adapter
	finished []string
	answered []string
	status   remote.Status
}

func (f *finishAdapter) Finish(_ context.Context, h remote.Handle) error {
	f.finished = append(f.finished, h)
	return nil
}
func (f *finishAdapter) Poll(context.Context, remote.Handle) (remote.Status, error) {
	return f.status, nil
}
func (f *finishAdapter) Mailbox(context.Context) ([]remote.Letter, error) { return nil, nil }
func (f *finishAdapter) Pull(_ context.Context, _ remote.Handle, d string) error {
	return os.MkdirAll(d, 0o755)
}
func (f *finishAdapter) Answer(_ context.Context, id, text string) error {
	f.answered = append(f.answered, id+"|"+text)
	return nil
}

func remoteCompanyRoot(t *testing.T, id string) string {
	t.Helper()
	root := t.TempDir()
	dir := board.TaskDir(root, id)
	os.MkdirAll(dir, 0o755)
	writeFile(t, filepath.Join(dir, "task.md"), "# 제목\n\n```yaml\nstatus: pending\nparents: []\n```\n")
	writeFile(t, filepath.Join(dir, "log.md"), "")
	return root
}

func TestTaskAssignRemote(t *testing.T) {
	st, stateDir := newStore(t)
	root := remoteCompanyRoot(t, "PING-2")
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "hostinger", Profile: "tech-qa", WorkspaceRoot: "/w"})
	var out bytes.Buffer
	err := RunTask(context.Background(), &out, st, stateDir, []string{"assign", "PING-2", "hermes-qa", "--inbox", filepath.Join(root, "runtime", "inbox"), "--root", root}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	as, ok, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa"))
	if !ok || as.Remote == nil || as.Remote.Name != "hermes-qa" || as.Session != "hermes-qa" || as.Pane != "remote" || as.TaskDir == "" {
		t.Fatalf("등록: %+v", as)
	}
	tf, _ := board.ReadTaskFile(root, "PING-2")
	if tf.Status != "in_progress" {
		t.Errorf("status=%s", tf.Status)
	}
	if last := board.ReadLastLog(root, "PING-2"); !strings.Contains(last, "[ASSIGN]") || !strings.Contains(last, "hermes:tech-qa@hostinger") {
		t.Errorf("log=%s", last)
	}
	if agents, _ := st.List(); len(agents) != 0 {
		t.Error("원격은 agents/에 저장하지 않는다")
	}
	err = RunTask(context.Background(), &out, st, stateDir, []string{"assign", "PING-3", "hermes-qa", "--inbox", filepath.Join(root, "runtime", "inbox")}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "--replace") {
		t.Errorf("중복 등록: %v", err)
	}
}

func TestTaskListShowsRemote(t *testing.T) {
	st, stateDir := newStore(t)
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	task.Assign(stateDir, task.Assignment{TaskID: "PING-2", AgentID: task.RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote", Inbox: "/i",
		AssignedAt: time.Now(), Remote: &task.RemoteRef{Name: "hermes-qa", Handle: "t_1", LastState: state.StateWaiting}}, false)
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"list"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if s := out.String(); !strings.Contains(s, "hermes-qa (hermes)") || !strings.Contains(s, "WAIT") || strings.Contains(s, "gone") {
		t.Errorf("list=%s", s)
	}
	out.Reset()
	RunTask(context.Background(), &out, st, stateDir, []string{"list", "--json"}, time.Now())
	if !strings.Contains(out.String(), `"handle":"t_1"`) || !strings.Contains(out.String(), `"state":"WAITING"`) {
		t.Errorf("json=%s", out.String())
	}
}

func TestTaskDoneFinishesRemote(t *testing.T) {
	st, stateDir := newStore(t)
	root := remoteCompanyRoot(t, "PING-2")
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	task.Assign(stateDir, task.Assignment{TaskID: "PING-2", AgentID: task.RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote",
		Inbox: filepath.Join(root, "runtime", "inbox"), TaskDir: board.TaskDir(root, "PING-2"), AssignedAt: time.Now(),
		Remote: &task.RemoteRef{Name: "hermes-qa", Handle: "t_1", LastState: state.StateDoneUnread}}, false)
	fa := &finishAdapter{}
	prev := OpenRemote
	OpenRemote = func(remote.Remote, string) (remote.Adapter, error) { return fa, nil }
	t.Cleanup(func() { OpenRemote = prev })
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"done", "PING-2"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(fa.finished) != 1 || fa.finished[0] != "t_1" {
		t.Errorf("Finish 호출: %v", fa.finished)
	}
	if _, ok, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa")); ok {
		t.Error("등록 해제")
	}
	tf, _ := board.ReadTaskFile(root, "PING-2")
	if tf.Status != "done" {
		t.Errorf("status=%s", tf.Status)
	}
}

func TestTaskWatchPollsRemote(t *testing.T) {
	st, stateDir := newStore(t)
	root := remoteCompanyRoot(t, "PING-2")
	inbox := filepath.Join(root, "runtime", "inbox")
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w", Poll: "1s"})
	task.Assign(stateDir, task.Assignment{TaskID: "PING-2", AgentID: task.RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote",
		Inbox: inbox, TaskDir: board.TaskDir(root, "PING-2"), AssignedAt: time.Now(), Remote: &task.RemoteRef{Name: "hermes-qa", Handle: "t_1", LastState: state.StateWorking}}, false)
	fa := &finishAdapter{status: remote.Status{State: state.StateDoneUnread, Summary: "PONG-2", Seen: 3}}
	prev := OpenRemote
	OpenRemote = func(remote.Remote, string) (remote.Adapter, error) { return fa, nil }
	t.Cleanup(func() { OpenRemote = prev })
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var out bytes.Buffer
	_ = RunTask(ctx, &out, st, stateDir, []string{"watch", inbox, "--once"}, time.Now())
	if !strings.Contains(out.String(), `"to":"DONE_UNREAD"`) || !strings.Contains(out.String(), `"session":"hermes-qa"`) {
		t.Errorf("watch가 원격 전이를 흘려야 함: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "결과물", "PING-2", "remote")); err != nil {
		t.Error("done이면 결과물/PING-2/remote/ 회수")
	}
}

func TestTaskReplyAnswersLetter(t *testing.T) {
	st, stateDir := newStore(t)
	root := remoteCompanyRoot(t, "X-1")
	inbox := filepath.Join(root, "runtime", "inbox")
	board.RememberRoot(stateDir, root)
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	rep := task.LetterReport(inbox, "hermes-qa", remote.Letter{ID: "t_m1", From: "default", Text: "편지"}, time.Now())
	task.WriteReport(rep)
	task.Poll(inbox)
	fa := &finishAdapter{}
	prev := OpenRemote
	OpenRemote = func(remote.Remote, string) (remote.Adapter, error) { return fa, nil }
	t.Cleanup(func() { OpenRemote = prev })
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"reply", "t_m1", "답장", "본문"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(fa.answered) != 1 || fa.answered[0] != "t_m1|답장 본문" || !strings.Contains(out.String(), "hermes-qa") {
		t.Errorf("answered=%v out=%s", fa.answered, out.String())
	}
}
