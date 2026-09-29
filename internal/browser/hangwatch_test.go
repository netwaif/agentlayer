package browser

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
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

// 프레임 프로브(2026-09-23): 창 전체가 그림으로 굳은 상태는 UI 스레드 ping이 멀쩡하다 —
// GPU 프로세스의 vsync 시계(CVDisplayLink)가 죽어 프레임이 한 장도 안 나오는 상태라서.
// 보이는 탭 하나에서 작은 캡처가 예산 안에 오는지로 잡는다. 숨은 탭은 원래 프레임이
// 안 나오므로 후보에서 뺀다(오탐 방지).
func TestPickFrameTabPrefersVisibleWebTab(t *testing.T) {
	tabs := []frameTab{{Web: false, Visible: true}, {Web: true, Visible: false}, {Web: true, Visible: true}}
	if got := pickFrameTab(tabs); got != 2 {
		t.Fatalf("보이는 웹 탭(2)을 골라야 한다: %d", got)
	}
}

func TestPickFrameTabFallsBackToAnyVisible(t *testing.T) {
	tabs := []frameTab{{Web: true, Visible: false}, {Web: false, Visible: true}}
	if got := pickFrameTab(tabs); got != 1 {
		t.Fatalf("웹 탭이 다 숨었으면 보이는 아무 탭(1): %d", got)
	}
}

func TestPickFrameTabNoneVisible(t *testing.T) {
	if got := pickFrameTab([]frameTab{{Web: true}, {}}); got != -1 {
		t.Fatalf("보이는 탭이 없으면 -1(프로브 생략): %d", got)
	}
}

// 실브라우저: 건강한 헤드리스 브라우저는 프레임 프로브를 통과해야 한다(PingUI가 nil).
func TestPingUIIncludesFrameProbeIntegration(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok {
		t.Skip("Chrome 없음")
	}
	SetHeadlessForTest(true)
	defer SetHeadlessForTest(false)
	dir := t.TempDir()
	port := freePort(t)
	b, err := Connect(dir, port)
	if err != nil {
		t.Fatal(err)
	}
	defer b.MustClose()
	p := b.MustPage("data:text/html,<title>frame</title><p>hi")
	defer p.MustClose()
	if err := PingUI(b, 3*time.Second); err != nil {
		t.Fatalf("건강한 브라우저의 PingUI: %v", err)
	}
	if err := FrameProbe(b, 3*time.Second); err != nil {
		t.Fatalf("건강한 브라우저의 프레임 프로브: %v", err)
	}
}

// 2026-09-28 실측: 디스플레이가 꺼진 동안에는 프레임이 원래 안 나온다(vsync 시계가 멎는다). 그 동안의 프레임 프로브 실패로 멀쩡한 브라우저를 죽이면 안 된다.
func TestHangWatchIgnoresNoFrameWhileDisplayAsleep(t *testing.T) {
	dir := t.TempDir()
	var l hangLog
	now := time.Now()
	ops := fakeHangOps(&l, fmt.Errorf("%w: timeout", ErrNoFrame))
	ops.DisplayAsleep = func() bool { return true }
	for i := 0; i < 5; i++ {
		if r, _ := HangWatch(dir, 100, ops, now.Add(time.Duration(i)*11*time.Second)); r {
			t.Fatal("디스플레이가 꺼진 동안 재시작하면 안 됨")
		}
	}
	if l.killed != 0 || l.sampled != 0 || loadHangState(dir).Fails != 0 {
		t.Fatalf("죽이지도 세지도 않아야 함: %+v fails=%d", l, loadHangState(dir).Fails)
	}
}

// 화면이 켜진 직후에는 프레임이 곧바로 안 돌아올 수 있다 — 꺼짐을 본 뒤 hangWakeGrace 동안은
// 프레임 실패를 세지 않고, 그 뒤에도 계속 실패하면 그때부터 센다.
func TestHangWatchWakeGraceThenCounts(t *testing.T) {
	dir := t.TempDir()
	var l hangLog
	now := time.Now()
	asleep := true
	ops := fakeHangOps(&l, fmt.Errorf("%w: timeout", ErrNoFrame))
	ops.DisplayAsleep = func() bool { return asleep }
	HangWatch(dir, 100, ops, now)
	asleep = false
	for _, d := range []time.Duration{5, 10, 15} {
		if r, _ := HangWatch(dir, 100, ops, now.Add(d*time.Second)); r || l.killed != 0 {
			t.Fatalf("켜진 뒤 유예(%v) 안에는 재시작 금지", d*time.Second)
		}
	}
	base := now.Add(hangWakeGrace + time.Second)
	HangWatch(dir, 100, ops, base)
	HangWatch(dir, 100, ops, base.Add(11*time.Second))
	if r, _ := HangWatch(dir, 100, ops, base.Add(22*time.Second)); !r || l.killed != 1 {
		t.Fatalf("유예가 지난 뒤 3회 연속 실패면 재시작: r=%v %+v", r, l)
	}
}

