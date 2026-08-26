// Package ui는 관제 TUI를 구현한다. mat과 같은 bubbletea 패턴:
// 2초 폴링, 파일 watch 없음(안정성), Update는 순수 함수에 가깝게.
package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/netwaif/agentlayer/internal/cli"
	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/starter"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
	"github.com/netwaif/agentlayer/internal/usage"
	"github.com/netwaif/agentlayer/internal/wiring"
	"github.com/netwaif/agentlayer/internal/wt"
)

const (
	pollInterval  = 2 * time.Second
	usageInterval = 15 * time.Second // coach subprocess·rollout 파싱은 느긋하게
)

// refreshMsg는 저장소를 다시 읽은 결과.
type refreshMsg struct {
	agents     []*state.Agent
	wtBranches map[string]string // worktree 경로 → 브랜치
	now        time.Time
}

type tickMsg time.Time

type usageTickMsg time.Time

// usageMsg는 coach·세션 컨텍스트 수집 결과.
type usageMsg struct {
	payload     *usage.Payload
	ctx         map[string]usage.CtxInfo
	starter     []starter.Task
	discord   map[string]bool   // CWD → Discord 연결 여부 (⌁ 마크)
	defModels map[string]string // CLI별 기본 모델 설정 (빈 값 = 미설정/자동)
}

// jumpDoneMsg는 점프 실행 후 종료 신호.
type jumpDoneMsg struct{ err error }

// gitDoneMsg는 lazygit에서 돌아온 뒤 새로고침 신호.
type gitDoneMsg struct{ err error }

// previewMsg는 선택 pane 화면 미리보기.
type previewMsg struct {
	paneID  string
	content string
}

// Model은 TUI 상태.
type Model struct {
	store        *state.Store
	tm           tmuxx.Tmux
	agents       []*state.Agent
	cursor       int
	now          time.Time
	width        int
	height       int
	err          error
	showUsage    bool   // u 키: 사용량 전용 뷰
	showInfo     bool   // i 키: 선택 에이전트 상세 카드
	infoText     string // 상세 카드 렌더 결과
	pendingCmd   string // "wake"|"close": y 확인 대기 중
	notice       string // 하단 안내줄 (에러 아님)
	preview      string // 선택 pane 화면 미리보기
	previewPane  string
	usagePay     *usage.Payload
	ctx          map[string]usage.CtxInfo // CWD(절대경로) → 모델·ctx%
	wtBranch     map[string]string        // worktree 경로 → 브랜치
	discordWired map[string]bool          // CWD → Discord 연결 (⌁)
	starterTasks []starter.Task           // MultiAgent 활성 작업
	defModels    map[string]string        // CLI별 기본 모델 설정
	// 주입점 (테스트용)
	coachRunner func() ([]byte, error)
	snapshotDir string
	codexRoot   string
	geminiDir   string
	starterRoot string
	homeDir     string
}

