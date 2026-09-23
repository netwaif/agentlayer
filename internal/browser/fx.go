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
	"github.com/go-rod/rod/lib/proto"
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

// syncJS — 미러 쓰기·fx 신호 쓰기·요청 회수를 한 왕복으로 묶는 페이지 스크립트.
// 파일로 뺀 이유: node 테스트(fx/content_test.mjs)가 같은 소스를 읽어 ack 규칙을 직접 검증한다.
//
//go:embed fx/sync.js
var syncJS string

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

// Cancel은 서버로 끝내 가지 않은 호출(게이트가 막고 프록시가 직접 응답한 줄)의 추적을
// 푼다. 안 풀면 그 id의 응답이 영영 안 와 inflight가 남고, 이후 어떤 응답도 "마지막"이
// 되지 못해 FX 종료 신호가 나가지 않는다.
func (t *FxTracker) Cancel(line []byte) {
	var msg struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(line, &msg) != nil || len(msg.ID) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.inflight, string(msg.ID))
}

// Inflight — 아직 응답을 못 받은 추적 중 호출 수(테스트가 누수를 확인한다).
func (t *FxTracker) Inflight() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.inflight)
}

// inputTools — 방패를 내리고 AI 커서를 움직이는 도구(스펙 3절).
var inputTools = map[string]bool{
	"click": true, "hover": true, "drag": true, "fill": true,
	"fill_form": true, "type_text": true, "press_key": true, "upload_file": true,
}

// IsInputTool은 name이 위 입력 도구 목록에 있는지.
func IsInputTool(name string) bool { return inputTools[name] }

// FxValue는 <html data-agentlayer-fx>에 쓸 값. 프록시가 미러 왕복에 신호를 실어 보낼 때
// (SyncTabs의 fxValue 인자) 같은 값을 만들어 쓰도록 노출한다.
func FxValue(tool string, on bool) string { return fxValue(tool, on) }

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

// forEach — 항목마다 고루틴으로 fn을 돌리고 budget까지만 기다린다. 느린 것은 버린다.
func forEach[T any](items []T, budget time.Duration, fn func(T)) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	for _, it := range items {
		wg.Add(1)
		go func(it T) { defer wg.Done(); fn(it) }(it)
	}
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(budget):
	}
}

// forEachPage — 탭마다 고루틴으로 fn을 돌리고 budget까지만 기다린다.
func forEachPage(pages []*rod.Page, budget time.Duration, fn func(*rod.Page)) {
	forEach(pages, budget, fn)
}

// webTarget — 웹 탭 하나(타깃 id와 URL). 핸들은 아직 만들지 않는다.
type webTarget struct {
	id  proto.TargetTargetID
	url string
}

// webTargets — 웹 URL인 page 타깃만. targetURL이 있으면 그 url인 것만.
//
// 호출당 오버헤드(최종 리뷰 IMPORTANT 3·4): 예전엔 b.Pages() 뒤 탭마다 p.Info()를
// 직렬로 불러 CDP 왕복이 탭 수만큼 늘었다. Target.getTargets 한 번이면 타입·URL이 다
// 실려 오므로 왕복은 하나면 된다. 그 하나도 b.Timeout(fxBudget)으로 묶어, 굳은
// 브라우저에서 rod 기본(무제한) 컨텍스트로 게이트가 눌러앉지 않게 한다.
//
// 페이지 핸들은 여기서 만들지 않는다 — rod의 PageFromTarget은 페이지 컨텍스트를
// "그 시점 브라우저 컨텍스트"에서 파생시켜 캐시에 넣으므로, 타임아웃 클론으로 만들면
// 그 페이지가 fxBudget 뒤 영구히 죽은 채로 캐시에 남는다. 핸들 생성은 원본 b로 하되
// forEach의 예산 안(고루틴)에서 한다.
func webTargets(b *rod.Browser, targetURL string) ([]webTarget, error) {
	res, err := proto.TargetGetTargets{}.Call(b.Timeout(fxBudget))
	if err != nil {
		return nil, err
	}
	var out []webTarget
	for _, ti := range res.TargetInfos {
		if ti.Type != "page" || !IsWebURL(ti.URL) {
			continue
		}
		if targetURL != "" && ti.URL != targetURL {
			continue
		}
		out = append(out, webTarget{id: ti.TargetID, url: ti.URL})
	}
	return out, nil
}

// stickyTarget — 작업 탭을 CDP 타깃 id로 기억한다.
//
// 프록시는 작업 탭을 PageMap의 url(마지막 "## Pages" 목록)로만 안다. 그런데 클릭으로
// 페이지가 이동하거나 SPA가 pushState로 주소를 바꾸면 그 탭의 실제 url은 바뀌는데
// PageMap은 다음 탭 목록 응답까지 옛 url을 들고 있다. 그동안 url 일치가 실패해
// 정작 조작 중인 탭에 "AI가 다른 탭에서 작업 중" 띠가 뜨고 방패·커서·알약은 빠졌다
// (2026-09-23 위키백과 검색 클릭·CGV 예매 SPA에서 실측). 그래서 url이 한 번 일치한
// 타깃의 id를 기억해 두고, url 일치가 없으면 그 타깃이 아직 살아 있는 한 그대로
// 작업 탭으로 본다. 에이전트가 다른 탭으로 옮기면 새 목록의 url이 그 탭과 일치해
// 기억이 갱신된다.
type stickyTarget struct {
	mu sync.Mutex
	id proto.TargetTargetID
}

var sticky stickyTarget

