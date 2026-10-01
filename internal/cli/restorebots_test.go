package cli

import (
	"bytes"
	"errors"
	"os"
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
		StartUnit: func(label string) error {
			r.calls = append(r.calls, "unit "+label)
			return r.fail[label]
		},
		UnitRunning: func(label string) bool { return r.running[label] },
	}
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
	want := []string{"unit com.codex-discord.qa", "unit com.codex-discord.qa-tui", "tmux new-session -d -s collab-bot /bin/zsh -lc 'cd /x/collab; exec bot-up.sh -n collab'", "unit com.folder-bot.off"}
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
	if strings.Join(rec.calls, ",") != "unit com.codex-discord.qa-tui" || !strings.Contains(out.String(), "collab-bot: 세션이 이미 있어 건너뜀") {
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
	if err := RunRestore(&out, st, tmuxx.Tmux{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(seenDefault) != 0 || len(rec.calls) != 1 || !strings.HasPrefix(rec.calls[0], "tmux new-session -d -s collab-bot") {
		t.Errorf("첫 실행: 기본 전부 해제, 고른 봇만 기동: default=%v calls=%v", seenDefault, rec.calls)
	}
	if b, _ := os.ReadFile(filepath.Join(st.Dir, restoreBotsFile)); !strings.Contains(string(b), `"collab-bot"`) {
		t.Errorf("선택 기억: %s", b)
	}
	if err := RunRestore(&out, st, tmuxx.Tmux{}, nil); err != nil {
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
	if err := RunRestore(&out, st, tmuxx.Tmux{}, nil); err != nil || len(rec.calls) != 0 || !strings.Contains(out.String(), "취소") {
		t.Errorf("취소: %v %v %s", err, rec.calls, out.String())
	}
}

// --bots: 그 봇만 띄움(체크리스트 없음). 목록에 없는 이름은 오류이고 아무것도 띄우지 않는다. --dry-run은 목록만.
func TestRunRestoreBotsFlag(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	rec := &botEnvRec{}
	stubRestoreEnv(t, rec.env(offBots()))
	var out bytes.Buffer
	if err := RunRestore(&out, st, tmuxx.Tmux{}, []string{"--bots", "codex-qa,off-bot"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(rec.calls, ",") != "unit com.codex-discord.qa,unit com.codex-discord.qa-tui,unit com.folder-bot.off" {
		t.Errorf("calls=%v", rec.calls)
	}
	if b, _ := os.ReadFile(filepath.Join(st.Dir, restoreBotsFile)); !strings.Contains(string(b), `"codex-qa"`) || strings.Contains(string(b), "collab") {
		t.Errorf("--bots 선택도 기억: %s", b)
	}
	rec.calls = nil
	err := RunRestore(&out, st, tmuxx.Tmux{}, []string{"--bots", "codex-qa,nope"})
	if err == nil || !strings.Contains(err.Error(), "nope") || !strings.Contains(err.Error(), "collab-bot") || len(rec.calls) != 0 {
		t.Errorf("없는 이름은 오류·미기동: %v %v", err, rec.calls)
	}
	out.Reset()
	if err := RunRestore(&out, st, tmuxx.Tmux{}, []string{"--bots", "collab-bot", "--dry-run"}); err != nil || len(rec.calls) != 0 {
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
	if err := RunRestore(&out, st, tmuxx.Tmux{}, []string{"--dry-run"}); err != nil {
		t.Fatal(err)
	}
	o := out.String()
	for _, want := range []string{"[claude-3]", "(dry-run) 꺼져 있는 봇", "[봇] codex-qa", "codex", "유닛(com.codex-discord.qa → com.codex-discord.qa-tui)", "[봇] collab-bot", "사이드카"} {
		if !strings.Contains(o, want) {
			t.Errorf("dry-run에 %q 기대:\n%s", want, o)
		}
	}
	out.Reset()
	if err := RunRestore(&out, st, tmuxx.Tmux{}, []string{"--dry-run", "--no-bots"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "꺼져 있는 봇") || !strings.Contains(out.String(), "[claude-3]") {
		t.Errorf("--no-bots: %s", out.String())
	}
	// --yes: 죽은 세션이 없으면(예전 메시지) 봇은 안내만, 띄우지 않는다
	st2, _ := state.NewStore(t.TempDir())
	out.Reset()
	if err := RunRestore(&out, st2, tmuxx.Tmux{}, []string{"--yes"}); err != nil || len(rec.calls) != 0 {
		t.Fatalf("--yes: %v %v", err, rec.calls)
	}
	if !strings.Contains(out.String(), "복원할 죽은 세션이 없습니다.") || !strings.Contains(out.String(), "--yes로 띄우지 않습니다") {
		t.Errorf("--yes 출력: %s", out.String())
	}
	// 봇이 하나도 없으면 예전 출력 그대로
	stubRestoreEnv(t, (&botEnvRec{}).env(nil))
	out.Reset()
	if err := RunRestore(&out, st2, tmuxx.Tmux{}, []string{"--yes"}); err != nil || out.String() != "복원할 죽은 세션이 없습니다.\n" {
		t.Errorf("봇 없음 불변: %q", out.String())
	}
}