// UI 스레드 무응답(프레임 실패가 아닌 오류)은 디스플레이가 꺼져 있어도 진짜 행이다.
func TestHangWatchUIHangCountsEvenWhenDisplayAsleep(t *testing.T) {
	dir := t.TempDir()
	var l hangLog
	now := time.Now()
	ops := fakeHangOps(&l, errors.New("timeout"))
	ops.DisplayAsleep = func() bool { return true }
	HangWatch(dir, 100, ops, now)
	HangWatch(dir, 100, ops, now.Add(11*time.Second))
	if r, _ := HangWatch(dir, 100, ops, now.Add(22*time.Second)); !r || l.killed != 1 {
		t.Fatalf("UI 행은 그대로 재시작: r=%v %+v", r, l)
	}
}

func TestParseDisplayAsleep(t *testing.T) {
	on := "\n      Driver ID  Current State  Max State  Current State Description\nIODisplayWrangler           4          4  USEABLE\n"
	off := "\n      Driver ID  Current State  Max State  Current State Description\nIODisplayWrangler           1          4  SLEEP\n"
	if a, ok := parseDisplayAsleep(on); a || !ok {
		t.Fatalf("켜짐: asleep=%v ok=%v", a, ok)
	}
	if a, ok := parseDisplayAsleep(off); !a || !ok {
		t.Fatalf("꺼짐: asleep=%v ok=%v", a, ok)
	}
	if a, ok := parseDisplayAsleep("No such driver\n"); a || ok {
		t.Fatalf("모르면 꺼짐으로 보지 않음: asleep=%v ok=%v", a, ok)
	}
}

func TestParseConsoleLocked(t *testing.T) {
	if !parseConsoleLocked("    | {\n    |   \"IOConsoleLocked\" = Yes\n    | }\n") {
		t.Fatal("잠금을 못 읽음")
	}
	if parseConsoleLocked("    |   \"IOConsoleLocked\" = No\n") {
		t.Fatal("풀린 화면을 잠금으로 읽음")
	}
	if parseConsoleLocked("no such key\n") {
		t.Fatal("키가 없으면 잠기지 않은 것으로 봐야 한다")
	}
}

