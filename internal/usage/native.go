package usage

// Claude Code 상태줄과 /usage 화면, Codex 세션 기록과 app-server에서 한도 값을 읽는다.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// NativeWindow는 저장 파일의 창 하나 — 공식 값 그대로(사용률 0~100, 재설정 시각 Unix 초).
type NativeWindow struct {
	UsedPct  float64 `json:"used_pct"`
	ResetsAt int64   `json:"resets_at"`
}

// NativeFile은 제공자 하나의 저장 파일.
type NativeFile struct {
	Provider string                  `json:"provider"`
	Source   string                  `json:"source"`  // claude-statusline | codex-rollout | codex-app-server
	Updated  time.Time               `json:"updated"` // 값을 받은(측정한) 때
	Plan     string                  `json:"plan,omitempty"`
	Windows  map[string]NativeWindow `json:"windows"`          // 5h | 7d | daily | <모델>_7d
	Labels   map[string]string       `json:"labels,omitempty"` // /usage 화면 이름(예: Current week (Fable))
}

// NativeDir은 저장 폴더.
func NativeDir(stateDir string) string { return filepath.Join(stateDir, "usage") }

// ClaudeFileName은 Claude 저장 파일 이름 — CLAUDE_CONFIG_DIR이 따로면(다른 계정) 구분자를 붙인다.
func ClaudeFileName(configDir string) string {
	if configDir == "" {
		return "claude.json"
	}
	h := sha256.Sum256([]byte(filepath.Clean(configDir)))
	return "claude-" + hex.EncodeToString(h[:])[:8] + ".json"
}

func readNative(path string) *NativeFile {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f NativeFile
	if json.Unmarshal(b, &f) != nil || f.Windows == nil {
		return nil
	}
	return &f
}

// writeNative는 임시 파일 → rename으로 원자적으로 쓴다(권한 600, 폴더 700).
func writeNative(path string, f *NativeFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0o600)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".usage-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// mergeWindows는 새 측정(fresh)을 창마다 합친다. 재설정 시각이 같거나 늦으면 새 값이 이기고, 더 이르면(지난 창) 버린다.
// 같은 창에서 사용률이 내려갈 수 있으므로 새 값을 쓴다.
// 쉬던 세션이 되풀이하는 낡은 값은 호출자(SaveClaudeStatus)가 세션별로 먼저 거른다.
// 이번에 없는 창은 지난 값을 둔다(Claude Code는 재설정 시각이 지난 창을 빼고 보낸다 — 「재설정됨」으로 보이게).
func mergeWindows(old, fresh map[string]NativeWindow) (map[string]NativeWindow, bool) {
	out := map[string]NativeWindow{}
	for k, v := range old {
		out[k] = v
	}
	changed := false
	for k, v := range fresh {
		o, ok := out[k]
		if !ok || v.ResetsAt >= o.ResetsAt {
			out[k] = v
			changed = changed || !ok || o != v
		}
	}
	return out, changed
}

// ── 읽기: 저장 파일 → 지금 Payload 꼴(coach --json과 같은 꼴 + 값마다 시각·출처) ──

var winMinutes = map[string]float64{"5h": 300, "daily": 1440, "7d": 10080}

// leftPct는 공급자 화면에 맞춰 남은 비율을 계산한다.
// Claude는 사용률을 내림하므로 8.6% 사용은 92% 남음으로 표시한다. Codex는 남은 비율을 반올림한다.
func leftPct(source string, used float64) float64 {
	var shown float64
	if strings.HasPrefix(source, "claude") {
		shown = math.Floor(used + 1e-9) // 상태줄 값은 round(e*1000)/10 — 9.0이 8.9999…로 내려가지 않게
	} else {
		shown = 100 - math.Round(100-used)
	}
	return math.Max(0, math.Min(100, 100-shown))
}

// nativeProvider는 저장 파일 하나를 Provider로 — 판단(등급·권고)은 coach.py classify와 같은 규칙(classify.go).
// from이 있으면 창마다 그 값을 준 파일의 시각·출처를 쓴다. 모델별 창(<모델>_7d)은
// coach처럼 표시 전용 — 판단에는 넣지 않는다.
func nativeProvider(f *NativeFile, now time.Time, from ...map[string]*NativeFile) Provider {
	pr := Provider{OK: true, Plan: f.Plan, Windows: map[string]Window{}, Source: f.Source}
	pr.MeasuredAt = f.Updated.UTC().Format(time.RFC3339)
	var cls []classWin
	keys := make([]string, 0, len(f.Windows))
	for k := range f.Windows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		w := f.Windows[k]
		src := f
		if len(from) > 0 && from[0][k] != nil {
			src = from[0][k]
		}
		left := math.Max(0, math.Min(100, 100-w.UsedPct)) // 판단(classify)은 원값 그대로
		leftR := leftPct(src.Source, w.UsedPct)
		win := Window{LeftPct: &leftR, MeasuredAt: src.Updated.UTC().Format(time.RFC3339), Source: src.Source, Label: f.Labels[k]}
		mins := time.Unix(w.ResetsAt, 0).Sub(now).Minutes()
		span, ok := winMinutes[k]
		if !ok {
			span = winMinutes["7d"] // 모델별 주간 창
		}
		if w.ResetsAt > 0 && -mins > span {
			continue // 재설정 뒤 창 한 바퀴가 지나도록 새 값이 없다 — 없어진 창(Ultra의 5시간)이거나 못 읽는 창, 지난 숫자를 보이지 않는다
		}
		if w.ResetsAt > 0 && mins <= 0 {
			win.ResetPassed = true // 재설정 시각이 지났다 — 숫자는 지난 값(흐리게), 새 값 기다림
			zero := 0.0
			win.ResetMin = &zero
		} else if w.ResetsAt > 0 {
			m := math.Round(mins)
			win.ResetMin = &m
			if wm, ok := winMinutes[k]; ok {
				cls = append(cls, classWin{wm: wm, left: left / 100, mins: mins, timeLeft: mins / wm})
			}
		}
		pr.Windows[k] = win
	}
	pr.Level, pr.Action, pr.Reason = classify(cls)
	return pr
}

