package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// 행 감시(스펙 4절): 창 전체가 굳어 강제 종료만 통하던 증상. 원인 미상이라 감지·채집·재시작까지.
// hangFailsNeeded=3 — 파일 열기 같은 네이티브 모달은 UI 스레드를 중첩 런루프로 잠깐 묶어
// ping 한두 번은 실패할 수 있다. 세 번 연속(≥20초 동안 계속 무응답)까지 요구해 그런 오탐을 줄인다.
const (
	hangPing        = 3 * time.Second
	hangMinGap      = 10 * time.Second
	hangFailsNeeded = 3
	// hangPingFast — 연결 시도가 이 안에 실패하면 "죽은 기록"(즉시 거부)으로 보고, 이보다
	// 오래 걸려 실패하면 "진짜 행"(소켓을 붙든 채 타임아웃)으로 본다.
	hangPingFast = 500 * time.Millisecond
)

type HangOps struct {
	Ping     func() error // UI 스레드를 타는 CDP 호출(타임아웃 포함)
	Sample   func(pid int, out string) error
	Kill     func(pid int) error
	Relaunch func() error
	Notify   func(msg string)
}

type hangState struct {
	Fails  int       `json:"fails"`
	LastAt time.Time `json:"last_at"`
	// Pid — 실패를 센 브라우저의 pid. pid가 바뀌었다는 건 그 사이 브라우저가 죽고 다시
	// 떴다는 뜻이라 예전 실패 횟수를 이어서 세면 안 된다(멀쩡한 새 브라우저를 2회 만에
	// 죽일 수 있다). 다르면 0부터 다시 센다.
	Pid int `json:"pid,omitempty"`
}

func hangStatePath(dir string) string { return filepath.Join(dir, "hangwatch.json") }

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
	s := loadHangState(dir)
	if ops.Ping() == nil {
		if s.Fails != 0 {
			saveHangState(dir, hangState{})
		}
		return false, ""
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
		return false, ""
	}
	// Sample·Kill·Relaunch가 nil이면(부분만 채운 ops) 여기서 죽지 않게 각각 막는다.
	diag := filepath.Join(dir, "hang", now.Format("20060102-150405")+".txt")
	_ = os.MkdirAll(filepath.Dir(diag), 0o755)
	if ops.Sample == nil {
		diag = ""
	} else if err := ops.Sample(pid, diag); err != nil {
		diag = ""
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
	saveHangState(dir, hangState{})
	restarted := relaunchErr == nil
	var msg string
	if restarted {
		msg = "에이전트 브라우저가 멈춰 재시작했습니다"
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
			_, err = proto.BrowserGetWindowForTarget{TargetID: p.TargetID}.Call(p.Timeout(timeout))
			return err
		}
	}
	_, err = proto.BrowserGetVersion{}.Call(bt)
	return err
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
		ping, err := connect(ws, budget)
		if err == nil {
			return ping(budget)
		}
		if now().Sub(start) >= hangPingFast {
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
	ping, err := connect(pws, budget)
	if err != nil {
		return err
	}
	return ping(budget)
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
	}
}
