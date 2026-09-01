package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/ysmood/gson"
)

// selectorJS는 요소의 안정적 CSS 셀렉터를 만든다 (id 만나면 거기서 절단).
const selectorJS = `() => {
	const seg = (e) => {
		if (e.id) return '#' + CSS.escape(e.id);
		let s = e.tagName.toLowerCase();
		if (e.classList.length) s += '.' + [...e.classList].slice(0, 2).map(CSS.escape).join('.');
		const p = e.parentElement;
		if (p) {
			const same = [...p.children].filter(c => c.tagName === e.tagName);
			if (same.length > 1) s += ':nth-of-type(' + (same.indexOf(e) + 1) + ')';
		}
		return s;
	};
	const parts = [];
	let e = this;
	while (e && e.tagName !== 'HTML') { parts.unshift(seg(e)); if (e.id) break; e = e.parentElement; }
	return parts.join(' > ');
}`

// overlayJS는 (x, y, agents)를 받아 shadow DOM 입력 오버레이를 띄운다.
// Enter → window.agentlayerPick(JSON), Esc → cancel. 페이지 CSS와 격리.
const overlayJS = `(x, y, agents) => {
	const old = document.getElementById('agentlayer-overlay');
	if (old) old.remove();
	const host = document.createElement('div');
	host.id = 'agentlayer-overlay';
	host.style.cssText = 'position:fixed;z-index:2147483647;left:' +
		Math.min(x, innerWidth - 340) + 'px;top:' + Math.min(y, innerHeight - 90) + 'px;';
	const root = host.attachShadow({mode: 'open'});
	root.innerHTML = ` + "`" + `
		<style>
			.box{background:#1c1c1e;color:#eee;border:1px solid #555;border-radius:8px;
				padding:10px;font:13px -apple-system,sans-serif;width:320px;
				box-shadow:0 4px 16px rgba(0,0,0,.5)}
			input,select{width:100%;box-sizing:border-box;background:#2c2c2e;color:#eee;
				border:1px solid #444;border-radius:5px;padding:6px;font:inherit;margin-top:6px}
			.hint{color:#888;font-size:11px;margin-top:6px}
		</style>
		<div class="box">
			<div>이 요소, 어떻게 고칠까요?</div>
			<input placeholder="지시 입력 후 Enter (Esc 취소)">
			<select></select>
			<div class="hint">전송 대상 에이전트</div>
		</div>` + "`" + `;
	const input = root.querySelector('input'), sel = root.querySelector('select');
	for (const a of agents) {
		const o = document.createElement('option');
		o.value = a.id; o.textContent = a.label;
		sel.appendChild(o);
	}
	if (agents.length <= 1) sel.style.display = root.querySelector('.hint').style.display = 'none';
	const submit = (cancel) => {
		window.agentlayerPick(JSON.stringify({text: input.value, agent: sel.value, cancel}));
		host.remove();
	};
	input.addEventListener('keydown', (e) => {
		if (e.key === 'Enter') submit(false);
		if (e.key === 'Escape') submit(true);
		e.stopPropagation();
	});
	document.documentElement.appendChild(host);
	input.focus();
}`

// ActivePage는 전면 탭을 고른다 (CDP 타깃 목록의 첫 page).
func ActivePage(b *rod.Browser) (*rod.Page, error) {
	pages, err := b.Pages()
	if err != nil || len(pages) == 0 {
		return nil, fmt.Errorf("열린 탭이 없습니다 — 브라우저에서 대상 페이지를 먼저 여세요")
	}
	return pages[0], nil
}

type pickSubmit struct {
	Text   string `json:"text"`
	Agent  string `json:"agent"`
	Cancel bool   `json:"cancel"`
}

