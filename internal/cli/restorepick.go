// restorepick: `agentlayer restore`를 터미널에서 인자 없이 치면 뜨는 체크리스트.
// dry-run으로 ID를 읽고 다시 나열하는 대신 그 자리에서 골라 실행한다.
package cli

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/netwaif/agentlayer/internal/wiring"
)

var (
	pickTitle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaf5f")).Bold(true)
	pickCursor   = lipgloss.NewStyle().Background(lipgloss.Color("#3a3a3a")).Foreground(lipgloss.Color("#e4e4e4")).Bold(true)
	pickChecked  = lipgloss.NewStyle().Foreground(lipgloss.Color("#43B581"))
	pickDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("#6a6a6a"))
	pickHelpKey  = lipgloss.NewStyle().Foreground(lipgloss.Color("#d0d0d0")).Bold(true)
	pickHelpText = lipgloss.NewStyle().Foreground(lipgloss.Color("#8a8a8a"))
)

// 주입점 — 테스트에서 터미널 판정과 체크리스트 실행을 바꿔 끼운다.
var (
	restoreIsTerminal = func() bool { return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd()) }
	runRestorePicker  = func(items []RestoreItem) ([]RestoreItem, bool) {
		final, err := tea.NewProgram(newRestorePicker(items)).Run()
		if err != nil {
			return nil, false
		}
		m := final.(restorePicker)
		if m.cancelled {
			return nil, false
		}
		return m.Selected(), true
	}
)

// runRestorePickerWithBots — 봇 묶음이 있을 때의 체크리스트(기존 runRestorePicker는 그대로 — 봇이 없으면 예전 경로).
// remembered는 지난번 고른 봇(기본 체크). 돌려주는 값: 고른 복원 항목, 고른 봇, 확정 여부.
var runRestorePickerWithBots = func(items []RestoreItem, bots []wiring.OffBot, remembered map[string]bool) ([]RestoreItem, []wiring.OffBot, bool) {
	final, err := tea.NewProgram(newRestorePickerWithBots(items, bots, remembered)).Run()
	if err != nil {
		return nil, nil, false
	}
	m := final.(restorePicker)
	if m.cancelled {
		return nil, nil, false
	}
	return m.Selected(), m.SelectedBots(), true
}

type restorePicker struct {
	items     []RestoreItem
	checked   []bool
	bots      []wiring.OffBot // 꺼져 있는 봇 묶음 — items 아래에 이어진다(커서는 두 묶음을 한 줄로 오간다)
	botOn     []bool
	cursor    int
	done      bool
	cancelled bool
}

// newRestorePickerWithBots — 복원 항목은 전부 체크(예전 그대로), 봇은 지난번 선택만 체크(없으면 전부 해제).
func newRestorePickerWithBots(items []RestoreItem, bots []wiring.OffBot, remembered map[string]bool) restorePicker {
	m := newRestorePicker(items)
	m.bots = bots
	m.botOn = make([]bool, len(bots))
	for i, b := range bots {
		m.botOn[i] = remembered[b.Session]
	}
	return m
}

// total — 커서가 오가는 줄 수(복원 항목 + 봇).
func (m restorePicker) total() int { return len(m.items) + len(m.bots) }

// toggle — 커서 위치의 항목(복원 또는 봇)을 뒤집는다.
func (m *restorePicker) toggle(i int) {
	if i < len(m.items) {
		m.checked[i] = !m.checked[i]
	} else if j := i - len(m.items); j < len(m.bots) {
		m.botOn[j] = !m.botOn[j]
	}
}

// newRestorePicker는 전부 체크된 상태로 시작한다 — 기본이 "다 살리기"라
// 그냥 enter면 이전 동작과 같다.
func newRestorePicker(items []RestoreItem) restorePicker {
	checked := make([]bool, len(items))
	for i := range checked {
		checked[i] = true
	}
	return restorePicker{items: items, checked: checked}
}

func (m restorePicker) Init() tea.Cmd { return nil }

