package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// 행 감시(스펙 4절): 창 전체가 굳어 강제 종료만 통하던 증상. 감지·채집·재시작까지.
// 실측(2026-09-23): 그 증상의 정체는 UI 스레드 행이 아니라 GPU 프로세스의 vsync 시계가
// 죽어 프레임이 안 나오는 상태였다(PingUI의 프레임 프로브 주석 참고). Chrome 내부 트리거는
// 미상 — 기동 플래그의 --vmodule 로그(프로필 chrome_debug.log)가 다음 재발 때의 증거다.
// hangFailsNeeded=3 — 파일 열기 같은 네이티브 모달은 UI 스레드를 중첩 런루프로 잠깐 묶어
// ping 한두 번은 실패할 수 있다. 세 번 연속(≥20초 동안 계속 무응답)까지 요구해 그런 오탐을 줄인다.
const (
	hangPing        = 3 * time.Second
	hangMinGap      = 10 * time.Second
	hangFailsNeeded = 3
	// hangPingFast — 연결 시도가 이 안에 실패하면 "죽은 기록"(즉시 거부)으로 보고, 이보다
	// 오래 걸려 실패하면 "진짜 행"(소켓을 붙든 채 타임아웃)으로 본다.
	hangPingFast = 500 * time.Millisecond
	// hangWakeGrace — 디스플레이가 꺼진 것을 마지막으로 본 뒤 이 시간 동안은 프레임 실패를
	// 세지 않는다(꺼짐 판정과 ping 사이의 경합만 덮는다). 2026-09-29 실측으로 2분에서 줄였다:
	// 디스플레이가 1분 넘게 꺼졌다 켜지면 프레임은 스스로 돌아오지 않는다(아래 ErrNoFrame 주석) —
	// 오래 기다려 봐야 굳은 브라우저를 그만큼 더 방치할 뿐이다.
	hangWakeGrace = 20 * time.Second
	// hangRestartGrace — 행 감시가 재시작한 직후에는 어떤 실패도 세지 않는다. 막 뜬 브라우저는
	// 복원한 탭을 한꺼번에 읽느라 ping 예산(3초)을 넘길 수 있고, 그걸 세면 재시작이 재시작을 부른다
	// (2026-09-29 실측: 탭 46개 복원 뒤 1분 간격 연쇄 재시작).
	hangRestartGrace = 90 * time.Second
)

// ErrNoFrame — 프레임 프로브 실패. UI 스레드 무응답과 구분한다: 디스플레이가 꺼져 있으면
// 프레임은 원래 안 나온다(2026-09-28 실측 — 9/25·9/26·9/28의 강제 재시작 47건이 전부
// 디스플레이 꺼짐 구간이거나 켜진 직후였다).
//
// 굳음의 트리거는 디스플레이 꺼짐이다(2026-09-29 실측, Intel iMac·macOS 14.8.5·Chrome for Testing).
//   - 디스플레이가 꺼지면 vsync 시계가 멎는다: CADisplayLink는 20초 안에, 기본(CVDisplayLink)은
//     20초까지는 버티고 100초에는 멎어 있다.
//   - 켜져도 돌아오지 않는다: 켜진 뒤 90초까지 프레임 0. 기동 플래그(시계 종류·disable-gpu-vsync·
//     disable-gpu·occluded/renderer 백그라운딩·rod 기본 플래그 전부 제거) 어느 조합도 같았다.
//   - 되살리기도 안 된다: bringToFront·창 이동/크기 ±1·최소화 복원·activate·새 창 전부 실패.
//     9/22·9/23의 첫 굳음 두 번도 디스플레이가 꺼진 구간(17:22~22:56, 18:19~23:00)이었다.
//
// 그래서 방침은 "꺼진 동안은 세지 않고, 켜진 뒤에도 프레임이 없으면 재시작하되 탭을 되살린다".
var ErrNoFrame = errors.New("프레임 없음(화면 굳음)")