// RunPick은 검사 모드 → 클릭 대기 → 오버레이 입력 → 저장·전송 한 사이클.
func RunPick(page *rod.Page, agents []*state.Agent, lsof RunLsof, stateDir string,
	send func(paneID, text string) error, out io.Writer) error {

	// 1. 클릭될 노드를 기다린다 (하이라이트는 Chrome 내장).
	// EachEvent는 호출 시점에 구독하므로 반드시 setInspectMode 전에 건다.
	// 콜백은 wait() 호출 스택에서 동기 실행되므로 채널 없이 지역 변수로 받는다.
	// 탭 닫힘(Inspector.detached)도 대기를 풀어 무한 블록을 막는다.
	evCtx, cancelEv := context.WithCancel(page.GetContext())
	defer cancelEv()
	var nodeID proto.DOMBackendNodeID
	picked := false
	wait := page.Context(evCtx).EachEvent(
		func(e *proto.OverlayInspectNodeRequested) bool {
			nodeID, picked = e.BackendNodeID, true
			return true
		},
		func(e *proto.InspectorDetached) bool { return true },
	)
	// 에러 경로에서도 구독을 정리한다 — cancel로 이벤트 채널을 닫으면
	// wait()가 즉시 반환한다 (RunPick은 루프에서 반복 호출되므로 누수 금지).
	abort := func(err error) error { cancelEv(); wait(); return err }
	// setInspectMode는 DOM 에이전트를, 이벤트 발화는 Overlay 활성화를 요구한다
	// (rod EachEvent의 자동 enable만으로는 headless에서 이벤트가 오지 않음 — 실측).
	if err := (proto.DOMEnable{}).Call(page); err != nil {
		return abort(err)
	}
	if err := (proto.OverlayEnable{}).Call(page); err != nil {
		return abort(err)
	}
	if err := (proto.InspectorEnable{}).Call(page); err != nil {
		return abort(err)
	}
	alpha := 0.4
	err := proto.OverlaySetInspectMode{
		Mode:            proto.OverlayInspectModeSearchForNode,
		HighlightConfig: &proto.OverlayHighlightConfig{ContentColor: &proto.DOMRGBA{R: 111, G: 168, B: 220, A: &alpha}},
	}.Call(page)
	if err != nil {
		return abort(err)
	}
	fmt.Fprintln(out, "브라우저에서 수정할 요소를 클릭하세요…")
	wait()
	cancelEv()
	if !picked {
		return fmt.Errorf("클릭을 받기 전에 페이지가 닫혔습니다 — 탭을 다시 열고 시도하세요")
	}
	_ = proto.OverlaySetInspectMode{Mode: proto.OverlayInspectModeNone}.Call(page)

	// 2. 노드 → 요소 → 맥락 추출
	obj, err := proto.DOMResolveNode{BackendNodeID: nodeID}.Call(page)
	if err != nil {
		return err
	}
	el, err := page.ElementFromObject(obj.Object)
	if err != nil {
		return err
	}
	selObj, err := el.Eval(selectorJS)
	if err != nil {
		return fmt.Errorf("셀렉터 계산 실패 (요소가 사라졌을 수 있음): %w", err)
	}
	sel := selObj.Value.Str()
	html, _ := el.HTML()
	if len(html) > 4000 {
		html = html[:4000] + "\n<!-- 절단됨 -->"
	}
	shot, _ := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 0)
	info, err := page.Info()
	if err != nil {
		return err
	}

	// 3. 라우팅 후보 산출 → 오버레이 입력
	cands := Candidates(agents, info.URL, lsof)
	if len(cands) == 0 {
		return fmt.Errorf("전송할 에이전트가 없습니다 (agentlayer status 확인)")
	}
	opts := make([]map[string]string, 0, len(cands))
	for _, a := range cands {
		opts = append(opts, map[string]string{"id": a.ID, "label": a.Kind + " · " + a.Tmux.Session})
	}
	done := make(chan pickSubmit, 1)
	stop, err := page.Expose("agentlayerPick", func(v gson.JSON) (interface{}, error) {
		var s pickSubmit
		_ = json.Unmarshal([]byte(v.Str()), &s)
		done <- s
		return nil, nil
	})
	if err != nil {
		return err
	}
	defer func() { _ = stop() }()
	// 오버레이가 뜬 뒤 페이지가 이동/닫히면 제출은 영원히 안 온다 —
	// 메인 프레임 이동·탭 닫힘을 감지해 대기를 끊는다.
	navCtx, cancelNav := context.WithCancel(page.GetContext())
	defer cancelNav()
	navWait := page.Context(navCtx).EachEvent(
		func(e *proto.PageFrameNavigated) bool { return e.Frame.ParentID == "" },
		func(e *proto.InspectorDetached) bool { return true },
	)
	gone := make(chan struct{})
	go func() { navWait(); close(gone) }()
	shape, err := el.Shape()
	x, y := 20.0, 20.0
	if err == nil && shape.Box() != nil {
		box := shape.Box()
		x, y = box.X, box.Y+box.Height+8
	}
	if _, err := page.Eval(overlayJS, x, y, opts); err != nil {
		return err
	}
	var sub pickSubmit
	select {
	case sub = <-done:
	case <-gone:
		select {
		case sub = <-done: // 제출 직후 이동한 경합 — 제출을 우선
		default:
			return fmt.Errorf("오버레이 입력 전에 페이지가 이동하거나 닫혔습니다 — 다시 시도하세요")
		}
	}
	if sub.Cancel || sub.Text == "" {
		fmt.Fprintln(out, "취소됨")
		return nil
	}

	// 4. 저장 + 대상 pane으로 한 줄 전송
	target := cands[0]
	for _, a := range cands {
		if a.ID == sub.Agent {
			target = a
		}
	}
	md, png, err := SavePick(stateDir, PickContext{
		URL: info.URL, Selector: sel, HTML: html, Instruction: sub.Text, Shot: shot,
	}, time.Now())
	if err != nil {
		return err
	}
	line := PromptLine(sub.Text, md, png)
	if err := send(target.Tmux.PaneID, line); err != nil {
		return err
	}
	fmt.Fprintf(out, "→ %s(%s)에게 전송: %s\n", target.ID, target.Kind, line)
	return nil
}
