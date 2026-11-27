package usage

// Claude Code의 /usage 화면을 전용 가상 터미널에서 읽는다.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ProbeTimeout = 45 * time.Second
	probeFile    = "claude-usage.json"
	probeLock    = "claude-probe.lock"
	probeAttempt = "claude-probe.attempt" // 마지막으로 띄운 때(실패해도 지금 간격 안에는 다시 안 띄움)
)

var ErrProbeBusy = errors.New("다른 /usage 읽기가 이미 도는 중")

// ── 화면 해석 ──

var (
	ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?<>=]*[ -/]*[@-~]`)
	ansiOSC = regexp.MustCompile(`\x1b[\]P_^][^\x07\x1b]*(\x07|\x1b\\)`)
	ansiEsc = regexp.MustCompile(`\x1b[()][0-9A-Za-z]|\x1b[=>78DEHMNOc]`)
	spaces  = regexp.MustCompile(`[ \t\r\n]+`)
	// 「Current session ███ 68% used Resets 3:10am (Asia/Seoul)」·「Current week (all models) … 86% used Resets Oct 20 at 4pm (Asia/Seoul)」·
	// 「Current week (Fable) … 0% used Resets Oct 20 at 4pm (Asia/Seoul)」. 아래 「What's contributing…」의 「% of your usage」는 걸리지 않는다.
	usageLine = regexp.MustCompile(`(Current session|Current week \(([^)]{1,40})\))[^%]{0,400}?(\d{1,3}(?:\.\d+)?)% used(?:\s*·)?\s*(?:Resets\s+([^()]{1,40}?)\s*\(([A-Za-z_]+(?:/[A-Za-z_+\-]+)*)\))?`)
)

// screenText는 터미널 출력에서 제어열을 빼고 공백을 하나로.
func screenText(b []byte) string {
	b = ansiOSC.ReplaceAll(b, nil)
	b = ansiCSI.ReplaceAll(b, []byte(" "))
	b = ansiEsc.ReplaceAll(b, nil)
	b = bytes.ReplaceAll(b, []byte{0x1b}, nil)
	return spaces.ReplaceAllString(string(b), " ")
}

// UsageScreenWindow는 /usage 화면의 창 하나.
type UsageScreenWindow struct {
	Key      string  // 5h | 7d | <모델>_7d(예: fable_7d — CodexBar 비교 칸과 같은 이름)
	Label    string  // 화면 이름(예: Current week (Fable))
	UsedPct  float64 // 화면에 표시된 사용 비율
	ResetsAt int64   // Unix 초, 못 읽으면 0
}

// windowKey는 화면 이름 → 저장 키.
func windowKey(label, scope string) string {
	if label == "Current session" {
		return "5h"
	}
	s := strings.ToLower(strings.TrimSpace(scope))
	if s == "all models" || s == "" {
		return "7d"
	}
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, s)
	return strings.Trim(s, "_") + "_7d"
}

// parseUsageScreen은 화면 글자에서 창들(같은 이름이 여러 번 그려지면 마지막 것).
func parseUsageScreen(text string, now time.Time) []UsageScreenWindow {
	var order []string
	byKey := map[string]UsageScreenWindow{}
	for _, m := range usageLine.FindAllStringSubmatch(text, -1) {
		pct, err := strconv.ParseFloat(m[3], 64)
		if err != nil || pct < 0 || pct > 100 {
			continue
		}
		k := windowKey(m[1], m[2])
		w := UsageScreenWindow{Key: k, Label: m[1], UsedPct: pct}
		if m[4] != "" && m[5] != "" {
			if t, ok := parseResetText(m[4], m[5], now); ok {
				w.ResetsAt = t.Unix()
			}
		}
		if _, seen := byKey[k]; !seen {
			order = append(order, k)
		}
		byKey[k] = w
	}
	out := make([]UsageScreenWindow, 0, len(order))
	for _, k := range order {
		out = append(out, byKey[k])
	}
	return out
}

var (
	reClock = regexp.MustCompile(`^(\d{1,2})(?::(\d{2}))?\s*(am|pm)$`)
	reDate  = regexp.MustCompile(`(?i)^([a-z]{3})\s+(\d{1,2})(?:,\s*(\d{4}))?\s+at\s+(.+)$`)
)

// parseResetText는 「3:10am」·「4pm」·「Oct 20 at 4pm」(+ 시간대 이름) → 시각.
// 날짜가 없으면 오늘(지났으면 내일), 연도가 없으면 올해(한참 지났으면 내년).
func parseResetText(s, tz string, now time.Time) (time.Time, bool) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Time{}, false
	}
	s = strings.TrimSpace(s)
	clock := func(c string) (int, int, bool) {
		m := reClock.FindStringSubmatch(strings.ToLower(strings.TrimSpace(c)))
		if m == nil {
			return 0, 0, false
		}
		h, _ := strconv.Atoi(m[1])
		mi := 0
		if m[2] != "" {
			mi, _ = strconv.Atoi(m[2])
		}
		if h < 1 || h > 12 || mi > 59 {
			return 0, 0, false
		}
		if h == 12 {
			h = 0
		}
		if m[3] == "pm" {
			h += 12
		}
		return h, mi, true
	}
	n := now.In(loc)
	if d := reDate.FindStringSubmatch(s); d != nil {
		mon, err := time.Parse("Jan", strings.ToUpper(d[1][:1])+strings.ToLower(d[1][1:]))
		if err != nil {
			return time.Time{}, false
		}
		day, _ := strconv.Atoi(d[2])
		h, mi, ok := clock(d[4])
		if !ok {
			return time.Time{}, false
		}
		year := n.Year()
		if d[3] != "" {
			year, _ = strconv.Atoi(d[3])
		}
		t := time.Date(year, mon.Month(), day, h, mi, 0, 0, loc)
		if d[3] == "" && t.Before(n.Add(-24*time.Hour)) {
			t = t.AddDate(1, 0, 0)
		}
		return t, true
	}
	h, mi, ok := clock(s)
	if !ok {
		return time.Time{}, false
	}
	t := time.Date(n.Year(), n.Month(), n.Day(), h, mi, 0, 0, loc)
	if t.Before(n.Add(-time.Minute)) {
		t = t.AddDate(0, 0, 1)
	}
	return t, true
}

// ── 실행 ──

// ProbeArgs는 claude에 넘기는 인자 — 도구·훅·MCP·원격 시작을 끈다(저장된 설정은 그대로, 이 실행에만).
var ProbeArgs = []string{"--allowed-tools", "", "--strict-mcp-config", "--settings", `{"remoteControlAtStartup":false,"disableAllHooks":true}`}

type screenBuf struct {
	mu sync.Mutex
	b  []byte
}

func (s *screenBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	s.b = append(s.b, p...)
	if len(s.b) > 4<<20 { // 4MB 넘으면 앞을 버린다
		s.b = append([]byte(nil), s.b[len(s.b)-(2<<20):]...)
	}
	s.mu.Unlock()
	return len(p), nil
}
func (s *screenBuf) since(n int) ([]byte, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > len(s.b) {
		n = len(s.b)
	}
	return append([]byte(nil), s.b[n:]...), len(s.b)
}

// ProbeClaudeUsage는 공식 claude를 전용 폴더(probeDir)의 가상 터미널에서 띄워 /usage 화면을 읽고 끝낸다.
// 신뢰 확인 창은 이 전용 폴더에서만 「Yes, I trust this folder」로 넘긴다. claudeBin이 비면 PATH·흔한 위치에서 찾는다.
func ProbeClaudeUsage(ctx context.Context, claudeBin, probeDir string, now func() time.Time) ([]UsageScreenWindow, error) {
	if claudeBin == "" {
		claudeBin = LookupTool("claude")
	}
	if claudeBin == "" {
		return nil, fmt.Errorf("claude 없음")
	}
	if err := os.MkdirAll(probeDir, 0o700); err != nil {
		return nil, err
	}
	master, slave, err := openPTY()
	if err != nil {
		return nil, err
	}
	defer master.Close()
	_ = setWinsize(master, 60, 160)
	cmd := exec.Command(claudeBin, ProbeArgs...)
	cmd.Dir = probeDir
	cmd.Env = append(extendedEnv(), "TERM=xterm-256color", "COLUMNS=160", "LINES=60")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	ptyAttr(cmd)
	if err := cmd.Start(); err != nil {
		slave.Close()
		return nil, err
	}
	slave.Close()
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	defer func() {
		// 정상 종료부터(Esc로 화면 닫고 /exit — claude가 자기 상태를 저장하고 끝난다), 3초 안에 안 끝나면 프로세스 그룹째 강제 종료
		_, _ = master.Write([]byte("\x1b"))
		time.Sleep(300 * time.Millisecond)
		_, _ = master.Write([]byte("/exit"))
		time.Sleep(300 * time.Millisecond)
		_, _ = master.Write([]byte("\r"))
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
		killPTYGroup(cmd) // 남는 자식 없게
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()
	buf := &screenBuf{}
	go func() {
		b := make([]byte, 32<<10)
		for {
			n, err := master.Read(b)
			if n > 0 {
				_, _ = buf.Write(b[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	send := func(s string) { _, _ = master.Write([]byte(s)) }
	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()
	var (
		trusted, asked        bool
		askedAt, trustSeen    time.Time
		mark, seen, trustMark int
		best                  []UsageScreenWindow
		bestAt                time.Time
	)
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			if len(best) > 0 {
				return best, nil
			}
			return nil, fmt.Errorf("/usage 화면을 %s 안에 못 읽음%s", ProbeTimeout, debugTail(buf))
		case <-done:
			if len(best) > 0 {
				return best, nil
			}
			return nil, fmt.Errorf("claude가 먼저 끝남%s", debugTail(buf))
		case <-tick.C:
		}
		all, _ := buf.since(0)
		text := screenText(all)
		if !trusted && strings.Contains(text, "Yes, I trust this folder") {
			// 창이 보이자마자 키를 보내면 claude가 아직 키 입력 받을 준비(원시 모드) 전이라 엉켜 「No, exit」로 끝난다(10-18 실측) — 2초 기다린다
			if trustSeen.IsZero() {
				trustSeen = time.Now()
			}
			if time.Since(trustSeen) < 2*time.Second {
				continue
			}
			trusted = true
			send("\x1b[B") // 기본 선택이 「No, exit」 — 한 칸 내려 「Yes」
			time.Sleep(300 * time.Millisecond)
			send("\r")
			_, trustMark = buf.since(0)
			continue
		}
		if !asked {
			// 입력칸이 준비되면 /usage 한 번 — 시작 배너(「Claude Code v…」)가 신뢰 확인 뒤에 그려졌을 때.
			// 신뢰 확인 창 문장(「Claude Code'll be able…」)에도 「Claude Code」가 있어 그것만으로는 안 된다(그 창에 /usage를 치면 「No, exit」로 끝난다).
			fresh, _ := buf.since(trustMark)
			ft := screenText(fresh)
			if time.Since(start) > 3*time.Second && strings.Contains(ft, "Claude Code v") && !strings.Contains(ft, "Yes, I trust this folder") {
				time.Sleep(1500 * time.Millisecond) // 입력칸이 자리 잡게
				_, mark = buf.since(0)
				send("/usage")
				time.Sleep(400 * time.Millisecond)
				send("\r")
				asked, askedAt = true, time.Now()
			}
			continue
		}
		after, n := buf.since(mark)
		if n == seen {
			if len(best) > 0 && time.Since(bestAt) > 1500*time.Millisecond {
				return best, nil // 화면이 1.5초 넘게 그대로 — 다 그려졌다(닫기는 defer가 Esc·/exit)
			}
			continue
		}
		seen = n
		ws := parseUsageScreen(screenText(after), now())
		//  주간 창만 있어도 읽은 것이다 — Ultra 화면(2.1.316~)에는 「Current session」이 없어 5h·7d 둘 다를 기다리다 10-31부터 매번 실패했다
		if hasKeys(ws, "7d") && len(ws) >= len(best) {
			best, bestAt = ws, time.Now()
		}
		if !hasKeys(best, "7d") && time.Since(askedAt) > 12*time.Second && time.Since(askedAt) < 13*time.Second {
			send("\r") // 화면이 안 열렸으면 한 번 더
		}
	}
}

func hasKeys(ws []UsageScreenWindow, keys ...string) bool {
	for _, k := range keys {
		ok := false
		for _, w := range ws {
			if w.Key == k {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// RefreshClaudeUsage는 잠금을 잡고 /usage를 읽어 <state>/usage/claude-usage.json에 쓴다(숫자·시각·출처만).
func RefreshClaudeUsage(ctx context.Context, stateDir, claudeBin string, now time.Time) error {
	return refreshClaude(ctx, stateDir, claudeBin, now, false)
}

// RefreshOfficial은 두 공급자의 한도를 각각 조회한다. 한쪽이 실패해도 다른 쪽의 값은 저장한다.
func RefreshOfficial(stateDir string, now time.Time) error {
	claudeErr := RefreshClaudeUsage(context.Background(), stateDir, "", now)
	if claudeErr != nil {
		claudeErr = fmt.Errorf("Claude /usage: %w", claudeErr)
	}
	codexErr := RefreshCodex(stateDir, "", time.Now())
	if codexErr != nil {
		codexErr = fmt.Errorf("Codex app-server: %w", codexErr)
	}
	return errors.Join(claudeErr, codexErr)
}

// ProbeCodexBin은 뒤에서 잴 때 함께 다시 읽는 codex — 비면 PATH에서 찾는다(시험은 가짜로 바꾼다).
var ProbeCodexBin = ""

// AutoProbeOnce는 뒤에서 한 번 잰다(`usage probe claude` — AutoProbe가 띄운다). Claude /usage 화면과 같은 차례에 Codex도 다시 읽는다
// 전엔 Codex를 「다시 읽기」 때만 다시 읽어, Codex를 안 쓰는 동안 Codex 고리가 묵고 칩 나이(가장 오래된 공급자)도 함께 늘었다.
// Codex 기록 꼬리(쓸 때마다 남음)나 저장 값이 갱신 간격보다 새로우면 띄우지 않는다. codex가 없거나 못 읽으면 넘어간다(Claude는 그대로 잰다).
func AutoProbeOnce(ctx context.Context, stateDir string, now time.Time) error {
	return refreshClaude(ctx, stateDir, "", now, true)
}

func codexDue(stateDir string, now time.Time) bool {
	home, _ := os.UserHomeDir()
	cx := codexLatest(stateDir, home)
	return cx == nil || now.Sub(cx.Updated) >= probeInterval
}

func refreshClaude(ctx context.Context, stateDir, claudeBin string, now time.Time, withCodex bool) error {
	dir := NativeDir(stateDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	lock, ok := tryLock(filepath.Join(dir, probeLock))
	if !ok {
		return ErrProbeBusy
	}
	defer lock.Close()
	if withCodex && !ProbeDue(stateDir, now) {
		return nil
	}
	_ = os.WriteFile(filepath.Join(dir, probeAttempt), []byte(now.UTC().Format(time.RFC3339)+"\n"), 0o600)
	if withCodex && codexDue(stateDir, now) {
		_ = RefreshCodex(stateDir, ProbeCodexBin, time.Now()) // 단계 갱신(아래)이 새 Codex 값을 보게 먼저
	}
	ws, err := ProbeClaudeUsage(ctx, claudeBin, filepath.Join(dir, "claude-probe"), time.Now)
	if err != nil {
		return err
	}
	f := &NativeFile{Provider: "claude", Source: "claude-usage-screen", Updated: time.Now().UTC(), Windows: map[string]NativeWindow{}, Labels: map[string]string{}}
	for _, w := range ws {
		f.Windows[w.Key] = NativeWindow{UsedPct: w.UsedPct, ResetsAt: w.ResetsAt}
		f.Labels[w.Key] = w.Label
	}
	if err := writeNative(filepath.Join(dir, probeFile), f); err != nil {
		return err
	}
	return nil
}

const probeInterval = 30 * time.Minute

// ProbeDue는 마지막으로 시도한 때부터 갱신 간격이 지났는지 확인한다.
func ProbeDue(stateDir string, now time.Time) bool {
	dir := NativeDir(stateDir)
	if b, err := os.ReadFile(filepath.Join(dir, probeAttempt)); err == nil {
		if t, err := time.Parse(time.RFC3339, string(trimSpace(b))); err == nil {
			return now.Sub(t) >= probeInterval
		}
	}
	f := readNative(filepath.Join(dir, probeFile))
	return f == nil || now.Sub(f.Updated) >= probeInterval
}

// AutoProbe는 값이 묵었을 때 뒤에서 /usage 읽기를 띄우는 함수 — main이 채운다(`agentlayer usage probe claude`를 떼어 실행).
// 비어 있으면(시험·라이브러리) 아무것도 띄우지 않는다.
var AutoProbe func(stateDir string)

// mergeClaude는 상태줄 값과 /usage 화면 값을 창마다 합친다 — 늦은 재설정이 이기고, 같은 창이면 나중에 잰 값,
// 모델별 창은 화면에만 있다. 창마다 그 값을 받은 때·출처를 따로 둔다.
func mergeClaude(status, screen *NativeFile) (*NativeFile, map[string]*NativeFile) {
	from := map[string]*NativeFile{}
	var base *NativeFile
	for _, f := range []*NativeFile{status, screen} {
		if f == nil {
			continue
		}
		if base == nil {
			base = &NativeFile{Provider: "claude", Source: f.Source, Updated: f.Updated, Windows: map[string]NativeWindow{}, Labels: map[string]string{}}
		}
		for k, w := range f.Windows {
			o, ok := base.Windows[k]
			// 같은 창이면 나중에 잰 값이 이긴다(사용률은 같은 창에서도 내려갈 수 있다 — 2026-10-18 주간 97%→2%)
			if !ok || w.ResetsAt > o.ResetsAt || (w.ResetsAt == o.ResetsAt && f.Updated.After(from[k].Updated)) {
				base.Windows[k] = w
				from[k] = f
			}
			if l := f.Labels[k]; l != "" {
				base.Labels[k] = l
			}
		}
		if f.Updated.After(base.Updated) {
			base.Updated, base.Source = f.Updated, f.Source
		}
	}
	return base, from
}

// roundPct는 화면용 정수 반올림.
func roundPct(f float64) float64 { return math.Round(f) }

// debugTail은 AGENTLAYER_PROBE_DEBUG=1일 때만 화면 끝 400자를 오류에 붙인다(평소에는 화면 내용을 남기지 않는다).
func debugTail(buf *screenBuf) string {
	if os.Getenv("AGENTLAYER_PROBE_DEBUG") != "1" {
		return ""
	}
	all, _ := buf.since(0)
	t := screenText(all)
	if len(t) > 400 {
		t = t[len(t)-400:]
	}
	return " — 화면 끝: " + t
}
