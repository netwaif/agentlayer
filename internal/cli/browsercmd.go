// browser 서브커맨드: 에이전트 전용 브라우저(전용 프로필 Chrome) 관제.
package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/netwaif/agentlayer/internal/browser"
	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/discord"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
	"github.com/netwaif/agentlayer/internal/usage"
	"github.com/netwaif/agentlayer/internal/wt"
)

// RunBrowser는 browser 서브커맨드를 디스패치한다.
// 빈 인자=기동. Task 6~8이 shot/errors/preview case를 여기에 추가한다.
func RunBrowser(out io.Writer, args []string) error {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
		args = args[1:] // 서브커맨드 뒤 인자 — Task 6~8의 case가 사용
	}
	switch sub {
	case "":
		return browserLaunch(out)
	case "pick":
		return browserPick(out, args)
	case "shot":
		return browserShot(out, args)
	case "errors":
		return browserErrors(out, args)
	case "preview":
		return browserPreview(out, args)
	case "cookies":
		return browserCookies(out, args)
	case "mcp":
		return browserMCP(out)
	case "open":
		return browserOpen(out, args)
	case "mcp-serve":
		return browserMCPServe()
	case "autopreview":
		return browserAutoPreview(out)
	default:
		return fmt.Errorf("모르는 browser 서브커맨드 %q — 'agentlayer help' 참고", sub)
	}
}

// browserLaunch는 전용 브라우저를 기동(또는 기존 인스턴스에 attach)한다.
func browserLaunch(out io.Writer) error {
	if _, err := browser.Connect(state.DefaultDir(), config.Load().BrowserPortOrDefault()); err != nil {
		return err
	}
	fmt.Fprintln(out, "에이전트 전용 브라우저 준비 완료 (프로필: browser-profile — 로그인 세션 유지)")
	return nil
}

