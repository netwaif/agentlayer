package usage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 시험 — 모두 가짜 입력(상태줄 JSON·rollout·app-server 흉내). 실제 사용량 조회는 하지 않는다.

var t0 = time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC)

func statusJSON(fivePct float64, fiveReset int64, sevenPct float64, sevenReset int64) []byte {
	return []byte(`{"session_id":"s-secret","cwd":"/private/path","model":{"display_name":"Opus"},"rate_limits":{"five_hour":{"used_percentage":` +
		jsonNum(fivePct) + `,"resets_at":` + jsonInt(fiveReset) + `},"seven_day":{"used_percentage":` + jsonNum(sevenPct) + `,"resets_at":` + jsonInt(sevenReset) + `}}}`)
}
func jsonNum(f float64) string { b, _ := json.Marshal(f); return string(b) }
func jsonInt(i int64) string   { b, _ := json.Marshal(i); return string(b) }

func TestParseClaudeStatusOnlyRateLimits(t *testing.T) {
	ws := parseClaudeStatus(statusJSON(23.5, 1738425600, 41.2, 1738857600))
	if len(ws) != 2 || ws["5h"].UsedPct != 23.5 || ws["7d"].ResetsAt != 1738857600 {
		t.Fatalf("창 두 개: %+v", ws)
	}
	if parseClaudeStatus([]byte(`{"model":{}}`)) != nil {
		t.Error("rate_limits 없으면 nil(Pro·Max 아님 또는 첫 응답 전)")
	}
	if len(parseClaudeStatus([]byte(`{"rate_limits":{"five_hour":{"used_percentage":5}}}`))) != 0 {
		t.Error("resets_at 없는 창은 버린다")
	}
}

func TestSaveClaudeStatusAtomicPrivateAndOnlyNumbers(t *testing.T) {
	dir := t.TempDir()
	saved, err := SaveClaudeStatus(dir, "", statusJSON(10, t0.Add(2*time.Hour).Unix(), 30, t0.Add(72*time.Hour).Unix()), t0)
	if err != nil || !saved {
		t.Fatalf("저장: %v %v", saved, err)
	}
	p := filepath.Join(NativeDir(dir), "claude.json")
	info, err := os.Stat(p)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("권한 600: %v %v", info, err)
	}
	if d, _ := os.Stat(NativeDir(dir)); d.Mode().Perm() != 0o700 {
		t.Errorf("폴더 700: %v", d.Mode().Perm())
	}
	b, _ := os.ReadFile(p)
	for _, secret := range []string{"s-secret", "/private/path", "Opus"} {
		if bytes.Contains(b, []byte(secret)) {
			t.Errorf("숫자·시각·출처만 저장해야 한다 — %q가 들어갔다", secret)
		}
	}
	if m, _ := filepath.Glob(filepath.Join(NativeDir(dir), ".usage-*.tmp")); len(m) != 0 {
		t.Errorf("임시 파일이 남았다: %v", m)
	}
	// 같은 값이면 다시 쓰지 않는다
	if saved, _ := SaveClaudeStatus(dir, "", statusJSON(10, t0.Add(2*time.Hour).Unix(), 30, t0.Add(72*time.Hour).Unix()), t0.Add(30*time.Second)); saved {
		t.Error("값이 같으면(1분 안) 쓰지 않는다")
	}
	// 5시간 창이 빠진 입력(재설정 지남) — 지난 값은 남긴다
	if _, err := SaveClaudeStatus(dir, "", []byte(`{"rate_limits":{"seven_day":{"used_percentage":31,"resets_at":`+jsonInt(t0.Add(72*time.Hour).Unix())+`}}}`), t0.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	f := readNative(p)
	if f.Windows["5h"].UsedPct != 10 || f.Windows["7d"].UsedPct != 31 {
		t.Errorf("없는 창은 지난 값 유지: %+v", f.Windows)
	}
}

func TestClaudeFileNamePerConfigDir(t *testing.T) {
	if ClaudeFileName("") != "claude.json" {
		t.Error("기본 설정 폴더는 claude.json")
	}
	a, b := ClaudeFileName("/x/a"), ClaudeFileName("/x/b")
	if a == b || !strings.HasPrefix(a, "claude-") {
		t.Errorf("설정 폴더마다 다른 파일: %s %s", a, b)
	}
}

