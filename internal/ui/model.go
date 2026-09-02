// Package ui는 관제 TUI를 구현한다. mat과 같은 bubbletea 패턴:
// 2초 폴링, 파일 watch 없음(안정성), Update는 순수 함수에 가깝게.
package ui

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/netwaif/agentlayer/internal/browser"
	"github.com/netwaif/agentlayer/internal/cli"
	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/popup"
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

// previewTickMsg는 미리보기 전용 틱 — 목록 폴링(tickMsg)과 주기를 분리해
// config preview_interval로 따로 조절한다.
type previewTickMsg time.Time

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

// browserDoneMsg는 browser pick/shot(ExecProcess)에서 돌아온 신호.
type browserDoneMsg struct{ err error }

// devTickMsg는 dev 서버 스캔 틱(10s) — lsof 전수 스캔이라 목록 폴링보다 느리게.
type devTickMsg time.Time

// devServersMsg는 스캔 결과.
type devServersMsg []browser.DevServer

// noticeMsg는 비동기 작업의 한 줄 안내(에러면 err).
type noticeMsg struct {
	text string
	err  error
}

const devScanInterval = 10 * time.Second

// attachDoneMsg는 tmux attach(밖에서 enter)에서 detach로 돌아온 신호.
type attachDoneMsg struct{ err error }

// previewMsg는 선택 pane 화면 미리보기.
type previewMsg struct {
	paneID  string
	content string
}

// Model은 TUI 상태.
type Model struct {
	store           *state.Store
	tm              tmuxx.Tmux
	agents          []*state.Agent
	cursor          int
	now             time.Time
	width           int
	height          int
	err             error
	showUsage       bool            // u 키: 사용량 전용 뷰
	showInfo        bool            // i 키: 선택 에이전트 상세 카드
	infoText        string          // 상세 카드 렌더 결과
	pendingCmd      string          // "wake"|"close"|"resume"|"broadcast": y 확인 대기 중
	pendingResume   *state.Agent    // pendingCmd=="resume"일 때 대상
	broadcastText   string          // pendingCmd=="broadcast"일 때 보낼 메시지
	inputMode       bool            // B 키: 전체지시 메시지 입력 중
	input           textinput.Model // 입력 위젯 (커서 이동·중간 편집·붙여넣기)
	notice          string          // 하단 안내줄 (에러 아님)
	insideTmux      bool            // false면 enter가 점프 대신 attach (tmux 밖 ssh 실행 등)
	preview         string          // 선택 pane 화면 미리보기
	previewPane     string
	previewInterval time.Duration // config preview_interval (기본 1s)
	usagePay        *usage.Payload
	ctx             map[string]usage.CtxInfo // 에이전트 ID → 모델·ctx%
	wtBranch        map[string]string        // worktree 경로 → 브랜치
	discordWired    map[string]bool          // CWD → Discord 연결 (⌁)
	starterTasks    []starter.Task           // MultiAgent 활성 작업
	defModels       map[string]string        // CLI별 기본 모델 설정
	devServers      []browser.DevServer      // 에이전트 폴더 아래 listen 중인 dev 서버 (🌐 뱃지·p)
	browserPort     int                      // 전용 Chrome CDP 포트 — dev 서버 목록에서 제외
	// 주입점 (테스트용)
	devScan       func(paths map[string]string) []browser.DevServer        // dev 서버 스캔
	openPreview   func(servers []browser.DevServer) error                  // p: 프리뷰 창 열기
	browserCmd    func(args ...string) tea.Cmd                             // b/s: agentlayer browser … 실행
	popupRecord   func(cols, rows int, cursor string)                      // 팝업 안이면 크기·커서 기록 (재오픈 복원용)
	restoreCursor string                                                   // 재오픈 직후 복원할 에이전트 ID
	spawnWindow   func(session, name, dir, command string) (string, error) // resume 창 생성
	activeSession func() string                                            // 활성 클라이언트의 세션
	hasSession    func(name string) bool                                   // 세션 생존 (완전일치)
	jumpPane      func(session, pane string) error                         // 세션 전환+창 선택
	sendAll       func(message string, handoffOnly bool) (int, int, error) // 일괄 전송
	coachRunner   func() ([]byte, error)
	snapshotDir   string
	codexRoot     string
	geminiDir     string
	starterRoot   string
	homeDir       string
}

