package usage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 제어열·진행 막대·두 번 그려짐·아래 「What's contributing…」(% of your usage — 창이 아니다)
const fakeUsageScreen = "\x1b[2J\x1b[H\x1b[1mCurrent session\x1b[22m \x1b[38;5;75m██████████████\x1b[39m 68% used\r\n Resets 3:10am (Asia/Seoul)\r\n" +
	"Current week (all models) ███████████ 86% used\r\nResets Oct 20 at 4pm (Asia/Seoul)\r\n" +
	"What's contributing to your limits usage? Scanning local sessions… Refreshing…\r\n" +
	"\x1b[12;1HCurrent week (Fable) \x1b[38;5;75m\x1b[39m 0% used\r\n Resets Oct 20 at 4pm (Asia/Seoul)\r\n" +
	"Last 24h · 91% of your usage was at >150k context · 61% of your usage was while 4+ sessions ran in parallel\r\n" +
	"\x1b[3;1HCurrent session ███████████████ 69% used Resets 3:10am (Asia/Seoul)\r\n" + // 다시 그려짐(마지막 값이 이긴다)
	"You've used 86% of your weekly limit · resets Oct 20 at 4pm (Asia/Seoul)\r\n"

var seoul = time.FixedZone("KST", 9*60*60)

func TestParseUsageScreen(t *testing.T) {
	now := time.Date(2026, 10, 18, 2, 0, 0, 0, seoul)
	ws := parseUsageScreen(screenText([]byte(fakeUsageScreen)), now)
	got := map[string]UsageScreenWindow{}
	for _, w := range ws {
		got[w.Key] = w
	}
	if len(ws) != 3 {
		t.Fatalf("창 셋(5h·7d·fable_7d): %+v", ws)
	}
	if got["5h"].UsedPct != 69 || got["5h"].ResetsAt != time.Date(2026, 10, 18, 3, 10, 0, 0, seoul).Unix() {
		t.Errorf("현재 세션(다시 그린 값): %+v", got["5h"])
	}
	if got["7d"].UsedPct != 86 || got["7d"].ResetsAt != time.Date(2026, 10, 20, 16, 0, 0, 0, seoul).Unix() || got["7d"].Label != "Current week (all models)" {
		t.Errorf("전체 주간: %+v", got["7d"])
	}
	if f := got["fable_7d"]; f.UsedPct != 0 || f.ResetsAt != time.Date(2026, 10, 20, 16, 0, 0, 0, seoul).Unix() {
		t.Errorf("Fable 주간(CodexBar 비교 칸과 같은 이름 fable_7d): %+v", f)
	}
}

func TestWindowKeyNames(t *testing.T) {
	cases := map[[2]string]string{{"Current session", ""}: "5h", {"Current week", "all models"}: "7d", {"Current week", "Fable"}: "fable_7d", {"Current week", "Opus"}: "opus_7d", {"Current week", "Sonnet 5"}: "sonnet_5_7d"}
	for in, want := range cases {
		if got := windowKey(in[0], in[1]); got != want {
			t.Errorf("%v → %s, 기대 %s", in, got, want)
		}
	}
}

func TestParseResetText(t *testing.T) {
	now := time.Date(2026, 10, 18, 3, 30, 0, 0, seoul)
	cases := []struct {
		s    string
		want time.Time
	}{
		{"3:10am", time.Date(2026, 10, 19, 3, 10, 0, 0, seoul)}, // 오늘 3:10은 지났다 → 내일
		{"4pm", time.Date(2026, 10, 18, 16, 0, 0, 0, seoul)},
		{"12am", time.Date(2026, 10, 19, 0, 0, 0, 0, seoul)},
		{"Oct 20 at 4pm", time.Date(2026, 10, 20, 16, 0, 0, 0, seoul)},
		{"Jan 2 at 9:05am", time.Date(2027, 1, 2, 9, 5, 0, 0, seoul)}, // 연도 넘김
		{"Oct 20, 2026 at 4pm", time.Date(2026, 10, 20, 16, 0, 0, 0, seoul)},
	}
	for _, c := range cases {
		got, ok := parseResetText(c.s, "Asia/Seoul", now)
		if !ok || !got.Equal(c.want) {
			t.Errorf("%q → %v %v, 기대 %v", c.s, got, ok, c.want)
		}
	}
	if _, ok := parseResetText("soon", "Asia/Seoul", now); ok {
		t.Error("모르는 꼴은 못 읽음")
	}
	if _, ok := parseResetText("4pm", "Nowhere/City", now); ok {
		t.Error("모르는 시간대는 못 읽음")
	}
}

