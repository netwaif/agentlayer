package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
	"github.com/netwaif/agentlayer/internal/wiring"
)

// 체크리스트의 "꺼져 있는 봇" 묶음 — 탐지는 wiring, 띄우기·기억·플래그는 여기. 자동 기동이 꺼진 봇이 없으면 예전과 같다.

func offBots() []wiring.OffBot {
	return []wiring.OffBot{
		{Session: "codex-qa", Engine: "codex", Unit: "com.codex-discord.qa-tui", Daemon: "com.codex-discord.qa"},
		{Session: "collab-bot", Engine: "claude", Sidecar: "/bin/zsh -lc 'cd /x/collab; exec bot-up.sh -n collab'"},
		{Session: "off-bot", Engine: "claude", Unit: "com.folder-bot.off"},
	}
}

// 기동 호출을 기록하는 환경. running은 이미 도는 유닛, exists는 떠 있는 세션.
type botEnvRec struct {
	calls   []string
	fail    map[string]error
	running map[string]bool
	exists  map[string]bool
}

func (r *botEnvRec) env(bots []wiring.OffBot) RestoreEnv {
	return RestoreEnv{
		SessionExists: func(s string) bool { return r.exists[s] },
		OffBots:       func() []wiring.OffBot { return bots },
		StartSession: func(session, cmd string) error {
			r.calls = append(r.calls, "tmux new-session -d -s "+session+" "+cmd)
			return r.fail[session]
		},
		StartUnit: func(label string, sessionUnit bool) error {
			kind := "daemon-unit "
			if sessionUnit {
				kind = "session-unit "
			}
			r.calls = append(r.calls, kind+label)
			return r.fail[label]
		},
		UnitRunning: func(label string) bool { return r.running[label] },
	}
}

// isoTmux — 테스트 전용 tmux 서버(-L 소켓). RunRestore가 세션을 만드는 경로에 들어가도 사용자의 tmux를 건드리지 않는다.
func isoTmux(t *testing.T) tmuxx.Tmux {
	t.Helper()
	sock := fmt.Sprintf("al-bots-%d", os.Getpid())
	t.Cleanup(func() { exec.Command("tmux", "-f", "/dev/null", "-L", sock, "kill-server").Run() })
	return tmuxx.Tmux{Args: []string{"-f", "/dev/null", "-L", sock}}
}

func stubRestoreEnv(t *testing.T, env RestoreEnv) {
	t.Helper()
	old := restoreEnvFn
	restoreEnvFn = func(tmuxx.Tmux) RestoreEnv { return env }
	t.Cleanup(func() { restoreEnvFn = old })
}

// 꺼져 있는 봇의 세션에 남은 죽은 레코드(메인·스레드 창)는 구동 유닛 관할처럼 일반 복원에서 뺀다. ID 명시면 강제.
func TestPlanRestoreSkipsOffBotSessions(t *testing.T) {
	dir := t.TempDir()
	main := deadAgent("claude-1", "claude", "collab-bot", 0, dir)
	thread := deadAgent("claude-2", "claude", "collab-bot", 3, dir)
	thread.Tmux.WindowName = "t170966"
	other := deadAgent("claude-3", "claude", "work", 0, dir)
	rec := &botEnvRec{}
	plan := PlanRestore([]*state.Agent{main, thread, other}, rec.env(offBots()), RestoreOpts{})
	if len(plan.Items) != 1 || plan.Items[0].Agent.ID != "claude-3" {
		t.Fatalf("봇 세션 레코드는 제외: %+v", plan.Items)
	}
	if len(plan.Skipped) != 2 || !strings.Contains(plan.Skipped[0], "봇 관할") || !strings.Contains(plan.Skipped[0], "사이드카") || !strings.Contains(plan.Skipped[1], "claude-2") {
		t.Errorf("건너뜀 사유: %v", plan.Skipped)
	}
	if len(plan.Bots) != 3 {
		t.Errorf("계획에 봇 묶음: %+v", plan.Bots)
	}
	plan = PlanRestore([]*state.Agent{main}, rec.env(offBots()), RestoreOpts{Explicit: true})
	if len(plan.Items) != 1 {
		t.Errorf("ID 명시면 강제: %+v", plan)
	}
	// OffBots가 없으면(예전 환경) 예전 그대로
	plan = PlanRestore([]*state.Agent{main}, envExists(func(string) bool { return false }), RestoreOpts{})
	if len(plan.Items) != 1 || len(plan.Bots) != 0 || len(plan.Skipped) != 0 {
		t.Errorf("봇 없는 환경은 불변: %+v", plan)
	}
}

