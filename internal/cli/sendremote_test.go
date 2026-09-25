package cli

import (
	"bytes"
	"context"
	"errors"
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

type scriptedAdapter struct {
	remote.Adapter
	status      remote.Status
	dispatched  []string // "taskID|title|body|parent"
	replied     []string
	dispatchErr error
	handle      string
}

func (s *scriptedAdapter) Dispatch(_ context.Context, id, title, body string, parent remote.Handle) (remote.Handle, error) {
	s.dispatched = append(s.dispatched, id+"|"+title+"|"+body+"|"+parent)
	return s.handle, s.dispatchErr
}
func (s *scriptedAdapter) Poll(context.Context, remote.Handle) (remote.Status, error) {
	return s.status, nil
}
func (s *scriptedAdapter) Reply(_ context.Context, _ remote.Handle, text string) error {
	s.replied = append(s.replied, text)
	return nil
}

// 회사 루트 + 원격 등록 + 원격 업무 등록. 어댑터는 ad로 바꿔 끼운다.
func remoteSendFixture(t *testing.T, ad *scriptedAdapter) (st *state.Store, stateDir, root string) {
	t.Helper()
	st, stateDir = newStore(t)
	root = t.TempDir()
	dir := board.TaskDir(root, "PING-2")
	os.MkdirAll(dir, 0o755)
	writeFile(t, filepath.Join(dir, "task.md"), "# 연결 시험\n\n```yaml\nstatus: in_progress\nparents: []\n```\n")
	writeFile(t, filepath.Join(dir, "log.md"), "")
	if err := remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "tech-qa", WorkspaceRoot: "/w"}); err != nil {
		t.Fatal(err)
	}
	if err := task.Assign(stateDir, task.Assignment{TaskID: "PING-2", AgentID: task.RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote",
		Inbox: filepath.Join(root, "runtime", "inbox"), TaskDir: dir, AssignedAt: time.Now(), Remote: &task.RemoteRef{Name: "hermes-qa"}}, false); err != nil {
		t.Fatal(err)
	}
	prev := OpenRemote
	OpenRemote = func(remote.Remote, string) (remote.Adapter, error) { return ad, nil }
	t.Cleanup(func() { OpenRemote = prev })
	return
}

func TestSendRemoteDispatchesFirstTime(t *testing.T) {
	ad := &scriptedAdapter{handle: "t_new", status: remote.Status{State: state.StateIdle}}
	st, stateDir, root := remoteSendFixture(t, ad)
	var out bytes.Buffer
	if err := RunSend(&out, strings.NewReader("본문 첫 줄\n둘째 줄\n"), st, stateDir, nil, []string{"hermes-qa", "-"}); err != nil {
		t.Fatal(err)
	}
	if len(ad.dispatched) != 1 || ad.dispatched[0] != "PING-2|연결 시험|본문 첫 줄\n둘째 줄|" {
		t.Errorf("dispatched=%v", ad.dispatched)
	}
	as, _, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa"))
	if as.Remote.Handle != "t_new" || as.Remote.LastState != state.StateWorking {
		t.Errorf("등록 갱신: %+v", as.Remote)
	}
	if !strings.Contains(board.ReadLastLog(root, "PING-2"), "[SEND]") {
		t.Error("[SEND] 로그")
	}
	if !strings.Contains(out.String(), "hermes-qa") {
		t.Errorf("출력: %s", out.String())
	}
}

func TestSendRemoteRetryAfterDispatchFailure(t *testing.T) {
	ad := &scriptedAdapter{handle: "t_new", dispatchErr: errors.New("카드 t_new 기동 실패: skipped_per_profile_capped"), status: remote.Status{State: state.StateIdle}}
	st, stateDir, _ := remoteSendFixture(t, ad)
	err := RunSend(&bytes.Buffer{}, nil, st, stateDir, nil, []string{"hermes-qa", "지시"})
	if err == nil || !strings.Contains(err.Error(), "skipped_per_profile_capped") {
		t.Fatalf("기동 실패는 에러: %v", err)
	}
	as, _, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa"))
	if as.Remote.Handle != "t_new" {
		t.Errorf("카드는 남긴다(handle 저장): %+v", as.Remote)
	}
	ad.dispatchErr = nil
	if err := RunSend(&bytes.Buffer{}, nil, st, stateDir, nil, []string{"hermes-qa", "지시"}); err != nil {
		t.Fatal(err)
	}
	if len(ad.dispatched) != 2 {
		t.Errorf("재시도 Dispatch: %v", ad.dispatched)
	}
}

func TestSendRemoteGate(t *testing.T) {
	cases := []struct {
		state   state.AgentState
		wantErr string
		replies int
		disp    int
		parent  string
	}{
		{state.StateWaiting, "", 1, 0, ""},
		{state.StateWorking, "작업 중", 0, 0, ""},
		{state.StateError, "비정상", 0, 0, ""},
		{state.StateDoneUnread, "", 0, 1, "t_old"},
	}
	for _, c := range cases {
		ad := &scriptedAdapter{handle: "t_next", status: remote.Status{State: c.state}}
		st, stateDir, _ := remoteSendFixture(t, ad)
		as, _, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa"))
		as.Remote.Handle, as.Remote.LastState = "t_old", c.state
		task.Save(stateDir, *as)
		err := RunSend(&bytes.Buffer{}, nil, st, stateDir, nil, []string{"hermes-qa", "답"})
		if c.wantErr == "" && err != nil {
			t.Errorf("%s: %v", c.state, err)
		}
		if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("%s: want %q got %v", c.state, c.wantErr, err)
		}
		if len(ad.replied) != c.replies || len(ad.dispatched) != c.disp {
			t.Errorf("%s: replied=%v dispatched=%v", c.state, ad.replied, ad.dispatched)
		}
		if c.disp == 1 && !strings.HasSuffix(ad.dispatched[0], "|"+c.parent) {
			t.Errorf("%s: DONE 뒤 지시는 parent=%s: %v", c.state, c.parent, ad.dispatched)
		}
	}
}

func TestSendRemoteRequiresAssignment(t *testing.T) {
	st, stateDir := newStore(t)
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	err := RunSend(&bytes.Buffer{}, nil, st, stateDir, nil, []string{"hermes-qa", "지시"})
	if err == nil || !strings.Contains(err.Error(), "task assign") {
		t.Errorf("등록 없으면 task assign 안내: %v", err)
	}
}