type HangOps struct {
	Ping     func() error // UI 스레드를 타는 CDP 호출(타임아웃 포함)
	Sample   func(pid int, out string) error
	Kill     func(pid int) error
	Relaunch func() error
	Notify   func(msg string)
	// DisplayAsleep — 화면이 안 보이는 상태인지(디스플레이 꺼짐 또는 화면 잠금). nil이거나
	// 알 수 없으면 보이는 것으로 본다. 프레임 실패(ErrNoFrame)일 때만 부른다.
	DisplayAsleep func() bool
	// Tabs — 죽이기 직전에 열려 있던 웹 탭 주소(굳은 브라우저도 CDP 목록은 즉답한다).
	// Reopen — 재기동 뒤 그 주소들을 다시 열고 연 개수를 돌려준다. 둘 다 nil이면 복원 없음.
	Tabs   func() []string
	Reopen func(urls []string) int
}

type hangState struct {
	Fails  int       `json:"fails"`
	LastAt time.Time `json:"last_at"`
	// Pid — 실패를 센 브라우저의 pid. pid가 바뀌었다는 건 그 사이 브라우저가 죽고 다시
	// 떴다는 뜻이라 예전 실패 횟수를 이어서 세면 안 된다(멀쩡한 새 브라우저를 2회 만에
	// 죽일 수 있다). 다르면 0부터 다시 센다.
	Pid int `json:"pid,omitempty"`
	// AsleepAt — 프레임 실패 때 디스플레이가 꺼져 있던 마지막 시각(hangWakeGrace 기준).
	AsleepAt time.Time `json:"asleep_at,omitempty"`
	// RestartedAt — 행 감시가 마지막으로 재시작한 시각(hangRestartGrace 기준).
	RestartedAt time.Time `json:"restarted_at,omitempty"`
}

func hangStatePath(dir string) string { return filepath.Join(dir, "hangwatch.json") }

// 행 감시 기록 — ping이 실패할 때마다 사유와 판정을 한 줄씩 남긴다. 덤프(sample)만으로는
// "왜 죽였는지"를 알 수 없어 원인 조사가 매번 전원 기록 대조로 돌아갔다(2026-09-28).
// 성공한 ping은 적지 않는다(훅마다 돌아 양이 많다).
const hangLogMax = 256 << 10

func hangLogPath(dir string) string { return filepath.Join(dir, "hang", "hangwatch.log") }

func writeHangLog(dir string, now time.Time, pid int, verdict string, err error) {
	path := hangLogPath(dir)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if st, serr := os.Stat(path); serr == nil && st.Size() > hangLogMax {
		// 넘치면 뒤쪽 절반만 남긴다 — 최근 기록이 조사 대상이다.
		if b, rerr := os.ReadFile(path); rerr == nil {
			b = b[len(b)/2:]
			if i := strings.IndexByte(string(b), '\n'); i >= 0 {
				b = b[i+1:]
			}
			_ = os.WriteFile(path, b, 0o600)
		}
	}
	f, oerr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if oerr != nil {
		return
	}
	defer f.Close()
	msg := strings.ReplaceAll(fmt.Sprint(err), "\n", " ")
	fmt.Fprintf(f, "%s pid=%d %s · %s\n", now.Format("2006-01-02 15:04:05"), pid, verdict, msg)
}

