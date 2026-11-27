package usage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// TapMaxInput은 상태줄 JSON 상한 — 그 이상은 읽지 않고(저장도 안 함) 원래 명령에 넘긴다.
const TapMaxInput = 1 << 20

// claudeStatus는 상태줄 입력 중 우리가 보는 칸만(공식 문서 「Customize your status line」의 rate_limits).
// 다른 칸(경로·세션·모델·비용 등)은 읽지도 저장하지도 않는다.
type claudeStatus struct {
	SessionID  string `json:"session_id"` // 저장하지 않는다 — 세션마다 「새 값인가」를 가리는 데 해시로만 쓴다
	RateLimits *struct {
		FiveHour *claudeLimit `json:"five_hour"`
		SevenDay *claudeLimit `json:"seven_day"`
	} `json:"rate_limits"`
}
type claudeLimit struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *int64   `json:"resets_at"`
}

// parseClaudeStatus는 상태줄 JSON에서 창들만 꺼낸다(없으면 빈 맵 — Pro·Max가 아니거나 세션 첫 응답 전).
func parseClaudeStatus(b []byte) map[string]NativeWindow {
	var s claudeStatus
	if json.Unmarshal(b, &s) != nil || s.RateLimits == nil {
		return nil
	}
	out := map[string]NativeWindow{}
	put := func(k string, l *claudeLimit) {
		if l == nil || l.UsedPercentage == nil || l.ResetsAt == nil || *l.ResetsAt <= 0 {
			return
		}
		out[k] = NativeWindow{UsedPct: *l.UsedPercentage, ResetsAt: *l.ResetsAt}
	}
	put("5h", s.RateLimits.FiveHour)
	put("7d", s.RateLimits.SevenDay)
	return out
}

// claudeSessionOf는 상태줄 JSON의 session_id 해시(없으면 "").
func claudeSessionOf(b []byte) string {
	var s claudeStatus
	if json.Unmarshal(b, &s) != nil || s.SessionID == "" {
		return ""
	}
	h := sha256.Sum256([]byte(s.SessionID))
	return hex.EncodeToString(h[:])[:16]
}

// windowsSig는 창 값의 비교용 문자열.
func windowsSig(ws map[string]NativeWindow) string {
	var b strings.Builder
	for _, k := range sortedKeys(ws) {
		fmt.Fprintf(&b, "%s=%g@%d;", k, ws[k].UsedPct, ws[k].ResetsAt)
	}
	return b.String()
}

// seenFile — 세션(해시)마다 마지막으로 보낸 창 값. 상태줄은 응답이 없어도 다시 그려지며 그 세션이 마지막으로 받은
// rate_limits를 그대로 또 보낸다 — 같은 세션이 같은 값을 되풀이하면 새 측정이 아니다.
const claudeSeenFile = "claude-seen.json"

type seenEntry struct {
	Sig string    `json:"sig"`
	At  time.Time `json:"at"`
}

// SaveClaudeStatus는 상태줄 JSON 하나를 받아 새 측정일 때만 저장한다. 저장했으면 true.
//
// 같은 창에서도 사용률은 내려갈 수 있다. 같은 세션의 반복 값과 이미 지난 창은 버리고 새 측정값을 쓴다.
func SaveClaudeStatus(stateDir, configDir string, input []byte, now time.Time) (bool, error) {
	fresh := parseClaudeStatus(input)
	if len(fresh) == 0 {
		return false, nil
	}
	dir := NativeDir(stateDir)
	path := filepath.Join(dir, ClaudeFileName(configDir))
	old := readNative(path)
	var oldWin map[string]NativeWindow
	if old != nil {
		oldWin = old.Windows
	}
	sig := windowsSig(fresh)
	sid := claudeSessionOf(input)
	seenPath := filepath.Join(dir, claudeSeenFile)
	seen := map[string]seenEntry{}
	if b, err := os.ReadFile(seenPath); err == nil {
		_ = json.Unmarshal(b, &seen)
	}
	repeat := sid != "" && seen[sid].Sig == sig
	if !repeat && sid != "" {
		seen[sid] = seenEntry{Sig: sig, At: now.UTC()}
		for k, e := range seen { // 8일 넘은 세션은 잊는다(주간 창보다 길게)
			if now.Sub(e.At) > 8*24*time.Hour {
				delete(seen, k)
			}
		}
		if b, err := json.Marshal(seen); err == nil {
			_ = os.MkdirAll(dir, 0o700)
			_ = os.WriteFile(seenPath, b, 0o600)
		}
	}
	if repeat {
		// 되풀이는 새 측정이 아니다 — 지금 값과 같을 때만 시각을 새로
		if old == nil || windowsSig(old.Windows) != windowsSig(mergeWindowsOnly(oldWin, fresh)) || now.Sub(old.Updated) < time.Minute {
			return false, nil
		}
		return true, writeNative(path, &NativeFile{Provider: "claude", Source: "claude-statusline", Updated: now.UTC(), Windows: oldWin})
	}
	merged, changed := mergeWindows(oldWin, fresh)
	if !changed && old != nil && now.Sub(old.Updated) < time.Minute {
		return false, nil
	}
	return true, writeNative(path, &NativeFile{Provider: "claude", Source: "claude-statusline", Updated: now.UTC(), Windows: merged})
}

// mergeWindowsOnly는 mergeWindows의 결과 창만.
func mergeWindowsOnly(old, fresh map[string]NativeWindow) map[string]NativeWindow {
	m, _ := mergeWindows(old, fresh)
	return m
}

// RunTap은 `agentlayer usage tap claude -- <원래 명령…>`의 몸체.
// stdin을 그대로 원래 명령에 넘기고(출력·종료 코드 그대로), 그 사이 rate_limits만 저장한다.
// 저장이 실패해도 상태줄은 막지 않는다 — 오류는 삼킨다. 원래 명령이 없으면 아무것도 출력하지 않는다.
func RunTap(stateDir, configDir string, stdin io.Reader, stdout, stderr io.Writer, cmdArgs []string, now time.Time) int {
	input, _ := io.ReadAll(io.LimitReader(stdin, TapMaxInput+1))
	if len(input) <= TapMaxInput {
		_, _ = SaveClaudeStatus(stateDir, configDir, input, now)
	}
	if len(cmdArgs) == 0 {
		return 0
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	// 상한을 넘은 입력도 원래 명령에는 전부 넘긴다
	cmd.Stdin = io.MultiReader(bytesReader(input), stdin)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		return 127
	}
	return 0
}

type byteReader struct {
	b []byte
	i int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
func bytesReader(b []byte) io.Reader { return &byteReader{b: b} }

// ClaudeConfigDir는 Claude Code 설정 폴더(CLAUDE_CONFIG_DIR, 없으면 ~/.claude)와 「따로 지정됐는지」.
func ClaudeConfigDir(home string) (dir string, custom bool) {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d, true
	}
	return filepath.Join(home, ".claude"), false
}