func New(st *state.Store, tm tmuxx.Tmux) Model {
	root := config.Load().StarterRoot
	if root == "" {
		root = starter.DefaultRoot()
	}
	home, _ := os.UserHomeDir()
	return Model{store: st, tm: tm, now: time.Now(),
		coachRunner: usage.CoachRunner,
		snapshotDir: usage.SnapshotsDir(),
		codexRoot:   usage.CodexSessionsRoot(),
		geminiDir:   usage.GeminiDir(),
		starterRoot: root,
		homeDir:     home}
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
	starterRoot, home, geminiDir := m.starterRoot, m.homeDir, m.geminiDir
	return func() tea.Msg {
		pay := usage.FetchCached(st.Dir, 5*time.Minute, runner, time.Now())
		ctx := usage.LoadSnapshots(snapDir)
		if agents, err := st.List(); err == nil {
			for _, a := range agents {
				if a.CWD == "" {
					continue
				}
				if _, ok := ctx[a.CWD]; ok {
					continue
				}
				switch a.Kind {
				case "codex":
					if info := usage.CodexLatest(codexRoot, a.CWD); info.Model != "" || info.UsedPct != nil {
						ctx[a.CWD] = info
					}
				case "gemini":
					// 소스 둘 중 신선한 쪽: stock CLI 세션 파일 vs hook이 기록한 모델(agy)
					info := usage.GeminiLatest(geminiDir, a.CWD)
					if a.Model != "" && (info.Model == "" || a.UpdatedAt.After(info.TS)) {
						info = usage.CtxInfo{Model: a.Model, TS: a.UpdatedAt}
					}
					if info.Model != "" {
						ctx[a.CWD] = info
					}
				}
			}
		}
		dc := map[string]bool{}
		if agents, err := st.List(); err == nil {
			wp := wiring.DefaultPaths()
			for _, a := range agents {
				if a.CWD == "" || dc[a.CWD] {
					continue
				}
				w := wiring.Collect(wp, a.CWD, a.Tmux.Session, nil)
				dc[a.CWD] = w.DiscordConnected()
			}
		}
		return usageMsg{payload: pay, ctx: ctx, starter: starter.ActiveTasks(starterRoot), discord: dc,
			defModels: usage.DefaultModels(home)}
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
		branches := map[string]string{}
		if metas, err := wt.ListMetas(st.Dir); err == nil {
			for _, m := range metas {
				branches[m.Path] = m.Branch
			}
		}
		return refreshMsg{agents: agents, wtBranches: branches, now: now}
	}
}

// previewCmd는 선택 pane의 화면 꼬리를 가져온다 (표시 전용).
func (m Model) previewCmd() tea.Cmd {
	a := m.selected()
	if a == nil || a.State == state.StateDead {
		return func() tea.Msg { return previewMsg{} }
	}
	tm, pane, lines := m.tm, a.Tmux.PaneID, m.previewHeight()
	if lines < 3 {
		return nil
	}
	return func() tea.Msg {
		content, err := tm.CapturePane(pane, lines)
		if err != nil {
			return previewMsg{paneID: pane}
		}
		return previewMsg{paneID: pane, content: content}
	}
}