func loadHangState(dir string) hangState {
	var s hangState
	if b, err := os.ReadFile(hangStatePath(dir)); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

func saveHangState(dir string, s hangState) {
	b, _ := json.Marshal(s)
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(hangStatePath(dir), b, 0o600)
}

// HangWatch — ping 실패가 hangMinGap 이상 떨어져 hangFailsNeeded번 이어지면 행으로 확정하고
// 채집 → 강제 종료 → 재기동 → 알림. 돌려주는 diag는 채집 파일 경로(없으면 ""). 재기동까지
// 성공했을 때만 restarted=true — 강제 종료는 했는데 재기동이 실패하면 false로 돌려주고
// 알림 문구도 "재시작했습니다"가 아니라 "강제 종료했습니다·재기동 실패"로 갈아 끼운다.
func HangWatch(dir string, pid int, ops HangOps, now time.Time) (bool, string) {
	if pid <= 0 || ops.Ping == nil {
		return false, ""
	}
	// 판정은 한 번에 하나만 — 훅 둘이 겹치면 같은 굳음을 두 번 확정해 재시작·탭 복원이 두 번 돈다
	// (2026-09-29 실측: 2초 간격으로 덤프 2건).
	_ = os.MkdirAll(dir, 0o755)
	if lf, err := os.OpenFile(filepath.Join(dir, "hangwatch.lock"), os.O_CREATE|os.O_RDWR, 0o600); err == nil {
		defer lf.Close()
		if syscall.Flock(int(lf.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
			return false, ""
		}
		defer syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
	}
	s := loadHangState(dir)
	err := ops.Ping()
	if err == nil {
		if s.Fails != 0 {
			saveHangState(dir, hangState{AsleepAt: s.AsleepAt, RestartedAt: s.RestartedAt})
		}
		return false, ""
	}
	if !s.RestartedAt.IsZero() && now.Sub(s.RestartedAt) < hangRestartGrace {
		writeHangLog(dir, now, pid, fmt.Sprintf("건너뜀(재시작 뒤 유예 %s)", now.Sub(s.RestartedAt).Round(time.Second)), err)
		return false, ""
	}
	if errors.Is(err, ErrNoFrame) {
		// 화면이 꺼져 있으면 프레임이 없는 게 정상이다 — 세지도 죽이지도 않는다.
		if ops.DisplayAsleep != nil && ops.DisplayAsleep() {
			saveHangState(dir, hangState{AsleepAt: now, RestartedAt: s.RestartedAt})
			writeHangLog(dir, now, pid, "건너뜀(화면 꺼짐·잠금)", err)
			return false, ""
		}
		if !s.AsleepAt.IsZero() && now.Sub(s.AsleepAt) < hangWakeGrace {
			writeHangLog(dir, now, pid, fmt.Sprintf("건너뜀(켜진 뒤 유예 %s)", now.Sub(s.AsleepAt).Round(time.Second)), err)
			if s.Fails != 0 {
				saveHangState(dir, hangState{AsleepAt: s.AsleepAt, RestartedAt: s.RestartedAt})
			}
			return false, ""
		}
	}
	if s.Pid != 0 && s.Pid != pid {
		s = hangState{} // 그 사이 브라우저가 바뀌었다 — 예전 실패는 이 프로세스의 것이 아니다
	}
	if s.Fails > 0 && now.Sub(s.LastAt) < hangMinGap {
		return false, "" // 너무 이른 재판정은 세지 않는다(훅이 잦다)
	}
	s.Fails++
	s.LastAt = now
	s.Pid = pid
	if s.Fails < hangFailsNeeded {
		saveHangState(dir, s)
		writeHangLog(dir, now, pid, fmt.Sprintf("실패 %d/%d", s.Fails, hangFailsNeeded), err)
		return false, ""
	}
	writeHangLog(dir, now, pid, fmt.Sprintf("실패 %d/%d → 강제 재시작", s.Fails, hangFailsNeeded), err)
	// Sample·Kill·Relaunch가 nil이면(부분만 채운 ops) 여기서 죽지 않게 각각 막는다.
	diag := filepath.Join(dir, "hang", now.Format("20060102-150405")+".txt")
	_ = os.MkdirAll(filepath.Dir(diag), 0o755)
	if ops.Sample == nil {
		diag = ""
	} else if err := ops.Sample(pid, diag); err != nil {
		diag = ""
	}
	var tabs []string
	if ops.Tabs != nil {
		tabs = ops.Tabs()
	}
	if ops.Kill != nil {
		_ = ops.Kill(pid)
	}
	var relaunchErr error
	if ops.Relaunch == nil {
		relaunchErr = fmt.Errorf("재기동 수단 없음")
	} else {
		relaunchErr = ops.Relaunch()
	}
	saveHangState(dir, hangState{RestartedAt: now})
	restarted := relaunchErr == nil
	var msg string
	if restarted {
		msg = "에이전트 브라우저가 멈춰 재시작했습니다"
		if ops.Reopen != nil && len(tabs) > 0 {
			msg += fmt.Sprintf(" · 탭 %d/%d개 복원", ops.Reopen(tabs), len(tabs))
		}
	} else {
		msg = fmt.Sprintf("에이전트 브라우저가 멈춰 강제 종료했습니다 · 재기동 실패: %v", relaunchErr)
	}
	if diag != "" {
		msg += " · 진단: " + diag
	}
	if ops.Notify != nil {
		ops.Notify(msg)
	}
	return restarted, diag
}

// PingUI — 첫 웹 탭의 창 정보를 묻는다(UI 스레드 경유). 웹 탭이 없으면 Browser.getVersion.
// b는 이미 호출자가 Timeout을 걸어 둔 채로 넘어온다 — 여기서 다시 Timeout(timeout)을 걸어도
// rod의 Timeout은 부모 컨텍스트에서 파생된 자식 컨텍스트라 부모 데드라인보다 늦게까지
// 못 간다(rod context.go: WithTimeout(b.ctx, d)) — 그래서 전체 Ping은 처음 건 타임아웃 안에서만 돈다.
func PingUI(b *rod.Browser, timeout time.Duration) error {
	bt := b.Timeout(timeout)
	pages, err := bt.Pages()
	if err != nil {
		return err
	}
	for _, p := range pages {
		if info, err := p.Info(); err == nil && IsWebURL(info.URL) {
			if _, err := (proto.BrowserGetWindowForTarget{TargetID: p.TargetID}).Call(p.Timeout(timeout)); err != nil {
				return err
			}
			return frameProbePages(pages, timeout)
		}
	}
	if _, err := (proto.BrowserGetVersion{}).Call(bt); err != nil {
		return err
	}
	return frameProbePages(pages, timeout)
}

// 프레임 프로브(2026-09-23 실측): "창 전체가 그림으로 굳음"은 UI 스레드가 멀쩡한 채로
// GPU 프로세스의 vsync 시계(CVDisplayLink 스레드)가 사라져 프레임이 한 장도 안 나오는
// 상태였다 — CDP·JS는 즉답, 탭 목록도 정상, 그러나 screenshot·click은 프레임을 기다리다
// 타임아웃, 새 창을 만들어도 마찬가지(트레이스: SetNeedsCommit만 있고 BeginFrame 0).
// 4시간 동안 위 ping이 통과해 감시가 못 잡았다. 그래서 보이는 탭 하나에서 8×8 캡처가
// 예산 안에 오는지를 같이 본다. 숨은 탭(가려진 창·최소화·비활성 탭)은 원래 프레임이 안
// 나오므로 후보에서 빼 오탐을 막고, 보이는 탭이 없으면 판정을 건너뛴다.
type frameTab struct {
	Web     bool
	Visible bool
}

// pickFrameTab — 보이는 웹 탭 우선, 없으면 보이는 아무 탭, 그것도 없으면 -1.
func pickFrameTab(tabs []frameTab) int {
	any := -1
	for i, t := range tabs {
		if !t.Visible {
			continue
		}
		if t.Web {
			return i
		}
		if any < 0 {
			any = i
		}
	}
	return any
}

// FrameProbe — 보이는 탭 하나가 timeout 안에 프레임을 내놓는지.
func FrameProbe(b *rod.Browser, timeout time.Duration) error {
	pages, err := b.Timeout(timeout).Pages()
	if err != nil {
		return err
	}
	return frameProbePages(pages, timeout)
}

func frameProbePages(pages rod.Pages, timeout time.Duration) error {
	tabs := make([]frameTab, len(pages))
	for i, p := range pages {
		info, err := p.Info()
		if err != nil {
			continue
		}
		res, err := proto.RuntimeEvaluate{Expression: "document.visibilityState", ReturnByValue: true}.Call(p.Timeout(timeout))
		if err != nil || res.Result == nil {
			continue
		}
		tabs[i] = frameTab{Web: IsWebURL(info.URL), Visible: res.Result.Value.Str() == "visible"}
		if tabs[i].Web && tabs[i].Visible {
			break // 보이는 웹 탭을 찾았다 — 나머지 탭까지 물으면 탭이 많을 때 예산을 다 쓴다
		}
	}
	i := pickFrameTab(tabs)
	if i < 0 {
		return nil
	}
	_, err := proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
		Clip:   &proto.PageViewport{X: 0, Y: 0, Width: 8, Height: 8, Scale: 1},
	}.Call(pages[i].Timeout(timeout))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoFrame, err)
	}
	return nil
}

