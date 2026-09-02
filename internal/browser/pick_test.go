package browser

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/ysmood/gson"
)

func headlessPage(t *testing.T, html string) *rod.Page {
	t.Helper()
	bin, ok := launcher.LookPath()
	if !ok {
		t.Skip("Chrome 없음")
	}
	u := launcher.New().Bin(bin).Headless(true).MustLaunch()
	b := rod.New().ControlURL(u).MustConnect()
	t.Cleanup(func() { b.MustClose() })
	return b.MustPage("data:text/html," + html)
}

func TestSelectorJS(t *testing.T) {
	p := headlessPage(t, `<div id="app"><ul><li>a</li><li class="x y">b</li></ul></div>`)
	el := p.MustElement("li.x")
	sel := el.MustEval(selectorJS).Str()
	if got := p.MustElement(sel).MustText(); got != "b" {
		t.Fatalf("셀렉터 %q가 원 요소를 못 찾음: %q", sel, got)
	}
}

func TestOverlaySubmitRoundTrip(t *testing.T) {
	p := headlessPage(t, `<button id="b">저장</button>`)
	got := make(chan string, 1)
	p.MustExpose("agentlayerPick", func(v gson.JSON) (interface{}, error) {
		got <- v.Str()
		return nil, nil
	})
	p.MustEval(overlayJS, 10, 10, []map[string]string{{"id": "claude-1", "label": "claude · dev"}})
	// 오버레이 입력창에 값 넣고 Enter를 시뮬레이션
	p.MustEval(`() => {
		const root = document.getElementById('agentlayer-overlay').shadowRoot;
		root.querySelector('input').value = '파랗게';
		root.querySelector('input').dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter'}));
	}`)
	r := <-got
	for _, want := range []string{"파랗게", "claude-1"} {
		if !strings.Contains(r, want) {
			t.Errorf("제출 JSON에 %q 없음: %s", want, r)
		}
	}
}

func pickAgents() []*state.Agent {
	return []*state.Agent{{
		ID: "claude-1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "dev", PaneID: "%1"},
	}}
}

func noLsof(...string) ([]byte, error) { return nil, fmt.Errorf("테스트에서 lsof 없음") }

// 탭이 닫히면 클릭 대기가 무한 블록하지 않고 에러로 반환해야 한다.
func TestRunPickTabCloseReturnsError(t *testing.T) {
	p := headlessPage(t, `<button id="b">저장</button>`)
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunPick(p, pickAgents(), noLsof, t.TempDir(),
			func(paneID, text string) error { return nil }, &bytes.Buffer{})
	}()
	time.Sleep(time.Second) // RunPick이 검사 모드 대기에 들어갈 시간
	_ = p.Close()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("탭 닫힘인데 에러가 아님")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("탭 닫힘 후에도 RunPick이 반환하지 않음 (무한 대기)")
	}
}

// Esc 취소 제출이 오면 전송 없이 정상 종료해야 한다 (클릭→오버레이 전체 흐름).
func TestRunPickCancelDoesNotSend(t *testing.T) {
	p := headlessPage(t, `<button id="b">저장</button>`)
	pt := p.MustElement("#b").MustShape().OnePointInside()
	sent := false
	var out bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunPick(p, pickAgents(), noLsof, t.TempDir(),
			func(paneID, text string) error { sent = true; return nil }, &out)
	}()
	// 검사 모드가 켜질 때까지 클릭을 반복 시도, 오버레이가 뜨면 진행
	deadline := time.Now().Add(15 * time.Second)
	for {
		if res, err := p.Eval(`() => !!document.getElementById('agentlayer-overlay')`); err == nil && res.Value.Bool() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("오버레이가 뜨지 않음 (검사 모드 클릭 미인식)")
		}
		_ = p.Mouse.MoveTo(*pt)
		_ = p.Mouse.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(100 * time.Millisecond)
	}
	p.MustEval(`() => {
		const root = document.getElementById('agentlayer-overlay').shadowRoot;
		root.querySelector('input').dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape'}));
	}`)
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrPickCancelled) {
			t.Fatalf("취소는 ErrPickCancelled로 끝나야 함(호출자가 루프를 접는다): %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("취소 제출 후에도 RunPick이 반환하지 않음")
	}
	if sent {
		t.Error("취소인데 pane 전송이 일어남")
	}
	if !strings.Contains(out.String(), "취소됨") {
		t.Errorf("출력에 취소 안내 없음: %q", out.String())
	}
}