func TestReadNativeMergesStatusAndUsageScreen(t *testing.T) {
	dir, home := t.TempDir(), t.TempDir()
	now := time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC)
	r5, r7 := now.Add(2*time.Hour).Unix(), now.Add(60*time.Hour).Unix()
	// 상태줄: 더 새로 받은 5h·7d
	_ = writeNative(filepath.Join(NativeDir(dir), "claude.json"), &NativeFile{Provider: "claude", Source: "claude-statusline", Updated: now.Add(-2 * time.Minute), Windows: map[string]NativeWindow{"5h": {UsedPct: 70, ResetsAt: r5}, "7d": {UsedPct: 86, ResetsAt: r7}}})
	// /usage 화면: 20분 전, 5h는 더 작은 값(같은 창 — 사용률은 줄지 않으니 상태줄 값), Fable은 화면에만
	_ = writeNative(filepath.Join(NativeDir(dir), probeFile), &NativeFile{Provider: "claude", Source: "claude-usage-screen", Updated: now.Add(-20 * time.Minute), Windows: map[string]NativeWindow{"5h": {UsedPct: 60, ResetsAt: r5}, "7d": {UsedPct: 86, ResetsAt: r7}, "fable_7d": {UsedPct: 12, ResetsAt: r7}}, Labels: map[string]string{"fable_7d": "Current week (Fable)"}})
	p := ReadNative(dir, home, now)
	c := p.Providers["claude"]
	if *c.Windows["5h"].LeftPct != 30 || c.Windows["5h"].Source != "claude-statusline" {
		t.Errorf("5h는 상태줄(같은 창 큰 사용률): %+v", c.Windows["5h"])
	}
	f := c.Windows["fable_7d"]
	if f.LeftPct == nil || *f.LeftPct != 88 || f.Source != "claude-usage-screen" || f.Label != "Current week (Fable)" || f.MeasuredAt != "2026-10-18T02:40:00Z" {
		t.Errorf("Fable 창(화면에서, 그 시각·출처): %+v", f)
	}
	if c.Level == "" {
		t.Error("판단은 5h·7d로(모델별 창은 표시 전용)")
	}
	// 상태줄 파일만 있을 때는 예전과 같다
	_ = os.Remove(filepath.Join(NativeDir(dir), probeFile))
	if _, ok := ReadNative(dir, home, now).Providers["claude"].Windows["fable_7d"]; ok {
		t.Error("화면 값이 없으면 모델별 창 없음")
	}
}

func TestProbeDue(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC)
	if !ProbeDue(dir, now) {
		t.Fatal("값이 없으면 읽을 때")
	}
	probePath := filepath.Join(NativeDir(dir), probeFile)
	if err := writeNative(probePath, &NativeFile{Updated: now, Windows: map[string]NativeWindow{}}); err != nil {
		t.Fatal(err)
	}
	if ProbeDue(dir, now.Add(29*time.Minute)) || !ProbeDue(dir, now.Add(30*time.Minute)) {
		t.Fatal("시도 기록이 없으면 값이 30분 묵었을 때")
	}
	attempt := now.Add(30 * time.Minute)
	if err := os.WriteFile(filepath.Join(NativeDir(dir), probeAttempt), []byte(attempt.Format(time.RFC3339)), 0o600); err != nil {
		t.Fatal(err)
	}
	if ProbeDue(dir, attempt.Add(29*time.Minute)) || !ProbeDue(dir, attempt.Add(30*time.Minute)) {
		t.Fatal("측정에 실패해도 시도한 지 30분 뒤에 다시 읽는다")
	}
}

// 가짜 claude: 신뢰 확인 창 → (아래 화살표 + Enter) → 시작 화면 → /usage 를 받으면 화면을 그린다. 받은 인자를 파일로 남긴다.
const fakeClaude = `#!/bin/sh
printf '%s\n' "$@" > "$PROBE_ARGS"
pwd > "$PROBE_CWD"
printf 'Quick safety check\r\nClaude Code'"'"'ll be able to read, edit, and execute files here.\r\n\x1b[1m❯ No, exit\x1b[22m\r\n  Yes, I trust this folder\r\nEnter to confirm\r\n'
stty raw -echo 2>/dev/null
got=""
while :; do
  c=$(dd bs=1 count=1 2>/dev/null) || exit 0
  [ -z "$c" ] && continue
  got="$got$c"
  case "$got" in
    *"[B"*) case "$got" in *"$(printf '\r')"*) printf '\x1b[2J ▐▛███▜▌ Claude Code v9\r\n> '; got=""; trusted=1 ;; esac ;;
  esac
  case "$got" in
    *"/usage"*"$(printf '\r')"*) [ -n "$trusted" ] || exit 7; printf '%s' "$SCREEN"; got="" ;;
    *"/exit"*"$(printf '\r')"*) echo exited > "$PROBE_EXIT"; exit 0 ;;
  esac
done
`

