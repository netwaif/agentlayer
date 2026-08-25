// Package usage는 외부 데이터(coach 사용량, 세션 컨텍스트)를 읽기 전용으로
// 소비한다. 어느 소스가 없어도 에러 대신 빈 값을 돌려줘서 코어 관제를
// 멈추지 않는다.
package usage

import (
	"encoding/json"
	"fmt"
	"os/exec"
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

// CoachRunner는 기본 실행기: PATH의 coach를 부른다.
func CoachRunner() ([]byte, error) {
	return exec.Command("coach", "--json").Output()
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