// pingConnect — ws에 budget 안에서 붙어, 이후 UI ping에 쓸 함수를 돌려준다.
// DefaultHangOps는 실제 rod 연결로 채우고, 테스트는 가짜로 채워 넣는다.
type pingConnect func(ws string, budget time.Duration) (ping func(timeout time.Duration) error, err error)

// pingProbe — 고정 포트에서 살아 있는 CDP를 한 번 찾아본다(browser.probePort 대응).
type pingProbe func(budget time.Duration) (ws string, alive bool, err error)

// decideHangPing은 DefaultHangOps.Ping의 판단 로직 — 실브라우저 없이 단위 테스트하려고
// 연결·프로브를 주입받는다.
//
// 기록된 WS가 있으면 먼저 그걸로 붙는다. 연결이 hangPingFast 안에 "빠르게" 실패하면
// (연결 거부 등) 죽은 프로세스의 낡은 기록으로 보고 remove()로 지운 뒤 포트를 다시
// 프로브한다. hangPingFast보다 "느리게" 실패하면(소켓을 붙든 채 타임아웃) 그건 진짜
// 행이므로 대체 경로로 넘기지 않고 바로 에러를 돌려준다 — 그래야 최근에 다른 경로로
// 재기동된 정상 브라우저를 "연결 거부"로 오판해 죽이는 일이 없다.
func decideHangPing(hasWS bool, ws string, budget time.Duration, connect pingConnect, probe pingProbe, remove func(), now func() time.Time) error {
	if now == nil {
		now = time.Now
	}
	if hasWS {
		start := now()
		ping, err := connectWithin(budget, ws, connect)
		if err == nil {
			return ping(budget)
		}
		// 연결이 예산 안에 끝나지도 않았다 = 붙는 중에 멈춤 = 진짜 행. 경과 시간과
		// 무관하게 여기서 끝낸다(대체 경로로 가면 또 같은 곳에서 멈춘다).
		if errors.Is(err, ErrConnectTimeout) || now().Sub(start) >= hangPingFast {
			return err // 느리게 실패 = 진짜 행 — 대체 경로로 넘기지 않는다
		}
		remove() // 빠르게 거부됨 = 죽은 기록 — 정리하고 포트를 다시 본다
	}
	if budget < 2*time.Second {
		return fmt.Errorf("행 판정 예산 부족")
	}
	pws, alive, err := probe(budget)
	if err != nil {
		return err
	}
	if !alive {
		return fmt.Errorf("에이전트 브라우저 없음")
	}
	ping, err := connectWithin(budget, pws, connect)
	if err != nil {
		return err
	}
	return ping(budget)
}

