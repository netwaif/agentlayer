// Package ui는 관제 TUI를 구현한다. mat과 같은 bubbletea 패턴:
// 2초 폴링, 파일 watch 없음(안정성), Update는 순수 함수에 가깝게.
package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
)

const pollInterval = 2 * time.Second

// refreshMsg는 저장소를 다시 읽은 결과.
type refreshMsg struct {
	agents []*state.Agent
	now    time.Time
}

type tickMsg time.Time

// jumpDoneMsg는 점프 실행 후 종료 신호.
type jumpDoneMsg struct{ err error }

// Model은 TUI 상태.
type Model struct {
	store  *state.Store
	tm     tmuxx.Tmux
	agents []*state.Agent
	cursor int
	now    time.Time
	width  int
	height int
	err    error
}

func New(st *state.Store, tm tmuxx.Tmux) Model {
	return Model{store: st, tm: tm, now: time.Now()}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// refreshCmd는 tmux 동기화 + 저장소 재조회를 백그라운드에서 수행한다.
func (m Model) refreshCmd() tea.Cmd {
	st, tm := m.store, m.tm
	return func() tea.Msg {
		now := time.Now()
		if panes, err := tm.ListPanes(); err == nil {
			_ = scan.Sync(st, panes, now)
		}
		agents, err := st.List()
		if err != nil {
			return refreshMsg{agents: nil, now: now}
		}
		return refreshMsg{agents: agents, now: now}
	}
}

// jumpCmd는 선택 에이전트의 pane으로 이동 후 종료를 지시한다.
func (m Model) jumpCmd(a *state.Agent) tea.Cmd {
	tm := m.tm
	ref := tmuxx.Ref{Session: a.Tmux.Session, Window: a.Tmux.Window, PaneID: a.Tmux.PaneID}
	return func() tea.Msg {
		return jumpDoneMsg{err: tm.JumpTo(ref)}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.refreshCmd(), tickCmd())

	case refreshMsg:
		m.agents = msg.agents
		m.now = msg.now
		if m.cursor >= len(m.agents) {
			m.cursor = max(0, len(m.agents)-1)
		}
		return m, nil

	case jumpDoneMsg:
		m.err = msg.err
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.cursor < len(m.agents)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "r":
			return m, m.refreshCmd()
		case "o": // 읽음 처리만 (점프 없이)
			if a := m.selected(); a != nil {
				_ = m.store.MarkRead(a.ID, time.Now())
				return m, m.refreshCmd()
			}
		case "enter": // 점프 + 읽음 처리
			if a := m.selected(); a != nil {
				_ = m.store.MarkRead(a.ID, time.Now())
				return m, m.jumpCmd(a)
			}
		}
	}
	return m, nil
}

func (m Model) selected() *state.Agent {
	if m.cursor >= 0 && m.cursor < len(m.agents) {
		return m.agents[m.cursor]
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