// ResetStickyTarget — 테스트용. 기억한 작업 탭을 지운다.
func ResetStickyTarget() { sticky.mu.Lock(); sticky.id = ""; sticky.mu.Unlock() }

// resolveTarget — ts 중 작업 탭의 타깃 id. targetURL이 비었으면("", false): 미상.
// 규칙: ① url이 일치하는 탭(여럿이면 기억한 id 우선, 없으면 첫 번째) → 기억 갱신
// ② 일치가 없으면 기억한 id가 ts에 살아 있을 때 그것 ③ 둘 다 아니면 없음.
func resolveTarget(ts []webTarget, targetURL string) (proto.TargetTargetID, bool) {
	if targetURL == "" {
		return "", false
	}
	sticky.mu.Lock()
	defer sticky.mu.Unlock()
	var first proto.TargetTargetID
	alive := false
	for _, t := range ts {
		if t.id == sticky.id {
			alive = true
			if t.url == targetURL {
				return t.id, true
			}
		}
		if t.url == targetURL && first == "" {
			first = t.id
		}
	}
	if first != "" {
		sticky.id = first
		return first, true
	}
	if alive {
		return sticky.id, true
	}
	return "", false
}

// evalTabs — 탭마다 병렬로 핸들을 얻어 fn을 돌린다. 전체 마감은 fxBudget 하나,
// 각 Eval도 fxBudget으로 묶는다(굳은 탭이 예산을 넘겨도 호출자는 제때 돌아온다).
func evalTabs(b *rod.Browser, ts []webTarget, fn func(p *rod.Page, t webTarget)) {
	forEach(ts, fxBudget, func(t webTarget) {
		p, err := b.PageFromTarget(t.id) // 대개 캐시 적중 — 새 탭일 때만 attach 왕복
		if err != nil || p == nil {
			return
		}
		fn(p, t)
	})
}

// SignalFx는 웹 페이지에 시작/끝 신호를 쓴다. targetURL이 비어 있으면 모든 웹 탭에
// 쓴다(어느 탭을 조작하는지 프록시가 모를 때). 보이는 탭만 그리므로 전부에 써도 무해하다.
// 탭마다 병렬로 쓰고 fxBudget 하나로 전체를 마감 — 느린 탭 때문에 도구 호출이 밀리지 않는다.
// 실패는 효과 누락일 뿐이라 삼킨다.
func SignalFx(b *rod.Browser, tool string, on bool, targetURL string) error {
	ts, err := webTargets(b, "")
	if err != nil {
		return err
	}
	// 작업 탭이 특정되면 그 탭에만 — url 일치가 깨진 동안(페이지 이동 직후)에도
	// 기억한 타깃을 쓴다(stickyTarget 주석). 미상이면 모든 탭.
	if id, ok := resolveTarget(ts, targetURL); ok {
		only := ts[:0:0]
		for _, t := range ts {
			if t.id == id {
				only = append(only, t)
			}
		}
		ts = only
	}
	v := fxValue(tool, on)
	evalTabs(b, ts, func(p *rod.Page, _ webTarget) {
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
// fxOn이 비어 있지 않으면 같은 Eval에서 <html data-agentlayer-fx>까지 써서, 도구 호출
// 하나에 필요한 CDP 왕복을 탭마다 한 번으로 줄인다(최종 리뷰 IMPORTANT 3). fx 신호는
// SignalFx와 같은 규칙으로 작업 탭에만 쓴다 — targetURL이 미상이면 모든 탭.
// 탭마다 병렬로 왕복하고 fxBudget으로 전체를 마감한다.
// 반환하는 error는 탭 목록 조회 실패(연결이 죽었을 때)뿐이다 — 탭이 하나도 없어 요청이
// 비어 있는 정상 상태(nil, nil)와 구분해야 호출자가 죽은 연결을 감지해 버릴 수 있다.
func SyncTabs(b *rod.Browser, c Control, targetURL, targetTitle, fxOn string) ([]TabRequest, error) {
	ts, err := webTargets(b, "")
	if err != nil {
		return nil, err
	}
	var col reqCollector
	// 작업 탭 판정은 url 일치가 아니라 resolveTarget(타깃 id, 이동 뒤에도 유지)으로.
	targetID, hasTarget := resolveTarget(ts, targetURL)
	evalTabs(b, ts, func(p *rod.Page, t webTarget) {
		target := hasTarget && t.id == targetID
		fx := ""
		if fxOn != "" && (target || !hasTarget) {
			fx = fxOn
		}
		res, err := p.Timeout(fxBudget).Eval(syncJS,
			MirrorValue(c, target, targetTitle), ownerAttr, requestAttr, fxAttr, fx, c.LastRequestMs)
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
	ev, _, ok := LatestRequestAfter(reqs, 0)
	return ev, ok
}

// LatestRequestAfter — ack(이미 반영한 요청의 ms)보다 새 요청 중 가장 최신과 그 ms.
// 같은 클릭이 탭 여러 개에서(또는 느린 탭 때문에 여러 왕복에 걸쳐) 돌아와도 ack보다
// 오래된 것은 버려 두 번 반영되지 않는다(최종 리뷰 IMPORTANT 2).
func LatestRequestAfter(reqs []TabRequest, ack int64) (Event, int64, bool) {
	var best TabRequest
	found := false
	for _, r := range reqs {
		if r.At <= ack {
			continue
		}
		if !found || r.At > best.At {
			best, found = r, true
		}
	}
	if !found {
		return 0, 0, false
	}
	return best.Event, best.At, true
}