// connectWithin — 주입된 connect를 budget으로 묶는다. rod의 WS 업그레이드 읽기에는
// 데드라인이 없어(CallWithin 주석 참고) 굳은 브라우저에 붙으려다 영원히 멈춘다.
// 감시자가 멈추면 감시가 아니다 — 훅의 autopreview가 통째로 눌러앉던 실측 증상.
func connectWithin(budget time.Duration, ws string, connect pingConnect) (func(time.Duration) error, error) {
	return CallWithin(budget, func() (func(time.Duration) error, error) { return connect(ws, budget) })
}

// waitPortFree는 강제 종료 뒤 죽어가는 소켓이 완전히 닫힐 때까지(최대 maxWait, step 간격)
// 기다린다 — 그래야 재기동이 "포트 사용 중" 오류 없이 바로 붙는다.
func waitPortFree(port int, maxWait, step time.Duration) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, step)
		if err != nil {
			return // 연결 거부 = 포트 비었음
		}
		conn.Close()
		time.Sleep(step)
	}
}

// DefaultHangOps — 실제 수단. Sample은 macOS `sample`만(리눅스는 생략해 diag "").
func DefaultHangOps(stateDir string, port int, goos string, notify func(string)) HangOps {
	return HangOps{
		Ping: func() error {
			in, loadErr := LoadInstance(stateDir)
			connect := func(ws string, budget time.Duration) (func(time.Duration) error, error) {
				// b.Timeout(budget)은 TCP 다이얼까지만 막는다 — WS 업그레이드 읽기는
				// decideHangPing 쪽 connectWithin이 예산으로 묶는다.
				b := newBrowser(ws).Timeout(budget)
				if err := b.Connect(); err != nil {
					return nil, err
				}
				return func(t time.Duration) error { return PingUI(b, t) }, nil
			}
			probe := func(budget time.Duration) (string, bool, error) {
				return probePort(port)
			}
			remove := func() { RemoveInstance(stateDir) }
			return decideHangPing(loadErr == nil, in.WSURL, hangPing, connect, probe, remove, nil)
		},
		Sample: func(pid int, out string) error {
			if goos != "darwin" {
				return fmt.Errorf("sample 없음")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			err := exec.CommandContext(ctx, "sample", fmt.Sprint(pid), "2", "-file", out).Run()
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("sample 타임아웃(20초)")
			}
			return err
		},
		Kill: func(pid int) error {
			RemoveInstance(stateDir)
			// 프로세스 그룹 kill은 그 pid가 실제로 그룹 리더일 때만 — 아니면 우리를 띄운
			// 셸·에이전트가 속한 남의 그룹을 통째로 죽일 수 있다(최종 리뷰 IMPORTANT 5).
			if pgid, err := syscall.Getpgid(pid); err == nil && pgid == pid {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
			return syscall.Kill(pid, syscall.SIGKILL)
		},
		Relaunch: func() error {
			waitPortFree(port, 5*time.Second, 100*time.Millisecond)
			_, err := Connect(stateDir, port)
			return err
		},
		Notify: notify,
		Tabs:   func() []string { return OpenTabs(stateDir, port, hangPing) },
		Reopen: func(urls []string) int { return ReopenTabs(stateDir, port, urls) },
		DisplayAsleep: func() bool {
			if goos != "darwin" {
				return false
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, "pmset", "-g", "powerstate", "IODisplayWrangler").Output()
			if err != nil {
				return false
			}
			if asleep, _ := parseDisplayAsleep(string(out)); asleep {
				return true
			}
			// 디스플레이는 켜졌는데 잠금 화면인 구간 — 창이 가려져 프레임이 안 나온다.
			// 잠금을 오래 안 풀면 유예(hangWakeGrace)가 끝나 멀쩡한 브라우저를 죽인다.
			lctx, lcancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer lcancel()
			lout, lerr := exec.CommandContext(lctx, "ioreg", "-n", "Root", "-d1").Output()
			if lerr != nil {
				return false
			}
			return parseConsoleLocked(string(lout))
		},
	}
}

// parseDisplayAsleep — `pmset -g powerstate IODisplayWrangler` 출력에서 현재 전원 상태를
// 읽는다. 4(USEABLE) 미만이면 꺼짐·어두워짐. 줄이 없으면(드라이버 없음) ok=false.
func parseDisplayAsleep(out string) (asleep, ok bool) {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && f[0] == "IODisplayWrangler" {
			cur, err1 := strconv.Atoi(f[1])
			max, err2 := strconv.Atoi(f[2])
			if err1 != nil || err2 != nil {
				return false, false
			}
			return cur < max, true
		}
	}
	return false, false
}

