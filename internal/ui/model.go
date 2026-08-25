// Package ui는 관제 TUI를 구현한다. mat과 같은 bubbletea 패턴:
// 2초 폴링, 파일 watch 없음(안정성), Update는 순수 함수에 가깝게.
package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
	"github.com/netwaif/agentlayer/internal/usage"
)

const (
	pollInterval  = 2 * time.Second
	usageInterval = 15 * time.Second // coach subprocess·rollout 파싱은 느긋하게
)

// refreshMsg는 저장소를 다시 읽은 결과.
type refreshMsg struct {
	agents []*state.Agent
	now    time.Time
}

type tickMsg time.Time

type usageTickMsg time.Time

// usageMsg는 coach·세션 컨텍스트 수집 결과.
type usageMsg struct {
	payload *usage.Payload
	ctx     map[string]usage.CtxInfo
}

// jumpDoneMsg는 점프 실행 후 종료 신호.
type jumpDoneMsg struct{ err error }

// Model은 TUI 상태.
type Model struct {
	store     *state.Store
	tm        tmuxx.Tmux
	agents    []*state.Agent
	cursor    int
	now       time.Time
	width     int
	height    int
	err       error
	showUsage bool // u 키: 사용량 전용 뷰
	usagePay  *usage.Payload
	ctx       map[string]usage.CtxInfo // CWD(절대경로) → 모델·ctx%
	// 주입점 (테스트용)
	coachRunner func() ([]byte, error)
	snapshotDir string
	codexRoot   string
}

func New(st *state.Store, tm tmuxx.Tmux) Model {
	return Model{store: st, tm: tm, now: time.Now(),
		coachRunner: usage.CoachRunner,
		snapshotDir: usage.SnapshotsDir(),
		codexRoot:   usage.CodexSessionsRoot()}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), tickCmd(), m.usageCmd(), usageTickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func usageTickCmd() tea.Cmd {
	return tea.Tick(usageInterval, func(t time.Time) tea.Msg { return usageTickMsg(t) })
}

// usageCmd는 coach 사용량과 폴더별 세션 컨텍스트를 백그라운드에서 모은다.
// coach는 콜드 실행이 분 단위라 5분 파일 캐시 + 실행 중복 방지로 감싼다.
// 어떤 소스가 없어도 관제는 계속된다.
func (m Model) usageCmd() tea.Cmd {
	runner, snapDir, codexRoot, st := m.coachRunner, m.snapshotDir, m.codexRoot, m.store
	return func() tea.Msg {
		pay := usage.FetchCached(st.Dir, 5*time.Minute, runner, time.Now())
		ctx := usage.LoadSnapshots(snapDir)
		if agents, err := st.List(); err == nil {
			for _, a := range agents {
				if a.Kind != "codex" || a.CWD == "" {
					continue
				}
				if _, ok := ctx[a.CWD]; !ok {
					if info := usage.CodexLatest(codexRoot, a.CWD); info.Model != "" || info.UsedPct != nil {
						ctx[a.CWD] = info
					}
				}
			}
		}
		return usageMsg{payload: pay, ctx: ctx}
	}
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

	case usageTickMsg:
		return m, tea.Batch(m.usageCmd(), usageTickCmd())

	case usageMsg:
		if msg.payload != nil {
			m.usagePay = msg.payload
		}
		if msg.ctx != nil {
			m.ctx = msg.ctx
		}
		return m, nil

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
			return m, tea.Batch(m.refreshCmd(), m.usageCmd())
		case "u":
			m.showUsage = !m.showUsage
			return m, nil
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
