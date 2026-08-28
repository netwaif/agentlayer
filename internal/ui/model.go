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

// usageMsg는 coach 사용량 수집 결과 (느림 — 콜드 실행은 분 단위).
type usageMsg struct {
	payload *usage.Payload
}

// ctxMsg는 빠른 로컬 수집 결과 — 파일 읽기뿐이라 즉시 뜬다.
// coach가 느려도 이 정보(모델·ctx·⌁·기본모델·MultiAgent)는 기다리지 않는다.
type ctxMsg struct {
	ctx       map[string]usage.CtxInfo
	starter   []starter.Task
	discord   map[string]bool   // CWD → Discord 연결 여부 (⌁ 마크)
	defModels map[string]string // CLI별 기본 모델 설정 (빈 값 = 미설정/자동)
}

// jumpDoneMsg는 점프 실행 후 종료 신호.
type jumpDoneMsg struct{ err error }

// gitDoneMsg는 lazygit에서 돌아온 뒤 새로고침 신호.
type gitDoneMsg struct{ err error }

// attachDoneMsg는 tmux attach(밖에서 enter)에서 detach로 돌아온 신호.
type attachDoneMsg struct{ err error }

// previewMsg는 선택 pane 화면 미리보기.
type previewMsg struct {
	paneID  string
	content string
}

// Model은 TUI 상태.
type Model struct {
	store         *state.Store
	tm            tmuxx.Tmux
	agents        []*state.Agent
	cursor        int
	now           time.Time
	width         int
	height        int
	err           error
	showUsage     bool         // u 키: 사용량 전용 뷰
	showInfo      bool         // i 키: 선택 에이전트 상세 카드
	infoText      string       // 상세 카드 렌더 결과
	pendingCmd    string       // "wake"|"close"|"resume": y 확인 대기 중
	pendingResume *state.Agent // pendingCmd=="resume"일 때 대상
	notice        string       // 하단 안내줄 (에러 아님)
	insideTmux    bool         // false면 enter가 점프 대신 attach (tmux 밖 ssh 실행 등)
	preview       string       // 선택 pane 화면 미리보기
	previewPane   string
	usagePay      *usage.Payload
	ctx           map[string]usage.CtxInfo // 에이전트 ID → 모델·ctx%
	wtBranch      map[string]string        // worktree 경로 → 브랜치
	discordWired  map[string]bool          // CWD → Discord 연결 (⌁)
	starterTasks  []starter.Task           // MultiAgent 활성 작업
	defModels     map[string]string        // CLI별 기본 모델 설정
	// 주입점 (테스트용)
	newWindow   func(name, dir, command string) error // resume 창 생성
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
		insideTmux:  os.Getenv("TMUX") != "",
		newWindow:   tm.NewWindow,
		coachRunner: usage.CoachRunner,
		snapshotDir: usage.SnapshotsDir(),
		codexRoot:   usage.CodexSessionsRoot(),
		geminiDir:   usage.GeminiDir(),
		starterRoot: root,
		homeDir:     home}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), tickCmd(), m.ctxCmd(), m.usageCmd(), usageTickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func usageTickCmd() tea.Cmd {
	return tea.Tick(usageInterval, func(t time.Time) tea.Msg { return usageTickMsg(t) })
}

// usageCmd는 coach 사용량만 백그라운드에서 가져온다.
// coach는 콜드 실행이 분 단위라 5분 파일 캐시 + 실행 중복 방지로 감싼다.
// 느린 건 이것뿐이므로 빠른 로컬 정보(ctxCmd)와 분리해 화면을 막지 않는다.
func (m Model) usageCmd() tea.Cmd {
	runner, st := m.coachRunner, m.store
	return func() tea.Msg {
		return usageMsg{payload: usage.FetchCached(st.Dir, 5*time.Minute, runner, time.Now())}
	}
}

