package task

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

// 연결 실패는 등록 파일에 남아야 한다 — task list는 task watch와 다른 프로세스라 메모리의 outages를 못 본다.
func TestPollRemotesOnceRecordsUnreachableAndClears(t *testing.T) {
	stateDir, _, inbox := remoteFixture(t)
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	delete(outages, "hermes-qa")
	t.Cleanup(func() { delete(outages, "hermes-qa") })
	as, _, _ := Load(stateDir, RemoteAgentID("hermes-qa"))
	as.Remote.LastState = state.StateWorking
	Save(stateDir, *as)
	fa := &fakeAdapter{pollErr: os.ErrDeadlineExceeded}
	open := func(name string) (remote.Adapter, *remote.Remote, error) {
		r, _, _ := remote.Load(stateDir, name)
		return fa, r, nil
	}
	warn := func(string) {}
	t0 := time.Unix(1790000000, 0)
	PollRemotesOnce(context.Background(), stateDir, inbox, open, warn, t0, false)
	PollRemotesOnce(context.Background(), stateDir, inbox, open, warn, t0.Add(time.Minute), false)
	as, _, _ = Load(stateDir, RemoteAgentID("hermes-qa"))
	if as.Remote.UnreachableSince != t0.Unix() {
		t.Fatalf("첫 실패 시각이 남아야 함: %+v", as.Remote)
	}
	if as.Remote.LastState != state.StateWorking || as.Remote.Handle != "t_1" {
		t.Errorf("마지막 관측 상태·handle은 그대로: %+v", as.Remote)
	}
	// 다시 닿으면(상태 변화가 없어도) 지운다
	fa.pollErr = nil
	fa.status = remote.Status{State: state.StateWorking}
	PollRemotesOnce(context.Background(), stateDir, inbox, open, warn, t0.Add(2*time.Minute), false)
	as, _, _ = Load(stateDir, RemoteAgentID("hermes-qa"))
	if as.Remote.UnreachableSince != 0 {
		t.Errorf("연결이 돌아오면 표시를 지운다: %+v", as.Remote)
	}
	// 끊겼다가 돌아오며 상태까지 바뀐 경우에도 지워진 채로 저장된다
	fa.pollErr = os.ErrDeadlineExceeded
	PollRemotesOnce(context.Background(), stateDir, inbox, open, warn, t0.Add(3*time.Minute), false)
	fa.pollErr = nil
	fa.status = remote.Status{State: state.StateWaiting, Ask: "어느 파일?", Seen: 5}
	PollRemotesOnce(context.Background(), stateDir, inbox, open, warn, t0.Add(4*time.Minute), false)
	as, _, _ = Load(stateDir, RemoteAgentID("hermes-qa"))
	if as.Remote.UnreachableSince != 0 || as.Remote.LastState != state.StateWaiting {
		t.Errorf("복구 + 전이: %+v", as.Remote)
	}
}

// 폴러가 사본을 든 사이 등록이 바뀌었으면(send의 handle 교체·task done) 표시하지 않는다.
func TestUnreachableMarkDoesNotOverwriteNewerAssignment(t *testing.T) {
	stateDir, _, _ := remoteFixture(t)
	stale, _, _ := Load(stateDir, RemoteAgentID("hermes-qa"))
	cur := *stale
	cur.Remote = &RemoteRef{Name: "hermes-qa", Handle: "t_new", LastState: state.StateWorking}
	Save(stateDir, cur)
	if err := setUnreachable(stateDir, stale, 123); err != nil {
		t.Fatal(err)
	}
	got, _, _ := Load(stateDir, RemoteAgentID("hermes-qa"))
	if got.Remote.Handle != "t_new" || got.Remote.UnreachableSince != 0 {
		t.Errorf("handle이 바뀐 등록은 건드리지 않는다: %+v", got.Remote)
	}
	Done(stateDir, "PING-2")
	if err := setUnreachable(stateDir, stale, 123); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Load(stateDir, RemoteAgentID("hermes-qa")); ok {
		t.Error("지운 등록을 되살리면 안 됨")
	}
}