// ReadNative는 저장 파일들을 합친다. 아무 값도 없으면 nil(화면은 「아직 못 읽음」).
// stateDir: 저장 폴더의 상위, home: Codex 기록을 찾을 홈.
func ReadNative(stateDir, home string, now time.Time) *Payload {
	provs := map[string]Provider{}
	var newest time.Time
	// Claude: 계정(설정 폴더)이 여럿이면 가장 최근에 받은 것
	var claude *NativeFile
	if m, _ := filepath.Glob(filepath.Join(NativeDir(stateDir), "claude*.json")); len(m) > 0 {
		for _, p := range m {
			if filepath.Base(p) == probeFile {
				continue
			}
			if f := readNative(p); f != nil && (claude == nil || f.Updated.After(claude.Updated)) {
				claude = f
			}
		}
	}
	// /usage 화면 값(모델별 주간 창 포함)과 창마다 합친다
	if merged, from := mergeClaude(claude, readNative(filepath.Join(NativeDir(stateDir), probeFile))); merged != nil {
		provs["claude"] = nativeProvider(merged, now, from)
		newest = merged.Updated
	}
	if cx := codexLatest(stateDir, home); cx != nil {
		provs["codex"] = nativeProvider(cx, now)
		if cx.Updated.After(newest) {
			newest = cx.Updated
		}
	}
	if len(provs) == 0 {
		return nil
	}
	return &Payload{TS: newest.UTC().Format("2006-01-02T15:04:05Z"), Providers: provs}
}

// ── 경로 고르기(기능 플래그): 공식 경로(native, 기본) / CodexBar 경로(coach — 비공식, 사용자가 명시로 고를 때만) ──

const (
	SourceNative   = "native"
	SourceCodexBar = "codexbar"
)

// SourceOf는 지금 쓰는 경로. AGENTLAYER_USAGE_SOURCE가 먼저, 없으면 <state>/usage/source 파일 — 어느 쪽이든 「codexbar」라고
// 명시했을 때만 CodexBar 경로, 그 밖(없음·빈 값·모르는 값)은 전부 공식 경로.
func SourceOf(stateDir string) string {
	v := os.Getenv("AGENTLAYER_USAGE_SOURCE")
	if v == "" {
		if b, err := os.ReadFile(filepath.Join(NativeDir(stateDir), "source")); err == nil {
			v = string(trimSpace(b))
		}
	}
	if v == SourceCodexBar {
		return SourceCodexBar
	}
	return SourceNative
}

// SetSource는 경로 플래그를 파일로 남긴다.
func SetSource(stateDir, src string) error {
	if src != SourceNative && src != SourceCodexBar {
		return fmt.Errorf("모르는 경로 %q (native|codexbar)", src)
	}
	if err := os.MkdirAll(NativeDir(stateDir), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(NativeDir(stateDir), "source"), []byte(src+"\n"), 0o600)
}

func trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == ' ' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

// Current는 화면들이 부르는 한 곳 — 새 경로면 저장 파일을 바로 읽고(즉시·캐시 불필요), CodexBar 경로면 예전대로 coach 캐시.
func Current(stateDir string, ttl time.Duration, runner func() ([]byte, error), now time.Time) *Payload {
	if SourceOf(stateDir) == SourceNative {
		maybeAutoProbe(stateDir, now)
		home, _ := os.UserHomeDir()
		return ReadNative(stateDir, home, now)
	}
	return FetchCached(stateDir, ttl, runner, now)
}

// CurrentCached는 첫 화면에 저장된 값만 표시한다.
func CurrentCached(stateDir string, now time.Time) *Payload {
	if SourceOf(stateDir) == SourceNative {
		home, _ := os.UserHomeDir()
		return ReadNative(stateDir, home, now)
	}
	return ReadCached(stateDir)
}

// maybeAutoProbe는 /usage 값(모델별 주간 창)이 갱신 간격보다 묵었으면 뒤에서 한 번 읽게 한다 — 기다리지 않는다.
func maybeAutoProbe(stateDir string, now time.Time) {
	if AutoProbe != nil && ProbeDue(stateDir, now) {
		AutoProbe(stateDir)
	}
}

// SpawnProbe는 현재 표시를 막지 않도록 별도 프로세스에서 공식 한도를 읽는다.
func SpawnProbe(stateDir string) {
	bin, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(bin, "usage", "probe", "claude")
	cmd.Env = append(os.Environ(), "AGENTLAYER_STATE_DIR="+stateDir)
	detachProbe(cmd)
	if cmd.Start() == nil {
		go func() { _ = cmd.Wait() }()
	}
}
