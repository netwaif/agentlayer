package browser

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/go-rod/rod"
)

// FX = 에이전트가 브라우저를 조작하는 동안 사람 눈에 보이는 효과(AI 커서·클릭 리플·
// 뷰포트 안쪽 테두리 글로우·입력 요소 하이라이트). 구현은 셋으로 나뉜다.
//   - fx/ 확장(콘텐츠 스크립트): 그림을 그린다. 기동 플래그 --load-extension으로 프로필에 붙는다.
//     Chrome for Testing·Chromium만 지원(브랜드 Chrome 137+는 플래그를 무시 → 효과만 없음).
//   - FxTracker: mcp-serve 프록시가 tools/call과 그 응답을 짝지어 "에이전트가 쓰는 중" 구간을 잡는다.
//     id마다 도구명을 들고 있어 End에서 어느 도구였는지 돌려준다.
//   - IsInputTool: 방패를 내리고 AI 커서를 움직이는 입력 도구인지(스펙 3절).
//   - SignalFx: 그 구간의 시작·끝을 CDP로 페이지에 알린다(<html data-agentlayer-fx>).
//     forEachPage로 탭마다 병렬로 쓰고 fxBudget 하나로 전체를 마감한다.
//   - SyncTabs: 소유권 미러(<html data-agentlayer-owner>) 쓰기와 사용자 버튼 요청
//     (<html data-agentlayer-request>) 회수를 탭마다 한 번의 Eval 왕복으로 묶는다.
// 읽기 도구(스냅샷·스크린샷)도 구간에 넣는다 — 호출 하나는 수백 ms라 조작 도구만 켜면
// 깜빡이다 끝난다(2026-09-05 실측 "거의 안 보임"). 끄는 쪽은 콘텐츠 스크립트가 2.5초 유지한다.

//go:embed fx/manifest.json fx/content.js
var fxFS embed.FS

// fxAttr는 콘텐츠 스크립트가 감시하는 <html> 속성.
const fxAttr = "data-agentlayer-fx"

