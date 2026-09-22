// internal/browser/control.go
package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// 소유권 = 브라우저를 지금 누가 쓰는가. 정본은 <stateDir>/browser-control.json 하나 —
// mcp-serve 프록시가 클로드 세션마다 하나씩 떠서 파일이어야 전부 같은 값을 본다.
// 설계: docs/superpowers/specs/2026-09-22-agent-browser-control-design.md 1절.
type Owner string

const (
	OwnerIdle  Owner = "idle"
	OwnerAgent Owner = "agent"
	OwnerUser  Owner = "user"
)

// ControlExpiry — 마지막 도구 호출 뒤 이 시간 동안 호출이 없으면 agent 소유가 풀린다.
const ControlExpiry = 20 * time.Second

type Control struct {
	Owner    Owner     `json:"owner"`
	Agent    string    `json:"agent,omitempty"`   // 마지막으로 도구를 부른 에이전트 ID
	Label    string    `json:"label,omitempty"`   // 알약에 보일 작업명
	Since    time.Time `json:"since"`             // 현재 owner가 된 시각
	LastCall time.Time `json:"last_call"`         // 마지막 도구 호출
	Stopped  bool      `json:"stopped,omitempty"` // 사용자가 「중단」을 눌렀는지
	Waiting  int       `json:"waiting,omitempty"` // 게이트에 잡혀 있는 호출 수(마지막으로 쓴 프록시 값)
}

type Event int

const (
	EvCall       Event = iota // 도구 호출 통과
	EvUserTake                // 「내가 조작하기」
	EvUserReturn              // 「AI에게 돌려주기」
	EvUserStop                // 「중단」
)

// Apply는 전이 표(스펙 1절)를 그대로 옮긴 순수 함수. stopped 상태에서 EvCall은 무시한다 —
// 게이트가 그 호출을 통과시키지 않기 때문에 상태도 바뀌면 안 된다.
func Apply(c Control, ev Event, agent, label string, now time.Time) Control {
	switch ev {
	case EvCall:
		if c.Owner == OwnerUser {
			return c
		}
		if c.Owner != OwnerAgent {
			c.Since = now
		}
		c.Owner = OwnerAgent
		c.Agent, c.Label, c.LastCall = agent, label, now
	case EvUserTake:
		c.Owner, c.Since, c.Stopped = OwnerUser, now, false
	case EvUserStop:
		c.Owner, c.Since, c.Stopped = OwnerUser, now, true
	case EvUserReturn:
		c.Owner, c.Since, c.Stopped, c.LastCall = OwnerAgent, now, false, now
	}
	return c
}

// Expired — agent 소유가 호출 없이 ControlExpiry를 넘겼는지. user 소유는 만료가 없다.
func (c Control) Expired(now time.Time) bool {
	return c.Owner == OwnerAgent && now.Sub(c.LastCall) >= ControlExpiry
}

// Effective는 만료를 반영한 사본.
func (c Control) Effective(now time.Time) Control {
	if c.Expired(now) {
		c.Owner = OwnerIdle
	}
	return c
}

func controlPath(dir string) string { return filepath.Join(dir, "browser-control.json") }

// LoadControl — 파일이 없거나 깨졌으면 idle.
func LoadControl(dir string) Control {
	b, err := os.ReadFile(controlPath(dir))
	if err != nil {
		return Control{Owner: OwnerIdle}
	}
	var c Control
	if json.Unmarshal(b, &c) != nil || c.Owner == "" {
		return Control{Owner: OwnerIdle}
	}
	return c
}

// SaveControl — 임시 파일 → rename, 0600(board.RememberRoot와 같은 규칙).
func SaveControl(dir string, c Control) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	p := controlPath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".browser-control.*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// mirrorSafe — 미러 값은 ':'로 나누므로 본문의 ':'는 비슷한 글자(U+2236)로 바꾼다.
func mirrorSafe(s string) string { return strings.ReplaceAll(s, ":", "∶") }

// MirrorValue는 <html data-agentlayer-owner>에 쓸 값.
// owner:agent:label:since_ms:last_ms:stopped(0/1):waiting:target(0/1):title
func MirrorValue(c Control, target bool, title string) string {
	b := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	return strings.Join([]string{
		string(c.Owner), mirrorSafe(c.Agent), mirrorSafe(c.Label),
		strconv.FormatInt(c.Since.UnixMilli(), 10), strconv.FormatInt(c.LastCall.UnixMilli(), 10),
		b(c.Stopped), strconv.Itoa(c.Waiting), b(target), mirrorSafe(title),
	}, ":")
}

// ParseRequest는 콘텐츠 스크립트가 <html data-agentlayer-request>에 남긴 버튼 요청.
func ParseRequest(v string) (Event, int64, bool) {
	kind, msStr, ok := strings.Cut(v, ":")
	if !ok {
		return 0, 0, false
	}
	ms, err := strconv.ParseInt(msStr, 10, 64)
	if err != nil || ms == 0 {
		return 0, 0, false
	}
	switch kind {
	case "user":
		return EvUserTake, ms, true
	case "agent":
		return EvUserReturn, ms, true
	case "stop":
		return EvUserStop, ms, true
	}
	return 0, 0, false
}
