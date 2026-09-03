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

type restorePicker struct {
	items     []RestoreItem
	checked   []bool
	cursor    int
	done      bool
	cancelled bool
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
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case " ", "x":
		if len(m.items) > 0 {
			m.checked[m.cursor] = !m.checked[m.cursor]
		}
	case "a":
		all := true
		for _, c := range m.checked {
			all = all && c
		}
		for i := range m.checked {
			m.checked[i] = !all
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
	n := 0
	for _, c := range m.checked {
		if c {
			n++
		}
	}
	b.WriteString("\n" + pickHelpText.Render(fmt.Sprintf("%d/%d 선택  ", n, len(m.items))))
	for _, h := range [][2]string{{"space", "토글"}, {"a", "전체"}, {"j/k", "이동"}, {"enter", "실행"}, {"q", "취소"}} {
		b.WriteString(pickHelpKey.Render(h[0]) + pickHelpText.Render(" "+h[1]+"  "))
	}
	b.WriteString("\n")
	return b.String()
}