// 띄우기: 사이드카는 tmux new-session, 유닛은 유닛 기동, 브리지는 데몬(안 돌 때만) → TUI 순. 떠 있는 세션은 건너뛰고 실패는 한 줄.
func TestLaunchOffBotsOrderAndFailures(t *testing.T) {
	rec := &botEnvRec{fail: map[string]error{"off-bot": nil, "com.folder-bot.off": errors.New("kickstart: no such service")}}
	var out bytes.Buffer
	launchOffBots(&out, rec.env(nil), offBots())
	want := []string{"daemon-unit com.codex-discord.qa", "session-unit com.codex-discord.qa-tui", "tmux new-session -d -s collab-bot /bin/zsh -lc 'cd /x/collab; exec bot-up.sh -n collab'", "session-unit com.folder-bot.off"}
	if strings.Join(rec.calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("기동 호출:\n%s", strings.Join(rec.calls, "\n"))
	}
	o := out.String()
	if !strings.Contains(o, "✔ 봇 codex-qa 기동") || !strings.Contains(o, "✔ 봇 collab-bot 기동 (사이드카)") || !strings.Contains(o, "✖ 봇 off-bot 기동 실패: 유닛 com.folder-bot.off: kickstart: no such service") || !strings.Contains(o, "봇 2개 기동") {
		t.Errorf("출력:\n%s", o)
	}
	// 데몬이 이미 돌면 생략, 세션이 있으면 건너뜀
	rec = &botEnvRec{running: map[string]bool{"com.codex-discord.qa": true}, exists: map[string]bool{"collab-bot": true}}
	out.Reset()
	launchOffBots(&out, rec.env(nil), offBots()[:2])
	if strings.Join(rec.calls, ",") != "session-unit com.codex-discord.qa-tui" || !strings.Contains(out.String(), "collab-bot: 세션이 이미 있어 건너뜀") {
		t.Errorf("데몬 생략·세션 건너뜀: %v %s", rec.calls, out.String())
	}
}

// 체크리스트: 복원 항목은 전부 체크(예전), 봇은 지난번 선택만 체크. 커서가 두 묶음을 오가고 a는 묶음 안에서만.
func TestRestorePickerWithBots(t *testing.T) {
	items := pickItems(t.TempDir())
	m := newRestorePickerWithBots(items, offBots(), map[string]bool{"collab-bot": true})
	if got := m.SelectedBots(); len(got) != 1 || got[0].Session != "collab-bot" {
		t.Fatalf("기본 체크는 지난번 선택: %+v", got)
	}
	v := m.View()
	for _, want := range []string{"꺼져 있는 봇", "[ ] codex-qa", "[x] collab-bot", "[ ] off-bot", "사이드카", "com.codex-discord.qa → com.codex-discord.qa-tui", "4/6 선택"} {
		if !strings.Contains(v, want) {
			t.Errorf("화면에 %q 기대:\n%s", want, v)
		}
	}
	// 커서: 항목 3개 지나 봇 첫 줄 → space → 봇 묶음에서 a(전부 끔/켬)
	m = drive(m, "j", "j", "j", "space", "enter")
	if got := m.SelectedBots(); len(got) != 2 || got[0].Session != "codex-qa" || len(m.Selected()) != 3 {
		t.Fatalf("봇 토글: %+v items=%d", got, len(m.Selected()))
	}
	m = newRestorePickerWithBots(items, offBots(), nil)
	m = drive(m, "j", "j", "j", "a")
	if len(m.SelectedBots()) != 3 || len(m.Selected()) != 3 {
		t.Errorf("봇 묶음 a는 봇만 전부 켬: bots=%d items=%d", len(m.SelectedBots()), len(m.Selected()))
	}
	m = drive(m, "k", "a")
	if len(m.SelectedBots()) != 3 || len(m.Selected()) != 0 {
		t.Errorf("항목 묶음 a는 항목만: bots=%d items=%d", len(m.SelectedBots()), len(m.Selected()))
	}
	// 봇 없으면 예전 화면 그대로
	if strings.Contains(newRestorePicker(items).View(), "꺼져 있는 봇") {
		t.Error("봇이 없으면 봇 묶음 표시 없음")
	}
}

