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
//   - FxTracker: mcp-serve 프록시가 tools/call과 그 응답을 짝지어 "조작 중" 구간을 잡는다.
//   - SignalFx: 그 구간의 시작·끝을 CDP로 페이지에 알린다(<html data-agentlayer-fx>).
// 읽기 도구(스냅샷·스크린샷·목록 조회)는 구간에 넣지 않는다 — 글로우가 켜지면 "손댄다"는 뜻.

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

// readOnlyTools는 페이지를 바꾸지 않는 chrome-devtools-mcp 도구. 나머지(미래 도구 포함)는
// 조작으로 본다 — 모르는 도구가 생겼을 때 "AI가 뭔가 한다"가 표시되는 쪽이 안전.
var readOnlyTools = map[string]bool{
	"take_snapshot": true, "take_screenshot": true, "take_heapsnapshot": true,
	"list_pages": true, "list_console_messages": true, "list_network_requests": true,
	"get_console_message": true, "get_network_request": true,
	"wait_for": true, "select_page": true,
	"performance_start_trace": true, "performance_stop_trace": true, "performance_analyze_insight": true,
	"lighthouse_audit": true,
}

// IsActionTool은 이 도구 호출 동안 효과를 켜야 하는지.
func IsActionTool(name string) bool { return !readOnlyTools[name] }

// FxTracker는 MCP stdio 한 줄씩 받아 조작 도구의 요청 id를 기억하고 응답과 짝짓는다.
// 겹치는 호출이 있으면 마지막 응답에서만 종료 신호를 낸다.
type FxTracker struct {
	mu       sync.Mutex
	inflight map[string]bool
}

// Start는 클라이언트→서버 줄을 본다. 조작 도구의 tools/call이면 (도구 이름, true).
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
	if !IsActionTool(msg.Params.Name) {
		return "", false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.inflight == nil {
		t.inflight = map[string]bool{}
	}
	t.inflight[string(msg.ID)] = true
	return msg.Params.Name, true
}

// End는 서버→클라이언트 줄을 본다. 추적 중인 마지막 호출의 응답이면 true.
func (t *FxTracker) End(line []byte) bool {
	var msg struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(line, &msg) != nil || len(msg.ID) == 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.inflight[string(msg.ID)] {
		return false
	}
	delete(t.inflight, string(msg.ID))
	return len(t.inflight) == 0
}

// fxValue는 속성 값. 타임스탬프를 붙여 같은 도구가 연달아 와도 변경으로 잡히게 한다.
func fxValue(tool string, on bool) string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	if on {
		return "on:" + tool + ":" + ts
	}
	return "off:" + ts
}

// fxSignalTimeout은 페이지 하나에 신호를 쓰는 상한 — 도구 호출 앞에 끼어드는 지연이므로 짧게.
const fxSignalTimeout = 400 * time.Millisecond

// SignalFx는 열려 있는 웹 페이지 전부에 시작/끝 신호를 쓴다. 보이는 탭만 그리므로
// 전부에 써도 무해하고, 어느 탭을 조작하는지 프록시는 모른다. 실패는 효과 누락일 뿐이라 삼킨다.
func SignalFx(b *rod.Browser, tool string, on bool) error {
	pages, err := b.Pages()
	if err != nil {
		return err
	}
	v := fxValue(tool, on)
	for _, p := range pages {
		info, err := p.Info()
		if err != nil || !IsWebURL(info.URL) {
			continue
		}
		_, _ = p.Timeout(fxSignalTimeout).Eval(
			fmt.Sprintf(`(v) => document.documentElement.setAttribute(%q, v)`, fxAttr), v)
	}
	return nil
}