// parseConsoleLocked — `ioreg -n Root -d1` 출력의 "IOConsoleLocked" = Yes 여부.
// 키가 없으면(리눅스·옛 macOS) 잠기지 않은 것으로 본다.
func parseConsoleLocked(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, `"IOConsoleLocked"`) {
			continue
		}
		i := strings.IndexByte(line, '=')
		return i >= 0 && strings.TrimSpace(line[i+1:]) == "Yes"
	}
	return false
}

// maxRestoreTabs — 재시작 때 되살리는 탭 수 상한(폭주 방지).
const maxRestoreTabs = 30

// OpenTabs — 떠 있는 브라우저의 웹 탭 주소 목록(중복 제거, 열린 순서). 굳은 브라우저도 탭 목록은
// 즉답하지만, 연결 자체가 멎을 수 있어 budget으로 묶는다. 못 읽으면 nil.
func OpenTabs(stateDir string, port int, budget time.Duration) []string {
	urls, _ := CallWithin(budget, func() ([]string, error) {
		ws := ""
		if in, err := LoadInstance(stateDir); err == nil {
			ws = in.WSURL
		}
		if ws == "" {
			pws, alive, err := probePort(port)
			if err != nil || !alive {
				return nil, fmt.Errorf("브라우저 없음")
			}
			ws = pws
		}
		b := newBrowser(ws).Timeout(budget)
		if err := b.Connect(); err != nil {
			return nil, err
		}
		pages, err := b.Pages()
		if err != nil {
			return nil, err
		}
		var raw []string
		for _, p := range pages {
			if info, err := p.Info(); err == nil {
				raw = append(raw, info.URL)
			}
		}
		return restorableTabs(raw), nil
	})
	return urls
}

