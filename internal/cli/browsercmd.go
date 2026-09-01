// browser 서브커맨드: 에이전트 전용 브라우저(전용 프로필 Chrome) 관제.
package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/netwaif/agentlayer/internal/browser"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
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
		return browserPick(out)
	case "shot":
		return browserShot(out, args)
	case "errors":
		return browserErrors(out, args)
	case "preview":
		return browserPreview(out, args)
	case "cookies":
		return browserCookies(out, args)
	default:
		return fmt.Errorf("모르는 browser 서브커맨드 %q — 'agentlayer help' 참고", sub)
	}
}

// browserLaunch는 전용 브라우저를 기동(또는 기존 인스턴스에 attach)한다.
func browserLaunch(out io.Writer) error {
	if _, err := browser.Connect(state.DefaultDir()); err != nil {
		return err
	}
	fmt.Fprintln(out, "에이전트 전용 브라우저 준비 완료 (프로필: browser-profile — 로그인 세션 유지)")
	return nil
}

// browserPick은 활성 탭에서 요소 지목 사이클을 연속으로 돈다.
// RunPick이 탭 닫힘·페이지 이탈 시 에러를 반환하므로 무한 블록은 없다.
func browserPick(out io.Writer) error {
	st, err := state.NewStore(state.DefaultDir())
	if err != nil {
		return err
	}
	b, err := browser.Connect(state.DefaultDir())
	if err != nil {
		return err
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	if len(agents) == 0 {
		return fmt.Errorf("등록된 에이전트가 없습니다 (agentlayer status 확인)")
	}
	tm := tmuxx.Tmux{}
	send := func(paneID, text string) error { return tm.SendText(paneID, text) }
	for { // 연속 지목 — Ctrl-C로 종료
		page, err := browser.ActivePage(b)
		if err != nil {
			return err
		}
		if err := browser.RunPick(page, agents, browser.ExecLsof, state.DefaultDir(), send, out); err != nil {
			return err
		}
	}
}

// parseShotArgs는 shot의 url·--send를 인자 위치와 무관하게 파싱한다.
// Go flag는 첫 비플래그 인자에서 멈추므로 `shot <url> --send`가 --send를
// 조용히 삼키지 않게, 위치 인자를 걷어내며 끝까지 재파싱한다.
func parseShotArgs(args []string) (url string, send bool, err error) {
	fs := flag.NewFlagSet("shot", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // usage 자동 출력 억제 — 반환 에러로 충분
	sendFlag := fs.Bool("send", false, "에이전트 pane으로 경로 전송")
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return "", false, err
		}
		if fs.NArg() == 0 {
			break
		}
		rest = append(rest, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(rest) > 1 {
		return "", false, fmt.Errorf("잉여 인자 %q — 사용법: agentlayer browser shot [url] [--send]", rest[1:])
	}
	if len(rest) == 1 {
		url = rest[0]
	}
	return url, *sendFlag, nil
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
func sendToAgent(out io.Writer, in io.Reader, page *rod.Page, makeLine func(pageURL string) string) error {
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
	cands := browser.Candidates(agents, info.URL, browser.ExecLsof)
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
	b, err := browser.Connect(state.DefaultDir())
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
	return sendToAgent(out, os.Stdin, page, func(pageURL string) string {
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
	b, err := browser.Connect(state.DefaultDir())
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

// browserShot은 전체 페이지를 캡처해 경로를 출력하거나(--send면) pane으로 보낸다.
func browserShot(out io.Writer, args []string) error {
	url, send, err := parseShotArgs(args)
	if err != nil {
		return err
	}
	b, err := browser.Connect(state.DefaultDir())
	if err != nil {
		return err
	}
	var page *rod.Page
	if url != "" {
		// 새 탭은 닫지 않고 남긴다 — 캡처 결과를 사용자가 브라우저에서 확인할 수 있게.
		page, err = b.Page(proto.TargetCreateTarget{URL: url})
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
	if !send {
		fmt.Fprintln(out, path)
		return nil
	}
	return sendToAgent(out, os.Stdin, page, func(pageURL string) string {
		return fmt.Sprintf("브라우저 스크린샷 확인해줘: %s (페이지: %s)", path, pageURL)
	})
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
	b, err := browser.Connect(state.DefaultDir())
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return browser.ImportCookies(b, home, "", domains, time.Now(), out)
}
