package usage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

// Codex 사용량 — 네트워크 없이 공식 CLI가 내 디스크에 쓴 세션 기록(rollout-*.jsonl)의 끝 64KB에서 마지막 rate_limits.
// 세션 기록 형식: {"timestamp":…, "type":"event_msg", "payload":{"type":"token_count", …, "rate_limits":{"primary":{"used_percent","window_minutes","resets_at"},"secondary":{…},"plan_type":…}}}
// 공식 문서 필드가 아니어서 바뀌면 수동 새로 고침(app-server)으로 보완한다.

const codexTailBytes = 64 << 10

var winKeyOf = map[int]string{300: "5h", 1440: "daily", 10080: "7d"}

type codexLimit struct {
	UsedPercent   *float64 `json:"used_percent"`
	WindowMinutes *int     `json:"window_minutes"`
	ResetsAt      *int64   `json:"resets_at"`
}
type codexRollLine struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type       string `json:"type"`
		RateLimits *struct {
			Primary   *codexLimit `json:"primary"`
			Secondary *codexLimit `json:"secondary"`
			PlanType  string      `json:"plan_type"`
		} `json:"rate_limits"`
	} `json:"payload"`
}

// CodexScanFiles는 rate_limits를 찾아 최근 순으로 볼 rollout 파일 수.
const CodexScanFiles = 20

type rolloutFile struct {
	path string
	mod  time.Time
}

// recentRollouts는 ~/.codex/sessions 아래 rollout-*.jsonl을 최근에 고친 순으로 최대 n개.
func recentRollouts(home string, n int) []rolloutFile {
	var all []rolloutFile
	root := filepath.Join(home, ".codex", "sessions")
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if len(name) < 14 || name[:8] != "rollout-" || filepath.Ext(name) != ".jsonl" {
			return nil
		}
		if info, err := d.Info(); err == nil {
			all = append(all, rolloutFile{p, info.ModTime()})
		}
		return nil
	})
	sort.Slice(all, func(i, j int) bool { return all[i].mod.After(all[j].mod) })
	if len(all) > n {
		all = all[:n]
	}
	return all
}

// readTail은 파일 끝 n바이트(첫 줄은 잘렸을 수 있어 버린다).
func readTail(path string, n int64) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	off := int64(0)
	if info.Size() > n {
		off = info.Size() - n
	}
	b := make([]byte, info.Size()-off)
	if _, err := f.ReadAt(b, off); err != nil && err != io.EOF {
		return nil
	}
	if off > 0 {
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	return b
}

// parseCodexTail은 기록 꼬리에서 마지막 rate_limits. 못 찾으면 nil.
func parseCodexTail(tail []byte, fallback time.Time) *NativeFile {
	var last *NativeFile
	sc := bufio.NewScanner(bytes.NewReader(tail))
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.Contains(line, []byte(`"rate_limits"`)) {
			continue
		}
		var l codexRollLine
		if json.Unmarshal(line, &l) != nil || l.Payload.RateLimits == nil {
			continue
		}
		ws := map[string]NativeWindow{}
		for _, lim := range []*codexLimit{l.Payload.RateLimits.Primary, l.Payload.RateLimits.Secondary} {
			if lim == nil || lim.UsedPercent == nil || lim.WindowMinutes == nil || lim.ResetsAt == nil {
				continue
			}
			k, ok := winKeyOf[*lim.WindowMinutes]
			if !ok {
				k = fmt.Sprintf("%dm", *lim.WindowMinutes)
			}
			ws[k] = NativeWindow{UsedPct: *lim.UsedPercent, ResetsAt: *lim.ResetsAt}
		}
		if len(ws) == 0 {
			continue
		}
		at := fallback
		if t, err := time.Parse(time.RFC3339Nano, l.Timestamp); err == nil {
			at = t
		}
		last = &NativeFile{Provider: "codex", Source: "codex-rollout", Updated: at.UTC(), Plan: l.Payload.RateLimits.PlanType, Windows: ws}
	}
	return last
}

// codexLatest는 기록 꼬리와 수동 새로 고침(app-server) 저장 파일 중 더 새것.
func codexLatest(stateDir, home string) *NativeFile {
	var roll *NativeFile
	for _, f := range recentRollouts(home, CodexScanFiles) {
		if roll = parseCodexTail(readTail(f.path, codexTailBytes), f.mod); roll != nil {
			break
		}
	}
	srv := readNative(filepath.Join(NativeDir(stateDir), "codex.json"))
	if srv != nil && (roll == nil || srv.Updated.After(roll.Updated)) {
		return srv
	}
	return roll
}