// WithPopup은 팝업 바인딩으로 떴을 때의 배선 — 기록 파일과 커서 복원.
func (m Model) WithPopup(stateDir string) Model {
	m.popupRecord = func(cols, rows int, cursor string) {
		popup.Save(stateDir, popup.Record{PID: os.Getpid(), Cols: cols, Rows: rows, Cursor: cursor})
	}
	if rec, ok := popup.Load(stateDir); ok {
		m.restoreCursor = rec.Cursor
	}
	return m
}

func New(st *state.Store, tm tmuxx.Tmux) Model {
	cfg := config.Load()
	root := cfg.StarterRoot
	if root == "" {
		root = starter.DefaultRoot()
	}
	home, _ := os.UserHomeDir()
	ti := textinput.New()
	ti.Prompt = "⌨ 전체 전송 메시지: "
	ti.CharLimit = 500
	return Model{store: st, tm: tm, now: time.Now(),
		input:           ti,
		previewInterval: cfg.PreviewTick(),
		browserPort:     cfg.BrowserPortOrDefault(),
		insideTmux:      os.Getenv("TMUX") != "",
		spawnWindow:     tm.SpawnShellWindow,
		activeSession:   tm.ActiveSession,
		hasSession:      tm.HasSession,
		jumpPane:        tm.JumpToSessionPane,
		sendAll: func(message string, handoffOnly bool) (int, int, error) {
			return cli.SendAll(st, tm, message, handoffOnly)
		},
		coachRunner: usage.CoachRunner,
		devScan: func(paths map[string]string) []browser.DevServer {
			return browser.FilterHTML(browser.DevServers(browser.ExecLsof, paths), nil)
		},
		openPreview: func(servers []browser.DevServer) error {
			b, err := browser.Connect(state.DefaultDir(), cfg.BrowserPortOrDefault())
			if err != nil {
				return err
			}
			for _, s := range servers {
				if err := browser.OpenPreview(b, s); err != nil {
					return err
				}
			}
			return nil
		},
		browserCmd: func(args ...string) tea.Cmd {
			bin, err := os.Executable()
			if err != nil {
				bin = "agentlayer"
			}
			c := exec.Command(bin, append([]string{"browser"}, args[1:]...)...)
			return tea.ExecProcess(c, func(err error) tea.Msg { return browserDoneMsg{err: err} })
		},
		snapshotDir: usage.SnapshotsDir(),
		codexRoot:   usage.CodexSessionsRoot(),
		geminiDir:   usage.GeminiDir(),
		starterRoot: root,
		homeDir:     home}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), tickCmd(), m.previewTickCmd(), m.ctxCmd(),
		m.usageCacheCmd(), m.usageCmd(), usageTickCmd(), m.devScanCmd(), devTickCmd())
}

func devTickCmd() tea.Cmd {
	return tea.Tick(devScanInterval, func(t time.Time) tea.Msg { return devTickMsg(t) })
}

// devScanCmd는 에이전트 폴더(+worktree 브랜치)를 기준으로 dev 서버를 찾는다.
func (m Model) devScanCmd() tea.Cmd {
	paths := map[string]string{}
	for _, a := range m.agents {
		if a.CWD != "" {
			paths[a.CWD] = m.wtBranch[a.CWD]
		}
	}
	scan := m.devScan
	return func() tea.Msg {
		if len(paths) == 0 || scan == nil {
			return devServersMsg(nil)
		}
		return devServersMsg(scan(paths))
	}
}

