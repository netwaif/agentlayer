package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

// ctxAdapter는 받은 컨텍스트가 취소돼 있으면 실제 ssh처럼 그 에러로 끝난다.
type ctxAdapter struct {
	remote.Adapter
	polls, finishes []error // 호출 때의 ctx.Err()
}

func (c *ctxAdapter) Poll(ctx context.Context, _ remote.Handle) (remote.Status, error) {
	c.polls = append(c.polls, ctx.Err())
	return remote.Status{State: state.StateIdle}, ctx.Err()
}
func (c *ctxAdapter) Finish(ctx context.Context, _ remote.Handle) error {
	c.finishes = append(c.finishes, ctx.Err())
	return ctx.Err()
}

func canceledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// Ctrl-C(취소된 컨텍스트)가 send의 원격 호출까지 내려가야 ssh가 바로 끊긴다.
func TestSendRemotePassesContext(t *testing.T) {
	ad := &ctxAdapter{}
	st, stateDir, _ := remoteSendFixture(t, &scriptedAdapter{})
	OpenRemote = func(remote.Remote, string) (remote.Adapter, error) { return ad, nil }
	as, _, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa"))
	as.Remote.Handle = "t_1"
	task.Save(stateDir, *as)
	err := RunSend(canceledCtx(), &bytes.Buffer{}, nil, st, stateDir, nil, []string{"hermes-qa", "지시"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("취소는 에러로 돌아와야 함: %v", err)
	}
	if len(ad.polls) != 1 || !errors.Is(ad.polls[0], context.Canceled) {
		t.Errorf("어댑터가 받은 ctx가 취소돼 있어야 함: %v", ad.polls)
	}
}

// task done의 원격 카드 마감도 같은 컨텍스트를 받는다. 마감 실패는 경고일 뿐 — 등록 해제는 그대로.
func TestTaskDonePassesContext(t *testing.T) {
	st, stateDir := newStore(t)
	root := remoteCompanyRoot(t, "PING-2")
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	task.Assign(stateDir, task.Assignment{TaskID: "PING-2", AgentID: task.RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote",
		Inbox: filepath.Join(root, "runtime", "inbox"), TaskDir: board.TaskDir(root, "PING-2"), AssignedAt: time.Now(),
		Remote: &task.RemoteRef{Name: "hermes-qa", Handle: "t_1", LastState: state.StateDoneUnread}}, false)
	ad := &ctxAdapter{}
	prev := OpenRemote
	OpenRemote = func(remote.Remote, string) (remote.Adapter, error) { return ad, nil }
	t.Cleanup(func() { OpenRemote = prev })
	var out bytes.Buffer
	if err := RunTask(canceledCtx(), &out, st, stateDir, []string{"done", "PING-2"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(ad.finishes) != 1 || !errors.Is(ad.finishes[0], context.Canceled) {
		t.Errorf("Finish가 받은 ctx가 취소돼 있어야 함: %v", ad.finishes)
	}
	if _, ok, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa")); ok {
		t.Error("마감 실패여도 등록 해제는 진행")
	}
}