// ctxCmd는 빠른 로컬 정보를 모은다 — 전부 파일 읽기라 즉시 완료된다.
// 어떤 소스가 없어도 관제는 계속된다.
func (m Model) ctxCmd() tea.Cmd {
	snapDir, codexRoot, geminiDir, st := m.snapshotDir, m.codexRoot, m.geminiDir, m.store
	starterRoot, home := m.starterRoot, m.homeDir
	return func() tea.Msg {
		ctx := map[string]usage.CtxInfo{}
		dc := map[string]bool{}
		if agents, err := st.List(); err == nil {
			ctx = usage.AgentCtx(agents, usage.LoadSnapshots(snapDir), codexRoot, geminiDir)
			wp := wiring.DefaultPaths()
			for _, a := range agents {
				if a.CWD == "" || dc[a.CWD] {
					continue
				}
				w := wiring.Collect(wp, a.CWD, a.Tmux.Session, nil)
				dc[a.CWD] = w.DiscordConnected()
			}
		}
		return ctxMsg{ctx: ctx, starter: starter.ActiveTasks(starterRoot), discord: dc,
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
	// 헤더≈5 + 목록(+종류 구분선) + 하단(구분선·확인/알림줄·도움말·여유)=5
	// 확인줄(y/N) 자리를 항상 예약해야 C·W 프롬프트가 잘리지 않는다.
	used := 5 + len(m.agents) + max(0, m.kindGroupCount()-1) + 5
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

// attachCmd는 tmux 밖 실행용 enter — 이 터미널을 대상 세션에 attach한다.
// detach(C-b d)로 돌아오면 TUI가 이어진다. switch-client 기반 jumpCmd를
// 밖에서 쓰면 남의 클라이언트(책상 화면)가 전환되므로 반드시 이 경로로.
func (m Model) attachCmd(a *state.Agent) tea.Cmd {
	ref := tmuxx.Ref{Session: a.Tmux.Session, Window: a.Tmux.Window, PaneID: a.Tmux.PaneID}
	c := exec.Command(tmuxx.Bin(), tmuxx.AttachArgv(ref)...)
	return tea.ExecProcess(c, func(err error) tea.Msg { return attachDoneMsg{err: err} })
}

// startResume은 y 확인된 죽은 세션의 대화를 새 창에서 되살리고 그리로 이동한다.
// tmux 안: 현재 세션에 창 생성(자동 활성) 후 TUI 종료. 밖: 원 세션이 살아
// 있으면 거기 만들어 attach, 없으면 CLI 안내로 폴백.
func (m Model) startResume() (tea.Model, tea.Cmd) {
	a := m.pendingResume
	m.pendingResume = nil
	if a == nil {
		return m, nil
	}
	cmdStr, err := cli.ResumeCommand(a)
	if err != nil {
		m.err = err
		return m, nil
	}
	name := "resume-" + a.ID
	if m.insideTmux {
		if err := m.newWindow(name, a.CWD, cmdStr); err != nil {
			m.err = err
			return m, nil
		}
		return m, tea.Quit // 새 창이 현재 세션에서 활성 — 팝업이 닫히면 그 화면
	}
	if a.Tmux.Session != "" && m.tm.HasSession(a.Tmux.Session) {
		pane, err := m.tm.NewWindowIn(a.Tmux.Session, name, a.CWD)
		if err != nil {
			m.err = err
			return m, nil
		}
		_ = m.tm.SendText(pane, cmdStr)
		c := exec.Command(tmuxx.Bin(), "attach-session", "-t", "="+a.Tmux.Session)
		return m, tea.ExecProcess(c, func(err error) tea.Msg { return attachDoneMsg{err: err} })
	}
	m.notice = fmt.Sprintf("원 세션이 없어 여기서는 복구 불가 — 터미널에서 'agentlayer resume %s' 실행", a.ID)
	return m, nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.refreshCmd(), tickCmd())

	case usageTickMsg:
		return m, tea.Batch(m.usageCmd(), m.ctxCmd(), usageTickCmd())

	case usageMsg:
		if msg.payload != nil {
			m.usagePay = msg.payload
		}
		return m, nil

	case ctxMsg:
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

	case attachDoneMsg:
		// detach로 복귀 — TUI는 계속, 상태만 새로고침
		if msg.err != nil {
			m.err = fmt.Errorf("attach 종료: %v", msg.err)
		}
		return m, m.refreshCmd()

	case tea.KeyMsg:
		// 상세 카드가 떠 있으면 esc는 카드만 닫는다 (TUI 종료 아님)
		if m.showInfo && msg.String() == "esc" {
			m.showInfo = false
			return m, nil
		}
		// 확인 대기 중이면 y만 실행, 나머지는 취소
		if m.pendingCmd != "" {
			cmd := m.pendingCmd
			m.pendingCmd = ""
			if cmd == "resume" {
				if msg.String() == "y" {
					return m.startResume()
				}
				m.pendingResume = nil
				m.notice = "취소했습니다"
				return m, nil
			}
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
		// notice는 일회성 안내 — 다음 키 입력에서 지운다 (필요한 분기가 다시 설정)
		m.notice = ""
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
			return m, tea.Batch(m.refreshCmd(), m.usageCmd(), m.ctxCmd())
		case "u":
			m.showUsage = !m.showUsage
			m.showInfo = false
			return m, nil
		case "W": // 모든 세션 이어서하기
			m.pendingCmd = "wake"
			return m, nil
		case "C": // 모든 세션 마감
			m.pendingCmd = "close"
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
					// 죽은 pane의 좌표는 무효 — 점프 대신 복구(y/n 확인)로 진입
					if _, rerr := cli.ResumeCommand(a); rerr != nil {
						m.notice = fmt.Sprintf("죽은 세션입니다 — 복구 불가: %v (24시간 뒤 자동 정리)", rerr)
						return m, nil
					}
					m.pendingCmd = "resume"
					m.pendingResume = a
					return m, nil
				}
				_ = m.store.MarkRead(a.ID, time.Now())
				if !m.insideTmux {
					return m, m.attachCmd(a)
				}
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
		Ctx:    m.ctx[a.ID],
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