// ── 수동 새로 고침: 공식 `codex app-server`(JSONL, jsonrpc 칸 생략) account/rateLimits/read ──
// 문서: learn.chatgpt.com/docs/app-server — initialize(clientInfo) → initialized 알림 → account/rateLimits/read.
// 「experimental」이라 기본 경로가 아니고, 사용자가 새로 고침을 누를 때만 띄워 바로 끝낸다(시간 제한 10초).

const AppServerTimeout = 10 * time.Second

type appLimit struct {
	UsedPercent        *float64 `json:"usedPercent"`
	WindowDurationMins *int     `json:"windowDurationMins"`
	ResetsAt           *int64   `json:"resetsAt"`
}

// parseAppServerResult는 account/rateLimits/read 결과(rateLimits.primary/secondary)를 저장 꼴로.
func parseAppServerResult(raw json.RawMessage, now time.Time) (*NativeFile, error) {
	var r struct {
		RateLimits *struct {
			Primary   *appLimit `json:"primary"`
			Secondary *appLimit `json:"secondary"`
		} `json:"rateLimits"`
	}
	if err := json.Unmarshal(raw, &r); err != nil || r.RateLimits == nil {
		return nil, fmt.Errorf("rateLimits 없음")
	}
	ws := map[string]NativeWindow{}
	for _, l := range []*appLimit{r.RateLimits.Primary, r.RateLimits.Secondary} {
		if l == nil || l.UsedPercent == nil || l.WindowDurationMins == nil || l.ResetsAt == nil {
			continue
		}
		k, ok := winKeyOf[*l.WindowDurationMins]
		if !ok {
			k = fmt.Sprintf("%dm", *l.WindowDurationMins)
		}
		ws[k] = NativeWindow{UsedPct: *l.UsedPercent, ResetsAt: *l.ResetsAt}
	}
	if len(ws) == 0 {
		return nil, fmt.Errorf("창 없음")
	}
	return &NativeFile{Provider: "codex", Source: "codex-app-server", Updated: now.UTC(), Windows: ws}, nil
}

// RefreshCodex는 app-server를 잠깐 띄워 값을 받아 <state>/usage/codex.json에 쓴다. codexBin이 비면 PATH에서 찾는다.
func RefreshCodex(stateDir, codexBin string, now time.Time) error {
	if codexBin == "" {
		codexBin = LookupTool("codex")
	}
	if codexBin == "" {
		return fmt.Errorf("codex 없음")
	}
	ctx, cancel := context.WithTimeout(context.Background(), AppServerTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, codexBin, "app-server")
	cmd.Env = extendedEnv()
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		_ = in.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	send := func(v any) error {
		b, _ := json.Marshal(v)
		_, err := in.Write(append(b, '\n'))
		return err
	}
	if err := send(map[string]any{"method": "initialize", "id": 1, "params": map[string]any{"clientInfo": map[string]string{"name": "agentlayer", "title": "agentlayer usage", "version": "1"}}}); err != nil {
		return err
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	type resp struct {
		ID     *int            `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	sentRead := false
	for sc.Scan() {
		var r resp
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.ID == nil {
			continue // 알림 등
		}
		if len(r.Error) > 0 && string(r.Error) != "null" {
			return fmt.Errorf("app-server 오류: %s", string(r.Error))
		}
		switch *r.ID {
		case 1:
			if err := send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
				return err
			}
			if err := send(map[string]any{"method": "account/rateLimits/read", "id": 2}); err != nil {
				return err
			}
			sentRead = true
		case 2:
			f, err := parseAppServerResult(r.Result, now)
			if err != nil {
				return err
			}
			return writeNative(filepath.Join(NativeDir(stateDir), "codex.json"), f)
		}
	}
	if ctx.Err() != nil {
		return fmt.Errorf("app-server 시간 초과(%s)", AppServerTimeout)
	}
	if !sentRead {
		return fmt.Errorf("app-server가 initialize에 답하지 않음")
	}
	return fmt.Errorf("app-server가 rateLimits에 답하지 않음")
}

// sortedKeys(시험·출력용)
func sortedKeys(m map[string]NativeWindow) []string {
	k := make([]string, 0, len(m))
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}
