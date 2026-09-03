package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
)

func pickItems(dir string) []RestoreItem {
	return []RestoreItem{
		{Agent: deadAgent("claude-6", "claude", "agentlayer-dev", 0, dir), NewSession: true, Cmd: "claude"},
		{Agent: deadAgent("claude-7", "claude", "demo-b", 0, dir), NewSession: true, Cmd: "claude"},
		{Agent: deadAgent("codex-9", "codex", "demo-b", 2, dir), NewSession: false, Cmd: "codex"},
	}
}

func key(s string) tea.Msg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func drive(m restorePicker, keys ...string) restorePicker {
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(restorePicker)
	}
	return m
}

// 기본은 전부 체크. space로 하나 빼고 enter → 나머지만 선택된다.
func TestRestorePickerToggleAndConfirm(t *testing.T) {
	m := drive(newRestorePicker(pickItems(t.TempDir())), "space", "enter")
	if !m.done || m.cancelled {
		t.Fatalf("enter는 확정: done=%v cancelled=%v", m.done, m.cancelled)
	}
	got := m.Selected()
	if len(got) != 2 || got[0].Agent.ID != "claude-7" || got[1].Agent.ID != "codex-9" {
		t.Fatalf("첫 항목 뺀 둘 기대: %+v", ids(got))
	}
}

// j/k(↓↑)로 커서를 옮겨 토글한다.
func TestRestorePickerCursorMoves(t *testing.T) {
	m := drive(newRestorePicker(pickItems(t.TempDir())), "j", "down", "space", "k", "space", "enter")
	got := ids(m.Selected())
	if len(got) != 1 || got[0] != "claude-6" {
		t.Fatalf("둘째·셋째 뺀 첫째만 기대: %v", got)
	}
}

// a는 전체 토글: 다 켜져 있으면 다 끄고, 하나라도 꺼져 있으면 다 켠다.
func TestRestorePickerToggleAll(t *testing.T) {
	m := drive(newRestorePicker(pickItems(t.TempDir())), "a")
	if n := len(m.Selected()); n != 0 {
		t.Fatalf("a로 전부 해제 기대, %d개 남음", n)
	}
	m = drive(m, "j", "space", "a")
	if n := len(m.Selected()); n != 3 {
		t.Fatalf("하나 켜진 상태에서 a면 전부 켬 기대, %d개", n)
	}
}

// q·esc는 취소 — 아무것도 복원하지 않는다.
func TestRestorePickerCancel(t *testing.T) {
	for _, k := range []string{"q", "esc"} {
		m := drive(newRestorePicker(pickItems(t.TempDir())), k)
		if !m.done || !m.cancelled || m.Selected() != nil {
			t.Fatalf("%s는 취소: done=%v cancelled=%v sel=%v", k, m.done, m.cancelled, ids(m.Selected()))
		}
	}
}

// 화면: 체크 표시·ID·세션·명령이 보이고 키 안내가 붙는다.
func TestRestorePickerView(t *testing.T) {
	m := drive(newRestorePicker(pickItems(t.TempDir())), "j", "space")
	v := m.View()
	for _, want := range []string{"[x] claude-6", "[ ] claude-7", "demo-b", "← codex", "space", "enter"} {
		if !strings.Contains(v, want) {
			t.Errorf("화면에 %q 기대:\n%s", want, v)
		}
	}
}

// RunRestore는 터미널이면 체크리스트를 띄우고 고른 것만 복원한다.
// 체크리스트 실행은 주입점(runRestorePicker)으로 대체한다.
func TestRunRestoreUsesPickerSelection(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux 없음")
	}
	sock := fmt.Sprintf("al-pick-%d", os.Getpid())
	tm := tmuxx.Tmux{Args: []string{"-f", "/dev/null", "-L", sock}}
	t.Cleanup(func() { exec.Command("tmux", "-f", "/dev/null", "-L", sock, "kill-server").Run() })
	st, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, a := range []*state.Agent{
		deadAgent("claude-1", "claude", "one", 0, dir),
		deadAgent("claude-2", "claude", "two", 0, dir),
	} {
		if err := st.Save(a); err != nil {
			t.Fatal(err)
		}
	}
	origTTY, origPick := restoreIsTerminal, runRestorePicker
	t.Cleanup(func() { restoreIsTerminal, runRestorePicker = origTTY, origPick })
	restoreIsTerminal = func() bool { return true }
	runRestorePicker = func(items []RestoreItem) ([]RestoreItem, bool) {
		for _, it := range items {
			if it.Agent.ID == "claude-2" {
				return []RestoreItem{it}, true
			}
		}
		return nil, true
	}
	var buf bytes.Buffer
	if err := RunRestore(&buf, st, tm, nil); err != nil {
		t.Fatal(err)
	}
	if tm.HasSession("one") || !tm.HasSession("two") {
		t.Fatalf("고른 two만 복원돼야 함:\n%s", buf.String())
	}
	if _, err := st.Load("claude-1"); err != nil {
		t.Error("안 고른 레코드는 남아야 함")
	}

	// 취소하면 아무것도 만들지 않는다
	runRestorePicker = func([]RestoreItem) ([]RestoreItem, bool) { return nil, false }
	buf.Reset()
	if err := RunRestore(&buf, st, tm, nil); err != nil {
		t.Fatal(err)
	}
	if tm.HasSession("one") || !strings.Contains(buf.String(), "취소") {
		t.Fatalf("취소 시 복원 없음·안내 출력:\n%s", buf.String())
	}
}

// --yes는 터미널이어도 체크리스트 없이 전부 복원한다 (스크립트용).
func TestRunRestoreYesSkipsPicker(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux 없음")
	}
	sock := fmt.Sprintf("al-yes-%d", os.Getpid())
	tm := tmuxx.Tmux{Args: []string{"-f", "/dev/null", "-L", sock}}
	t.Cleanup(func() { exec.Command("tmux", "-f", "/dev/null", "-L", sock, "kill-server").Run() })
	st, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(deadAgent("claude-1", "claude", "one", 0, t.TempDir())); err != nil {
		t.Fatal(err)
	}
	origTTY, origPick := restoreIsTerminal, runRestorePicker
	t.Cleanup(func() { restoreIsTerminal, runRestorePicker = origTTY, origPick })
	restoreIsTerminal = func() bool { return true }
	runRestorePicker = func([]RestoreItem) ([]RestoreItem, bool) {
		t.Fatal("--yes면 체크리스트 금지")
		return nil, false
	}
	var buf bytes.Buffer
	if err := RunRestore(&buf, st, tm, []string{"--yes"}); err != nil {
		t.Fatal(err)
	}
	if !tm.HasSession("one") {
		t.Fatal("--yes는 전부 복원")
	}
}

func ids(items []RestoreItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Agent.ID)
	}
	return out
}