func TestProbeClaudeUsageWithFakeClaude(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "claude")
	if err := os.WriteFile(bin, []byte(fakeClaude), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROBE_ARGS", filepath.Join(tmp, "args"))
	t.Setenv("PROBE_CWD", filepath.Join(tmp, "cwd"))
	t.Setenv("SCREEN", fakeUsageScreen)
	t.Setenv("PROBE_EXIT", filepath.Join(tmp, "exit"))
	probe := filepath.Join(tmp, "probe")
	now := func() time.Time { return time.Date(2026, 10, 18, 2, 0, 0, 0, seoul) }
	start := time.Now()
	ws, err := ProbeClaudeUsage(context.Background(), bin, probe, now)
	if err != nil {
		t.Fatalf("%v (%s)", err, time.Since(start))
	}
	if !hasKeys(ws, "5h", "7d", "fable_7d") {
		t.Fatalf("창 셋: %+v", ws)
	}
	args, _ := os.ReadFile(filepath.Join(tmp, "args"))
	for _, want := range []string{"--allowed-tools", "--strict-mcp-config", `"disableAllHooks":true`} {
		if !strings.Contains(string(args), want) {
			t.Errorf("도구·MCP·훅 끄는 인자 %q: %s", want, args)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(tmp, "exit")); !strings.Contains(string(b), "exited") {
		t.Error("/exit로 정상 종료(claude가 자기 상태를 저장하게)")
	}
	if cwd, _ := os.ReadFile(filepath.Join(tmp, "cwd")); !strings.Contains(string(cwd), "probe") {
		t.Errorf("전용 폴더에서: %s", cwd)
	}
}

func TestRefreshClaudeUsageOneAtATime(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(NativeDir(dir), 0o700)
	lock, ok := tryLock(filepath.Join(NativeDir(dir), probeLock))
	if !ok {
		t.Fatal("잠금")
	}
	defer lock.Close()
	if err := RefreshClaudeUsage(context.Background(), dir, "/bin/false", time.Now()); err != ErrProbeBusy {
		t.Errorf("이미 도는 중이면 띄우지 않는다: %v", err)
	}
}

func TestProbeClaudeUsageGivesUpWithinTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "claude")
	_ = os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 120\n"), 0o700)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second) // ProbeTimeout(45초) 대신 짧게
	defer cancel()
	start := time.Now()
	if _, err := ProbeClaudeUsage(ctx, bin, filepath.Join(tmp, "p"), time.Now); err == nil || time.Since(start) > 8*time.Second {
		t.Errorf("시간 제한 안에서 포기하고 프로세스를 끝낸다: %v %s", err, time.Since(start))
	}
}

// Ultra 화면(Claude Code 2.1.316, 11-03)에는 「Current session」이 없다 — 주간 창만으로 읽혀야 한다
const ultraUsageScreen = "Current week (all models) ████████████ 90% used Resets Nov 3 at 4pm (Asia/Seoul)\r\n" +
	"What's contributing to your usage? Approximate · sessions on this machine only Scanning local sessions… Refreshing…\r\n" +
	"Current week (Fable) ███████ 14% used Resets Nov 3 at 3:59pm (Asia/Seoul)\r\n" +
	"You've used 90% of your weekly limit · resets 4pm (Asia/Seoul) · /usage-credits to keep using Claude Code\r\n"

func TestParseUsageScreenUltraNoSession(t *testing.T) {
	now := time.Date(2026, 11, 3, 14, 40, 0, 0, seoul)
	ws := parseUsageScreen(screenText([]byte(ultraUsageScreen)), now)
	if !hasKeys(ws, "7d") || hasKeys(ws, "5h") {
		t.Fatalf("주간 창만: %+v", ws)
	}
	for _, w := range ws {
		if w.Key == "7d" && (w.UsedPct != 90 || w.ResetsAt != time.Date(2026, 11, 3, 16, 0, 0, 0, seoul).Unix()) {
			t.Errorf("전체 주간: %+v", w)
		}
	}
}