// RunRestore(터미널): 봇 묶음이 있으면 봇까지 고르는 체크리스트로, 고른 봇을 띄우고 선택을 기억한다. 다음 실행의 기본 체크가 된다.
func TestRunRestorePicksAndRemembersBots(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	rec := &botEnvRec{}
	stubRestoreEnv(t, rec.env(offBots()))
	origTTY, origPick := restoreIsTerminal, runRestorePickerWithBots
	t.Cleanup(func() { restoreIsTerminal, runRestorePickerWithBots = origTTY, origPick })
	restoreIsTerminal = func() bool { return true }
	var seenDefault map[string]bool
	runRestorePickerWithBots = func(items []RestoreItem, bots []wiring.OffBot, remembered map[string]bool) ([]RestoreItem, []wiring.OffBot, bool) {
		seenDefault = remembered
		return nil, []wiring.OffBot{bots[1]}, true // collab-bot만
	}
	var out bytes.Buffer
	if err := RunRestore(&out, st, isoTmux(t), nil); err != nil {
		t.Fatal(err)
	}
	if len(seenDefault) != 0 || len(rec.calls) != 1 || !strings.HasPrefix(rec.calls[0], "tmux new-session -d -s collab-bot") {
		t.Errorf("첫 실행: 기본 전부 해제, 고른 봇만 기동: default=%v calls=%v", seenDefault, rec.calls)
	}
	if b, _ := os.ReadFile(filepath.Join(st.Dir, restoreBotsFile)); !strings.Contains(string(b), `"collab-bot"`) {
		t.Errorf("선택 기억: %s", b)
	}
	if err := RunRestore(&out, st, isoTmux(t), nil); err != nil {
		t.Fatal(err)
	}
	if !seenDefault["collab-bot"] || seenDefault["codex-qa"] {
		t.Errorf("둘째 실행의 기본 체크는 지난번 선택: %v", seenDefault)
	}
	// 취소면 아무것도 안 띄우고 기억도 안 바꾼다
	runRestorePickerWithBots = func([]RestoreItem, []wiring.OffBot, map[string]bool) ([]RestoreItem, []wiring.OffBot, bool) {
		return nil, nil, false
	}
	rec.calls = nil
	out.Reset()
	if err := RunRestore(&out, st, isoTmux(t), nil); err != nil || len(rec.calls) != 0 || !strings.Contains(out.String(), "취소") {
		t.Errorf("취소: %v %v %s", err, rec.calls, out.String())
	}
}