// InstallFx는 embed된 확장을 <stateDir>/browser-fx에 풀고 그 경로를 돌려준다.
// 바이너리 갱신 뒤에도 최신 스크립트가 쓰이도록 매번 덮어쓴다(멱등).
func InstallFx(stateDir string) (string, error) {
	dir := filepath.Join(stateDir, "browser-fx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for _, name := range []string{"manifest.json", "content.js"} {
		b, err := fxFS.ReadFile("fx/" + name)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// FxTracker는 MCP stdio 한 줄씩 받아 조작 도구의 요청 id를 기억하고 응답과 짝짓는다.
// inflight는 id → 도구명. 겹치는 호출이 있으면 마지막 응답에서만 종료 신호를 낸다.
type FxTracker struct {
	mu       sync.Mutex
	inflight map[string]string
}

// Start는 클라이언트→서버 줄을 본다. tools/call이면 (도구 이름, true).
func (t *FxTracker) Start(line []byte) (string, bool) {
	var msg struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if json.Unmarshal(line, &msg) != nil || msg.Method != "tools/call" || len(msg.ID) == 0 {
		return "", false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.inflight == nil {
		t.inflight = map[string]string{}
	}
	t.inflight[string(msg.ID)] = msg.Params.Name
	return msg.Params.Name, true
}

// End는 서버→클라이언트 줄을 본다. 추적 중인 호출의 응답이면 (도구명, 마지막 응답인지).
// 아니면 ("", false).
func (t *FxTracker) End(line []byte) (string, bool) {
	var msg struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(line, &msg) != nil || len(msg.ID) == 0 {
		return "", false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	tool, ok := t.inflight[string(msg.ID)]
	if !ok {
		return "", false
	}
	delete(t.inflight, string(msg.ID))
	return tool, len(t.inflight) == 0
}

// inputTools — 방패를 내리고 AI 커서를 움직이는 도구(스펙 3절).
var inputTools = map[string]bool{
	"click": true, "hover": true, "drag": true, "fill": true,
	"fill_form": true, "type_text": true, "press_key": true, "upload_file": true,
}

// IsInputTool은 name이 위 입력 도구 목록에 있는지.
func IsInputTool(name string) bool { return inputTools[name] }

// fxValue는 속성 값. 타임스탬프를 붙여 같은 도구가 연달아 와도 변경으로 잡히게 한다.
func fxValue(tool string, on bool) string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	if on {
		return "on:" + tool + ":" + ts
	}
	return "off:" + ts
}

// fxBudget — 탭 전부에 신호를 쓰는 전체 마감(호출 앞에 끼어드는 지연이므로 짧게).
const fxBudget = 300 * time.Millisecond

// forEachPage — 탭마다 고루틴으로 fn을 돌리고 budget까지만 기다린다. 느린 탭은 버린다.
func forEachPage(pages []*rod.Page, budget time.Duration, fn func(*rod.Page)) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	for _, p := range pages {
		wg.Add(1)
		go func(p *rod.Page) { defer wg.Done(); fn(p) }(p)
	}
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(budget):
	}
}

// webPage — 탭과 그 URL을 묶는다. Info()를 한 번만 불러 재사용하기 위함(SyncTabs가
// target 판정에 같은 URL을 또 조회하지 않도록).
type webPage struct {
	page *rod.Page
	url  string
}

// webPages — 웹 URL인 탭만. targetURL이 있으면 그 url인 탭만.
func webPages(b *rod.Browser, targetURL string) ([]webPage, error) {
	pages, err := b.Pages()
	if err != nil {
		return nil, err
	}
	var out []webPage
	for _, p := range pages {
		info, err := p.Info()
		if err != nil || !IsWebURL(info.URL) {
			continue
		}
		if targetURL != "" && info.URL != targetURL {
			continue
		}
		out = append(out, webPage{page: p, url: info.URL})
	}
	return out, nil
}

// pagesOf — forEachPage에 넘길 []*rod.Page만 뽑는다(forEachPage 시그니처는 그대로 둔다).
func pagesOf(wps []webPage) []*rod.Page {
	if len(wps) == 0 {
		return nil
	}
	out := make([]*rod.Page, len(wps))
	for i, wp := range wps {
		out[i] = wp.page
	}
	return out
}

// urlsByPage — forEachPage 콜백 안에서 다시 p.Info()를 부르지 않도록 페이지→URL을 미리 맵으로.
// 맵은 고루틴이 뜨기 전에 다 채워지고 이후엔 읽기만 하므로 동시 접근에 안전하다.
func urlsByPage(wps []webPage) map[*rod.Page]string {
	m := make(map[*rod.Page]string, len(wps))
	for _, wp := range wps {
		m[wp.page] = wp.url
	}
	return m
}

// SignalFx는 웹 페이지에 시작/끝 신호를 쓴다. targetURL이 비어 있으면 모든 웹 탭에
// 쓴다(어느 탭을 조작하는지 프록시가 모를 때). 보이는 탭만 그리므로 전부에 써도 무해하다.
// 탭마다 병렬로 쓰고 fxBudget 하나로 전체를 마감 — 느린 탭 때문에 도구 호출이 밀리지 않는다.
// 실패는 효과 누락일 뿐이라 삼킨다.
func SignalFx(b *rod.Browser, tool string, on bool, targetURL string) error {
	wps, err := webPages(b, targetURL)
	if err != nil {
		return err
	}
	v := fxValue(tool, on)
	forEachPage(pagesOf(wps), fxBudget, func(p *rod.Page) {
		_, _ = p.Timeout(fxBudget).Eval(
			fmt.Sprintf(`(v) => document.documentElement.setAttribute(%q, v)`, fxAttr), v)
	})
	return nil
}

const (
	ownerAttr   = "data-agentlayer-owner"
	requestAttr = "data-agentlayer-request"
)

// TabRequest — 콘텐츠 스크립트가 남긴 버튼 요청.
type TabRequest struct {
	Event Event
	At    int64 // ms
}

// syncJS — 미러 쓰기와 요청 회수를 한 왕복으로. 요청은 읽은 뒤 지운다.
const syncJS = `(mirror, ownerAttr, requestAttr) => {
	const h = document.documentElement;
	const r = h.getAttribute(requestAttr) || '';
	h.setAttribute(ownerAttr, mirror);
	if (r) h.removeAttribute(requestAttr);
	return r;
}`

// reqCollector — TabRequest를 mu로 지켜 모은다. forEachPage는 예산을 넘긴 고루틴을
// 기다리지 않고 반환하므로, 그 고루틴들이 반환 이후에도 add를 계속 부를 수 있다.
// snapshot은 그 시점의 값을 새 배킹 배열로 복사해 돌려주므로, 늦게 도착하는 add가
// 이미 반환된 슬라이스의 배킹 배열을 건드리는 일이 없다(레이스 없음).
type reqCollector struct {
	mu   sync.Mutex
	reqs []TabRequest
}

func (c *reqCollector) add(r TabRequest) {
	c.mu.Lock()
	c.reqs = append(c.reqs, r)
	c.mu.Unlock()
}

func (c *reqCollector) snapshot() []TabRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]TabRequest(nil), c.reqs...)
}

// SyncTabs — 모든 웹 탭에 현재 소유권을 미러하고(작업 탭은 target=1), 버튼 요청을 회수한다.
// 탭마다 병렬로 한 번의 Eval 왕복(쓰기+읽기)만 쓰고 fxBudget으로 전체를 마감한다.
// 반환하는 error는 b.Pages() 실패(연결이 죽었을 때)뿐이다 — 탭이 하나도 없어 요청이
// 비어 있는 정상 상태(nil, nil)와 구분해야 호출자가 죽은 연결을 감지해 버릴 수 있다.
func SyncTabs(b *rod.Browser, c Control, targetURL, targetTitle string) ([]TabRequest, error) {
	wps, err := webPages(b, "")
	if err != nil {
		return nil, err
	}
	urls := urlsByPage(wps)
	var col reqCollector
	forEachPage(pagesOf(wps), fxBudget, func(p *rod.Page) {
		target := targetURL != "" && urls[p] == targetURL
		res, err := p.Timeout(fxBudget).Eval(syncJS, MirrorValue(c, target, targetTitle), ownerAttr, requestAttr)
		if err != nil {
			return
		}
		if ev, at, ok := ParseRequest(res.Value.Str()); ok {
			col.add(TabRequest{Event: ev, At: at})
		}
	})
	return col.snapshot(), nil
}

// LatestRequest — 여러 탭에서 온 요청 중 가장 최신.
func LatestRequest(reqs []TabRequest) (Event, bool) {
	if len(reqs) == 0 {
		return 0, false
	}
	best := reqs[0]
	for _, r := range reqs[1:] {
		if r.At > best.At {
			best = r
		}
	}
	return best.Event, true
}
