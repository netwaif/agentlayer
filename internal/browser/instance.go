// Package browser는 에이전트 전용 브라우저(전용 프로필 Chrome)를 CDP로
// 관리한다. 코어 관제는 이 패키지 없이도 무영향 — 소비 전용 원칙.
package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

// Connect는 기록된 인스턴스에 attach하고, 없거나 죽었으면 전용 프로필로
// 새 Chrome을 기동한다. 멱등 — 몇 번을 불러도 브라우저는 1개.
func Connect(stateDir string) (*rod.Browser, error) {
	if in, err := LoadInstance(stateDir); err == nil {
		b := rod.New().ControlURL(in.WSURL)
		if err := b.Connect(); err == nil {
			return b, nil
		}
		RemoveInstance(stateDir) // 죽은 기록 자동 정리
	}
	bin, ok := launcher.LookPath()
	if !ok {
		return nil, fmt.Errorf("Chrome을 찾을 수 없습니다 — Google Chrome 또는 Chromium 설치 필요")
	}
	ws, err := launcher.New().Bin(bin).
		UserDataDir(filepath.Join(stateDir, "browser-profile")).
		Headless(launchHeadless).
		Leakless(false). // CLI가 끝나도 브라우저는 살아야 한다
		Launch()
	if err != nil {
		return nil, fmt.Errorf("Chrome 기동 실패: %w", err)
	}
	b := rod.New().ControlURL(ws)
	if err := b.Connect(); err != nil {
		return nil, err
	}
	if err := SaveInstance(stateDir, Instance{WSURL: ws}); err != nil {
		return nil, err
	}
	return b, nil
}
