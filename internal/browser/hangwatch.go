package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// 행 감시(스펙 4절): 창 전체가 굳어 강제 종료만 통하던 증상. 원인 미상이라 감지·채집·재시작까지.
const (
	hangPing        = 3 * time.Second
	hangMinGap      = 10 * time.Second
	hangFailsNeeded = 2
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
// 채집 → 강제 종료 → 재기동 → 알림. 돌려주는 diag는 채집 파일 경로(없으면 "").
func HangWatch(dir string, pid int, ops HangOps, now time.Time) (bool, string) {
	if pid <= 0 {
		return false, ""
	}
	s := loadHangState(dir)
	if ops.Ping() == nil {
		if s.Fails != 0 {
			saveHangState(dir, hangState{})
		}
		return false, ""
	}
	if s.Fails > 0 && now.Sub(s.LastAt) < hangMinGap {
		return false, "" // 너무 이른 재판정은 세지 않는다(훅이 잦다)
	}
	s.Fails++
	s.LastAt = now
	if s.Fails < hangFailsNeeded {
		saveHangState(dir, s)
		return false, ""
	}
	diag := filepath.Join(dir, "hang", now.Format("20060102-150405")+".txt")
	_ = os.MkdirAll(filepath.Dir(diag), 0o755)
	if err := ops.Sample(pid, diag); err != nil {
		diag = ""
	}
	_ = ops.Kill(pid)
	_ = ops.Relaunch()
	saveHangState(dir, hangState{})
	msg := "에이전트 브라우저가 멈춰 재시작했습니다"
	if diag != "" {
		msg += " · 진단: " + diag
	}
	if ops.Notify != nil {
		ops.Notify(msg)
	}
	return true, diag
}

// PingUI — 첫 웹 탭의 창 정보를 묻는다(UI 스레드 경유). 웹 탭이 없으면 Browser.getVersion.
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

// DefaultHangOps — 실제 수단. Sample은 macOS `sample`만(리눅스는 생략해 diag "").
// Ping은 Connect(고정 포트 재탐색·기동까지 갈 수 있는 전체 경로)를 쓰지 않는다 — 굳은
// 브라우저면 그 경로 자체가 3초를 넘겨 걸릴 수 있어서다. 기록된 인스턴스의 WS로만 붙고,
// 기록이 없을 때만 포트를 한 번 프로브한다. 전체가 hangPing 안에 끝나야 한다.
func DefaultHangOps(stateDir string, port int, goos string, notify func(string)) HangOps {
	return HangOps{
		Ping: func() error {
			ws := ""
			if in, err := LoadInstance(stateDir); err == nil {
				ws = in.WSURL
			} else {
				w, alive, perr := probePort(port)
				if perr != nil {
					return perr
				}
				if !alive {
					return fmt.Errorf("에이전트 브라우저 없음")
				}
				ws = w
			}
			b := newBrowser(ws).Timeout(hangPing)
			if err := b.Connect(); err != nil {
				return err
			}
			return PingUI(b, hangPing)
		},
		Sample: func(pid int, out string) error {
			if goos != "darwin" {
				return fmt.Errorf("sample 없음")
			}
			return exec.Command("sample", fmt.Sprint(pid), "2", "-file", out).Run()
		},
		Kill: func(pid int) error {
			RemoveInstance(stateDir)
			_ = syscall.Kill(-pid, syscall.SIGKILL) // 프로세스 그룹
			return syscall.Kill(pid, syscall.SIGKILL)
		},
		Relaunch: func() error { _, err := Connect(stateDir, port); return err },
		Notify:   notify,
	}
}
