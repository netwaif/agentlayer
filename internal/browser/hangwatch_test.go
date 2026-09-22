package browser

import (
	"errors"
	"net"
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

func TestHangWatchNeedsThreeFailsTenSecondsApart(t *testing.T) {
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
	if r, _ := HangWatch(dir, 100, ops, now.Add(11*time.Second)); r || l.killed != 0 {
		t.Fatal("2회째로는 재시작하면 안 됨(3회째부터)")
	}
	if loadHangState(dir).Fails != 2 {
		t.Fatalf("fails=%d", loadHangState(dir).Fails)
	}
	if r, _ := HangWatch(dir, 100, ops, now.Add(14*time.Second)); r || l.killed != 0 {
		t.Fatal("10초 안 지난 재판정은 세지 않음(2회차 이후)")
	}
	if loadHangState(dir).Fails != 2 {
		t.Fatalf("10초 안 재판정 뒤에도 fails=%d", loadHangState(dir).Fails)
	}
	r, diag := HangWatch(dir, 100, ops, now.Add(22*time.Second))
	if !r || l.sampled != 1 || l.killed != 1 || l.relaunched != 1 {
		t.Fatalf("3회 연속 실패면 채집·종료·재기동: r=%v %+v", r, l)
	}
	if !strings.Contains(diag, "hang/") || len(l.notes) != 1 || !strings.Contains(l.notes[0], "재시작") {
		t.Fatalf("진단 경로·알림: %q %v", diag, l.notes)
	}
	if loadHangState(dir).Fails != 0 {
		t.Fatal("재시작 뒤 실패 횟수 0")
	}
}

func TestHangWatchRelaunchFailureReportsFailure(t *testing.T) {
	dir := t.TempDir()
	var l hangLog
	now := time.Now()
	ops := fakeHangOps(&l, errors.New("timeout"))
	ops.Relaunch = func() error { l.relaunched++; return errors.New("포트 사용 중") }
	HangWatch(dir, 100, ops, now)
	HangWatch(dir, 100, ops, now.Add(11*time.Second))
	r, diag := HangWatch(dir, 100, ops, now.Add(22*time.Second))
	if r {
		t.Fatal("재기동 실패면 restarted=false")
	}
	if l.killed != 1 || l.relaunched != 1 {
		t.Fatalf("강제 종료는 되고 재기동은 시도돼야: %+v", l)
	}
	if len(l.notes) != 1 || !strings.Contains(l.notes[0], "재기동 실패") {
		t.Fatalf("재기동 실패가 알림에 드러나야: %v", l.notes)
	}
	if diag == "" {
		t.Fatal("채집은 됐으니 진단 경로가 있어야")
	}
	if loadHangState(dir).Fails != 0 {
		t.Fatal("재판정 루프 방지 위해 상태는 리셋돼야")
	}
}

// TestHangWatchResetsOnNewPid — 최종 리뷰 IMPORTANT 5-a: 관찰한 pid가 바뀌었으면
// 그 사이 브라우저가 죽고 새로 뜬 것이므로 예전 실패 횟수를 이어 세면 안 된다.
// (안 그러면 새 브라우저가 한 번만 삐끗해도 3회째로 세어 죽는다.)
func TestHangWatchResetsOnNewPid(t *testing.T) {
	dir := t.TempDir()
	var l hangLog
	now := time.Now()
	saveHangState(dir, hangState{Fails: 2, LastAt: now.Add(-time.Minute), Pid: 100})
	r, _ := HangWatch(dir, 200, fakeHangOps(&l, errors.New("timeout")), now)
	if r || l.killed != 0 {
		t.Fatalf("pid가 바뀌었으면 죽이면 안 됨: r=%v %+v", r, l)
	}
	s := loadHangState(dir)
	if s.Fails != 1 || s.Pid != 200 {
		t.Fatalf("새 pid로 1부터 다시 세야 함: %+v", s)
	}
}

// TestHangWatchPartialOpsDoesNotPanic — 최종 리뷰 IMPORTANT 5-d: Ping만 채운 ops로도
// 죽지 않는다(Sample·Kill·Relaunch nil 가드).
func TestHangWatchPartialOpsDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	ops := HangOps{Ping: func() error { return errors.New("timeout") }}
	now := time.Now()
	HangWatch(dir, 100, ops, now)
	HangWatch(dir, 100, ops, now.Add(11*time.Second))
	r, diag := HangWatch(dir, 100, ops, now.Add(22*time.Second))
	if r {
		t.Fatal("Relaunch가 없으면 재시작 성공일 수 없다")
	}
	if diag != "" {
		t.Fatalf("Sample이 없으면 진단 경로도 없다: %q", diag)
	}
}

func TestHangWatchNoPid(t *testing.T) {
	var l hangLog
	if r, _ := HangWatch(t.TempDir(), 0, fakeHangOps(&l, errors.New("x")), time.Now()); r || l.killed != 0 {
		t.Fatal("pid 0이면 아무것도 안 함")
	}
}

func TestHangWatchNilPingIsNoop(t *testing.T) {
	if r, diag := HangWatch(t.TempDir(), 100, HangOps{}, time.Now()); r || diag != "" {
		t.Fatal("Ping이 nil이면 아무것도 안 함")
	}
}

// fakeClock은 decideHangPing 테스트에서 now()를 호출 순서대로 offsets만큼 흐르게 한다 —
// 진짜 sleep 없이 "빠른 실패"·"느린 실패"를 재현한다.
func fakeClock(offsets ...time.Duration) func() time.Time {
	base := time.Now()
	i := 0
	return func() time.Time {
		d := offsets[i]
		if i < len(offsets)-1 {
			i++
		}
		return base.Add(d)
	}
}