// serversFor는 에이전트 폴더 아래에서 listen 중인 dev 서버(포트 오름차순).
func (m Model) serversFor(a *state.Agent) []browser.DevServer {
	if a == nil || a.CWD == "" {
		return nil
	}
	root := strings.TrimSuffix(a.CWD, "/")
	var out []browser.DevServer
	for _, s := range m.devServers {
		if s.Port == m.browserPort {
			continue // 전용 Chrome 자신
		}
		if s.CWD == root || strings.HasPrefix(s.CWD, root+"/") {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out
}

// devBadge는 행 끝의 🌐:3000,:5173 뱃지 텍스트. 없으면 "".
func (m Model) devBadge(a *state.Agent) string {
	servers := m.serversFor(a)
	if len(servers) == 0 {
		return ""
	}
	parts := make([]string, 0, len(servers))
	for _, s := range servers {
		parts = append(parts, ":"+strconv.Itoa(s.Port))
	}
	return "🌐" + strings.Join(parts, ",")
}

func (m Model) previewTickCmd() tea.Cmd {
	return tea.Tick(m.previewInterval, func(t time.Time) tea.Msg { return previewTickMsg(t) })
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func usageTickCmd() tea.Cmd {
	return tea.Tick(usageInterval, func(t time.Time) tea.Msg { return usageTickMsg(t) })
}

// usageCacheCmd는 디스크 캐시를 나이 불문 즉시 usageMsg로 올린다 —
// 첫 페인트가 콜드 coach를 기다리지 않게(stale-while-revalidate의 stale 쪽).
// 갱신은 usageCmd가 뒤에서 하고, 나이는 usageAge가 화면에 밝힌다.
func (m Model) usageCacheCmd() tea.Cmd {
	dir := m.store.Dir
	return func() tea.Msg { return usageMsg{payload: usage.ReadCached(dir)} }
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
// tmux 안(팝업): 활성 클라이언트의 세션에 명시 타겟으로 창 생성(자동 활성) 후
// TUI 종료. 밖: 원 세션이 살아 있으면 거기 만들어 attach, 없으면 CLI 안내 폴백.
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
		// 원 세션이 살아 있으면 대화는 제 집으로 — 세션 이름·목록 표시가 맞아떨어진다.
		// 죽었으면 활성 세션 폴백 (팝업 안에서는 tmux가 현재 세션을 특정 못 하므로 명시)
		target := ""
		if a.Tmux.Session != "" && m.hasSession(a.Tmux.Session) {
			target = a.Tmux.Session
		} else if target = m.activeSession(); target == "" {
			m.err = fmt.Errorf("활성 tmux 세션을 못 찾았습니다")
			return m, nil
		}
		pane, err := m.spawnWindow(target, name, a.CWD, cmdStr)
		if err != nil {
			m.err = err
			return m, nil
		}
		// 이중 행 방지 — 부활 성공한 원본 dead 레코드는 즉시 삭제 (restore와 동일)
		_ = m.store.Delete(a.ID)
		// 다른 세션에 만들었어도 그리로 이동 (활성 세션이면 무해한 재선택)
		_ = m.jumpPane(target, pane)
		return m, tea.Quit
	}
	if a.Tmux.Session != "" && m.hasSession(a.Tmux.Session) {
		if _, err := m.spawnWindow(a.Tmux.Session, name, a.CWD, cmdStr); err != nil {
			m.err = err
			return m, nil
		}
		_ = m.store.Delete(a.ID)
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
		m.recordPopup()
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.refreshCmd(), tickCmd())

	case previewTickMsg:
		return m, tea.Batch(m.previewCmd(), m.previewTickCmd())

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
		if m.restoreCursor != "" { // 팝업 재오픈 — 직전 선택 행으로
			for i, a := range m.agents {
				if a.ID == m.restoreCursor {
					m.cursor = i
				}
			}
			m.restoreCursor = ""
		}
		if m.cursor >= len(m.agents) {
			m.cursor = max(0, len(m.agents)-1)
		}
		return m, nil // 미리보기는 자체 틱(previewTickMsg)이 갱신

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

	case browserDoneMsg:
		// pick은 Ctrl-C로 끝나는 게 정상 흐름 — 에러로 띄우지 않는다
		return m, m.refreshCmd()

	case devTickMsg:
		return m, tea.Batch(m.devScanCmd(), devTickCmd())

	case devServersMsg:
		// 뱃지 갱신만. 브라우저를 띄우거나 앞으로 끌어오는 건 사용자가 b/s/p를 눌렀을 때뿐 —
		// 새 서버 자동 열기는 hook 경로(browser autopreview, preview-seen.json)가 정본이다.
		m.devServers = []browser.DevServer(msg)
		return m, nil

	case noticeMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.notice = msg.text
		}
		return m, nil

	case attachDoneMsg:
		// detach로 복귀 — TUI는 계속, 상태만 새로고침
		if msg.err != nil {
			m.err = fmt.Errorf("attach 종료: %v", msg.err)
		}
		return m, m.refreshCmd()

	case tea.KeyMsg:
		// 전체지시 입력 중이면 esc·enter만 직접 처리, 나머지는 입력 위젯에 위임
		if m.inputMode {
			switch msg.Type {
			case tea.KeyEsc:
				m.inputMode = false
				m.input.SetValue("")
				m.input.Blur()
				return m, nil
			case tea.KeyEnter:
				m.inputMode = false
				if text := strings.TrimSpace(m.input.Value()); text != "" {
					m.broadcastText = text
					m.pendingCmd = "broadcast"
				}
				m.input.SetValue("")
				m.input.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
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
			if cmd == "broadcast" {
				text := m.broadcastText
				m.broadcastText = ""
				if msg.String() == "y" {
					sent, total, err := m.sendAll(text, false) // broadcast는 전체 대상
					if err != nil {
						m.err = err
					} else {
						m.notice = fmt.Sprintf("%d/%d 세션에 %q 전송", sent, total, text)
					}
					return m, m.refreshCmd()
				}
				m.notice = "취소했습니다"
				return m, nil
			}
			if msg.String() == "y" {
				message := cli.WakeMessage
				if cmd == "close" {
					message = cli.CloseMessage
				}
				sent, total, err := m.sendAll(message, true)
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
			m.recordPopup()
			return m, m.previewCmd()
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
			m.recordPopup()
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
		case "B": // 전체지시 — 임의 메시지 브로드캐스트
			m.inputMode = true
			m.input.SetValue("")
			m.input.Focus()
			return m, textinput.Blink
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
		case "b": // 브라우저 요소 지목 → 선택 에이전트 pane으로 (후보 선택 없음)
			if a := m.selected(); a != nil {
				m.err = nil
				return m, m.browserCmd("browser", "pick", "--agent", a.ID)
			}
			return m, nil
		case "s": // 활성 탭 스크린샷 → 선택 에이전트 pane으로
			if a := m.selected(); a != nil {
				m.err = nil
				return m, m.browserCmd("browser", "shot", "--send", "--agent", a.ID)
			}
			return m, nil
		case "p": // 선택 에이전트 폴더의 dev 서버를 전용 브라우저 창으로
			a := m.selected()
			servers := m.serversFor(a)
			if len(servers) == 0 {
				m.notice = "이 에이전트 폴더에서 실행 중인 dev 서버가 없습니다 (10초마다 재탐색)"
				return m, nil
			}
			open := m.openPreview
			return m, func() tea.Msg {
				if err := open(servers); err != nil {
					return noticeMsg{err: err}
				}
				return noticeMsg{text: "프리뷰 열림: " + m.devBadge(a)}
			}
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

// recordPopup은 팝업 안일 때 크기·커서를 남긴다 — client-resized 훅이 재오픈 판단에 쓴다.
func (m Model) recordPopup() {
	if m.popupRecord == nil {
		return
	}
	cursor := ""
	if a := m.selected(); a != nil {
		cursor = a.ID
	}
	m.popupRecord(m.width, m.height, cursor)
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
