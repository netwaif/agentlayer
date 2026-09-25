package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

type checkOnlyAdapter struct {
	remote.Adapter
	err error
}

func (c checkOnlyAdapter) Check(context.Context) (remote.Info, error) {
	return remote.Info{Version: "Hermes Agent v0.20.0", ProfileOK: c.err == nil, RoundTrip: 120 * time.Millisecond}, c.err
}

func newStore(t *testing.T) (*state.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := state.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return st, dir
}

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteAddCheckListRm(t *testing.T) {
	st, dir := newStore(t)
	open := func(r remote.Remote, _ string) (remote.Adapter, error) { return checkOnlyAdapter{}, nil }
	var out bytes.Buffer
	args := []string{"add", "hermes-qa", "--kind", "hermes", "--ssh", "hostinger", "--profile", "tech-qa",
		"--exec", "docker exec -i -u hermes c1", "--workspace-root", "/opt/data/ai-company/결과물"}
	if err := RunRemote(context.Background(), &out, st, dir, open, args, time.Now()); err != nil {
		t.Fatal(err)
	}
	r, ok, _ := remote.Load(dir, "hermes-qa")
	if !ok || r.Exec[0] != "docker" || len(r.Exec) != 6 || r.Mailbox != "imac-manager" {
		t.Fatalf("등록: %+v", r)
	}
	if !strings.Contains(out.String(), "v0.20.0") {
		t.Errorf("add는 check 결과를 보여 준다: %s", out.String())
	}
	out.Reset()
	if err := RunRemote(context.Background(), &out, st, dir, open, []string{"list"}, time.Now()); err != nil || !strings.Contains(out.String(), "hermes-qa") {
		t.Errorf("list: %v %s", err, out.String())
	}
	out.Reset()
	if err := RunRemote(context.Background(), &out, st, dir, open, []string{"check", "hermes-qa"}, time.Now()); err != nil || !strings.Contains(out.String(), "tech-qa") {
		t.Errorf("check: %v %s", err, out.String())
	}
	if err := RunRemote(context.Background(), &out, st, dir, open, []string{"rm", "hermes-qa"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := remote.Load(dir, "hermes-qa"); ok {
		t.Error("rm 뒤 없어야 함")
	}
}

func TestRemoteAddRefusesOnCheckFailureUnlessNoCheck(t *testing.T) {
	st, dir := newStore(t)
	open := func(r remote.Remote, _ string) (remote.Adapter, error) {
		return checkOnlyAdapter{err: errors.New("프로필 없음")}, nil
	}
	base := []string{"add", "x", "--kind", "hermes", "--ssh", "h", "--profile", "p", "--workspace-root", "/w"}
	if err := RunRemote(context.Background(), &bytes.Buffer{}, st, dir, open, base, time.Now()); err == nil {
		t.Error("check 실패면 저장하지 않고 에러")
	}
	if _, ok, _ := remote.Load(dir, "x"); ok {
		t.Error("저장되면 안 됨")
	}
	if err := RunRemote(context.Background(), &bytes.Buffer{}, st, dir, open, append(base, "--no-check"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := remote.Load(dir, "x"); !ok {
		t.Error("--no-check면 저장")
	}
}

func TestRemoteAddRefusesLiveSessionName(t *testing.T) {
	st, dir := newStore(t)
	st.Save(&state.Agent{ID: "codex-3", Kind: "codex", State: state.StateIdle, Tmux: state.TmuxRef{Session: "codex-live", PaneID: "%3"}})
	open := func(r remote.Remote, _ string) (remote.Adapter, error) { return checkOnlyAdapter{}, nil }
	err := RunRemote(context.Background(), &bytes.Buffer{}, st, dir, open,
		[]string{"add", "codex-live", "--kind", "hermes", "--ssh", "h", "--profile", "p", "--workspace-root", "/w"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "tmux 세션") {
		t.Errorf("산 세션 이름과 같으면 거부: %v", err)
	}
}

func TestRemoteAddExecFromFile(t *testing.T) {
	st, dir := newStore(t)
	f := dir + "/oc.json"
	writeFile(t, f, `{"commands":{"dispatch":["./d.sh","{task_id}","{body_file}"],"poll":["./p.sh","{handle}"]},"poll":"10s"}`)
	open := func(r remote.Remote, _ string) (remote.Adapter, error) { return checkOnlyAdapter{}, nil }
	if err := RunRemote(context.Background(), &bytes.Buffer{}, st, dir, open, []string{"add", "oc", "--kind", "exec", "--file", f}, time.Now()); err != nil {
		t.Fatal(err)
	}
	r, _, _ := remote.Load(dir, "oc")
	if r.Kind != "exec" || len(r.Commands["dispatch"]) != 3 || r.PollInterval() != 10*time.Second {
		t.Errorf("%+v", r)
	}
}
