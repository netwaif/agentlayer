// Package browser는 에이전트 전용 브라우저(전용 프로필 Chrome)를 CDP로
// 관리한다. 코어 관제는 이 패키지 없이도 무영향 — 소비 전용 원칙.
package browser

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// Instance는 살아 있는 전용 브라우저의 CDP 접속점.
type Instance struct {
	WSURL string `json:"ws_url"`
}

func statePath(dir string) string { return filepath.Join(dir, "browser.json") }

func LoadInstance(dir string) (Instance, error) {
	var in Instance
	b, err := os.ReadFile(statePath(dir))
	if err != nil {
		return in, err
	}
	if err := json.Unmarshal(b, &in); err != nil {
		return in, err
	}
	if in.WSURL == "" {
		return in, fmt.Errorf("빈 인스턴스 기록")
	}
	return in, nil
}

func SaveInstance(dir string, in Instance) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(dir), b, 0o600)
}

func RemoveInstance(dir string) { _ = os.Remove(statePath(dir)) }

// launchHeadless는 테스트 전용 스위치 (export_test.go로만 접근).
var launchHeadless = false

// probeTimeout은 포트가 열려 있을 때 CDP 응답을 기다리는 상한.
const probeTimeout = 2 * time.Second

// probePort는 고정 포트에 떠 있는 Chrome의 CDP ws URL을 찾는다.
// 포트가 닫혀 있으면 ("", false, nil) — 기동해도 된다.
// 열려 있는데 CDP가 아니면 에러 — 다른 프로세스가 점유 중.
func probePort(port int) (string, bool, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, probeTimeout)
	if err != nil {
		return "", false, nil
	}
	conn.Close()
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Get("http://" + addr + "/json/version")
	if err != nil {
		return "", false, fmt.Errorf("포트 %d 사용 중(CDP 아님) — config browser_port 변경 필요", port)
	}
	defer resp.Body.Close()
	var v struct {
		WS string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil || v.WS == "" {
		return "", false, fmt.Errorf("포트 %d 사용 중(CDP 아님) — config browser_port 변경 필요", port)
	}
	return v.WS, true, nil
}

// newBrowser는 rod 클라이언트를 만든다. NoDefaultDevice가 핵심 — rod 기본값은
// 페이지마다 1280×800 노트북을 에뮬레이션해 사람이 보는 창 아래가 회색으로
// 비고(뷰포트가 창을 안 따라감), 스크린샷도 실제 창과 달라진다.
func newBrowser(ws string) *rod.Browser {
	return rod.New().ControlURL(ws).NoDefaultDevice()
}

// attach는 ws에 붙고 기록을 갱신한다.
func attach(stateDir, ws string) (*rod.Browser, error) {
	b := newBrowser(ws)
	if err := b.Connect(); err != nil {
		return nil, err
	}
	if err := SaveInstance(stateDir, Instance{WSURL: ws}); err != nil {
		return nil, err
	}
	return b, nil
}

// Connect는 기록된 인스턴스에 attach하고, 기록이 없거나 죽었으면 고정 포트를
// 프로브해 attach하며, 그래도 없으면 전용 프로필로 새 Chrome을 기동한다.
// 멱등 — 몇 번을 불러도 브라우저는 1개. 포트가 고정이라 MCP 클라이언트는
// 늘 http://127.0.0.1:<port>로 붙을 수 있다.
func Connect(stateDir string, port int) (*rod.Browser, error) {
	if in, err := LoadInstance(stateDir); err == nil {
		b := newBrowser(in.WSURL)
		if err := b.Connect(); err == nil {
			return b, nil
		}
		RemoveInstance(stateDir) // 죽은 기록 자동 정리
	}
	ws, alive, err := probePort(port)
	if err != nil {
		return nil, err
	}
	if alive {
		return attach(stateDir, ws)
	}
	bin, ok := launcher.LookPath()
	if !ok {
		return nil, fmt.Errorf("Chrome을 찾을 수 없습니다 — Google Chrome 또는 Chromium 설치 필요")
	}
	profile := filepath.Join(stateDir, "browser-profile")
	if err := EnsureProfileTheme(profile); err != nil {
		return nil, err
	}
	ws, err = launcher.New().Bin(bin).
		UserDataDir(profile).
		Headless(launchHeadless).
		Leakless(false). // CLI가 끝나도 브라우저는 살아야 한다
		RemoteDebuggingPort(port).
		Delete("no-startup-window"). // 기동 시 빈 창을 보여 "아무 일 없음"처럼 보이지 않게
		Delete("enable-automation"). // "자동화된 테스트 소프트웨어에 의해 제어" 인포바 제거 — 사람이 같이 쓰는 브라우저
		Launch()
	if err != nil {
		return nil, fmt.Errorf("Chrome 기동 실패: %w", err)
	}
	return attach(stateDir, ws)
}
