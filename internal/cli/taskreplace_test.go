package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

// --replace로 원격 업무를 다시 배정하면 서버의 옛 카드는 아무도 닫지 않는다 — 카드 ID를 적어 경고한다(삭제는 안 함).
func TestTaskAssignRemoteReplaceWarnsOrphanCard(t *testing.T) {
	st, stateDir := newStore(t)
	root := remoteCompanyRoot(t, "PING-3")
	inbox := filepath.Join(root, "runtime", "inbox")
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	task.Assign(stateDir, task.Assignment{TaskID: "PING-2", AgentID: task.RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote",
		Inbox: inbox, TaskDir: board.TaskDir(root, "PING-2"), AssignedAt: time.Now(),
		Remote: &task.RemoteRef{Name: "hermes-qa", Handle: "t_old1", LastState: state.StateError}}, false)
	fa := &finishAdapter{}
	prev := OpenRemote
	OpenRemote = func(remote.Remote, string) (remote.Adapter, error) { return fa, nil }
	t.Cleanup(func() { OpenRemote = prev })
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"assign", "PING-3", "hermes-qa", "--inbox", inbox, "--root", root, "--replace"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "⚠") || !strings.Contains(s, "t_old1") || !strings.Contains(s, "PING-2") {
		t.Errorf("옛 카드 ID·업무ID를 적은 경고가 있어야 함: %s", s)
	}
	if len(fa.finished) != 0 {
		t.Errorf("옛 카드를 자동으로 닫지 않는다: %v", fa.finished)
	}
	as, _, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa"))
	if as.TaskID != "PING-3" || as.Remote.Handle != "" {
		t.Errorf("새 등록: %+v %+v", as, as.Remote)
	}
}

// 카드를 만든 적 없는 등록(첫 send 전)을 바꿀 때는 남는 카드가 없으니 조용하다.
func TestTaskAssignRemoteReplaceWithoutCardIsQuiet(t *testing.T) {
	st, stateDir := newStore(t)
	root := remoteCompanyRoot(t, "PING-3")
	inbox := filepath.Join(root, "runtime", "inbox")
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	task.Assign(stateDir, task.Assignment{TaskID: "PING-2", AgentID: task.RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote",
		Inbox: inbox, AssignedAt: time.Now(), Remote: &task.RemoteRef{Name: "hermes-qa"}}, false)
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"assign", "PING-3", "hermes-qa", "--inbox", inbox, "--root", root, "--replace"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "옛 카드") {
		t.Errorf("카드가 없었으면 경고하지 않음: %s", out.String())
	}
}
