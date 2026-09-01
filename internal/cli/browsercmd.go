// browser 서브커맨드: 에이전트 전용 브라우저(전용 프로필 Chrome) 관제.
package cli

import (
	"fmt"
	"io"

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
