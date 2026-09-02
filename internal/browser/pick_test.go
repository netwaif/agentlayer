package browser

import (
	"bytes"
	"errors"
	"fmt"
	"image/png"
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

// 요소 스크린샷은 스크롤된 위치·DPR과 무관하게 그 요소를 담아야 한다.
// (rod el.Screenshot은 전체 캡처를 CSS 좌표로 잘라 DPR 2에서 엉뚱한 영역이 나왔다.)
func TestElementShotCapturesScrolledElement(t *testing.T) {
	p := headlessPage(t, `<div style="height:1500px"></div>
		<div id="t" style="width:200px;height:80px;background:red"></div>
		<div style="height:1500px"></div>`)
	// 레티나 재현 — DPR 1에서는 rod 크롭도 우연히 맞아 버그가 안 드러난다
	if err := (proto.EmulationSetDeviceMetricsOverride{Width: 800, Height: 600, DeviceScaleFactor: 2}).Call(p); err != nil {
		t.Fatal(err)
	}
	el, err := p.Element("#t")
	if err != nil {
		t.Fatal(err)
	}
	bin := ElementShot(p, el)
	if len(bin) == 0 {
		t.Fatal("스크린샷 없음")
	}
	img, err := png.Decode(bytes.NewReader(bin))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() < 200 || b.Dy() < 80 {
		t.Fatalf("크기 %dx%d — 요소(200x80)보다 작다", b.Dx(), b.Dy())
	}
	r, g, bl, _ := img.At(b.Dx()/2, b.Dy()/2).RGBA()
	if r>>8 < 200 || g>>8 > 50 || bl>>8 > 50 {
		t.Errorf("가운데 픽셀이 빨강이 아님: %d %d %d — 다른 영역을 잘랐다", r>>8, g>>8, bl>>8)
	}
}

// --once(send nil): 제출 한 줄이 pane이 아니라 out에 그대로 나와야 한다 —
// 에이전트가 "브라우저에서 지목할게"를 받아 자기 Bash로 실행하는 경로.
func TestRunPickOnceWritesLineToOut(t *testing.T) {
	p := headlessPage(t, `<button id="b">저장</button>`)
	pt := p.MustElement("#b").MustShape().OnePointInside()
	var out bytes.Buffer
	dir := t.TempDir()
	errCh := make(chan error, 1)
	go func() { errCh <- RunPick(p, SelfAgents(), noLsof, dir, nil, &out) }()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if res, err := p.Eval(`() => !!document.getElementById('agentlayer-overlay')`); err == nil && res.Value.Bool() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("오버레이가 뜨지 않음")
		}
		_ = p.Mouse.MoveTo(*pt)
		_ = p.Mouse.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(100 * time.Millisecond)
	}
	p.MustEval(`() => {
		const root = document.getElementById('agentlayer-overlay').shadowRoot;
		root.querySelector('input').value = '파랗게';
		root.querySelector('input').dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter'}));
	}`)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("once 모드 실패: %v\n%s", err, out.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("제출 후에도 RunPick이 반환하지 않음")
	}
	got := out.String()
	if !strings.Contains(got, "브라우저 요소 수정 요청: \"파랗게\"") || !strings.Contains(got, dir) {
		t.Errorf("요청 한 줄(지시+md 경로)이 out에 있어야 함: %q", got)
	}
	if strings.Contains(got, "에게 전송") {
		t.Errorf("once 모드는 pane 전송 문구가 없어야 함: %q", got)
	}
}

func TestIsWebURL(t *testing.T) {
	for u, want := range map[string]bool{
		"http://localhost:8100/": true, "https://x.com/": true, "data:text/html,<b>": true,
		"chrome://newtab/": false, "about:blank": false, "devtools://devtools/": false, "": false,
	} {
		if IsWebURL(u) != want {
			t.Errorf("IsWebURL(%q) = %v, want %v", u, !want, want)
		}
	}
}

// 맨 앞(최신) 탭이 chrome://newtab·about:blank여도 웹 페이지 탭을 골라야 한다.
func TestActivePageSkipsInternalTabs(t *testing.T) {
	p := headlessPage(t, `<h1>대상</h1>`)
	b := p.Browser()
	blank := b.MustPage("about:blank") // 더 최신 → 목록 맨 앞
	defer blank.MustClose()
	got, err := ActivePage(b)
	if err != nil {
		t.Fatal(err)
	}
	info := got.MustInfo()
	if !strings.HasPrefix(info.URL, "data:text/html") {
		t.Errorf("웹 탭을 골라야 하는데: %s", info.URL)
	}
}
