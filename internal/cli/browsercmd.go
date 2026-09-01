// browser 서브커맨드: 에이전트 전용 브라우저(전용 프로필 Chrome) 관제.
package cli

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/netwaif/agentlayer/internal/browser"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
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
	line := fmt.Sprintf("브라우저 스크린샷 확인해줘: %s (페이지: %s)", path, info.URL)
	if err := (tmuxx.Tmux{}).SendText(cands[0].Tmux.PaneID, line); err != nil {
		return err
	}
	fmt.Fprintf(out, "→ %s에게 전송\n", cands[0].ID)
	return nil
}
