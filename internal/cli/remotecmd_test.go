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
	if !ok || r.Exec[0] != "docker" || len(r.Exec) != 6 || r.Mailbox != "company-manager" {
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
	// 상대경로 명령은 정의 파일 위치 기준으로 절대화한다 — task watch는 회사 루트에서 돌기 때문
	if r.Commands["dispatch"][0] != dir+"/d.sh" || r.Commands["poll"][0] != dir+"/p.sh" {
		t.Errorf("절대화: %v %v", r.Commands["dispatch"], r.Commands["poll"])
	}
}

type setupAdapter struct {
	checkOnlyAdapter
	calls *int
	err   error
}

func (s setupAdapter) Setup(context.Context) ([]string, error) {
	*s.calls++
	return []string{"/opt/data/.local/bin/company-letter"}, s.err
}

// add는 점검 뒤 편지 준비물을 깔고, --no-setup이면 건너뛴다. 설치가 실패해도 등록은 남는다.
func TestRemoteAddInstallsLetterSetup(t *testing.T) {
	st, dir := newStore(t)
	calls := 0
	var setupErr error
	open := func(r remote.Remote, _ string) (remote.Adapter, error) {
		return setupAdapter{calls: &calls, err: setupErr}, nil
	}
	base := []string{"--kind", "hermes", "--local", "--profile", "tech-qa", "--workspace-root", "/home/u/.hermes/ai-company/결과물"}
	var out bytes.Buffer
	if err := RunRemote(context.Background(), &out, st, dir, open, append([]string{"add", "h1"}, base...), time.Now()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(out.String(), "설치: /opt/data/.local/bin/company-letter") || !strings.Contains(out.String(), "담당자 company-manager") {
		t.Fatalf("calls=%d out=%s", calls, out.String())
	}
	if err := RunRemote(context.Background(), &out, st, dir, open, append([]string{"add", "h2", "--no-setup"}, base...), time.Now()); err != nil || calls != 1 {
		t.Fatalf("--no-setup이면 깔지 않는다: calls=%d err=%v", calls, err)
	}
	setupErr = errors.New("permission denied")
	out.Reset()
	if err := RunRemote(context.Background(), &out, st, dir, open, append([]string{"add", "h3"}, base...), time.Now()); err != nil {
		t.Fatalf("설치 실패는 등록을 막지 않는다: %v", err)
	}
	if _, ok, _ := remote.Load(dir, "h3"); !ok || !strings.Contains(out.String(), "remote setup h3") {
		t.Fatalf("등록은 남고 안내가 나와야 한다: %s", out.String())
	}
	out.Reset()
	setupErr = nil
	if err := RunRemote(context.Background(), &out, st, dir, open, []string{"setup", "h3"}, time.Now()); err != nil || calls != 3 {
		t.Fatalf("setup 명령: calls=%d err=%v", calls, err)
	}
	// 준비물이 없는 어댑터는 조용히 지나간다
	plain := func(r remote.Remote, _ string) (remote.Adapter, error) { return checkOnlyAdapter{}, nil }
	if err := RunRemote(context.Background(), &out, st, dir, plain, []string{"setup", "h3"}, time.Now()); err != nil {
		t.Fatal(err)
	}
}