func (m restorePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "j", "down":
		if m.cursor < m.total()-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case " ", "x":
		if m.total() > 0 {
			m.toggle(m.cursor)
		}
	case "a":
		// 전체 토글은 커서가 있는 묶음 안에서 — 복원 항목과 봇은 성격이 달라 한꺼번에 켜지 않는다.
		if m.cursor < len(m.items) {
			all := true
			for _, c := range m.checked {
				all = all && c
			}
			for i := range m.checked {
				m.checked[i] = !all
			}
		} else {
			all := true
			for _, c := range m.botOn {
				all = all && c
			}
			for i := range m.botOn {
				m.botOn[i] = !all
			}
		}
	case "enter":
		m.done = true
		return m, tea.Quit
	case "q", "esc", "ctrl+c":
		m.done, m.cancelled = true, true
		return m, tea.Quit
	}
	return m, nil
}

// Selected는 체크된 항목을 원래 순서로 돌려준다. 취소면 nil.
func (m restorePicker) Selected() []RestoreItem {
	if m.cancelled {
		return nil
	}
	var out []RestoreItem
	for i, it := range m.items {
		if m.checked[i] {
			out = append(out, it)
		}
	}
	return out
}

// SelectedBots는 체크된 봇을 원래 순서로 돌려준다. 취소면 nil.
func (m restorePicker) SelectedBots() []wiring.OffBot {
	if m.cancelled {
		return nil
	}
	var out []wiring.OffBot
	for i, b := range m.bots {
		if m.botOn[i] {
			out = append(out, b)
		}
	}
	return out
}

func (m restorePicker) View() string {
	var b strings.Builder
	b.WriteString(pickTitle.Render("복원할 세션 고르기") + "\n\n")
	idW := 0
	for _, it := range m.items {
		if n := len(it.Agent.ID); n > idW {
			idW = n
		}
	}
	for i, it := range m.items {
		box := pickDim.Render("[ ]")
		if m.checked[i] {
			box = pickChecked.Render("[x]")
		}
		verb := "window 추가"
		if it.NewSession {
			verb = "세션 생성"
		}
		line := fmt.Sprintf("%s %-*s  %s %s  %s  ← %s", box, idW, it.Agent.ID, verb, it.Agent.Tmux.Session,
			pickDim.Render(ShortenHome(it.Agent.CWD)), it.Cmd)
		if i == m.cursor {
			line = pickCursor.Render("›") + " " + line
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	if len(m.bots) > 0 {
		// 꺼져 있는 봇 묶음 — 복원 항목 아래. 기본 체크는 지난번 선택.
		if len(m.items) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(pickTitle.Render("꺼져 있는 봇 (자동 기동 꺼짐 — 골라서 띄우기)") + "\n")
		sw := 0
		for _, bot := range m.bots {
			if n := len(bot.Session); n > sw {
				sw = n
			}
		}
		for i, bot := range m.bots {
			box := pickDim.Render("[ ]")
			if m.botOn[i] {
				box = pickChecked.Render("[x]")
			}
			engine := bot.Engine
			if engine == "" {
				engine = "?"
			}
			line := fmt.Sprintf("%s %-*s  %-7s  %s", box, sw, bot.Session, engine, pickDim.Render(bot.Method()))
			if len(m.items)+i == m.cursor {
				line = pickCursor.Render("›") + " " + line
			} else {
				line = "  " + line
			}
			b.WriteString(line + "\n")
		}
	}
	n := 0
	for _, c := range m.checked {
		if c {
			n++
		}
	}
	for _, c := range m.botOn {
		if c {
			n++
		}
	}
	b.WriteString("\n" + pickHelpText.Render(fmt.Sprintf("%d/%d 선택  ", n, m.total())))
	for _, h := range [][2]string{{"space", "토글"}, {"a", "전체"}, {"j/k", "이동"}, {"enter", "실행"}, {"q", "취소"}} {
		b.WriteString(pickHelpKey.Render(h[0]) + pickHelpText.Render(" "+h[1]+"  "))
	}
	b.WriteString("\n")
	return b.String()
}
