// Package browser는 에이전트 전용 브라우저(전용 프로필 Chrome)를 CDP로
// 관리한다. 코어 관제는 이 패키지 없이도 무영향 — 소비 전용 원칙.
package browser

import (
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// 배경 동작(스펙 2절): 에이전트가 검색을 시켜도 브라우저가 앞으로 튀어나오지 않게.
// macOS 한정 — 다른 OS는 no-op.

const EngineAppName = "Google Chrome for Testing"

type FrontOps struct {
	Frontmost func() (string, error) // 지금 앞에 있는 앱 이름
	Activate  func(name string) error
	Supported bool // false면 배경 동작 자체를 건너뛴다(리눅스 등) — Frontmost/Activate는 no-op이라도 값
}

func DefaultFrontOps(goos string) FrontOps {
	if goos != "darwin" {
		return FrontOps{Frontmost: func() (string, error) { return "", nil }, Activate: func(string) error { return nil }, Supported: false}
	}
	return FrontOps{
		Frontmost: func() (string, error) {
			out, err := exec.Command("osascript", "-e",
				`tell application "System Events" to get name of first application process whose frontmost is true`).Output()
			return strings.TrimSpace(string(out)), err
		},
		// 이름을 AppleScript 소스에 직접 이어붙이면(원래 구현) 따옴표·백슬래시가 섞인 앱
		// 이름에서 구문이 깨질 수 있다. argv로 넘겨 osascript가 문자열로 다루게 한다.
		Activate: func(name string) error {
			return exec.Command("osascript", "-e",
				"on run argv\n tell application \"System Events\" to set frontmost of process (item 1 of argv) to true\nend run",
				name).Run()
		},
		Supported: true,
	}
}

// frontSleep — 테스트가 RestoreFront의 대기 루프를 즉시 통과시키려고 갈아끼우는 훅.
var frontSleep = time.Sleep

// RestoreFront — launch 전 앞 앱을 기억했다가 launch 뒤 되돌린다. ops 실패는 삼키고 launch 오류만 돌려준다.
// Chrome은 창을 만들며 스스로 활성화하므로 launch 반환 뒤 잠깐(최대 3초, 0.5초 간격) 앞 앱이
// 브라우저로 바뀌는 것을 기다린 뒤 복원한다 — 너무 일찍 복원하면 Chrome이 다시 앞으로 온다.
func RestoreFront(ops FrontOps, launch func() error) error {
	prev, err := ops.Frontmost()
	if err != nil || prev == "" || prev == EngineAppName {
		return launch()
	}
	if err := launch(); err != nil {
		return err
	}
	for i := 0; i < 6; i++ {
		if cur, err := ops.Frontmost(); err == nil && cur == EngineAppName {
			break
		}
		frontSleep(500 * time.Millisecond)
	}
	_ = ops.Activate(prev)
	return nil
}

// RewriteNewPage — new_page 호출에 background가 없고 브라우저가 앞이 아니면 background:true.
func RewriteNewPage(line []byte, browserInFront bool) []byte {
	if browserInFront {
		return line
	}
	var msg map[string]any
	if json.Unmarshal(line, &msg) != nil || msg["method"] != "tools/call" {
		return line
	}
	params, _ := msg["params"].(map[string]any)
	if params == nil || params["name"] != "new_page" {
		return line
	}
	args, _ := params["arguments"].(map[string]any)
	if args == nil {
		args = map[string]any{}
		params["arguments"] = args
	}
	if _, set := args["background"]; set {
		return line
	}
	args["background"] = true
	out, err := json.Marshal(msg)
	if err != nil {
		return line
	}
	return append(out, '\n')
}
