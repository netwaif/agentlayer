package browser

import (
	"strings"
	"testing"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
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
