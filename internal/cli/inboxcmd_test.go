package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/netwaif/agentlayer/internal/remote"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

// `inbox wait`: 앱 세션이 Bash로 백그라운드 실행해 편지 한 통을 받는 명령. 테스트는 세션 PID·생사 판정을 끼운다.

func stubSession(t *testing.T, pid int, alive map[int]bool) {
	t.Helper()
	oldPID, oldAlive := sessionPIDFn, pidAliveFn
	sessionPIDFn = func() int { return pid }
	pidAliveFn = func(p int) bool { return alive[p] }
	t.Cleanup(func() { sessionPIDFn, pidAliveFn = oldPID, oldAlive })
}

var tnow = func() time.Time { return time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC) }

// waitInbox는 inbox wait를 고루틴으로 띄우고 주소록 항목이 생길 때까지 기다린다.
func waitInbox(t *testing.T, ctx context.Context, stateDir, name string, extra ...string) (done chan error, out, errb *bytes.Buffer) {
	t.Helper()
	out, errb = &bytes.Buffer{}, &bytes.Buffer{}
	args := append([]string{"wait", "--interval", "10ms", "--timeout", "3s"}, extra...)
	done = make(chan error, 1)
	go func() { done <- RunInboxWait(ctx, out, errb, stateDir, args, tnow) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, found, _ := LoadAddress(stateDir, name); found {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("주소록에 %s가 생기지 않음: %s", name, errb.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// 등록 → send <이름> → 편지 한 통을 stdout으로 → 종료 시 주소록 삭제. send는 via=inbox.
func TestInboxWaitReceivesOneLetter(t *testing.T) {
	stubSession(t, 4242, map[int]bool{4242: true})
	stateDir := t.TempDir()
	cwd, _ := os.Getwd()
	name := filepath.Base(cwd)
	ctx := context.Background()
	done, out, errb := waitInbox(t, ctx, stateDir, name)
	addr, _, _ := LoadAddress(stateDir, name)
	if addr.PID != 4242 || addr.Inbox != AddressInbox(stateDir, 4242) || addr.CWD != cwd || addr.RegisteredAt != tnow() {
		t.Errorf("주소록: %+v", addr)
	}
	if _, err := os.Stat(filepath.Join(addr.Inbox, "pending")); err != nil {
		t.Errorf("수신함 폴더가 있어야 함: %v", err)
	}
	st, _ := state.NewStore(stateDir)
	var sout bytes.Buffer
	if err := RunSend(ctx, &sout, strings.NewReader("첫 줄\n둘째 줄\n"), st, stateDir, &fakeSender{}, []string{"--json", name, "-"}); err != nil {
		t.Fatalf("send: %v (stderr: %s)", err, errb.String())
	}
	var res map[string]any
	_ = json.Unmarshal(sout.Bytes(), &res)
	if res["via"] != "inbox" || res["session"] != name || res["sent"] != true {
		t.Errorf("JSON: %s", sout.String())
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("정상 종료여야 함: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("편지를 받고 끝나지 않음")
	}
	if out.String() != "from: user\n\n첫 줄\n둘째 줄\n" {
		t.Errorf("stdout: %q", out.String())
	}
	if _, found, _ := LoadAddress(stateDir, name); found {
		t.Error("끝나면 주소록 항목을 지워야 함")
	}
	// 사람이 읽는 send 출력도 확인 — 다시 대기시켜 보낸다
	done, _, _ = waitInbox(t, ctx, stateDir, name)
	sout.Reset()
	if err := RunSend(ctx, &sout, nil, st, stateDir, &fakeSender{}, []string{name, "둘"}); err != nil {
		t.Fatal(err)
	}
	<-done
	if !strings.Contains(sout.String(), "전송 완료 → "+name+" (pid 4242) [inbox wait] (inbox)") {
		t.Errorf("출력: %s", sout.String())
	}
}

// 기간이 지나면 ErrInboxTimeout(종료 코드 2)이고 주소록 항목이 지워진다. 신호(ctx 취소)도 항목을 지운다.
func TestInboxWaitTimeoutAndSignal(t *testing.T) {
	stubSession(t, 4243, map[int]bool{4243: true})
	stateDir := t.TempDir()
	var out, errb bytes.Buffer
	err := RunInboxWait(context.Background(), &out, &errb, stateDir, []string{"wait", "--name", "짧게", "--timeout", "100ms", "--interval", "10ms"}, tnow)
	if !errors.Is(err, ErrInboxTimeout) || !strings.Contains(err.Error(), "답 없음(100ms)") {
		t.Errorf("타임아웃: %v", err)
	}
	if _, found, _ := LoadAddress(stateDir, "짧게"); found {
		t.Error("타임아웃 뒤 주소록 항목을 지워야 함")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done, _, _ := waitInbox(t, ctx, stateDir, "짧게", "--name", "짧게")
	cancel()
	select {
	case err := <-done:
		if err == nil || errors.Is(err, ErrInboxTimeout) || !strings.Contains(err.Error(), "중단") {
			t.Errorf("신호 중단: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("취소 뒤 끝나지 않음")
	}
	if _, found, _ := LoadAddress(stateDir, "짧게"); found {
		t.Error("중단 뒤 주소록 항목을 지워야 함")
	}
}

// 시작 시 pending에 이미 있던 편지는 옛 세션 앞으로 온 것 — quarantine으로 치우고 전달하지 않는다.
func TestInboxWaitPurgesStaleLetters(t *testing.T) {
	stubSession(t, 4244, map[int]bool{4244: true})
	stateDir := t.TempDir()
	inbox := AddressInbox(stateDir, 4244)
	r := DirectiveReport(inbox, "총괄", "", "낡은 지시", time.Now())
	if _, err := task.WriteReport(r); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	err := RunInboxWait(context.Background(), &out, &errb, stateDir, []string{"wait", "--name", "n", "--timeout", "150ms", "--interval", "10ms"}, tnow)
	if !errors.Is(err, ErrInboxTimeout) || out.Len() != 0 {
		t.Errorf("낡은 편지를 전달하면 안 됨: err=%v out=%q", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(inbox, "quarantine", "stale-"+r.ID+".json")); err != nil {
		t.Errorf("quarantine으로 치워야 함: %v", err)
	}
	if !strings.Contains(errb.String(), "quarantine") {
		t.Errorf("stderr 안내: %s", errb.String())
	}
}

// 이름 충돌: 다른 산 세션이 같은 이름이면 pid 끝 4자리를 붙인다. 죽은 항목은 덮어쓴다.
func TestInboxWaitNameCollision(t *testing.T) {
	stubSession(t, 4245, map[int]bool{4245: true, 111: true})
	stateDir := t.TempDir()
	if err := saveAddress(stateDir, Address{Name: "n", PID: 111, Inbox: AddressInbox(stateDir, 111)}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done, _, _ := waitInbox(t, ctx, stateDir, "n-4245", "--name", "n")
	cancel()
	<-done
	if a, found, _ := LoadAddress(stateDir, "n"); !found || a.PID != 111 {
		t.Error("남의 산 항목은 건드리지 않는다")
	}
	// 죽은 항목은 덮어쓴다
	stubSession(t, 4246, map[int]bool{4246: true})
	ctx, cancel = context.WithCancel(context.Background())
	done, _, _ = waitInbox(t, ctx, stateDir, "n", "--name", "n")
	deadline := time.Now().Add(3 * time.Second)
	for {
		a, _, _ := LoadAddress(stateDir, "n")
		if a != nil && a.PID == 4246 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("죽은 항목 덮어쓰기: %+v", a)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
}

// send <이름>: 항목은 있는데 PID가 죽었으면 항목을 지우고 오류. 아무도 집어 가지 않으면 회수하고 오류(폴백 없음).
func TestSendToAddressDeadOrUnattended(t *testing.T) {
	stubSession(t, 0, map[int]bool{5000: true})
	old := channelDeliverWait
	channelDeliverWait = 100 * time.Millisecond
	defer func() { channelDeliverWait = old }()
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = saveAddress(stateDir, Address{Name: "dead", PID: 4999, Inbox: AddressInbox(stateDir, 4999)})
	_ = saveAddress(stateDir, Address{Name: "busy", PID: 5000, Inbox: AddressInbox(stateDir, 5000)})
	f := &fakeSender{}
	var out bytes.Buffer
	err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"dead", "x"})
	if err == nil || !strings.Contains(err.Error(), "죽어") {
		t.Errorf("죽은 주소: %v", err)
	}
	if _, found, _ := LoadAddress(stateDir, "dead"); found {
		t.Error("죽은 항목은 지워야 함")
	}
	err = RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"busy", "x"})
	if err == nil || !strings.Contains(err.Error(), "집어 가지 않아") {
		t.Errorf("미수령: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(AddressInbox(stateDir, 5000), "pending", "*.json")); len(left) != 0 {
		t.Errorf("편지를 회수해야 함: %v", left)
	}
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"--file", "a", "busy", "x"}); !errors.Is(err, errNoFileHere) {
		t.Errorf("--file은 주소록 경로에 없음: %v", err)
	}
	if f.calls != 0 {
		t.Error("tmux 폴백 없음")
	}
	// 주소록에 없는 이름은 예전 오류 그대로
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"nobody", "x"}); err == nil || !strings.Contains(err.Error(), "찾지 못했습니다") {
		t.Errorf("예전 오류: %v", err)
	}
}

// tmux 세션 이름이 같으면 tmux가 우선한다(기존 동작 유지).
func TestSendPrefersTmuxOverAddress(t *testing.T) {
	stubSession(t, 0, map[int]bool{5001: true})
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(mkAgent("claude", "collab-bot", "%1", state.StateIdle))
	_ = saveAddress(stateDir, Address{Name: "collab-bot", PID: 5001, Inbox: AddressInbox(stateDir, 5001)})
	f := &fakeSender{}
	var out bytes.Buffer
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"collab-bot", "x"}); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 || f.pane != "%1" {
		t.Errorf("tmux 우선: %+v", f)
	}
}

func TestInboxUsageAndArgs(t *testing.T) {
	var out, errb bytes.Buffer
	for _, args := range [][]string{{}, {"list"}, {"wait", "--timeout", "x"}, {"wait", "--bogus"}, {"wait", "--name"}} {
		if err := RunInboxWait(context.Background(), &out, &errb, t.TempDir(), args, tnow); err == nil {
			t.Errorf("%v: 오류여야 함", args)
		}
	}
	if !validAddressName("기획서") || validAddressName("a/b") || validAddressName(".hidden") || validAddressName("") {
		t.Error("이름 검증")
	}
}

// 연결 모드(inbox open): 고유 주소 ID 발급·별칭, 대기가 꺼진 사이에 온 편지는 큐에 남고 다음 wait가 집는다, 주소는 close까지 유지.
func TestInboxOpenKeepQueuesLettersAcrossWaits(t *testing.T) {
	stubSession(t, 5150, map[int]bool{5150: true})
	stateDir := t.TempDir()
	ctx := context.Background()
	var out, errb bytes.Buffer
	if err := RunInboxWait(ctx, &out, &errb, stateDir, []string{"open", "--name", "pair"}, tnow); err != nil {
		t.Fatalf("open: %v %s", err, errb.String())
	}
	id := strings.TrimSpace(out.String())
	if !strings.HasPrefix(id, "al-") || len(id) != 9 {
		t.Fatalf("고유 주소 ID 기대(al-6자): %q", id)
	}
	byID, found, _ := LoadAddress(stateDir, id)
	byName, found2, _ := LoadAddress(stateDir, "pair")
	if !found || !found2 || byID.ID != id || byName.ID != id || !byID.Keep || byID.PID != 5150 {
		t.Fatalf("주소록 id·별칭 둘 다 keep으로 있어야 함: %+v / %+v", byID, byName)
	}
	// 같은 세션이 다시 open → 같은 ID(멱등)
	out.Reset()
	if err := RunInboxWait(ctx, &out, &errb, stateDir, []string{"open", "--name", "pair"}, tnow); err != nil || strings.TrimSpace(out.String()) != id {
		t.Fatalf("open 재실행은 같은 ID: %q err=%v", out.String(), err)
	}
	// 대기가 꺼진 상태에서 send → 회수하지 않고 큐에 남긴다
	st, _ := state.NewStore(stateDir)
	var sout bytes.Buffer
	if err := RunSend(ctx, &sout, nil, st, stateDir, &fakeSender{}, []string{"--json", id, "첫 편지"}); err != nil {
		t.Fatalf("send(대기 없음): %v", err)
	}
	var res map[string]any
	_ = json.Unmarshal(sout.Bytes(), &res)
	if res["via"] != "inbox" || res["sent"] != true {
		t.Errorf("JSON: %s", sout.String())
	}
	if m, _ := filepath.Glob(filepath.Join(byID.Inbox, "pending", "*.json")); len(m) != 1 {
		t.Fatalf("편지가 pending에 남아야 함: %v", m)
	}
	// 다음 wait가 그 편지를 집는다(purge 안 함), 끝나도 주소는 남는다
	out.Reset()
	if err := RunInboxWait(ctx, &out, &errb, stateDir, []string{"wait", "--name", "pair", "--interval", "10ms", "--timeout", "3s"}, tnow); err != nil {
		t.Fatalf("wait: %v %s", err, errb.String())
	}
	if !strings.Contains(out.String(), "첫 편지") {
		t.Errorf("큐에 있던 편지를 받아야 함: %q", out.String())
	}
	if _, found, _ := LoadAddress(stateDir, id); !found {
		t.Fatal("keep 주소는 wait가 끝나도 남아야 함")
	}
	// 두 번째 편지도 같은 흐름
	if err := RunSend(ctx, &sout, nil, st, stateDir, &fakeSender{}, []string{id, "둘째 편지"}); err != nil {
		t.Fatalf("send 2: %v", err)
	}
	out.Reset()
	if err := RunInboxWait(ctx, &out, &errb, stateDir, []string{"wait", "--name", "pair", "--interval", "10ms", "--timeout", "3s"}, tnow); err != nil || !strings.Contains(out.String(), "둘째 편지") {
		t.Fatalf("wait 2: %v %q", err, out.String())
	}
	// close → id·별칭 모두 삭제
	if err := RunInboxWait(ctx, &out, &errb, stateDir, []string{"close", "--name", "pair"}, tnow); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, f1, _ := LoadAddress(stateDir, id); f1 {
		t.Error("close 뒤 id 주소가 남음")
	}
	if _, f2, _ := LoadAddress(stateDir, "pair"); f2 {
		t.Error("close 뒤 별칭이 남음")
	}
}

// --remote <이름>:<카드>: 원격 카드가 끝나면(또는 질문하면) 그 결과를 편지처럼 내준다 — 헤르메스 양방향(ssh·로컬 무관).
func TestInboxWaitPollsRemoteHandle(t *testing.T) {
	stubSession(t, 6161, map[int]bool{6161: true})
	stateDir := t.TempDir()
	if err := remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"}); err != nil {
		t.Fatal(err)
	}
	ad := &scriptedAdapter{handle: "card-9", status: remote.Status{State: state.StateDoneUnread, Summary: "DONE: #기술검증팀 — 12:00"}}
	stubRemoteAdapter(t, ad)
	var out, errb bytes.Buffer
	err := RunInboxWait(context.Background(), &out, &errb, stateDir, []string{"wait", "--name", "rq", "--interval", "10ms", "--timeout", "3s",
		"--remote", "hermes-qa:card-9", "--remote-interval", "20ms"}, tnow)
	if err != nil {
		t.Fatalf("wait: %v %s", err, errb.String())
	}
	if !strings.HasPrefix(out.String(), "from: hermes-qa\n") || !strings.Contains(out.String(), "DONE: #기술검증팀") {
		t.Errorf("원격 카드 결과를 편지 형식으로: %q", out.String())
	}
	// 질문(WAITING)도 그대로 전달한다
	ad.status = remote.Status{State: state.StateWaiting, Ask: "파일을 덮어쓸까요?"}
	out.Reset()
	if err := RunInboxWait(context.Background(), &out, &errb, stateDir, []string{"wait", "--name", "rq", "--interval", "10ms", "--timeout", "3s",
		"--remote", "hermes-qa:card-9", "--remote-interval", "20ms"}, tnow); err != nil {
		t.Fatalf("wait(ask): %v", err)
	}
	if !strings.Contains(out.String(), "[WAITING]") || !strings.Contains(out.String(), "덮어쓸까요") {
		t.Errorf("질문 전달: %q", out.String())
	}
	// 등록 안 된 원격은 즉시 오류
	if err := RunInboxWait(context.Background(), &out, &errb, stateDir, []string{"wait", "--remote", "nope:c1"}, tnow); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("모르는 원격: %v", err)
	}
}
