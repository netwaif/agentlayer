package browser

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type hangLog struct {
	sampled, killed, relaunched int
	notes                       []string
}

func fakeHangOps(l *hangLog, pingErr error) HangOps {
	return HangOps{
		Ping:     func() error { return pingErr },
		Sample:   func(pid int, out string) error { l.sampled++; return nil },
		Kill:     func(pid int) error { l.killed++; return nil },
		Relaunch: func() error { l.relaunched++; return nil },
		Notify:   func(m string) { l.notes = append(l.notes, m) },
	}
}

func TestHangWatchHealthyResetsFails(t *testing.T) {
	dir := t.TempDir()
	var l hangLog
	now := time.Now()
	saveHangState(dir, hangState{Fails: 1, LastAt: now.Add(-time.Minute)})
	restarted, _ := HangWatch(dir, 100, fakeHangOps(&l, nil), now)
	if restarted || l.killed != 0 || loadHangState(dir).Fails != 0 {
		t.Fatal("정상이면 실패 횟수 0으로")
	}
}

func TestHangWatchNeedsTwoFailsTenSecondsApart(t *testing.T) {
	dir := t.TempDir()
	var l hangLog
	now := time.Now()
	ops := fakeHangOps(&l, errors.New("timeout"))
	if r, _ := HangWatch(dir, 100, ops, now); r || l.killed != 0 {
		t.Fatal("1회 실패로 재시작하면 안 됨")
	}
	if r, _ := HangWatch(dir, 100, ops, now.Add(3*time.Second)); r || l.killed != 0 {
		t.Fatal("10초 안 지난 재판정은 세지 않음")
	}
	if loadHangState(dir).Fails != 1 {
		t.Fatalf("fails=%d", loadHangState(dir).Fails)
	}
	r, diag := HangWatch(dir, 100, ops, now.Add(11*time.Second))
	if !r || l.sampled != 1 || l.killed != 1 || l.relaunched != 1 {
		t.Fatalf("2회 연속 실패면 채집·종료·재기동: r=%v %+v", r, l)
	}
	if !strings.Contains(diag, "hang/") || len(l.notes) != 1 || !strings.Contains(l.notes[0], "재시작") {
		t.Fatalf("진단 경로·알림: %q %v", diag, l.notes)
	}
	if loadHangState(dir).Fails != 0 {
		t.Fatal("재시작 뒤 실패 횟수 0")
	}
}

func TestHangWatchNoPid(t *testing.T) {
	var l hangLog
	if r, _ := HangWatch(t.TempDir(), 0, fakeHangOps(&l, errors.New("x")), time.Now()); r || l.killed != 0 {
		t.Fatal("pid 0이면 아무것도 안 함")
	}
}