// --bots: 그 봇만 띄움(체크리스트 없음). 목록에 없는 이름은 오류이고 아무것도 띄우지 않는다. --dry-run은 목록만.
func TestRunRestoreBotsFlag(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	rec := &botEnvRec{}
	stubRestoreEnv(t, rec.env(offBots()))
	var out bytes.Buffer
	if err := RunRestore(&out, st, isoTmux(t), []string{"--bots", "codex-qa,off-bot"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(rec.calls, ",") != "daemon-unit com.codex-discord.qa,session-unit com.codex-discord.qa-tui,session-unit com.folder-bot.off" {
		t.Errorf("calls=%v", rec.calls)
	}
	if b, _ := os.ReadFile(filepath.Join(st.Dir, restoreBotsFile)); !strings.Contains(string(b), `"codex-qa"`) || strings.Contains(string(b), "collab") {
		t.Errorf("--bots 선택도 기억: %s", b)
	}
	rec.calls = nil
	err := RunRestore(&out, st, isoTmux(t), []string{"--bots", "codex-qa,nope"})
	if err == nil || !strings.Contains(err.Error(), "nope") || !strings.Contains(err.Error(), "collab-bot") || len(rec.calls) != 0 {
		t.Errorf("없는 이름은 오류·미기동: %v %v", err, rec.calls)
	}
	out.Reset()
	if err := RunRestore(&out, st, isoTmux(t), []string{"--bots", "collab-bot", "--dry-run"}); err != nil || len(rec.calls) != 0 {
		t.Fatalf("dry-run: %v %v", err, rec.calls)
	}
	if !strings.Contains(out.String(), "[봇] collab-bot") || !strings.Contains(out.String(), "사이드카") {
		t.Errorf("dry-run 출력: %s", out.String())
	}
}

// --yes는 봇을 띄우지 않고(예전 결과) 안내만, --no-bots는 봇 묶음을 숨긴다, --dry-run은 봇 목록과 방법을 보여 준다.
func TestRunRestoreYesNoBotsDryRun(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	dir := t.TempDir()
	_ = st.Save(deadAgent("claude-3", "claude", "work", 0, dir))
	rec := &botEnvRec{}
	stubRestoreEnv(t, rec.env(offBots()))
	var out bytes.Buffer
	if err := RunRestore(&out, st, isoTmux(t), []string{"--dry-run"}); err != nil {
		t.Fatal(err)
	}
	o := out.String()
	for _, want := range []string{"[claude-3]", "(dry-run) 꺼져 있는 봇", "[봇] codex-qa", "codex", "유닛(com.codex-discord.qa → com.codex-discord.qa-tui)", "[봇] collab-bot", "사이드카"} {
		if !strings.Contains(o, want) {
			t.Errorf("dry-run에 %q 기대:\n%s", want, o)
		}
	}
	out.Reset()
	if err := RunRestore(&out, st, isoTmux(t), []string{"--dry-run", "--no-bots"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "꺼져 있는 봇") || !strings.Contains(out.String(), "[claude-3]") {
		t.Errorf("--no-bots: %s", out.String())
	}
	// --yes: 죽은 세션이 없으면(예전 메시지) 봇은 안내만, 띄우지 않는다
	st2, _ := state.NewStore(t.TempDir())
	out.Reset()
	if err := RunRestore(&out, st2, isoTmux(t), []string{"--yes"}); err != nil || len(rec.calls) != 0 {
		t.Fatalf("--yes: %v %v", err, rec.calls)
	}
	if !strings.Contains(out.String(), "복원할 죽은 세션이 없습니다.") || !strings.Contains(out.String(), "--yes로 띄우지 않습니다") {
		t.Errorf("--yes 출력: %s", out.String())
	}
	// 봇이 하나도 없으면 예전 출력 그대로
	stubRestoreEnv(t, (&botEnvRec{}).env(nil))
	out.Reset()
	if err := RunRestore(&out, st2, isoTmux(t), []string{"--yes"}); err != nil || out.String() != "복원할 죽은 세션이 없습니다.\n" {
		t.Errorf("봇 없음 불변: %q", out.String())
	}
}

// 유닛 기동은 막힐 수 있다(리눅스 oneshot TUI 유닛은 tui-up.sh 완주까지) — 유닛형은 시작 전에 한 줄을 먼저 찍는다.
func TestLaunchOffBotsAnnouncesUnitStart(t *testing.T) {
	rec := &botEnvRec{}
	var out bytes.Buffer
	launchOffBots(&out, rec.env(nil), offBots())
	o := out.String()
	a, b := strings.Index(o, "봇 codex-qa 기동 중"), strings.Index(o, "✔ 봇 codex-qa 기동")
	if a < 0 || b < 0 || a > b {
		t.Errorf("유닛형은 기동 중 줄이 결과 줄보다 먼저:\n%s", o)
	}
	if strings.Contains(o, "봇 collab-bot 기동 중") {
		t.Errorf("사이드카형(즉시 반환)은 기동 중 줄 없음:\n%s", o)
	}
}

// 유닛 기동 명령. 리눅스의 세션 유닛은 oneshot+RemainAfterExit라 세션만 죽고 유닛이 active(exited)면 start가 무동작이다 —
// 세션이 없을 때만 부르므로 restart(folder-bot botctl cmd_start와 같은 판단). 데몬 유닛은 start.
func TestUnitStartArgv(t *testing.T) {
	for _, c := range []struct {
		goos    string
		session bool
		want    string
	}{
		{"darwin", true, "launchctl kickstart gui/501/com.x"},
		{"darwin", false, "launchctl kickstart gui/501/com.x"},
		{"linux", true, "systemctl --user restart com.x"},
		{"linux", false, "systemctl --user start com.x"},
	} {
		if got := strings.Join(unitStartArgv(c.goos, 501, "com.x", c.session), " "); got != c.want {
			t.Errorf("%s session=%v: %q want %q", c.goos, c.session, got, c.want)
		}
	}
}

// 자동 기동을 끈 유닛형 봇의 죽은 레코드: 사유는 "봇 관할"이어야 한다 — "부팅 시 자동 기동"은 이 유닛에 거짓이다.
func TestPlanRestoreOffUnitBotReason(t *testing.T) {
	dir := t.TempDir()
	rec := &botEnvRec{}
	env := rec.env(offBots())
	env.LaunchAgents = func(session string) []string {
		if session == "off-bot" {
			return []string{"com.folder-bot.off"}
		}
		return nil
	}
	plan := PlanRestore([]*state.Agent{deadAgent("claude-1", "claude", "off-bot", 0, dir)}, env, RestoreOpts{})
	if len(plan.Items) != 0 || len(plan.Skipped) != 1 || !strings.Contains(plan.Skipped[0], "봇 관할(유닛(com.folder-bot.off))") || strings.Contains(plan.Skipped[0], "부팅 시 자동 기동") {
		t.Errorf("유닛형 꺼진 봇의 사유: %+v", plan)
	}
}

// ID 지정 복원은 봇 묶음을 쓰지 않는다 — 유닛 파일을 훑는 탐지도 부르지 않는다.
func TestPlanRestoreExplicitSkipsOffBotLookup(t *testing.T) {
	calls := 0
	env := RestoreEnv{OffBots: func() []wiring.OffBot { calls++; return offBots() }}
	plan := PlanRestore([]*state.Agent{deadAgent("claude-1", "claude", "collab-bot", 0, t.TempDir())}, env, RestoreOpts{Explicit: true})
	if calls != 0 || len(plan.Bots) != 0 || len(plan.Items) != 1 {
		t.Errorf("calls=%d plan=%+v", calls, plan)
	}
}

// --bots에 빈 값이 오면(스크립트의 빈 변수) 오류로 끝난다 — 전체 복원으로 빠지지 않는다.
func TestRunRestoreBotsFlagEmptyValue(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	_ = st.Save(deadAgent("claude-3", "claude", "work", 0, t.TempDir()))
	rec := &botEnvRec{}
	stubRestoreEnv(t, rec.env(offBots()))
	for _, args := range [][]string{{"--bots", ""}, {"--bots="}, {"--bots", "", "--dry-run"}, {"--bots", " , "}} {
		var out bytes.Buffer
		err := RunRestore(&out, st, isoTmux(t), args)
		if err == nil || !strings.Contains(err.Error(), "--bots에 세션 이름이 없습니다") || strings.Contains(out.String(), "claude-3") || len(rec.calls) != 0 {
			t.Errorf("%q: err=%v calls=%v out=%q", args, err, rec.calls, out.String())
		}
	}
	if _, err := st.Load("claude-3"); err != nil {
		t.Error("죽은 레코드는 그대로 남아야 한다")
	}
}

// --bots는 반복 실행해도 된다: 이미 떠 있는 봇 이름은 건너뛰고 나머지를 띄운다. 오타(떠 있지도 않고 목록에도 없음)만 오류. 같은 이름은 한 번만.
func TestRunRestoreBotsFlagIdempotent(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	rec := &botEnvRec{exists: map[string]bool{"codex-qa": true}}
	stubRestoreEnv(t, rec.env(offBots()[1:])) // codex-qa는 떠 있어 꺼진 봇 목록에 없다
	var out bytes.Buffer
	if err := RunRestore(&out, st, isoTmux(t), []string{"--bots", "codex-qa,collab-bot,collab-bot"}); err != nil {
		t.Fatalf("이미 떠 있는 봇이 섞여도 오류가 아니다: %v", err)
	}
	if len(rec.calls) != 1 || !strings.HasPrefix(rec.calls[0], "tmux new-session -d -s collab-bot") {
		t.Errorf("collab-bot만 한 번: %v", rec.calls)
	}
	if !strings.Contains(out.String(), "봇 codex-qa: 세션이 이미 있어 건너뜀") {
		t.Errorf("출력: %s", out.String())
	}
	rec.calls = nil
	if err := RunRestore(&out, st, isoTmux(t), []string{"--bots", "codex-qa,nope"}); err == nil || !strings.Contains(err.Error(), "nope") || strings.Contains(err.Error(), "이름: codex-qa") || len(rec.calls) != 0 {
		t.Errorf("오타는 오류: %v %v", err, rec.calls)
	}
}

// --bots와 --no-bots는 서로 반대 지시다.
func TestRunRestoreBotsWithNoBotsIsError(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	rec := &botEnvRec{}
	stubRestoreEnv(t, rec.env(offBots()))
	var out bytes.Buffer
	if err := RunRestore(&out, st, isoTmux(t), []string{"--bots", "collab-bot", "--no-bots"}); err == nil || !strings.Contains(err.Error(), "--no-bots") || len(rec.calls) != 0 {
		t.Errorf("err=%v calls=%v", err, rec.calls)
	}
}

// --no-bots는 봇 묶음만 숨긴다. 꺼져 있는 봇 세션의 죽은 레코드를 일반 복원에서 빼는 보호는 그대로다(2026-09-03 사고).
func TestRunRestoreNoBotsKeepsBotGuard(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	dir := t.TempDir()
	_ = st.Save(deadAgent("claude-4", "claude", "collab-bot", 0, dir))
	_ = st.Save(deadAgent("claude-3", "claude", "work", 0, dir))
	rec := &botEnvRec{}
	stubRestoreEnv(t, rec.env(offBots()))
	var out bytes.Buffer
	if err := RunRestore(&out, st, isoTmux(t), []string{"--dry-run", "--no-bots"}); err != nil {
		t.Fatal(err)
	}
	o := out.String()
	if strings.Contains(o, "[claude-4]") || !strings.Contains(o, "claude-4: 봇 관할") || !strings.Contains(o, "[claude-3]") || strings.Contains(o, "꺼져 있는 봇") {
		t.Errorf("--no-bots 출력:\n%s", o)
	}
}

// 기억은 이번에 화면에 나온 봇만 고친다 — 이미 떠 있어 목록에 없던 봇의 체크는 남는다. 화면에 나왔는데 푼 봇은 지운다.
func TestRunRestoreRemembersOnlyShownBots(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	rec := &botEnvRec{}
	stubRestoreEnv(t, rec.env(offBots()))
	origTTY, origPick := restoreIsTerminal, runRestorePickerWithBots
	t.Cleanup(func() { restoreIsTerminal, runRestorePickerWithBots = origTTY, origPick })
	restoreIsTerminal = func() bool { return true }
	pick := func(names ...string) {
		runRestorePickerWithBots = func(items []RestoreItem, bots []wiring.OffBot, _ map[string]bool) ([]RestoreItem, []wiring.OffBot, bool) {
			var out []wiring.OffBot
			for _, b := range bots {
				for _, n := range names {
					if b.Session == n {
						out = append(out, b)
					}
				}
			}
			return nil, out, true
		}
	}
	var out bytes.Buffer
	pick("codex-qa", "collab-bot") // 1일차
	if err := RunRestore(&out, st, isoTmux(t), nil); err != nil {
		t.Fatal(err)
	}
	// 2일차: codex-qa는 아직 떠 있어 목록에 없다. collab-bot만 고른다.
	stubRestoreEnv(t, rec.env(offBots()[1:]))
	pick("collab-bot")
	if err := RunRestore(&out, st, isoTmux(t), nil); err != nil {
		t.Fatal(err)
	}
	mem := loadRestoreBots(st.Dir)
	if !mem["codex-qa"] || !mem["collab-bot"] || mem["off-bot"] {
		t.Errorf("안 보인 codex-qa의 기억이 남아야 한다: %v", mem)
	}
	// --bots off-bot: 그 봇을 더할 뿐 체크리스트의 기억을 지우지 않는다
	if err := RunRestore(&out, st, isoTmux(t), []string{"--bots", "off-bot"}); err != nil {
		t.Fatal(err)
	}
	if mem = loadRestoreBots(st.Dir); !mem["codex-qa"] || !mem["collab-bot"] || !mem["off-bot"] {
		t.Errorf("--bots는 기억에 더한다: %v", mem)
	}
	// 화면에 나온 봇을 전부 풀고 enter — 푼 것도 선택이다(취소가 아니다)
	pick()
	out.Reset()
	if err := RunRestore(&out, st, isoTmux(t), nil); err != nil {
		t.Fatal(err)
	}
	if mem = loadRestoreBots(st.Dir); !mem["codex-qa"] || mem["collab-bot"] || mem["off-bot"] {
		t.Errorf("화면에 나온 봇을 풀면 기억에서 지운다: %v", mem)
	}
}

// 테스트 기본 환경은 실제 홈(~/Library/LaunchAgents·~/.config/folder-bot)을 읽지 않는다 — 홈에 꺼진 봇이 있어도
// restore 테스트가 흔들리지 않고, 실수로 실제 유닛을 띄우지도 않는다.
func TestRestoreEnvDefaultInTestsIsHermetic(t *testing.T) {
	home := t.TempDir()
	side := filepath.Join(home, ".config", "folder-bot")
	_ = os.MkdirAll(side, 0o755)
	_ = os.WriteFile(filepath.Join(side, "home-off-bot.tmux-cmd"), []byte("echo hi\n"), 0o644)
	t.Setenv("HOME", home)
	env := restoreEnvFn(tmuxx.Tmux{Args: []string{"-f", "/dev/null", "-L", "al-hermetic-none"}})
	if env.OffBots != nil {
		if got := env.OffBots(); len(got) != 0 {
			t.Errorf("테스트 기본 환경이 홈의 봇을 읽었다: %+v", got)
		}
	}
	if env.StartUnit == nil || env.StartUnit("com.x", true) == nil {
		t.Error("테스트 기본 환경의 유닛 기동은 실제 명령 대신 오류를 내야 한다")
	}
}

// 실제 환경(restoreEnvAt)은 주어진 경로에서 꺼져 있는 봇을 읽는다.
func TestRestoreEnvAtReadsOffBots(t *testing.T) {
	side := t.TempDir()
	_ = os.WriteFile(filepath.Join(side, "side-bot.tmux-cmd"), []byte("echo hi\n"), 0o644)
	env := restoreEnvAt(tmuxx.Tmux{Args: []string{"-f", "/dev/null", "-L", "al-envat-none"}}, wiring.Paths{SidecarDir: side})
	got := env.OffBots()
	if len(got) != 1 || got[0].Session != "side-bot" || env.StartUnit == nil || env.StartSession == nil || env.UnitRunning == nil {
		t.Errorf("OffBots=%+v", got)
	}
}