func TestRunTapPassesThroughOutputAndExitCode(t *testing.T) {
	dir := t.TempDir()
	in := statusJSON(50, t0.Add(time.Hour).Unix(), 20, t0.Add(48*time.Hour).Unix())
	var out, errb bytes.Buffer
	// 원래 상태줄 명령: stdin에서 모델 이름을 꺼내 찍고 종료 코드 3
	code := RunTap(dir, "", bytes.NewReader(in), &out, &errb, []string{"sh", "-c", `sed -n 's/.*"display_name":"\([A-Za-z]*\)".*/[\1]/p'; echo err >&2; exit 3`}, t0)
	if code != 3 || strings.TrimSpace(out.String()) != "[Opus]" || strings.TrimSpace(errb.String()) != "err" {
		t.Fatalf("원래 명령 그대로: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if f := readNative(filepath.Join(NativeDir(dir), "claude.json")); f == nil || f.Windows["5h"].UsedPct != 50 {
		t.Errorf("그 사이 저장: %+v", f)
	}
	// 원래 명령이 없으면 아무것도 안 찍는다
	out.Reset()
	if RunTap(dir, "", bytes.NewReader(in), &out, &errb, nil, t0) != 0 || out.Len() != 0 {
		t.Error("명령 없으면 빈 출력·0")
	}
	// 저장 폴더를 못 써도 상태줄은 막지 않는다
	out.Reset()
	if code := RunTap("/dev/null/nope", "", bytes.NewReader(in), &out, &errb, []string{"sh", "-c", "echo ok"}, t0); code != 0 || strings.TrimSpace(out.String()) != "ok" {
		t.Errorf("저장 실패해도 통과: %d %q", code, out.String())
	}
}

func TestReadNativePayloadShapeAndResetPassed(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	_, _ = SaveClaudeStatus(dir, "", statusJSON(70, t0.Add(-5*time.Minute).Unix(), 20, t0.Add(80*time.Hour).Unix()), t0.Add(-10*time.Minute))
	p := ReadNative(dir, home, t0)
	if p == nil {
		t.Fatal("값이 있으면 Payload")
	}
	c := p.Providers["claude"]
	if !c.OK || c.Source != "claude-statusline" || c.MeasuredAt != "2026-10-18T02:50:00Z" {
		t.Fatalf("출처·시각: %+v", c)
	}
	five, seven := c.Windows["5h"], c.Windows["7d"]
	if !five.ResetPassed || *five.LeftPct != 30 || *five.ResetMin != 0 {
		t.Errorf("재설정 지난 5h는 지난 값 + 표시: %+v", five)
	}
	if seven.ResetPassed || *seven.LeftPct != 80 || *seven.ResetMin != 4800 {
		t.Errorf("7d: left 80 · 4800분: %+v %v", seven, *seven.ResetMin)
	}
	if _, ok := p.Providers["antigravity"]; ok {
		t.Error("Antigravity는 표시하지 않는다")
	}
	// coach --json 꼴 그대로 읽힌다(새 칸은 덧붙임)
	b, _ := json.Marshal(p)
	var back Payload
	if json.Unmarshal(b, &back) != nil || back.Providers["claude"].Windows["7d"].LeftPct == nil {
		t.Error("Payload 왕복")
	}
	if !strings.Contains(string(b), `"left_pct":80`) || !strings.Contains(string(b), `"reset_min":4800`) {
		t.Errorf("기존 칸 이름 그대로: %s", b)
	}
	if ReadNative(t.TempDir(), home, t0) != nil {
		t.Error("값이 없으면 nil")
	}
}

func TestClassifyMatchesCoach(t *testing.T) {
	// coach.py classify와 같은 판정(대표 경우 몇 개)
	cases := []struct {
		ws    []classWin
		level string
	}{
		{[]classWin{{wm: 300, left: 0.8, mins: 200, timeLeft: 200.0 / 300}, {wm: 10080, left: 0.7, mins: 5000, timeLeft: 5000.0 / 10080}}, "green"},
		{[]classWin{{wm: 300, left: 0.8, mins: 200, timeLeft: 200.0 / 300}, {wm: 10080, left: 0.2, mins: 8000, timeLeft: 8000.0 / 10080}}, "red"},
		{[]classWin{{wm: 300, left: 0.1, mins: 20, timeLeft: 20.0 / 300}}, "wait"},
		{[]classWin{{wm: 300, left: 0.1, mins: 250, timeLeft: 250.0 / 300}}, "yellow"},
		{[]classWin{{wm: 300, left: 0.3, mins: 60, timeLeft: 60.0 / 300}}, "white"},
		{[]classWin{{wm: 10080, left: 0.6, mins: 3000, timeLeft: 3000.0 / 10080}}, "green"},
		{nil, ""},
	}
	for i, c := range cases {
		if lv, _, _ := classify(c.ws); lv != c.level {
			t.Errorf("경우 %d: %q, 기대 %q", i, lv, c.level)
		}
	}
	if durText(245) != "4시간 5분" || durText(1500) != "1일 1시간" || durText(12) != "12분" {
		t.Errorf("durText: %s %s %s", durText(245), durText(1500), durText(12))
	}
}

func writeCodexRollout(t *testing.T, home, name string, lines []string, mod time.Time) string {
	t.Helper()
	d := filepath.Join(home, ".codex", "sessions", "2026", "10", "18")
	if err := os.MkdirAll(d, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(d, name)
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(p, mod, mod)
	return p
}

func tokenLine(ts string, p5, p7 float64, r5, r7 int64) string {
	return `{"timestamp":"` + ts + `","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1}},"rate_limits":{"limit_id":"codex","primary":{"used_percent":` + jsonNum(p5) + `,"window_minutes":300,"resets_at":` + jsonInt(r5) + `},"secondary":{"used_percent":` + jsonNum(p7) + `,"window_minutes":10080,"resets_at":` + jsonInt(r7) + `},"credits":{"has_credits":false,"unlimited":false,"balance":"0"},"plan_type":"plus"}}}`
}

func TestCodexRolloutTailLastRateLimits(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	r5, r7 := t0.Add(3*time.Hour).Unix(), t0.Add(100*time.Hour).Unix()
	// 오래된 파일(다른 값)과 새 파일 — 새 파일의 마지막 rate_limits를 쓴다. 대화 줄은 건너뛴다.
	writeCodexRollout(t, home, "rollout-old.jsonl", []string{tokenLine("2026-10-17T01:00:00Z", 99, 99, r5, r7)}, t0.Add(-24*time.Hour))
	big := strings.Repeat(`{"type":"response_item","payload":{"type":"message","content":"`+strings.Repeat("x", 1000)+`"}}`+"\n", 100) // 꼬리 64KB 밖으로 밀리는 앞부분
	writeCodexRollout(t, home, "rollout-new.jsonl", []string{tokenLine("2026-10-18T01:00:00Z", 88, 88, r5, r7), big, tokenLine("2026-10-18T02:30:00Z", 12.5, 40, r5, r7), `{"type":"response_item","payload":{"type":"message","content":"비밀 대화"}}`}, t0.Add(-time.Hour))
	f := codexLatest(dir, home)
	if f == nil || f.Source != "codex-rollout" || f.Windows["5h"].UsedPct != 12.5 || f.Windows["7d"].UsedPct != 40 || f.Plan != "plus" {
		t.Fatalf("새 파일 마지막 값: %+v", f)
	}
	if !f.Updated.Equal(time.Date(2026, 10, 18, 2, 30, 0, 0, time.UTC)) {
		t.Errorf("측정 시각 = 그 줄의 timestamp: %v", f.Updated)
	}
	p := ReadNative(dir, home, t0)
	cx := p.Providers["codex"]
	if *cx.Windows["5h"].LeftPct != 88 || *cx.Windows["5h"].ResetMin != 180 || cx.Source != "codex-rollout" {
		t.Errorf("Payload 5h: %+v", cx.Windows["5h"])
	}
	// 수동 새로 고침 파일이 더 새로우면 그쪽
	_ = writeNative(filepath.Join(NativeDir(dir), "codex.json"), &NativeFile{Provider: "codex", Source: "codex-app-server", Updated: t0, Windows: map[string]NativeWindow{"5h": {UsedPct: 1, ResetsAt: r5}}})
	if f := codexLatest(dir, home); f.Source != "codex-app-server" || f.Windows["5h"].UsedPct != 1 {
		t.Errorf("더 새로운 app-server 값: %+v", f)
	}
}

func TestParseAppServerResult(t *testing.T) {
	raw := json.RawMessage(`{"rateLimits":{"limitId":"codex","limitName":null,"primary":{"usedPercent":25,"windowDurationMins":300,"resetsAt":1760000000},"secondary":null},"rateLimitsByLimitId":{}}`)
	f, err := parseAppServerResult(raw, t0)
	if err != nil || f.Windows["5h"].UsedPct != 25 || len(f.Windows) != 1 || f.Source != "codex-app-server" {
		t.Fatalf("%+v %v", f, err)
	}
	if _, err := parseAppServerResult(json.RawMessage(`{}`), t0); err == nil {
		t.Error("rateLimits 없으면 오류")
	}
}

// 가짜 codex(app-server 흉내): initialize에 답하고, initialized 뒤 account/rateLimits/read에 답한다
const fakeCodex = `#!/bin/sh
[ "$1" = "app-server" ] || exit 9
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*) echo '{"id":1,"result":{"userAgent":"fake"}}' ;;
    *'"method":"account/rateLimits/read"'*) echo '{"method":"note","params":{}}'; echo '{"id":2,"result":{"rateLimits":{"primary":{"usedPercent":33,"windowDurationMins":300,"resetsAt":1760000000},"secondary":{"usedPercent":5,"windowDurationMins":10080,"resetsAt":1760500000}}}}' ;;
  esac
done
`

func TestRefreshCodexWithFakeAppServer(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(bin, []byte(fakeCodex), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := RefreshCodex(dir, bin, t0); err != nil {
		t.Fatal(err)
	}
	f := readNative(filepath.Join(NativeDir(dir), "codex.json"))
	if f == nil || f.Windows["5h"].UsedPct != 33 || f.Windows["7d"].UsedPct != 5 {
		t.Fatalf("app-server 값 저장: %+v", f)
	}
	// 답하지 않는 codex는 시간 제한 안에서 끝난다
	hang := filepath.Join(t.TempDir(), "codex")
	_ = os.WriteFile(hang, []byte("#!/bin/sh\nexec sleep 30\n"), 0o700)
	start := time.Now()
	if err := RefreshCodex(dir, hang, t0); err == nil || time.Since(start) > AppServerTimeout+3*time.Second {
		t.Errorf("시간 제한: %v %v", err, time.Since(start))
	}
}

func TestSetupClaudeWrapUndoKeepsOtherKeys(t *testing.T) {
	cfg := t.TempDir()
	orig := `{
  "model": "opus",
  "statusLine": {"type": "command", "command": "jq -r '\"[\\(.model.display_name)]\"' | tr a-z A-Z", "padding": 2},
  "hooks": {"Stop": []}
}`
	p := filepath.Join(cfg, "settings.json")
	_ = os.WriteFile(p, []byte(orig), 0o600)
	res, err := SetupClaude(cfg, "/opt/agent layer/agentlayer", false, t0)
	if err != nil || !res.Changed || res.Backup == "" {
		t.Fatalf("감싸기: %+v %v", res, err)
	}
	if b, _ := os.ReadFile(res.Backup); string(b) != orig {
		t.Error(".bak은 고치기 전 원문")
	}
	var s map[string]any
	b, _ := os.ReadFile(p)
	if json.Unmarshal(b, &s) != nil {
		t.Fatalf("고친 파일이 JSON: %s", b)
	}
	if s["model"] != "opus" || s["hooks"] == nil || s["statusLine"].(map[string]any)["padding"].(float64) != 2 {
		t.Errorf("다른 칸 그대로: %v", s)
	}
	if strings.Index(string(b), `"model"`) > strings.Index(string(b), `"statusLine"`) || strings.Index(string(b), `"statusLine"`) > strings.Index(string(b), `"hooks"`) {
		t.Errorf("칸 순서 그대로: %s", b)
	}
	cmd := s["statusLine"].(map[string]any)["command"].(string)
	if inner, ok := UnwrapStatusCommand(cmd); !ok || inner != `jq -r '"[\(.model.display_name)]"' | tr a-z A-Z` {
		t.Errorf("원래 명령이 그대로 들어 있다: %q %q", cmd, inner)
	}
	// 실제 셸로 감싼 명령의 인자 모양 확인(tap 뒤 sh -c <원래>)
	if again, _ := SetupClaude(cfg, "/opt/agent layer/agentlayer", false, t0); again.Changed {
		t.Error("두 번 감싸지 않는다")
	}
	und, err := SetupClaude(cfg, "/opt/agent layer/agentlayer", true, t0)
	if err != nil || !und.Changed {
		t.Fatalf("되돌리기: %+v %v", und, err)
	}
	if und.Backup == res.Backup {
		t.Error("같은 초에 두 번 고쳐도 앞 사본을 덮지 않는다")
	}
	if b, _ := os.ReadFile(res.Backup); string(b) != orig {
		t.Error("첫 사본은 원문 그대로")
	}
	var back map[string]any
	b2, _ := os.ReadFile(p)
	if strings.Index(string(b2), `"type"`) > strings.Index(string(b2), `"command"`) || strings.Index(string(b2), `"command"`) > strings.Index(string(b2), `"padding"`) {
		t.Errorf("statusLine 안 칸 순서도 그대로: %s", b2)
	}
	_ = json.Unmarshal(b2, &back)
	if back["statusLine"].(map[string]any)["command"] != `jq -r '"[\(.model.display_name)]"' | tr a-z A-Z` {
		t.Errorf("원래 명령으로: %v", back["statusLine"])
	}
}

func TestSetupClaudeWithoutStatusLineAddsAndRemoves(t *testing.T) {
	cfg := t.TempDir() // settings.json 없음
	res, err := SetupClaude(cfg, "/usr/local/bin/agentlayer", false, t0)
	if err != nil || !res.Changed || res.Backup != "" || res.After != "'/usr/local/bin/agentlayer' usage tap claude --" {
		t.Fatalf("새로 넣기: %+v %v", res, err)
	}
	if _, err := SetupClaude(cfg, "/usr/local/bin/agentlayer", true, t0); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(cfg, "settings.json"))
	if strings.Contains(string(b), "statusLine") {
		t.Errorf("우리가 넣은 것은 지운다: %s", b)
	}
}

func TestWrapCommandRunsOriginalThroughShell(t *testing.T) {
	// 감싼 명령이 셸에서 원래 명령(파이프 포함)을 그대로 돌리는지 — tap 자리에 인자 출력기를 둔다
	orig := `echo 'a b' | tr a-z A-Z`
	w := WrapStatusCommand("/bin/echo", orig) // echo는 받은 인자를 그대로 찍는다
	out, err := execShell(w)
	if err != nil || strings.TrimSpace(out) != "usage tap claude -- sh -c "+orig {
		t.Fatalf("인자 그대로: %q %v", out, err)
	}
}

func TestSourceFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTLAYER_USAGE_SOURCE", "")
	if SourceOf(dir) != SourceNative {
		t.Error("기본은 공식 경로")
	}
	_ = SetSource(dir, SourceCodexBar)
	if SourceOf(dir) != SourceCodexBar {
		t.Error("파일로 명시하면 CodexBar")
	}
	_ = os.WriteFile(filepath.Join(NativeDir(dir), "source"), []byte("CodexBar?\n"), 0o600)
	if SourceOf(dir) != SourceNative {
		t.Error("모르는 값은 공식 경로")
	}
	t.Setenv("AGENTLAYER_USAGE_SOURCE", "codexbar")
	if SourceOf(dir) != SourceCodexBar {
		t.Error("환경 변수가 먼저")
	}
	if SetSource(dir, "x") == nil {
		t.Error("모르는 값 거절")
	}
}

func execShell(cmd string) (string, error) {
	out, err := exec.Command("sh", "-c", cmd).Output()
	return string(out), err
}

func TestMergeNewerWindowWinsAndSameWindowTakesFresh(t *testing.T) {
	r1, r2 := t0.Add(2*time.Hour).Unix(), t0.Add(7*time.Hour).Unix()
	old := map[string]NativeWindow{"5h": {UsedPct: 40, ResetsAt: r1}}
	if m, ch := mergeWindows(old, map[string]NativeWindow{"5h": {UsedPct: 10, ResetsAt: r1 - 18000}}); ch || m["5h"].UsedPct != 40 {
		t.Errorf("지난 창(resets_at 더 이름) 값은 버린다: %+v %v", m, ch)
	}
	// 같은 창에서 사용률이 내려가도 새 측정값을 쓴다.
	if m, ch := mergeWindows(old, map[string]NativeWindow{"5h": {UsedPct: 2, ResetsAt: r1}}); !ch || m["5h"].UsedPct != 2 {
		t.Errorf("같은 창 새 측정: %+v %v", m, ch)
	}
	if m, ch := mergeWindows(old, map[string]NativeWindow{"5h": {UsedPct: 2, ResetsAt: r2}}); !ch || m["5h"].UsedPct != 2 {
		t.Errorf("다음 창(resets_at 더 늦음)은 사용률이 작아도 이긴다: %+v %v", m, ch)
	}
}

func statusJSONFor(sid string, fivePct float64, fiveReset int64, sevenPct float64, sevenReset int64) []byte {
	return []byte(strings.Replace(string(statusJSON(fivePct, fiveReset, sevenPct, sevenReset)), `"s-secret"`, jsonStr(sid), 1))
}
func jsonStr(s string) string { b, _ := json.Marshal(s); return string(b) }

// 쉬던 세션은 다시 그려질 때 자기가 마지막으로 받은 값을 되풀이한다 — 그 되풀이로 다른 세션의 새 값을 덮지 않는다.
func TestStaleSessionDoesNotOverwriteNewValue(t *testing.T) {
	dir := t.TempDir()
	r5, r7 := t0.Add(2*time.Hour).Unix(), t0.Add(72*time.Hour).Unix()
	_, _ = SaveClaudeStatus(dir, "", statusJSONFor("idle", 50, r5, 30, r7), t0)
	_, _ = SaveClaudeStatus(dir, "", statusJSONFor("busy", 60, r5, 35, r7), t0.Add(time.Minute))
	if saved, _ := SaveClaudeStatus(dir, "", statusJSONFor("idle", 50, r5, 30, r7), t0.Add(5*time.Minute)); saved {
		t.Error("쉬던 세션의 되풀이로 덮지 않는다(시각도 새로 하지 않는다)")
	}
	// 지난 창(재설정 전) 값도 받지 않는다
	_, _ = SaveClaudeStatus(dir, "", statusJSONFor("old", 20, r5-18000, 35, r7), t0.Add(6*time.Minute))
	f := readNative(filepath.Join(NativeDir(dir), "claude.json"))
	if f.Windows["5h"].UsedPct != 60 || f.Windows["7d"].UsedPct != 35 {
		t.Errorf("새 값 유지: %+v %v", f.Windows, f.Updated)
	}
	if b, _ := os.ReadFile(filepath.Join(NativeDir(dir), claudeSeenFile)); strings.Contains(string(b), "idle") || strings.Contains(string(b), "busy") {
		t.Errorf("세션 ID 원문을 남기지 않는다: %s", b)
	}
}

// 새로 측정한 낮은 값은 저장하고 이전 세션이 반복한 값은 버린다.
func TestSameWindowUsageDropIsAccepted(t *testing.T) {
	dir := t.TempDir()
	r5, r7 := t0.Add(2*time.Hour).Unix(), t0.Add(72*time.Hour).Unix()
	_, _ = SaveClaudeStatus(dir, "", statusJSONFor("before", 40, r5, 97, r7), t0)
	if saved, _ := SaveClaudeStatus(dir, "", statusJSONFor("after", 5, r5, 2, r7), t0.Add(time.Hour)); !saved {
		t.Fatal("새 측정(사용률 내려감)은 저장한다")
	}
	_, _ = SaveClaudeStatus(dir, "", statusJSONFor("before", 40, r5, 97, r7), t0.Add(2*time.Hour))
	if f := readNative(filepath.Join(NativeDir(dir), "claude.json")); f.Windows["7d"].UsedPct != 2 {
		t.Errorf("7d=%v want 2", f.Windows["7d"].UsedPct)
	}
}

// 화면 값과 상태줄 값 — 같은 창이면 나중에 잰 쪽이 이긴다(크기 아님).
func TestMergeClaudeSameWindowNewerWins(t *testing.T) {
	r7 := t0.Add(72 * time.Hour).Unix()
	status := &NativeFile{Source: "claude-statusline", Updated: t0.Add(30 * time.Minute), Windows: map[string]NativeWindow{"7d": {UsedPct: 2, ResetsAt: r7}}}
	screen := &NativeFile{Source: "claude-usage-screen", Updated: t0, Windows: map[string]NativeWindow{"7d": {UsedPct: 97, ResetsAt: r7}, "fable_7d": {UsedPct: 0, ResetsAt: r7}}}
	m, from := mergeClaude(status, screen)
	if m.Windows["7d"].UsedPct != 2 || from["7d"] != status || m.Windows["fable_7d"].ResetsAt != r7 {
		t.Errorf("merged %+v", m.Windows)
	}
}

func TestSameValueRefreshesMeasuredAtAfterAMinute(t *testing.T) {
	dir := t.TempDir()
	in := statusJSON(10, t0.Add(2*time.Hour).Unix(), 30, t0.Add(72*time.Hour).Unix())
	_, _ = SaveClaudeStatus(dir, "", in, t0)
	if saved, _ := SaveClaudeStatus(dir, "", in, t0.Add(30*time.Second)); saved {
		t.Error("1분 안에는 다시 쓰지 않는다")
	}
	if saved, _ := SaveClaudeStatus(dir, "", in, t0.Add(2*time.Minute)); !saved {
		t.Error("1분 넘으면 시각만 새로")
	}
	if f := readNative(filepath.Join(NativeDir(dir), "claude.json")); !f.Updated.Equal(t0.Add(2 * time.Minute)) {
		t.Errorf("measured_at 갱신: %v", f.Updated)
	}
}

func TestRunTapOversizedInputPassesWhole(t *testing.T) {
	dir := t.TempDir()
	big := append(statusJSON(5, t0.Add(time.Hour).Unix(), 5, t0.Add(48*time.Hour).Unix()), bytes.Repeat([]byte(" "), TapMaxInput+500)...)
	var out, errb bytes.Buffer
	if code := RunTap(dir, "", bytes.NewReader(big), &out, &errb, []string{"sh", "-c", "wc -c | tr -d ' '"}, t0); code != 0 {
		t.Fatalf("code %d %s", code, errb.String())
	}
	if strings.TrimSpace(out.String()) != jsonInt(int64(len(big))) {
		t.Errorf("원래 명령에 전부: %q, 기대 %d", out.String(), len(big))
	}
	if readNative(filepath.Join(NativeDir(dir), "claude.json")) != nil {
		t.Error("상한 넘은 입력은 저장하지 않는다")
	}
}

func TestCodexSkipsRolloutWithoutRateLimits(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	r5, r7 := t0.Add(3*time.Hour).Unix(), t0.Add(100*time.Hour).Unix()
	writeCodexRollout(t, home, "rollout-ok.jsonl", []string{tokenLine("2026-10-18T01:00:00Z", 20, 30, r5, r7)}, t0.Add(-2*time.Hour))
	// 더 최근인데 rate_limits가 없는 실행(오류로 끝난 것) 둘
	writeCodexRollout(t, home, "rollout-err1.jsonl", []string{`{"type":"session_meta","payload":{}}`, `{"type":"event_msg","payload":{"type":"error","message":"x"}}`}, t0.Add(-time.Hour))
	writeCodexRollout(t, home, "rollout-err2.jsonl", []string{`{"type":"session_meta","payload":{}}`}, t0.Add(-30*time.Minute))
	f := codexLatest(dir, home)
	if f == nil || f.Windows["5h"].UsedPct != 20 {
		t.Fatalf("rate_limits가 나오는 첫 파일까지 내려간다: %+v", f)
	}
	// 최근 20개 안에 없으면 찾지 않는다(오래된 기록을 끝없이 뒤지지 않게)
	for i := 0; i < CodexScanFiles; i++ {
		writeCodexRollout(t, home, fmt.Sprintf("rollout-e%02d.jsonl", i), []string{`{"type":"session_meta","payload":{}}`}, t0.Add(-time.Duration(i)*time.Minute))
	}
	if f := codexLatest(dir, home); f != nil {
		t.Errorf("20개 밖이면 없음: %+v", f)
	}
}

func TestSetupClaudeKeepsSymlink(t *testing.T) {
	cfg, real := t.TempDir(), t.TempDir()
	target := filepath.Join(real, "settings.json")
	_ = os.WriteFile(target, []byte(`{"statusLine":{"type":"command","command":"echo hi"}}`), 0o600)
	link := filepath.Join(cfg, "settings.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	res, err := SetupClaude(cfg, "/usr/local/bin/agentlayer", false, t0)
	if err != nil || !res.Changed {
		t.Fatalf("%+v %v", res, err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("링크는 링크 그대로")
	}
	if b, _ := os.ReadFile(target); !strings.Contains(string(b), "usage tap claude") {
		t.Errorf("실제 파일을 고친다: %s", b)
	}
}

// 재설정 뒤 세션 창 한 바퀴가 지나도록 새 값이 없으면 그 창을 표시하지 않는다.
func TestReadNativeDropsDeadWindow(t *testing.T) {
	dir := t.TempDir()
	_, _ = SaveClaudeStatus(dir, "", statusJSON(16, t0.Add(-6*time.Hour).Unix(), 86, t0.Add(80*time.Hour).Unix()), t0.Add(-7*time.Hour))
	c := ReadNative(dir, t.TempDir(), t0).Providers["claude"]
	if _, ok := c.Windows["5h"]; ok {
		t.Errorf("한 바퀴 넘게 지난 5h는 빠진다: %+v", c.Windows)
	}
	if s, ok := c.Windows["7d"]; !ok || *s.LeftPct != 14 {
		t.Errorf("7d는 남는다: %+v", c.Windows)
	}
}

// 남은 양 = 100 − 공급자 화면의 사용% 정수 — Claude는 내림(「8.6% → 8% used」), 경계 8.4·8.5·8.6·0·100
func TestLeftPctMatchesShownUsed(t *testing.T) {
	for _, c := range []struct {
		src  string
		used float64
		want float64
	}{
		{"claude-usage-screen", 8.4, 92}, {"claude-statusline", 8.5, 92}, {"claude-statusline", 8.6, 92},
		{"claude-statusline", 9, 91}, {"claude-statusline", 8.9999999999, 91}, {"claude-statusline", 0, 100}, {"claude-statusline", 100, 0},
		{"codex-app-server", 8.4, 92}, {"codex-app-server", 8.6, 91}, {"codex-app-server", 0, 100}, {"codex-app-server", 100, 0},
	} {
		if got := leftPct(c.src, c.used); got != c.want {
			t.Errorf("%s %v%% 사용 → %v, 원함 %v", c.src, c.used, got, c.want)
		}
	}
	dir := t.TempDir()
	_, _ = SaveClaudeStatus(dir, "", statusJSON(8.6, t0.Add(2*time.Hour).Unix(), 9, t0.Add(80*time.Hour).Unix()), t0.Add(-time.Minute))
	c := ReadNative(dir, t.TempDir(), t0).Providers["claude"]
	if *c.Windows["5h"].LeftPct != 92 || *c.Windows["7d"].LeftPct != 91 {
		t.Errorf("상태줄 8.6·9 → 92·91: 5h %v · 7d %v", *c.Windows["5h"].LeftPct, *c.Windows["7d"].LeftPct)
	}
}