func TestDecideHangPingStaleWSHealthyProbe(t *testing.T) {
	var removed bool
	connect := func(ws string, budget time.Duration) (func(time.Duration) error, error) {
		if ws == "stale" {
			return nil, errors.New("connection refused")
		}
		return func(time.Duration) error { return nil }, nil
	}
	probe := func(budget time.Duration) (string, bool, error) { return "fresh", true, nil }
	err := decideHangPing(true, "stale", 3*time.Second, connect, probe, func() { removed = true }, fakeClock(0, 100*time.Millisecond))
	if err != nil {
		t.Fatalf("죽은 기록+정상 프로브면 nil: %v", err)
	}
	if !removed {
		t.Fatal("죽은 기록은 지워야")
	}
}

func TestDecideHangPingStaleWSProbeFails(t *testing.T) {
	connect := func(ws string, budget time.Duration) (func(time.Duration) error, error) {
		return nil, errors.New("connection refused")
	}
	probe := func(budget time.Duration) (string, bool, error) { return "", false, errors.New("포트 안 열림") }
	err := decideHangPing(true, "stale", 3*time.Second, connect, probe, func() {}, fakeClock(0, 100*time.Millisecond))
	if err == nil {
		t.Fatal("프로브까지 실패하면 에러")
	}
}

func TestDecideHangPingRecordedWSPingTimeout(t *testing.T) {
	pingErr := errors.New("ping timeout")
	connect := func(ws string, budget time.Duration) (func(time.Duration) error, error) {
		return func(time.Duration) error { return pingErr }, nil
	}
	probe := func(budget time.Duration) (string, bool, error) {
		t.Fatal("연결이 됐으면 프로브로 넘어가면 안 됨")
		return "", false, nil
	}
	err := decideHangPing(true, "ws", 3*time.Second, connect, probe, func() { t.Fatal("연결됐으면 기록을 지우면 안 됨") }, nil)
	if !errors.Is(err, pingErr) {
		t.Fatalf("ping 타임아웃이 그대로 전달돼야: %v", err)
	}
}

func TestDecideHangPingSlowFailureSkipsFallback(t *testing.T) {
	connect := func(ws string, budget time.Duration) (func(time.Duration) error, error) {
		return nil, errors.New("진짜 행")
	}
	probe := func(budget time.Duration) (string, bool, error) {
		t.Fatal("느리게 실패하면 대체 경로로 넘어가면 안 됨")
		return "", false, nil
	}
	err := decideHangPing(true, "ws", 3*time.Second, connect, probe,
		func() { t.Fatal("느린 실패는 죽은 기록이 아니다 — 지우면 안 됨") },
		fakeClock(0, 600*time.Millisecond))
	if err == nil {
		t.Fatal("느리게 실패하면 에러를 그대로 돌려줘야")
	}
}

// TestDecideHangPingConnectBlocksForever — 실측 CRITICAL: SIGSTOP된 브라우저에 붙으면
// rod의 WS 업그레이드 읽기가 데드라인 없이 멈춰(lib/cdp/websocket.go) 훅의 autopreview가
// 통째로 눌러앉았다. 연결 자체를 예산으로 묶어, 막힌 connect여도 예산 안에 에러로 돌아오고
// "죽은 기록"(stale)이 아니라 "진짜 행"으로 분류돼야 한다(기록 삭제·포트 프로브 금지).
func TestDecideHangPingConnectBlocksForever(t *testing.T) {
	blocked := make(chan struct{})
	t.Cleanup(func() { close(blocked) })
	connect := func(ws string, budget time.Duration) (func(time.Duration) error, error) {
		<-blocked // 영원히 멈춘 연결
		return nil, nil
	}
	probe := func(budget time.Duration) (string, bool, error) {
		t.Fatal("붙는 중에 멈춘 건 진짜 행 — 대체 경로로 넘어가면 안 됨")
		return "", false, nil
	}
	budget := 100 * time.Millisecond
	start := time.Now()
	err := decideHangPing(true, "ws", budget, connect, probe,
		func() { t.Fatal("연결 시간 초과는 죽은 기록이 아니다 — 지우면 안 됨") }, nil)
	if err == nil {
		t.Fatal("예산을 넘긴 연결은 에러여야 한다")
	}
	if !errors.Is(err, ErrConnectTimeout) {
		t.Fatalf("연결 시간 초과로 분류돼야: %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("예산(%v) 안에 돌아와야 한다: %v", budget, d)
	}
}

func TestCallWithinReturnsResultAndTimeout(t *testing.T) {
	v, err := CallWithin(time.Second, func() (int, error) { return 7, nil })
	if v != 7 || err != nil {
		t.Fatalf("정상 경로: %d %v", v, err)
	}
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	if _, err := CallWithin(50*time.Millisecond, func() (int, error) { <-stop; return 1, nil }); !errors.Is(err, ErrConnectTimeout) {
		t.Fatalf("막히면 ErrConnectTimeout: %v", err)
	}
}

func TestWaitPortFreeReturnsPromptlyWhenPortClosed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // 바로 닫아 포트를 비워 둔다
	start := time.Now()
	waitPortFree(port, 2*time.Second, 50*time.Millisecond)
	if time.Since(start) > time.Second {
		t.Fatal("이미 빈 포트인데 너무 오래 기다림")
	}
}