// parsePickArgs는 pick의 --agent(대상 고정)를 파싱한다. 위치 인자는 없다.
func parsePickArgs(args []string) (string, error) {
	fs := flag.NewFlagSet("pick", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	agent := fs.String("agent", "", "전송 대상 에이전트 ID 고정 (후보 선택 생략)")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() > 0 {
		return "", fmt.Errorf("잉여 인자 %q — 사용법: agentlayer browser pick [--agent <id>]", fs.Args())
	}
	return *agent, nil
}

// FilterAgentByID는 ID가 일치하는 에이전트 하나만 담은 목록. 관제탑에서 행을
// 고르고 b/s를 눌렀을 때 라우팅·후보 선택을 건너뛰는 용도.
func FilterAgentByID(agents []*state.Agent, id string) ([]*state.Agent, error) {
	for _, a := range agents {
		if a.ID == id {
			return []*state.Agent{a}, nil
		}
	}
	return nil, fmt.Errorf("에이전트 %q가 없습니다 (agentlayer status 확인)", id)
}

// browserPick은 활성 탭에서 요소 지목 사이클을 연속으로 돈다.
// RunPick이 탭 닫힘·페이지 이탈 시 에러를 반환하므로 무한 블록은 없다.
// --agent가 있으면 그 에이전트만 후보라 오버레이 선택이 곧바로 확정된다.
func browserPick(out io.Writer, args []string) error {
	agentID, err := parsePickArgs(args)
	if err != nil {
		return err
	}
	st, err := state.NewStore(state.DefaultDir())
	if err != nil {
		return err
	}
	b, err := browser.Connect(state.DefaultDir(), config.Load().BrowserPortOrDefault())
	if err != nil {
		return err
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	if agentID != "" {
		if agents, err = FilterAgentByID(agents, agentID); err != nil {
			return err
		}
	}
	if len(agents) == 0 {
		return fmt.Errorf("등록된 에이전트가 없습니다 (agentlayer status 확인)")
	}
	tm := tmuxx.Tmux{}
	send := func(paneID, text string) error { return tm.SendText(paneID, text) }
	// 터미널 esc/q/Ctrl-C로도 돌아갈 수 있게 — 관제탑 b 키에서 들어온 사용자가
	// 브라우저를 안 건드리고 취소할 길이 이것뿐이다.
	quit, restore := watchQuitKeys(os.Stdin)
	defer restore()
	for { // 연속 지목 — 오버레이 Esc·터미널 esc/q·Ctrl-C로 종료
		page, err := browser.ActivePage(b)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			select {
			case <-quit:
				cancel()
			case <-ctx.Done():
			}
		}()
		err = browser.RunPick(page.Context(ctx), agents, browser.ExecLsof, state.DefaultDir(), send, out)
		cancel()
		select {
		case <-quit:
			fmt.Fprintln(out, "돌아갑니다")
			return nil
		default:
		}
		if errors.Is(err, browser.ErrPickCancelled) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// quitKey는 pick 대기 중 종료로 볼 키 — esc, q, Ctrl-C.
func quitKey(b byte) bool { return b == 0x1b || b == 'q' || b == 0x03 }

// watchQuitKeys는 stdin이 터미널이면 raw 모드로 종료 키를 감시한다.
// 반환 채널은 종료 키에서 닫히고, restore는 터미널 상태를 되돌린다.
// 터미널이 아니면(파이프·테스트) 아무것도 감시하지 않는다.
func watchQuitKeys(in *os.File) (<-chan struct{}, func()) {
	quit := make(chan struct{})
	fd := in.Fd()
	if !term.IsTerminal(fd) {
		return quit, func() {}
	}
	st, err := term.MakeRaw(fd)
	if err != nil {
		return quit, func() {}
	}
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := in.Read(buf)
			if err != nil {
				return
			}
			if n == 1 && quitKey(buf[0]) {
				close(quit)
				return
			}
		}
	}()
	return quit, func() { _ = term.Restore(fd, st) }
}

// shotOpts는 shot의 인자. --send는 에이전트 pane, --notify는 알림 웹훅(폰 Discord).
type shotOpts struct {
	URL    string
	Send   bool
	Notify bool
	Agent  string // --send 대상 고정 (관제탑 s 키)
}

func parseShotArgs(args []string) (shotOpts, error) {
	fs := flag.NewFlagSet("shot", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // usage 자동 출력 억제 — 반환 에러로 충분
	var o shotOpts
	fs.BoolVar(&o.Send, "send", false, "에이전트 pane으로 경로 전송")
	fs.BoolVar(&o.Notify, "notify", false, "알림 웹훅으로 이미지 전송")
	fs.StringVar(&o.Agent, "agent", "", "--send 대상 에이전트 ID 고정")
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return shotOpts{}, err
		}
		if fs.NArg() == 0 {
			break
		}
		rest = append(rest, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(rest) > 1 {
		return shotOpts{}, fmt.Errorf("잉여 인자 %q — 사용법: agentlayer browser shot [url] [--send] [--notify]", rest[1:])
	}
	if len(rest) == 1 {
		o.URL = rest[0]
	}
	return o, nil
}

// parseErrorsArgs는 errors의 --send를 파싱한다 — 위치 인자는 없으므로
// 비플래그 인자가 남으면 잉여로 에러 처리한다.
func parseErrorsArgs(args []string) (send bool, err error) {
	fs := flag.NewFlagSet("errors", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // usage 자동 출력 억제 — 반환 에러로 충분
	sendFlag := fs.Bool("send", false, "에이전트 pane으로 경로 전송")
	if err := fs.Parse(args); err != nil {
		return false, err
	}
	if fs.NArg() > 0 {
		return false, fmt.Errorf("잉여 인자 %q — 사용법: agentlayer browser errors [--send]", fs.Args())
	}
	return *sendFlag, nil
}

// chooseAgent는 전송 대상을 확정한다 — 후보 1명이면 즉시, 복수면 번호
// 목록(kind·세션명)을 출력하고 in에서 번호를 읽어 선택한다(스펙 라우팅
// 규칙: 후보 복수면 선택). 잘못된 입력은 에러.
func chooseAgent(cands []*state.Agent, in io.Reader, out io.Writer) (*state.Agent, error) {
	if len(cands) == 1 {
		return cands[0], nil
	}
	fmt.Fprintln(out, "전송 대상 후보가 여럿입니다 — 번호를 선택하세요:")
	for i, a := range cands {
		fmt.Fprintf(out, "  %d) %s (%s · %s)\n", i+1, a.ID, a.Kind, a.Tmux.Session)
	}
	fmt.Fprint(out, "번호: ")
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return nil, fmt.Errorf("선택 입력을 읽지 못했습니다: %w", err)
	}
	line = strings.TrimSpace(line)
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(cands) {
		return nil, fmt.Errorf("잘못된 선택 %q — 1~%d 범위의 번호를 입력하세요", line, len(cands))
	}
	return cands[n-1], nil
}

// sendToAgent는 shot/errors 공통의 --send 꼬리 — 레지스트리에서 페이지 URL로
// 후보를 좁히고(복수면 chooseAgent 선택) 한 줄을 pane으로 보낸다.
// makeLine은 페이지 URL을 받아 전송할 한 줄을 만든다.
func sendToAgent(out io.Writer, in io.Reader, page *rod.Page, agentID string, makeLine func(pageURL string) string) error {
	st, err := state.NewStore(state.DefaultDir())
	if err != nil {
		return err
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	info, err := page.Info()
	if err != nil {
		return err
	}
	var cands []*state.Agent
	if agentID != "" {
		if cands, err = FilterAgentByID(agents, agentID); err != nil {
			return err
		}
	} else {
		cands = browser.Candidates(agents, info.URL, browser.ExecLsof)
	}
	if len(cands) == 0 {
		return fmt.Errorf("전송할 에이전트가 없습니다 (agentlayer status 확인)")
	}
	target, err := chooseAgent(cands, in, out)
	if err != nil {
		return err
	}
	if err := (tmuxx.Tmux{}).SendText(target.Tmux.PaneID, makeLine(info.URL)); err != nil {
		return err
	}
	fmt.Fprintf(out, "→ %s에게 전송\n", target.ID)
	return nil
}

// browserErrors는 활성 탭의 콘솔 에러·JS 예외를 Enter까지 온디맨드 수집해
// 덤프(stdout+파일)하고, --send면 shot과 같은 라우팅으로 한 줄 보낸다.
func browserErrors(out io.Writer, args []string) error {
	send, err := parseErrorsArgs(args)
	if err != nil {
		return err
	}
	b, err := browser.Connect(state.DefaultDir(), config.Load().BrowserPortOrDefault())
	if err != nil {
		return err
	}
	page, err := browser.ActivePage(b)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "수집 시작 — 버그를 재현한 뒤 Enter…")
	until := make(chan struct{})
	go func() {
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		close(until)
	}()
	lines := browser.CollectErrors(page, until)
	// 저장 실패해도 수집분이 유실되지 않게 라인부터 stdout에 찍는다.
	if len(lines) == 0 {
		fmt.Fprintln(out, "(수집된 에러 없음)")
	}
	for _, l := range lines {
		fmt.Fprintln(out, l)
	}
	path, err := browser.SaveErrors(state.DefaultDir(), lines, time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintln(out, path)
	if !send {
		return nil
	}
	return sendToAgent(out, os.Stdin, page, "", func(pageURL string) string {
		return fmt.Sprintf("콘솔 에러 로그 확인해줘: %s (페이지: %s)", path, pageURL)
	})
}

// parsePreviewArgs는 preview의 포트 목록 인자를 파싱한다 — 각각 1~65535
// 정수여야 하고, 비정수·범위 밖은 에러.
func parsePreviewArgs(args []string) ([]int, error) {
	var ports []int
	for _, a := range args {
		p, err := strconv.Atoi(a)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("포트가 아닌 인자 %q — 사용법: agentlayer browser preview [포트...]", a)
		}
		ports = append(ports, p)
	}
	return ports, nil
}

// browserPreview는 worktree의 dev 서버(또는 지정 포트)를 브랜치 라벨 창으로 연다.
func browserPreview(out io.Writer, args []string) error {
	ports, err := parsePreviewArgs(args)
	if err != nil {
		return err
	}
	var servers []browser.DevServer
	if len(ports) > 0 {
		for _, p := range ports {
			servers = append(servers, browser.DevServer{Port: p, Branch: fmt.Sprintf("포트 %d", p)})
		}
	} else {
		metas, err := wt.ListMetas(state.DefaultDir())
		if err != nil {
			return err
		}
		wts := make(map[string]string, len(metas))
		for _, m := range metas {
			wts[m.Path] = m.Branch
		}
		servers = browser.DevServers(browser.ExecLsof, wts)
		if len(servers) == 0 {
			fmt.Fprintln(out, "worktree에서 실행 중인 dev 서버가 없습니다 (포트를 인자로 지정 가능)")
			return nil
		}
	}
	// 열 서버가 확정된 뒤에 연결한다 — 0건 안내만 하고 끝날 때 브라우저 기동 방지.
	b, err := browser.Connect(state.DefaultDir(), config.Load().BrowserPortOrDefault())
	if err != nil {
		return err
	}
	for _, s := range servers {
		if err := browser.OpenPreview(b, s); err != nil {
			return err
		}
		fmt.Fprintf(out, "⎇%s http://localhost:%d 열림\n", s.Branch, s.Port)
	}
	return nil
}

// browserShot은 전체 페이지를 캡처해 경로를 출력하고, --send면 pane으로,
// --notify면 알림 웹훅으로 이미지를 보낸다 (둘 다 가능).
func browserShot(out io.Writer, args []string) error {
	o, err := parseShotArgs(args)
	if err != nil {
		return err
	}
	cfg := config.Load()
	if o.Notify && cfg.NotifyURL() == "" {
		return fmt.Errorf("--notify에는 config notify_webhook_url(또는 discord_webhook_url)이 필요합니다")
	}
	b, err := browser.Connect(state.DefaultDir(), cfg.BrowserPortOrDefault())
	if err != nil {
		return err
	}
	var page *rod.Page
	if o.URL != "" {
		// 새 탭은 닫지 않고 남긴다 — 캡처 결과를 사용자가 브라우저에서 확인할 수 있게.
		page, err = b.Page(proto.TargetCreateTarget{URL: o.URL})
		if err != nil {
			return err
		}
		if err := page.WaitLoad(); err != nil {
			return err
		}
	} else if page, err = browser.ActivePage(b); err != nil {
		return err
	}
	path, err := browser.Shot(page, state.DefaultDir(), time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintln(out, path)
	if o.Notify {
		info, err := page.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := discord.NewClient(cfg.NotifyURL()).PostFile(
			"브라우저 스크린샷: "+info.URL, filepath.Base(path), data); err != nil {
			return err
		}
		fmt.Fprintln(out, "→ 알림 웹훅으로 전송")
	}
	if !o.Send {
		return nil
	}
	return sendToAgent(out, os.Stdin, page, o.Agent, func(pageURL string) string {
		return fmt.Sprintf("브라우저 스크린샷 확인해줘: %s (페이지: %s)", path, pageURL)
	})
}

// browserOpen: agentlayer browser open <url> — 전용 브라우저에 탭을 열고 앞으로 가져온다.
// 터미널 링크 클릭을 실사용 브라우저 대신 여기로 보내는 진입점(iTerm2 Semantic History 등).
func browserOpen(out io.Writer, args []string) error {
	if len(args) != 1 || args[0] == "" {
		return fmt.Errorf("사용법: agentlayer browser open <url>")
	}
	b, err := browser.Connect(state.DefaultDir(), config.Load().BrowserPortOrDefault())
	if err != nil {
		return err
	}
	page, err := b.Page(proto.TargetCreateTarget{URL: args[0]})
	if err != nil {
		return err
	}
	if _, err := page.Activate(); err != nil {
		return err
	}
	fmt.Fprintln(out, "열림:", args[0])
	return nil
}

// browserCookies: agentlayer browser cookies import <도메인...>
// 실사용 크롬의 지정 도메인 쿠키만 골라 에이전트 프로필로 가져온다.
func browserCookies(out io.Writer, args []string) error {
	if len(args) == 0 || args[0] != "import" {
		return fmt.Errorf("사용법: agentlayer browser cookies import <도메인...> " +
			"(예: agentlayer browser cookies import youtube.com google.com)")
	}
	domains := args[1:]
	if len(domains) == 0 {
		return fmt.Errorf("가져올 도메인을 하나 이상 지정하세요 " +
			"(예: agentlayer browser cookies import youtube.com)")
	}
	b, err := browser.Connect(state.DefaultDir(), config.Load().BrowserPortOrDefault())
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return browser.ImportCookies(b, home, "", domains, time.Now(), out)
}

// browserMCPServe: MCP 클라이언트가 서버 명령으로 띄운다. Chrome을 보장한 뒤
// chrome-devtools-mcp로 프로세스를 갈아끼워 stdio를 그대로 넘긴다.
func browserMCPServe() error {
	cfg := config.Load()
	if _, err := browser.Connect(state.DefaultDir(), cfg.BrowserPortOrDefault()); err != nil {
		return err
	}
	npx := usage.LookupTool("npx")
	if npx == "" {
		return fmt.Errorf("npx를 찾을 수 없습니다 — Node.js 설치 필요 (brew install node)")
	}
	argv := MCPServeArgv(npx, cfg.BrowserPortOrDefault())
	return syscall.Exec(argv[0], argv, usage.ExtendedEnv()) // npx 셔뱅이 node를 PATH에서 찾는다
}

// browserAutoPreview: agentlayer browser autopreview — hook이 전이마다 백그라운드로
// 부른다. 살아 있는 에이전트 폴더 아래 새 dev 서버를 전용 브라우저에 한 번 연다.
// 브라우저는 새 서버가 있을 때만 건드린다(Connect가 Chrome을 띄우므로).
func browserAutoPreview(out io.Writer) error {
	cfg := config.Load()
	if !cfg.PreviewAutoEnabled() {
		return nil
	}
	st, err := state.NewStore(state.DefaultDir())
	if err != nil {
		return err
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	paths := map[string]string{}
	for _, a := range agents {
		if a.CWD != "" && a.State != state.StateDead {
			paths[a.CWD] = ""
		}
	}
	if len(paths) == 0 {
		return nil
	}
	port := cfg.BrowserPortOrDefault()
	scan := func(p map[string]string) []browser.DevServer {
		var out []browser.DevServer
		for _, s := range browser.DevServers(browser.ExecLsof, p) {
			if s.Port != port { // 전용 Chrome 자신 제외
				out = append(out, s)
			}
		}
		return out
	}
	var b *rod.Browser
	connect := func() *rod.Browser {
		if b == nil {
			b, _ = browser.Connect(state.DefaultDir(), port)
		}
		return b
	}
	hasTab := func(p int) bool {
		br := connect()
		if br == nil {
			return false
		}
		pages, err := br.Pages()
		if err != nil {
			return false
		}
		for _, pg := range pages {
			if info, err := pg.Info(); err == nil &&
				(strings.Contains(info.URL, fmt.Sprintf("localhost:%d", p)) || strings.Contains(info.URL, fmt.Sprintf("127.0.0.1:%d", p))) {
				return true
			}
		}
		return false
	}
	open := func(s browser.DevServer) error {
		br := connect()
		if br == nil {
			return fmt.Errorf("브라우저 연결 실패")
		}
		return browser.OpenPreview(br, s)
	}
	for _, s := range browser.AutoPreview(state.DefaultDir(), paths, scan, hasTab, open, time.Now()) {
		fmt.Fprintf(out, "🌐 http://localhost:%d 열림 (%s)\n", s.Port, s.CWD)
	}
	return nil
}

// MCPCommands는 claude·codex·gemini에 chrome-devtools-mcp를 전용 브라우저
// (고정 CDP 포트)로 붙이는 설치 명령. 실행하지 않고 출력만 — 각 CLI 설정
// 파일을 agentlayer가 건드리지 않는다. 문법은 세 CLI에서 실검증됨(2026-09-02).
func MCPCommands(port int) []string {
	url := fmt.Sprintf("--browserUrl=http://127.0.0.1:%d", port)
	return []string{
		"claude mcp add --scope user chrome-devtools -- npx chrome-devtools-mcp@latest " + url,
		"codex mcp add chrome-devtools -- npx chrome-devtools-mcp@latest " + url,
		"gemini mcp add --scope user chrome-devtools npx -- chrome-devtools-mcp@latest " + url,
	}
}

// browserMCP: agentlayer browser mcp — 에이전트별 MCP 설치 명령을 출력한다.
func browserMCP(out io.Writer) error {
	port := config.Load().BrowserPortOrDefault()
	fmt.Fprintf(out, "# 전용 브라우저 CDP: http://127.0.0.1:%d (config browser_port)\n", port)
	fmt.Fprintln(out, "# 쓰는 에이전트의 줄을 골라 실행하세요 — 등록 후 에이전트가 이 브라우저를 직접 조작·검사합니다")
	for _, l := range MCPCommands(port) {
		fmt.Fprintln(out, l)
	}
	return nil
}
