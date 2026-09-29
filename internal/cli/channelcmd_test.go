package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

func init() {
	channelSettle = 150 * time.Millisecond
	channelLockRetry = 20 * time.Millisecond
}

func TestLetterNotificationMeta(t *testing.T) {
	local := &task.Report{Version: 1, ID: strings.Repeat("a", 32), TaskID: "T-1", Session: "collab-bot", Kind: "claude", From: "WORKING", To: "DONE_UNREAD"}
	n := LetterNotification(local)
	want := map[string]string{"task": "T-1", "event": "DONE_UNREAD", "letter_id": local.ID, "origin": "local", "session": "collab-bot", "kind": "claude"}
	for k, v := range want {
		if n.Meta[k] != v {
			t.Errorf("meta[%s]=%q want %q", k, n.Meta[k], v)
		}
	}
	var back task.Report
	if err := json.Unmarshal([]byte(n.Content), &back); err != nil || back.TaskID != "T-1" {
		t.Errorf("content는 편지 JSON: %q", n.Content)
	}
	for _, r := range []*task.Report{
		{Kind: "hermes", Session: "hermes-qa", To: "DONE_UNREAD"},
		{Kind: "message", Session: "hermes-qa", To: "MESSAGE", Letter: "t_1234"},
	} {
		if got := LetterNotification(r).Meta["origin"]; got != "remote" {
			t.Errorf("%+v origin=%q want remote", r, got)
		}
	}
	if got := LetterNotification(&task.Report{Kind: "message", To: "MESSAGE"}).Meta["origin"]; got != "local" {
		t.Errorf("로컬 편지 origin=%q", got)
	}
}

func TestRunChannelUsageAndArgs(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	var out, errb bytes.Buffer
	if err := RunChannel(context.Background(), strings.NewReader(""), &out, &errb, st, t.TempDir(), "1.10.0", nil); err == nil || !strings.Contains(err.Error(), "사용법") {
		t.Errorf("인자 없음 → 사용법: %v", err)
	}
	if err := RunChannel(context.Background(), strings.NewReader(""), &out, &errb, st, t.TempDir(), "1.10.0", []string{"serve", t.TempDir(), "--bogus"}); err == nil || !strings.Contains(err.Error(), "알 수 없는 인자") {
		t.Errorf("모르는 인자 거부: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("오류 경로에서 stdout에 아무것도 쓰면 안 됨: %q", out.String())
	}
}

func TestRunChannelDeliversPendingLetter(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.NewStore(dir)
	inbox := filepath.Join(t.TempDir(), "inbox")
	id := strings.Repeat("b", 32)
	rep := &task.Report{Version: 1, ID: id, TaskID: "T-9", Session: "s", Kind: "claude", From: "WORKING", To: "WAITING", Ask: "승인?", At: time.Now(), Inbox: inbox}
	if _, err := task.WriteReport(rep); err != nil {
		t.Fatal(err)
	}
	inr, inw := io.Pipe()
	outr, outw := io.Pipe()
	var errb bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- RunChannel(context.Background(), inr, outw, &errb, st, dir, "1.10.0", []string{"serve", inbox, "--interval", "20ms"})
		outw.Close()
	}()
	go inw.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"))
	br := bufio.NewReader(outr)
	first, _ := br.ReadString('\n')
	if !strings.Contains(first, `"id":1`) {
		t.Fatalf("첫 줄은 initialize 응답: %s", first)
	}
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(line), &m)
	if m["method"] != "notifications/claude/channel" {
		t.Fatalf("알림 기대: %s", line)
	}
	meta := m["params"].(map[string]any)["meta"].(map[string]any)
	if meta["event"] != "WAITING" || meta["task"] != "T-9" || meta["letter_id"] != id {
		t.Errorf("meta: %v", meta)
	}
	if _, err := os.Stat(filepath.Join(inbox, "received", id+".json")); err != nil {
		t.Error("전달된 편지는 received/로 이동")
	}
	inw.Close() // 세션 종료
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("stdin EOF → nil: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stdin EOF 뒤 3초 안에 종료돼야 함")
	}
}

