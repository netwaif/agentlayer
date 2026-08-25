// Package usage는 외부 데이터(coach 사용량, 세션 컨텍스트)를 읽기 전용으로
// 소비한다. 어느 소스가 없어도 에러 대신 빈 값을 돌려줘서 코어 관제를
// 멈추지 않는다.
package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Window는 한도 창 하나 (5h/7d/Fable/계정명 등). coach가 데이터 없는
// 창은 null로 주므로 포인터로 받는다.
type Window struct {
	LeftPct  *float64 `json:"left_pct"`
	ResetMin *float64 `json:"reset_min"`
}

// Provider는 coach의 provider 블록 (claude/codex/antigravity).
type Provider struct {
	OK      bool              `json:"ok"`
	Plan    string            `json:"plan"`
	Email   string            `json:"email"`
	Level   string            `json:"level"` // red|yellow|wait|white|green
	Action  string            `json:"action"`
	Reason  string            `json:"reason"`
	Windows map[string]Window `json:"windows"`
}

// Payload는 coach --json 전체.
type Payload struct {
	TS        string              `json:"ts"`
	Providers map[string]Provider `json:"providers"`
}

// toolDirs는 PATH 탐색 실패 시 짚어볼 흔한 설치 위치.
// tmux 팝업·LaunchAgent는 최소 PATH(/usr/bin:/bin…)로 뜨는 일이 많다.
func toolDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	return append(dirs, "/opt/homebrew/bin", "/usr/local/bin")
}

// lookupTool은 name을 PATH에서, 없으면 흔한 위치에서 찾는다.
func lookupTool(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, d := range toolDirs() {
		p := filepath.Join(d, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// extendedEnv는 현재 env에서 PATH만 흔한 도구 위치까지 넓힌 것.
// coach가 내부에서 codexbar를 PATH로 찾으므로 자식 env도 넓혀줘야 한다.
func extendedEnv() []string {
	path := os.Getenv("PATH")
	for _, d := range toolDirs() {
		if !strings.Contains(":"+path+":", ":"+d+":") {
			path += ":" + d
		}
	}
	env := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PATH=") {
			env = append(env, kv)
		}
	}
	return append(env, "PATH="+path)
}

// CoachRunner는 기본 실행기: coach를 찾아 --json으로 부른다.
func CoachRunner() ([]byte, error) {
	bin := lookupTool("coach")
	if bin == "" {
		return nil, fmt.Errorf("coach 없음")
	}
	cmd := exec.Command(bin, "--json")
	cmd.Env = extendedEnv()
	return cmd.Output()
}

// Fetch는 coach 출력을 파싱한다. coach가 없거나 실패하면 (nil, nil) —
// 사용량 패널만 생략되고 관제는 계속된다.
func Fetch(runner func() ([]byte, error)) (*Payload, error) {
	out, err := runner()
	if err != nil {
		return nil, nil
	}
	var p Payload
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, nil // 깨진 출력도 패널 생략으로 처리
	}
	return &p, nil
}

// Gauge는 남은 비율을 유니코드 바로 그린다. nil(데이터 없음)은 빈 바.
func Gauge(pct *float64, width int) string {
	if pct == nil {
		return strings.Repeat("░", width)
	}
	filled := int(*pct/100*float64(width) + 0.5)
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// ResetLabel은 리셋까지 남은 시간을 "N분/시간/일 후"로 표기한다.
func ResetLabel(min *float64) string {
	if min == nil {
		return ""
	}
	d := time.Duration(*min) * time.Minute
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d분 후", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d시간 후", int(d.Hours()))
	default:
		// 일 단위는 반올림 — "5.7일"을 "6일 후"로 보여주는 게 체감에 맞다
		return fmt.Sprintf("%d일 후", int(d.Hours()/24+0.5))
	}
}