// previewHeight는 목록·헤더·도움말을 빼고 남는 미리보기 줄 수.
func (m Model) previewHeight() int {
	// 헤더≈5 + 목록 + 하단(구분선·확인/알림줄·도움말·여유)=5
	// 확인줄(y/N) 자리를 항상 예약해야 C·W 프롬프트가 잘리지 않는다.
	used := 5 + len(m.agents) + 5
	h := m.height - used
	if h > 40 {
		h = 40
	}
	return h
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
		m.starterTasks = msg.starter
		if msg.discord != nil {
			m.discordWired = msg.discord
		}
		m.defModels = msg.defModels
		return m, nil

	case refreshMsg:
		m.agents = msg.agents
		m.wtBranch = msg.wtBranches
		m.now = msg.now
		if m.cursor >= len(m.agents) {
			m.cursor = max(0, len(m.agents)-1)
		}
		return m, m.previewCmd() // 미리보기도 폴링 주기에 맞춰 라이브 갱신

	case previewMsg:
		m.preview = msg.content
		m.previewPane = msg.paneID
		return m, nil

	case jumpDoneMsg:
		m.err = msg.err
		return m, tea.Quit

	case gitDoneMsg:
		// lazygit에서 커밋 등이 일어났을 수 있으니 즉시 새로고침
		if msg.err != nil {
			m.err = fmt.Errorf("lazygit 종료: %v", msg.err)
		}
		return m, m.refreshCmd()

	case tea.KeyMsg:
		// 상세 카드가 떠 있으면 esc는 카드만 닫는다 (TUI 종료 아님)
		if m.showInfo && msg.String() == "esc" {
			m.showInfo = false
			return m, nil
		}
		// 일괄 명령 확인 대기 중이면 y만 실행, 나머지는 취소
		if m.pendingCmd != "" {
			cmd := m.pendingCmd
			m.pendingCmd = ""
			if msg.String() == "y" {
				message := cli.WakeMessage
				if cmd == "close" {
					message = cli.CloseMessage
				}
				sent, total, err := cli.SendAll(m.store, m.tm, message, true)
				if err != nil {
					m.err = err
				} else {
					m.notice = fmt.Sprintf("%d/%d 세션에 %q 전송 — 상태 변화는 이 화면에서 실시간으로 보입니다", sent, total, message)
				}
				return m, m.refreshCmd()
			}
			m.notice = "취소했습니다"
			return m, nil
		}
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.cursor < len(m.agents)-1 {
				m.cursor++
			}
			return m, m.previewCmd()
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, m.previewCmd()
		case "r":
			return m, tea.Batch(m.refreshCmd(), m.usageCmd())
		case "u":
			m.showUsage = !m.showUsage
			m.showInfo = false
			return m, nil
		case "W": // 모든 세션 이어서하기
			m.pendingCmd = "wake"
			m.notice = ""
			return m, nil
		case "C": // 모든 세션 마감
			m.pendingCmd = "close"
			m.notice = ""
			return m, nil
		case "g": // 선택 에이전트 폴더를 lazygit으로 (조작은 lazygit이 정본)
			if a := m.selected(); a != nil && a.CWD != "" {
				bin := usage.LookupTool("lazygit")
				if bin == "" {
					m.err = fmt.Errorf("lazygit이 설치돼 있지 않습니다 (brew install lazygit)")
					return m, nil
				}
				// lazygit은 저장소가 아니면 즉시 종료(깜빡임)하므로 미리 검사
				if exec.Command("git", "-C", a.CWD, "rev-parse", "--git-dir").Run() != nil {
					m.err = fmt.Errorf("%s는 git 저장소가 아닙니다 — worktree 태스크나 repo 폴더에서만 g가 동작합니다", a.Tmux.Session)
					return m, nil
				}
				m.err = nil
				c := exec.Command(bin, "-p", a.CWD)
				return m, tea.ExecProcess(c, func(err error) tea.Msg { return gitDoneMsg{err: err} })
			}
			return m, nil
		case "i":
			if m.showInfo {
				m.showInfo = false
				return m, nil
			}
			if a := m.selected(); a != nil {
				m.infoText = m.buildInfo(a)
				m.showInfo = true
			}
			return m, nil
		case "o": // 읽음 처리만 (점프 없이)
			if a := m.selected(); a != nil {
				_ = m.store.MarkRead(a.ID, time.Now())
				return m, m.refreshCmd()
			}
		case "enter": // 점프 + 읽음 처리
			if a := m.selected(); a != nil {
				if a.State == state.StateDead {
					// 죽은 pane의 좌표는 무효 — 점프 대신 복구 경로 안내
					if a.SessionID != "" {
						m.notice = fmt.Sprintf("죽은 세션입니다 — 대화 복구: agentlayer resume %s", a.ID)
					} else {
						m.notice = "죽은 세션입니다 (세션 ID 미기록 — 24시간 뒤 자동 정리)"
					}
					return m, nil
				}
				_ = m.store.MarkRead(a.ID, time.Now())
				return m, m.jumpCmd(a)
			}
		}
	}
	return m, nil
}

// buildInfo는 선택 에이전트의 배선 상세 카드를 조립한다 (동기, 파일 몇 개 읽기).
func (m Model) buildInfo(a *state.Agent) string {
	cfg := config.Load()
	d := cli.InfoData{
		Agent:  a,
		Wiring: wiring.Collect(wiring.DefaultPaths(), a.CWD, a.Tmux.Session, cfg.ChannelLabels),
		Ctx:    m.ctx[a.CWD],
		Labels: cfg.ChannelLabels,
	}
	if br, ok := m.wtBranch[a.CWD]; ok {
		d.Branch = br
	}
	var buf strings.Builder
	cli.RenderInfo(&buf, d, time.Now())
	return buf.String()
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