// restorableTabs — 되살릴 주소만 고른다: 웹 주소만, 중복 제거, 상한까지.
func restorableTabs(raw []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, u := range raw {
		if !IsWebURL(u) || seen[u] || len(out) >= maxRestoreTabs {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// missingTabs — want 중 지금 열려 있지 않은(open에 없는) 주소만.
func missingTabs(want, open []string) []string {
	have := map[string]bool{}
	for _, u := range open {
		have[u] = true
	}
	var out []string
	for _, u := range want {
		if !have[u] {
			out = append(out, u)
		}
	}
	return out
}

// ReopenTabs — 재기동한 브라우저에 주소들이 열려 있게 한다. 돌려주는 값은 열려 있게 된 개수.
// Chrome이 스스로 세션을 되살리는 경우가 있어(2026-09-29 실측: 강제 종료 뒤 기동하면 직전 탭이 돌아옴)
// 잠깐 기다린 뒤 빠진 주소만 배경 탭으로 연다 — 그냥 다 열면 탭이 재시작마다 두 배가 된다.
func ReopenTabs(stateDir string, port int, urls []string) int {
	n, _ := CallWithin(25*time.Second, func() (int, error) {
		b, err := Connect(stateDir, port)
		if err != nil {
			return 0, err
		}
		time.Sleep(3 * time.Second)
		var open []string
		if pages, err := b.Timeout(5 * time.Second).Pages(); err == nil {
			for _, p := range pages {
				if info, err := p.Info(); err == nil {
					open = append(open, info.URL)
				}
			}
		}
		missing := missingTabs(urls, open)
		n := len(urls) - len(missing)
		for _, u := range missing {
			if _, err := (proto.TargetCreateTarget{URL: u, Background: true}).Call(b.Timeout(5 * time.Second)); err == nil {
				n++
			}
		}
		return n, nil
	})
	return n
}