func TestRunChannelStopsWhenStdinCloses(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.NewStore(dir)
	inr, inw := io.Pipe()
	var errb bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- RunChannel(context.Background(), inr, io.Discard, &errb, st, dir, "", []string{"serve", filepath.Join(t.TempDir(), "inbox")})
	}()
	time.Sleep(50 * time.Millisecond)
	inw.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("nil 기대: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("EOF 뒤 종료돼야 함(감시 goroutine이 붙들면 안 됨)")
	}
}

// 경로 오타 — 상위 폴더까지 없으면 거부하고 아무것도 만들지 않는다. 수신함만 없으면 만들되 알린다.
func TestRunChannelRejectsMissingParent(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	root := t.TempDir()
	typo := filepath.Join(root, "runtmie", "inbox")
	var out, errb bytes.Buffer
	err := RunChannel(context.Background(), strings.NewReader(""), &out, &errb, st, t.TempDir(), "1.10.3", []string{"serve", typo})
	if err == nil || !strings.Contains(err.Error(), "상위 폴더가 없습니다") {
		t.Fatalf("상위 폴더 없음 → 거부: %v", err)
	}
	if _, serr := os.Stat(filepath.Join(root, "runtmie")); serr == nil {
		t.Fatal("거부했으면 폴더를 만들면 안 된다")
	}
	fresh := filepath.Join(root, "inbox")
	if err := RunChannel(context.Background(), strings.NewReader(""), &out, &errb, st, t.TempDir(), "1.10.3", []string{"serve", fresh}); err != nil {
		t.Fatalf("수신함만 없는 첫 기동은 통과: %v", err)
	}
	if !strings.Contains(errb.String(), "새로 만듭니다") {
		t.Fatalf("새로 만들 때는 알려야 한다: %q", errb.String())
	}
}

func pendingLetter(t *testing.T, inbox, id string) {
	t.Helper()
	rep := &task.Report{Version: 1, ID: id, TaskID: "T-9", Session: "s", Kind: "claude", From: "WORKING", To: "WAITING", Ask: "승인?", At: time.Now(), Inbox: inbox}
	if _, err := task.WriteReport(rep); err != nil {
		t.Fatal(err)
	}
}

// 상태 점검(claude mcp get·list)은 initialize 직후 접속을 끊는다 — 그 짧은 인스턴스가 편지를 집으면 안 된다.
func TestRunChannelHealthCheckDoesNotConsume(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.NewStore(dir)
	inbox := filepath.Join(t.TempDir(), "inbox")
	id := strings.Repeat("c", 32)
	pendingLetter(t, inbox, id)
	inr, inw := io.Pipe()
	var errb bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- RunChannel(context.Background(), inr, io.Discard, &errb, st, dir, "1.10.3", []string{"serve", inbox, "--interval", "10ms"})
	}()
	inw.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"))
	time.Sleep(40 * time.Millisecond) // channelSettle(150ms)보다 짧게 머물다 끊는다
	inw.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("종료 안 됨")
	}
	time.Sleep(250 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(inbox, "pending", id+".json")); err != nil {
		t.Fatalf("짧은 접속이 편지를 집었다: %v", err)
	}
}

// 수신함은 잠금을 쥔 채널 서버 하나만 소비한다 — 다른 서버가 쥐고 있으면 기다렸다가 풀리면 이어받는다.
func TestRunChannelWaitsForInboxLock(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.NewStore(dir)
	inbox := filepath.Join(t.TempDir(), "inbox")
	id := strings.Repeat("d", 32)
	pendingLetter(t, inbox, id)
	lf, err := os.OpenFile(filepath.Join(inbox, ".channel.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	inr, inw := io.Pipe()
	var errb bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- RunChannel(context.Background(), inr, io.Discard, &errb, st, dir, "1.10.3", []string{"serve", inbox, "--interval", "10ms"})
	}()
	inw.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"))
	time.Sleep(400 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(inbox, "pending", id+".json")); err != nil {
		t.Fatalf("잠금이 남의 것인데 편지를 집었다: %v", err)
	}
	_ = syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(inbox, "received", id+".json")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("잠금이 풀렸는데 편지를 이어받지 않음")
		}
		time.Sleep(20 * time.Millisecond)
	}
	inw.Close()
	<-done
}
