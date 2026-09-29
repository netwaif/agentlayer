package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

// 닿지 않는 원격의 상태 칸은 마지막 관측일 뿐이다 — 목록 행에 연결 끊김과 그 시간을 같이 보여 준다.
func TestTaskListShowsUnreachableRemote(t *testing.T) {
	st, stateDir := newStore(t)
	now := time.Unix(1790000000, 0)
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	remote.Save(stateDir, remote.Remote{Name: "hermes-ok", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	task.Assign(stateDir, task.Assignment{TaskID: "PING-2", AgentID: task.RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote", Inbox: "/i",
		AssignedAt: now.Add(-time.Hour), Remote: &task.RemoteRef{Name: "hermes-qa", Handle: "t_1", LastState: state.StateWorking,
			UnreachableSince: now.Add(-12 * time.Minute).Unix()}}, false)
	task.Assign(stateDir, task.Assignment{TaskID: "PING-3", AgentID: task.RemoteAgentID("hermes-ok"), Session: "hermes-ok", Pane: "remote", Inbox: "/i",
		AssignedAt: now.Add(-time.Hour), Remote: &task.RemoteRef{Name: "hermes-ok", Handle: "t_2", LastState: state.StateWorking}}, false)
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"list"}, now); err != nil {
		t.Fatal(err)
	}
	var bad, ok string
	for _, line := range strings.Split(out.String(), "\n") {
		switch {
		case strings.HasPrefix(line, "PING-2"):
			bad = line
		case strings.HasPrefix(line, "PING-3"):
			ok = line
		}
	}
	if !strings.Contains(bad, "WORK") || !strings.Contains(bad, "연결 끊김") || !strings.Contains(bad, "12m") {
		t.Errorf("끊긴 원격 행: %q", bad)
	}
	if ok == "" || strings.Contains(ok, "연결 끊김") {
		t.Errorf("멀쩡한 원격 행에는 표시 없음: %q", ok)
	}
	out.Reset()
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"list", "--json"}, now); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), `"unreachable":true`) != 1 || !strings.Contains(out.String(), `"unreachable_since":`) {
		t.Errorf("json=%s", out.String())
	}
}

// send가 원격에 닿았으면 끊김 표시를 지운다(task watch가 꺼져 있어도 낡은 표시가 남지 않게).
func TestSendRemoteClearsUnreachable(t *testing.T) {
	ad := &scriptedAdapter{handle: "t_new", status: remote.Status{State: state.StateDoneUnread}}
	st, stateDir, _ := remoteSendFixture(t, ad)
	as, _, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa"))
	as.Remote.Handle, as.Remote.UnreachableSince = "t_old", 1790000000
	task.Save(stateDir, *as)
	if err := RunSend(context.Background(), &bytes.Buffer{}, nil, st, stateDir, nil, []string{"hermes-qa", "후속"}); err != nil {
		t.Fatal(err)
	}
	as, _, _ = task.Load(stateDir, task.RemoteAgentID("hermes-qa"))
	if as.Remote.UnreachableSince != 0 {
		t.Errorf("닿았으면 지운다: %+v", as.Remote)
	}
}