// 실패한 ping은 사유와 판정을 기록에 남긴다 — 덤프만으로는 왜 죽였는지 알 수 없었다.
func TestHangWatchLogsFailureReason(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	ops := HangOps{Ping: func() error { return fmt.Errorf("%w: context deadline exceeded", ErrNoFrame) }}
	ops.DisplayAsleep = func() bool { return true }
	HangWatch(dir, 42, ops, now)
	ops.DisplayAsleep = func() bool { return false }
	base := now.Add(hangWakeGrace + time.Second)
	for i := 0; i < hangFailsNeeded; i++ {
		HangWatch(dir, 42, ops, base.Add(time.Duration(i)*(hangMinGap+time.Second)))
	}
	b, err := os.ReadFile(hangLogPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{"건너뜀(화면 꺼짐·잠금)", "실패 1/3", "실패 3/3 → 강제 재시작", "pid=42", "context deadline exceeded"} {
		if !strings.Contains(got, want) {
			t.Fatalf("기록에 %q 없음:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "\n"); n != 4 {
		t.Fatalf("기록 줄 수 = %d, 4여야 함:\n%s", n, got)
	}
}

// 재시작은 탭을 날린다 — 죽이기 전에 읽어 둔 주소를 재기동 뒤에 다시 연다.
func TestHangWatchRestoresTabs(t *testing.T) {
	dir := t.TempDir()
	var l hangLog
	var order []string
	ops := fakeHangOps(&l, errors.New("timeout"))
	ops.Tabs = func() []string {
		order = append(order, "tabs")
		return []string{"https://a.example/", "https://b.example/"}
	}
	kill, relaunch := ops.Kill, ops.Relaunch
	ops.Kill = func(pid int) error { order = append(order, "kill"); return kill(pid) }
	ops.Relaunch = func() error { order = append(order, "relaunch"); return relaunch() }
	var reopened []string
	ops.Reopen = func(u []string) int { order = append(order, "reopen"); reopened = u; return len(u) }
	now := time.Now()
	HangWatch(dir, 100, ops, now)
	HangWatch(dir, 100, ops, now.Add(11*time.Second))
	if r, _ := HangWatch(dir, 100, ops, now.Add(22*time.Second)); !r {
		t.Fatal("3회 실패면 재시작")
	}
	if strings.Join(order, ",") != "tabs,kill,relaunch,reopen" {
		t.Fatalf("순서: %v", order)
	}
	if len(reopened) != 2 || len(l.notes) != 1 || !strings.Contains(l.notes[0], "탭 2/2개 복원") {
		t.Fatalf("복원 %v, 알림 %v", reopened, l.notes)
	}
}

// 재기동이 실패하면 탭을 열 곳이 없다 — Reopen을 부르지 않는다.
func TestHangWatchNoReopenWhenRelaunchFails(t *testing.T) {
	dir := t.TempDir()
	var l hangLog
	ops := fakeHangOps(&l, errors.New("timeout"))
	ops.Tabs = func() []string { return []string{"https://a.example/"} }
	ops.Relaunch = func() error { return errors.New("포트 사용 중") }
	called := false
	ops.Reopen = func(u []string) int { called = true; return 0 }
	now := time.Now()
	for i := 0; i < 3; i++ {
		HangWatch(dir, 100, ops, now.Add(time.Duration(i)*11*time.Second))
	}
	if called {
		t.Fatal("재기동 실패인데 탭을 열려 함")
	}
}

// 판정은 한 번에 하나 — 다른 판정이 도는 중이면 ping도 하지 않고 물러난다.
func TestHangWatchSingleFlight(t *testing.T) {
	dir := t.TempDir()
	lf, err := os.OpenFile(filepath.Join(dir, "hangwatch.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	pinged := false
	ops := HangOps{Ping: func() error { pinged = true; return errors.New("timeout") }}
	HangWatch(dir, 100, ops, time.Now())
	if pinged || loadHangState(dir).Fails != 0 {
		t.Fatal("다른 판정이 도는 중에는 ping도 세기도 하지 않는다")
	}
}

func TestRestorableTabs(t *testing.T) {
	got := restorableTabs([]string{"chrome://newtab/", "https://a.example/", "about:blank", "https://a.example/", "http://localhost:3000/"})
	if strings.Join(got, " ") != "https://a.example/ http://localhost:3000/" {
		t.Fatalf("웹 주소만·중복 제거: %v", got)
	}
	var many []string
	for i := 0; i < maxRestoreTabs+5; i++ {
		many = append(many, fmt.Sprintf("https://x.example/%d", i))
	}
	if n := len(restorableTabs(many)); n != maxRestoreTabs {
		t.Fatalf("상한 %d, 실제 %d", maxRestoreTabs, n)
	}
}

// 재시작 직후의 실패는 세지 않는다 — 복원한 탭을 읽느라 느린 브라우저를 또 죽이면 연쇄 재시작이 된다.
func TestHangWatchRestartGrace(t *testing.T) {
	dir := t.TempDir()
	var l hangLog
	ops := fakeHangOps(&l, errors.New("timeout"))
	now := time.Now()
	for i := 0; i < 3; i++ {
		HangWatch(dir, 100, ops, now.Add(time.Duration(i)*11*time.Second))
	}
	if l.killed != 1 {
		t.Fatalf("첫 재시작: %+v", l)
	}
	at := now.Add(22 * time.Second)
	for i := 1; i <= 6; i++ {
		if r, _ := HangWatch(dir, 200, ops, at.Add(time.Duration(i)*11*time.Second)); r || l.killed != 1 {
			t.Fatalf("재시작 뒤 유예 안에 또 재시작(%d번째)", i)
		}
	}
	base := at.Add(hangRestartGrace + time.Second)
	for i := 0; i < 3; i++ {
		HangWatch(dir, 200, ops, base.Add(time.Duration(i)*11*time.Second))
	}
	if l.killed != 2 {
		t.Fatalf("유예가 지난 뒤 계속 실패면 재시작: %+v", l)
	}
}

func TestMissingTabs(t *testing.T) {
	got := missingTabs([]string{"https://a/", "https://b/", "https://c/"}, []string{"chrome://newtab/", "https://b/"})
	if strings.Join(got, " ") != "https://a/ https://c/" {
		t.Fatalf("빠진 주소만: %v", got)
	}
}
