# 에이전트 전용 브라우저 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `agentlayer browser` 서브커맨드 5종 — 전용 프로필 Chrome 기동, 요소 찍어 수정 요청(pick), 스크린샷(shot), 콘솔 에러(errors), worktree 프리뷰(preview).

**Architecture:** 신설 `internal/browser/`가 rod(CDP)로 시스템 Chrome을 전용 프로필로 제어. 라우팅은 페이지 포트→lsof cwd→에이전트 레코드 최장일치, 전달은 tmuxx.SendText 한 줄(맥락은 파일). CLI 진입은 `internal/cli/browsercmd.go`+main.go 라우팅.

**Tech Stack:** Go 1.25, github.com/go-rod/rod(순수 Go, 자동 다운로드 금지), lsof(macOS 내장), 기존 state/tmuxx/wt 패키지.

**Spec:** `docs/superpowers/specs/2026-09-01-agent-browser-design.md`

## Global Constraints

- 주석·에러 문구·테스트 이름은 기존 코드처럼 한국어. 코드 식별자는 영어.
- rod의 브라우저 자동 다운로드 금지 — `launcher.LookPath()`로 시스템 Chrome만. 없으면 명확한 에러.
- 브라우저는 CLI 종료 후에도 살아야 함 — `Leakless(false)` 필수.
- Chrome 필요한 통합 테스트는 `launcher.LookPath()` 실패 시 `t.Skip` (tmux 통합 테스트와 같은 패턴). 테스트는 반드시 headless로.
- tmux를 부르는 테스트는 기존 규칙대로 `-f /dev/null`.
- goreleaser·brew 설정 무변경(단일 바이너리 유지). go.mod에는 rod만 추가.
- 3사 공통: 전달 경로는 tmuxx.SendText뿐 — 받는 CLI를 가리지 않는다.
- 각 태스크 완료 시 `go build ./... && go test ./...` 통과 후 커밋. 커밋 메시지 끝에 기존 Co-Authored-By 컨벤션.
- rod API 시그니처가 계획과 다르면(버전 차) `go doc github.com/go-rod/rod/...`으로 확인해 맞추되 설계 의도는 유지.

---

### Task 1: 인스턴스 상태 + 기동/attach (`internal/browser/instance.go`)

**Files:**
- Create: `internal/browser/instance.go`, `internal/browser/instance_test.go`, `internal/browser/export_test.go`
- Modify: `go.mod` (rod 추가: `go get github.com/go-rod/rod@latest`)

**Interfaces:**
- Produces: `browser.Connect(stateDir string) (*rod.Browser, error)` — 기존 인스턴스 attach, 없으면 기동(멱등). `browser.Instance{WSURL string}`, `LoadInstance(dir) (Instance, error)`, `SaveInstance(dir, Instance) error`, `RemoveInstance(dir)`.
- 상태 파일: `<stateDir>/browser.json`, 프로필: `<stateDir>/browser-profile/`.

- [ ] **Step 1: 상태 파일 라운드트립 실패 테스트 작성**

```go
// internal/browser/instance_test.go
package browser

import "testing"

func TestInstanceStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadInstance(dir); err == nil {
		t.Error("파일 없으면 에러")
	}
	if err := SaveInstance(dir, Instance{WSURL: "ws://127.0.0.1:9222/x"}); err != nil {
		t.Fatal(err)
	}
	in, err := LoadInstance(dir)
	if err != nil || in.WSURL != "ws://127.0.0.1:9222/x" {
		t.Fatalf("라운드트립: %+v %v", in, err)
	}
	RemoveInstance(dir)
	if _, err := LoadInstance(dir); err == nil {
		t.Error("Remove 후에는 에러")
	}
}
```

- [ ] **Step 2: 실패 확인** — `go test ./internal/browser/` → 컴파일 실패(미구현) 확인
- [ ] **Step 3: 최소 구현**

```go
// internal/browser/instance.go
// Package browser는 에이전트 전용 브라우저(전용 프로필 Chrome)를 CDP로
// 관리한다. 코어 관제는 이 패키지 없이도 무영향 — 소비 전용 원칙.
package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// Instance는 살아 있는 전용 브라우저의 CDP 접속점.
type Instance struct {
	WSURL string `json:"ws_url"`
}

func statePath(dir string) string { return filepath.Join(dir, "browser.json") }

func LoadInstance(dir string) (Instance, error) {
	var in Instance
	b, err := os.ReadFile(statePath(dir))
	if err != nil {
		return in, err
	}
	if err := json.Unmarshal(b, &in); err != nil {
		return in, err
	}
	if in.WSURL == "" {
		return in, fmt.Errorf("빈 인스턴스 기록")
	}
	return in, nil
}

func SaveInstance(dir string, in Instance) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(dir), b, 0o600)
}

func RemoveInstance(dir string) { _ = os.Remove(statePath(dir)) }

// launchHeadless는 테스트 전용 스위치 (export_test.go로만 접근).
var launchHeadless = false

// Connect는 기록된 인스턴스에 attach하고, 없거나 죽었으면 전용 프로필로
// 새 Chrome을 기동한다. 멱등 — 몇 번을 불러도 브라우저는 1개.
func Connect(stateDir string) (*rod.Browser, error) {
	if in, err := LoadInstance(stateDir); err == nil {
		b := rod.New().ControlURL(in.WSURL)
		if err := b.Connect(); err == nil {
			return b, nil
		}
		RemoveInstance(stateDir) // 죽은 기록 자동 정리
	}
	bin, ok := launcher.LookPath()
	if !ok {
		return nil, fmt.Errorf("Chrome을 찾을 수 없습니다 — Google Chrome 또는 Chromium 설치 필요")
	}
	ws, err := launcher.New().Bin(bin).
		UserDataDir(filepath.Join(stateDir, "browser-profile")).
		Headless(launchHeadless).
		Leakless(false). // CLI가 끝나도 브라우저는 살아야 한다
		Launch()
	if err != nil {
		return nil, fmt.Errorf("Chrome 기동 실패: %w", err)
	}
	b := rod.New().ControlURL(ws)
	if err := b.Connect(); err != nil {
		return nil, err
	}
	if err := SaveInstance(stateDir, Instance{WSURL: ws}); err != nil {
		return nil, err
	}
	return b, nil
}
```

```go
// internal/browser/export_test.go
package browser

func SetHeadlessForTest(v bool) { launchHeadless = v }
```

- [ ] **Step 4: 통과 확인** — `go test ./internal/browser/ -run TestInstanceState`
- [ ] **Step 5: Connect 멱등 통합 테스트 작성 (Chrome 가드)**

```go
func TestConnectIdempotentIntegration(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok {
		t.Skip("Chrome 없음")
	}
	SetHeadlessForTest(true)
	defer SetHeadlessForTest(false)
	dir := t.TempDir()
	b1, err := Connect(dir)
	if err != nil {
		t.Fatal(err)
	}
	in1, _ := LoadInstance(dir)
	b2, err := Connect(dir) // 두 번째는 attach여야 한다
	if err != nil {
		t.Fatal(err)
	}
	in2, _ := LoadInstance(dir)
	if in1.WSURL != in2.WSURL {
		t.Error("재기동됨 — attach가 아니다")
	}
	b2.MustClose()
	_ = b1
}
```
(import에 `"github.com/go-rod/rod/lib/launcher"` 추가)

- [ ] **Step 6: 통과 확인** — `go test ./internal/browser/ -run TestConnect -v`
- [ ] **Step 7: 커밋** — `git add go.mod go.sum internal/browser/ && git commit -m "feat(browser): 전용 브라우저 인스턴스 기동/attach (rod·전용 프로필·Leakless off)"`

---

### Task 2: 라우팅 (`internal/browser/route.go`)

**Files:**
- Create: `internal/browser/route.go`, `internal/browser/route_test.go`

**Interfaces:**
- Consumes: `state.Agent`(기존 — `CWD`, `State`, `Tmux.PaneID` 필드)
- Produces: `browser.PortCWD(port int, run RunLsof) (string, error)`, `browser.Candidates(agents []*state.Agent, pageURL string, run RunLsof) []*state.Agent`, `type RunLsof func(args ...string) ([]byte, error)`, `browser.ExecLsof` (실 실행기)

- [ ] **Step 1: 실패 테스트 작성**

```go
// internal/browser/route_test.go
package browser

import (
	"fmt"
	"testing"

	"github.com/netwaif/agentlayer/internal/state"
)

func fakeLsof(portOut, cwdOut string) RunLsof {
	return func(args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "cwd" {
				return []byte(cwdOut), nil
			}
		}
		return []byte(portOut), nil
	}
}

func TestPortCWD(t *testing.T) {
	run := fakeLsof("p8231\n", "p8231\nfcwd\nn/Users/x/proj\n")
	cwd, err := PortCWD(3000, run)
	if err != nil || cwd != "/Users/x/proj" {
		t.Fatalf("got %q %v", cwd, err)
	}
}

func TestPortCWDNoListener(t *testing.T) {
	if _, err := PortCWD(3000, fakeLsof("", "")); err == nil {
		t.Error("리스너 없으면 에러")
	}
}

func TestCandidatesLongestMatch(t *testing.T) {
	agents := []*state.Agent{
		{ID: "a", CWD: "/Users/x", State: state.StateWaiting},
		{ID: "b", CWD: "/Users/x/proj", State: state.StateWorking},
		{ID: "dead", CWD: "/Users/x/proj", State: state.StateDead},
	}
	run := fakeLsof("p1\n", "p1\nfcwd\nn/Users/x/proj/sub\n")
	got := Candidates(agents, "http://localhost:3000/page", run)
	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("최장일치 산 에이전트 1명이어야: %v", ids(got))
	}
}

func TestCandidatesFallbackAll(t *testing.T) {
	agents := []*state.Agent{
		{ID: "a", CWD: "/Users/x", State: state.StateWaiting},
		{ID: "dead", CWD: "/Users/y", State: state.StateDead},
	}
	got := Candidates(agents, "https://example.com/", nil)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("외부 사이트는 산 에이전트 전원 폴백: %v", ids(got))
	}
}

func ids(as []*state.Agent) (r []string) {
	for _, a := range as {
		r = append(r, a.ID)
	}
	return
}

var _ = fmt.Sprintf
```
(state.StateDead 등 상수명은 `internal/state/types.go` 실물 확인 후 맞출 것)

- [ ] **Step 2: 실패 확인** — `go test ./internal/browser/ -run 'TestPortCWD|TestCandidates'`
- [ ] **Step 3: 구현**

```go
// internal/browser/route.go
package browser

import (
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

	"github.com/netwaif/agentlayer/internal/state"
)

// RunLsof는 lsof 실행 주입점 (테스트 대체용).
type RunLsof func(args ...string) ([]byte, error)

// ExecLsof는 실제 lsof를 부른다.
func ExecLsof(args ...string) ([]byte, error) {
	return exec.Command("lsof", args...).Output()
}

// PortCWD는 localhost PORT를 리스닝하는 프로세스의 작업 폴더를 찾는다.
// dev 서버 → 작업 폴더 → 에이전트 라우팅의 핵심 고리.
func PortCWD(port int, run RunLsof) (string, error) {
	out, err := run("-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-Fp")
	if err != nil || len(out) == 0 {
		return "", fmt.Errorf("포트 %d 리스너 없음", port)
	}
	pid := ""
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "p") {
			pid = l[1:]
			break
		}
	}
	if pid == "" {
		return "", fmt.Errorf("포트 %d pid 파싱 실패", port)
	}
	out, err = run("-a", "-p", pid, "-d", "cwd", "-Fn")
	if err != nil {
		return "", err
	}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "n") {
			return l[1:], nil
		}
	}
	return "", fmt.Errorf("pid %s cwd 파싱 실패", pid)
}

// Candidates는 페이지 URL로 전송 대상 에이전트 후보를 좁힌다.
// localhost면 포트→cwd→최장일치, 아니면 산 에이전트 전원(오버레이에서 선택).
func Candidates(agents []*state.Agent, pageURL string, run RunLsof) []*state.Agent {
	alive := make([]*state.Agent, 0, len(agents))
	for _, a := range agents {
		if a.State != state.StateDead {
			alive = append(alive, a)
		}
	}
	u, err := url.Parse(pageURL)
	if err != nil {
		return alive
	}
	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" {
		return alive
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return alive
	}
	cwd, err := PortCWD(port, run)
	if err != nil {
		return alive
	}
	best, bestLen := []*state.Agent{}, 0
	for _, a := range alive {
		if a.CWD == "" {
			continue
		}
		p := strings.TrimSuffix(a.CWD, "/")
		if cwd != p && !strings.HasPrefix(cwd, p+"/") {
			continue
		}
		if len(p) > bestLen {
			best, bestLen = []*state.Agent{a}, len(p)
		} else if len(p) == bestLen {
			best = append(best, a)
		}
	}
	if len(best) == 0 {
		return alive
	}
	return best
}
```

- [ ] **Step 4: 통과 확인** — `go test ./internal/browser/ -run 'TestPortCWD|TestCandidates' -v`
- [ ] **Step 5: 커밋** — `git commit -m "feat(browser): 포트→cwd→에이전트 최장일치 라우팅"`

---

### Task 3: pick 산출물 저장·프롬프트 (`internal/browser/context.go`)

**Files:**
- Create: `internal/browser/context.go`, `internal/browser/context_test.go`

**Interfaces:**
- Produces: `browser.PickContext{URL, Selector, HTML, Instruction string; Shot []byte}`, `browser.SavePick(stateDir string, c PickContext, now time.Time) (mdPath, pngPath string, err error)`, `browser.PromptLine(instruction, mdPath, pngPath string) string`

- [ ] **Step 1: 실패 테스트 작성**

```go
// internal/browser/context_test.go
package browser

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestSavePickWritesFiles(t *testing.T) {
	dir := t.TempDir()
	c := PickContext{URL: "http://localhost:3000/", Selector: "div.card > button",
		HTML: "<button>저장</button>", Instruction: "파랗게", Shot: []byte{1, 2}}
	md, png, err := SavePick(dir, c, time.Date(2026, 9, 1, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(md)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"div.card > button", "<button>저장</button>", "http://localhost:3000/"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("맥락 파일에 %q 없음", want)
		}
	}
	if fi, err := os.Stat(png); err != nil || fi.Size() != 2 {
		t.Error("스크린샷 파일 저장돼야")
	}
}

func TestSavePickNoShot(t *testing.T) {
	_, png, err := SavePick(t.TempDir(), PickContext{Selector: "p"}, time.Now())
	if err != nil || png != "" {
		t.Fatalf("샷 없으면 png 경로 빈 값: %q %v", png, err)
	}
}

func TestPromptLineSingleLine(t *testing.T) {
	got := PromptLine("이 버튼\n파랗게", "/tmp/a.md", "/tmp/a.png")
	if strings.Contains(got, "\n") {
		t.Error("한 줄이어야 — 여러 줄 send-keys는 조기 제출됨")
	}
	for _, want := range []string{"이 버튼 파랗게", "/tmp/a.md", "/tmp/a.png"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q 포함해야: %q", want, got)
		}
	}
}
```

- [ ] **Step 2: 실패 확인** — `go test ./internal/browser/ -run 'TestSavePick|TestPromptLine'`
- [ ] **Step 3: 구현**

```go
// internal/browser/context.go
package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PickContext는 요소 찍기 한 건의 전달 맥락.
type PickContext struct {
	URL, Selector, HTML, Instruction string
	Shot                             []byte // 요소 영역 PNG (없으면 nil)
}

// SavePick은 맥락을 md(+png)로 저장한다. pane에는 경로만 한 줄로 가므로
// 에이전트가 읽을 실질 내용은 전부 여기 담는다.
func SavePick(stateDir string, c PickContext, now time.Time) (string, string, error) {
	dir := filepath.Join(stateDir, "picks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	ts := now.Format("20060102-150405")
	md := filepath.Join(dir, ts+".md")
	body := fmt.Sprintf(`# 브라우저 요소 수정 요청

- 지시: %s
- 페이지: %s
- 셀렉터: `+"`%s`"+`

## 요소 HTML

`+"```html\n%s\n```\n", c.Instruction, c.URL, c.Selector, c.HTML)
	if err := os.WriteFile(md, []byte(body), 0o644); err != nil {
		return "", "", err
	}
	png := ""
	if len(c.Shot) > 0 {
		png = filepath.Join(dir, ts+".png")
		if err := os.WriteFile(png, c.Shot, 0o644); err != nil {
			return "", "", err
		}
	}
	return md, png, nil
}

// PromptLine은 pane에 보낼 한 줄. 여러 줄 send-keys는 CLI 입력창에서
// 조기 제출되므로 반드시 한 줄이어야 한다.
func PromptLine(instruction, mdPath, pngPath string) string {
	one := strings.Join(strings.Fields(instruction), " ")
	s := fmt.Sprintf("브라우저 요소 수정 요청: %q — 맥락 파일을 읽고 반영해줘: %s", one, mdPath)
	if pngPath != "" {
		s += " (요소 스크린샷: " + pngPath + ")"
	}
	return s
}
```

- [ ] **Step 4: 통과 확인** 후 **Step 5: 커밋** — `git commit -m "feat(browser): pick 맥락 파일 저장·한 줄 프롬프트 조립"`

---

### Task 4: pick 파이프라인 (`internal/browser/pick.go`)

**Files:**
- Create: `internal/browser/pick.go`, `internal/browser/pick_test.go`

**Interfaces:**
- Consumes: Task 1 `Connect`(호출측이 함), Task 2 `Candidates`, Task 3 `SavePick`/`PromptLine`
- Produces: `browser.RunPick(page *rod.Page, agents []*state.Agent, lsof RunLsof, stateDir string, send func(paneID, text string) error, out io.Writer) error` — 한 번의 찍기(클릭→오버레이→전송)를 수행. 반복 루프는 CLI(Task 5)가 돈다. `browser.ActivePage(b *rod.Browser) (*rod.Page, error)`.
- 오버레이 제출 JSON: `{"text":"지시","agent":"<agent ID>","cancel":false}`

- [ ] **Step 1: 셀렉터 JS·오버레이 JS 통합 테스트 작성 (Chrome 가드, headless)**

```go
// internal/browser/pick_test.go
package browser

import (
	"testing"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
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
```
(import에 `"strings"`, `"github.com/ysmood/gson"` 추가. gson은 rod의 의존성이라 go.sum에 이미 있음)

- [ ] **Step 2: 실패 확인** — selectorJS/overlayJS 미정의로 컴파일 실패
- [ ] **Step 3: JS 상수 + RunPick 구현**

```go
// internal/browser/pick.go
package browser

import (
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

	// 1. 클릭될 노드를 기다린다 (하이라이트는 Chrome 내장)
	wait := make(chan proto.DOMBackendNodeID, 1)
	go page.EachEvent(func(e *proto.OverlayInspectNodeRequested) bool {
		wait <- e.BackendNodeID
		return true
	})()
	err := proto.OverlaySetInspectMode{
		Mode:            proto.OverlayInspectModeSearchForNode,
		HighlightConfig: &proto.OverlayHighlightConfig{ContentColor: &proto.DOMRGBA{R: 111, G: 168, B: 220, A: 0.4}},
	}.Call(page)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "브라우저에서 수정할 요소를 클릭하세요…")
	nodeID := <-wait
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
	sel := el.MustEval(selectorJS).Str()
	html, _ := el.HTML()
	if len(html) > 4000 {
		html = html[:4000] + "\n<!-- 절단됨 -->"
	}
	shot, _ := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 0)
	info := page.MustInfo()

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
	defer stop()
	box, err := el.Shape()
	x, y := 20.0, 20.0
	if err == nil && box.Box() != nil {
		x, y = box.Box().X, box.Box().Y+box.Box().Height+8
	}
	if _, err := page.Eval(overlayJS, x, y, opts); err != nil {
		return err
	}
	sub := <-done
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
```
(rod의 `page.Expose` 반환 시그니처·`el.Shape()`·`EachEvent` 핸들러 bool 반환 여부는 `go doc`으로 확인해 맞출 것 — 의도: 바인딩 노출/요소 좌표/1회 이벤트 대기)

- [ ] **Step 4: 통과 확인** — `go test ./internal/browser/ -run 'TestSelectorJS|TestOverlay' -v`
- [ ] **Step 5: 커밋** — `git commit -m "feat(browser): pick 파이프라인 — 검사 모드·shadow DOM 오버레이·라우팅 전송"`

---

### Task 5: CLI 진입 — browser·pick (`internal/cli/browsercmd.go` + main.go + help)

**Files:**
- Create: `internal/cli/browsercmd.go`, `internal/cli/browsercmd_test.go`
- Modify: `main.go` (case "browser" 라우팅), `internal/cli/helpcmd.go` (명령 목록 — helpcmd_test가 감시하므로 반드시 갱신)

**Interfaces:**
- Consumes: `browser.Connect`, `browser.ActivePage`, `browser.RunPick`, `browser.ExecLsof`, `state.NewStore(state.DefaultDir())`, `tmuxx.Tmux{}.SendText` (실 시그니처는 `internal/tmuxx/tmux.go` 확인 — pane에 텍스트+Enter를 보내는 기존 함수 사용)
- Produces: `cli.RunBrowser(out io.Writer, args []string) error` — 서브커맨드 디스패치: 빈 인자=기동, pick/shot/errors/preview. Task 6~8이 여기에 case를 추가.

- [ ] **Step 1: 디스패치 실패 테스트 작성**

```go
// internal/cli/browsercmd_test.go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunBrowserUnknownSub(t *testing.T) {
	var buf bytes.Buffer
	err := RunBrowser(&buf, []string{"없는명령"})
	if err == nil || !strings.Contains(err.Error(), "없는명령") {
		t.Fatalf("미지 서브커맨드는 에러: %v", err)
	}
}
```

- [ ] **Step 2: 실패 확인** → **Step 3: 구현**

```go
// internal/cli/browsercmd.go
// browser 서브커맨드: 에이전트 전용 브라우저(전용 프로필 Chrome) 관제.
package cli

import (
	"fmt"
	"io"

	"github.com/netwaif/agentlayer/internal/browser"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
)

func RunBrowser(out io.Writer, args []string) error {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
		args = args[1:]
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

func browserLaunch(out io.Writer) error {
	if _, err := browser.Connect(state.DefaultDir()); err != nil {
		return err
	}
	fmt.Fprintln(out, "에이전트 전용 브라우저 준비 완료 (프로필: browser-profile — 로그인 세션 유지)")
	return nil
}

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
```
main.go 라우팅(기존 switch에 추가):

```go
	case "browser":
		err = cli.RunBrowser(os.Stdout, rest)
```
helpcmd.go 목록에 추가(형식은 기존 행과 동일하게):

```
  browser [pick|shot|errors|preview]  에이전트 전용 브라우저 (기동/요소찍기/캡처/콘솔에러/wt 프리뷰)
```

- [ ] **Step 4: 통과 확인** — `go test ./internal/cli/ ./... ` (helpcmd_test 목록 검증 포함)
- [ ] **Step 5: 수동 검증** — `make install && agentlayer browser` 로 Chrome 창 확인, dev 서버 페이지 열고 `agentlayer browser pick`으로 요소 클릭→오버레이→본 세션 pane 수신 확인
- [ ] **Step 6: 커밋** — `git commit -m "feat(cli): agentlayer browser·pick 진입 — 기동/연속 요소찍기"`

---

### Task 6: shot (`browsercmd.go` + `internal/browser/shot.go`)

**Files:**
- Create: `internal/browser/shot.go`, `internal/browser/shot_test.go`
- Modify: `internal/cli/browsercmd.go` (case "shot")

**Interfaces:**
- Produces: `browser.Shot(page *rod.Page, stateDir string, now time.Time) (string, error)` — 전체 페이지 PNG를 picks/에 저장, 경로 반환.
- CLI: `agentlayer browser shot [url] [--send]` — url 있으면 새 탭에서 캡처, `--send`면 pick 라우팅으로 후보 1순위 pane에 `브라우저 스크린샷: <png> (페이지: <url>)` 한 줄 전송, 기본은 경로 stdout.

- [ ] **Step 1: 실패 테스트 작성 (Chrome 가드)**

```go
// internal/browser/shot_test.go
package browser

import (
	"os"
	"testing"
	"time"
)

func TestShotSavesPNG(t *testing.T) {
	p := headlessPage(t, `<h1>hello</h1>`)
	path, err := Shot(p, t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) < 100 {
		t.Fatalf("PNG 저장돼야: %d바이트 %v", len(b), err)
	}
}
```

- [ ] **Step 2: 실패 확인** → **Step 3: 구현**

```go
// internal/browser/shot.go
package browser

import (
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Shot은 페이지 전체 스크린샷을 picks/에 저장하고 경로를 돌려준다.
func Shot(page *rod.Page, stateDir string, now time.Time) (string, error) {
	dir := filepath.Join(stateDir, "picks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	b, err := page.Screenshot(true, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, now.Format("20060102-150405")+"-shot.png")
	return path, os.WriteFile(path, b, 0o644)
}
```
CLI case (browsercmd.go에 추가 — flag.NewFlagSet("shot")로 `--send` 파싱, url 인자는 `fs.Arg(0)`; url 있으면 `b.Page(proto.TargetCreateTarget{URL: url})` 새 탭, 없으면 ActivePage. `--send`면 `browser.Candidates`로 1순위 pane에 `fmt.Sprintf("브라우저 스크린샷: %s (페이지: %s)", path, pageURL)` 전송, 아니면 `fmt.Fprintln(out, path)`):

```go
	case "shot":
		return browserShot(out, args)
```

```go
func browserShot(out io.Writer, args []string) error {
	fs := flag.NewFlagSet("shot", flag.ContinueOnError)
	sendFlag := fs.Bool("send", false, "에이전트 pane으로 경로 전송")
	if err := fs.Parse(args); err != nil {
		return err
	}
	b, err := browser.Connect(state.DefaultDir())
	if err != nil {
		return err
	}
	var page *rod.Page
	if url := fs.Arg(0); url != "" {
		page, err = b.Page(proto.TargetCreateTarget{URL: url})
		if err != nil {
			return err
		}
		page.MustWaitLoad()
	} else if page, err = browser.ActivePage(b); err != nil {
		return err
	}
	path, err := browser.Shot(page, state.DefaultDir(), time.Now())
	if err != nil {
		return err
	}
	if !*sendFlag {
		fmt.Fprintln(out, path)
		return nil
	}
	st, err := state.NewStore(state.DefaultDir())
	if err != nil {
		return err
	}
	agents, _ := st.List()
	info := page.MustInfo()
	cands := browser.Candidates(agents, info.URL, browser.ExecLsof)
	if len(cands) == 0 {
		return fmt.Errorf("전송할 에이전트가 없습니다")
	}
	line := fmt.Sprintf("브라우저 스크린샷 확인해줘: %s (페이지: %s)", path, info.URL)
	if err := (tmuxx.Tmux{}).SendText(cands[0].Tmux.PaneID, line); err != nil {
		return err
	}
	fmt.Fprintf(out, "→ %s에게 전송\n", cands[0].ID)
	return nil
}
```
(import에 flag·time·rod·proto 추가)

- [ ] **Step 4: 통과 확인** → **Step 5: 커밋** — `git commit -m "feat(browser): shot — 전체 캡처·--send 라우팅 전달"`

---

### Task 7: errors (`internal/browser/errors.go`)

**Files:**
- Create: `internal/browser/errors.go`, `internal/browser/errors_test.go`
- Modify: `internal/cli/browsercmd.go` (case "errors")

**Interfaces:**
- Produces: `browser.CollectErrors(page *rod.Page, until <-chan struct{}) []string` — 호출 시점부터 until 신호까지 콘솔 error/warning·JS 예외를 문자열로 수집. `browser.SaveErrors(stateDir string, lines []string, now time.Time) (string, error)` — picks/<ts>-errors.txt 저장.
- CLI: `agentlayer browser errors [--send]` — "재현 후 Enter" 안내 → stdin Enter까지 수집 → 덤프(stdout+파일), `--send`면 shot과 같은 라우팅으로 한 줄 전송.

- [ ] **Step 1: 실패 테스트 작성 (Chrome 가드)**

```go
// internal/browser/errors_test.go
package browser

import (
	"strings"
	"testing"
	"time"
)

func TestCollectErrorsCapturesConsoleAndException(t *testing.T) {
	p := headlessPage(t, `<body></body>`)
	until := make(chan struct{})
	got := make(chan []string, 1)
	go func() { got <- CollectErrors(p, until) }()
	time.Sleep(300 * time.Millisecond) // 구독 안착
	p.MustEval(`() => { console.error('망함'); setTimeout(() => { throw new Error('예외다') }, 0) }`)
	time.Sleep(500 * time.Millisecond)
	close(until)
	lines := <-got
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"망함", "예외다"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%q 수집돼야: %s", want, joined)
		}
	}
}
```

- [ ] **Step 2: 실패 확인** → **Step 3: 구현**

```go
// internal/browser/errors.go
package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// CollectErrors는 until까지 페이지의 콘솔 error/warning과 JS 예외를 모은다.
// 온디맨드 — 상주 감시가 아니다.
func CollectErrors(page *rod.Page, until <-chan struct{}) []string {
	ctx, cancel := context.WithCancel(context.Background())
	p := page.Context(ctx)
	var mu sync.Mutex
	var lines []string
	add := func(s string) { mu.Lock(); lines = append(lines, s); mu.Unlock() }
	go p.EachEvent(func(e *proto.RuntimeExceptionThrown) {
		add("[exception] " + e.ExceptionDetails.Text + " " + e.ExceptionDetails.Exception.Description)
	}, func(e *proto.RuntimeConsoleAPICalled) {
		if e.Type != proto.RuntimeConsoleAPICalledTypeError && e.Type != proto.RuntimeConsoleAPICalledTypeWarning {
			return
		}
		s := "[console." + string(e.Type) + "]"
		for _, a := range e.Args {
			s += " " + a.Value.String()
		}
		add(s)
	})()
	<-until
	cancel()
	mu.Lock()
	defer mu.Unlock()
	return lines
}

// SaveErrors는 수집분을 picks/에 저장한다.
func SaveErrors(stateDir string, lines []string, now time.Time) (string, error) {
	dir := filepath.Join(stateDir, "picks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, now.Format("20060102-150405")+"-errors.txt")
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if body == "" {
		body = "(수집된 에러 없음)\n"
	}
	return path, os.WriteFile(path, []byte(body), 0o644)
}

var _ = fmt.Sprintf
```
CLI case: `browserErrors(out, args)` — Connect→ActivePage→`fmt.Fprintln(out, "수집 시작 — 버그를 재현한 뒤 Enter…")`→`until` 채널을 stdin `bufio.NewReader(os.Stdin).ReadString('\n')` 고루틴으로 닫음→CollectErrors→SaveErrors→stdout에 라인들+파일 경로. `--send`면 `콘솔 에러 로그 확인해줘: <path> (페이지: <url>)` 전송(Task 6 라우팅과 동일 패턴).

- [ ] **Step 4: 통과 확인** → **Step 5: 커밋** — `git commit -m "feat(browser): errors — 콘솔 에러·JS 예외 온디맨드 수집"`

---

### Task 8: preview (`internal/browser/preview.go`)

**Files:**
- Create: `internal/browser/preview.go`, `internal/browser/preview_test.go`
- Modify: `internal/cli/browsercmd.go` (case "preview")

**Interfaces:**
- Consumes: `wt.ListMetas(state.DefaultDir())` (기존 — `.Path`·`.Branch` 필드)
- Produces: `browser.DevServers(run RunLsof, wtPaths map[string]string) []DevServer` — 리스닝 포트 전수 스캔→pid cwd→worktree 경로 아래인 것만. `type DevServer{Port int; CWD, Branch string}`. `browser.OpenPreview(b *rod.Browser, s DevServer) error` — 새 창으로 열고 로드 후 `document.title = '⎇' + branch + ' — ' + document.title` 라벨.

- [ ] **Step 1: 실패 테스트 작성 (스캔 파싱 — 순수)**

```go
// internal/browser/preview_test.go
package browser

import "testing"

func TestDevServersFiltersByWorktree(t *testing.T) {
	run := func(args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "cwd" { // pid별 cwd 질의
				if args[2] == "11" {
					return []byte("p11\nfcwd\nn/Users/x/wts/feat-a\n"), nil
				}
				return []byte("p22\nfcwd\nn/Users/x/elsewhere\n"), nil
			}
		}
		// 전체 리스너 스캔: pid 11은 3000, pid 22는 9999
		return []byte("p11\nf3\nn*:3000\np22\nf4\nn127.0.0.1:9999\n"), nil
	}
	wts := map[string]string{"/Users/x/wts/feat-a": "feat-a"}
	got := DevServers(run, wts)
	if len(got) != 1 || got[0].Port != 3000 || got[0].Branch != "feat-a" {
		t.Fatalf("worktree 아래 dev 서버만: %+v", got)
	}
}
```

- [ ] **Step 2: 실패 확인** → **Step 3: 구현**

```go
// internal/browser/preview.go
package browser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// DevServer는 worktree 안에서 리스닝 중인 dev 서버 하나.
type DevServer struct {
	Port        int
	CWD, Branch string
}

// DevServers는 localhost 리스너를 전수 스캔해 worktree 경로 아래 것만 추린다.
// wtPaths는 경로→브랜치 맵.
func DevServers(run RunLsof, wtPaths map[string]string) []DevServer {
	out, err := run("-nP", "-iTCP", "-sTCP:LISTEN", "-Fpn")
	if err != nil {
		return nil
	}
	pid, seen := "", map[string]bool{}
	ports := map[string][]int{} // pid → ports
	for _, l := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(l, "p"):
			pid = l[1:]
		case strings.HasPrefix(l, "n"):
			addr := l[1:]
			i := strings.LastIndex(addr, ":")
			if i < 0 {
				continue
			}
			port, err := strconv.Atoi(addr[i+1:])
			key := pid + ":" + addr[i+1:]
			if err == nil && !seen[key] {
				seen[key] = true
				ports[pid] = append(ports[pid], port)
			}
		}
	}
	var res []DevServer
	for pid, ps := range ports {
		out, err := run("-a", "-p", pid, "-d", "cwd", "-Fn")
		if err != nil {
			continue
		}
		cwd := ""
		for _, l := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(l, "n") {
				cwd = l[1:]
			}
		}
		for wp, branch := range wtPaths {
			p := strings.TrimSuffix(wp, "/")
			if cwd == p || strings.HasPrefix(cwd, p+"/") {
				for _, port := range ps {
					res = append(res, DevServer{Port: port, CWD: cwd, Branch: branch})
				}
			}
		}
	}
	return res
}

// OpenPreview는 dev 서버를 새 창으로 열고 제목에 ⎇브랜치를 새긴다.
func OpenPreview(b *rod.Browser, s DevServer) error {
	page, err := b.Page(proto.TargetCreateTarget{
		URL: fmt.Sprintf("http://localhost:%d", s.Port), NewWindow: true,
	})
	if err != nil {
		return err
	}
	if err := page.WaitLoad(); err != nil {
		return err
	}
	_, err = page.Eval(`(b) => { document.title = '⎇' + b + ' — ' + document.title }`, s.Branch)
	return err
}
```
CLI case: `browserPreview(out, args)` — 인자 있으면 각 인자를 포트로 파싱해 `DevServer{Port: p, Branch: "포트 " + arg}`로 직접 열고, 없으면 `wt.ListMetas(state.DefaultDir())`로 `map[Path]Branch` 구성→`DevServers(browser.ExecLsof, wts)`→발견 0이면 "worktree에서 실행 중인 dev 서버가 없습니다 (포트를 인자로 지정 가능)" 안내→각각 OpenPreview, `⎇%s http://localhost:%d 열림` 출력.

- [ ] **Step 4: 통과 확인** → **Step 5: 커밋** — `git commit -m "feat(browser): preview — wt dev 서버 자동 탐지·라벨 창"`

---

### Task 9: 마무리 — README·전체 검증·수동 시나리오

**Files:**
- Modify: `README.md` (browser 섹션 — 명령 5종·전용 프로필 경로·lsof 라우팅 설명, preview_interval 문서화 형식 참고)

- [ ] **Step 1: README에 browser 섹션 추가** — 명령 표(5종)+프로필 위치(`~/.local/state/agentlayer/browser-profile`)+"로그인 세션은 이 프로필에 보존, 실사용 Chrome과 격리" 한 줄. 매뉴얼(txt)은 선반영 금지 — 건드리지 않는다.
- [ ] **Step 2: 전체 검증** — `go vet ./... && go test ./...` 전 패키지 통과 확인
- [ ] **Step 3: 수동 e2e 시나리오** — `make install` 후:
  1. `agentlayer browser` → Chrome 창(전용 프로필) 확인
  2. 임의 localhost dev 서버(또는 `python3 -m http.server 8000` + 아무 HTML) 열기
  3. `agentlayer browser pick` → 요소 클릭 → 오버레이 입력 → 본 세션 pane에 한 줄 수신 + picks/*.md·png 생성 확인
  4. `agentlayer browser shot` → 경로 출력·파일 확인
  5. `agentlayer browser errors` → 콘솔에서 `console.error('x')` 후 Enter → 덤프 확인
  6. (worktree 있으면) `agentlayer browser preview` 또는 `preview 8000`
- [ ] **Step 4: 커밋** — `git commit -m "docs: README browser 섹션 + 수동 e2e 확인"`

---

## Self-Review 결과 (계획 작성 후 점검)

- 스펙 커버리지: 기동/pick/shot/errors/preview·라우팅·저장 형식·에러 처리·테스트·배포 전 항목이 Task 1~9에 매핑됨. TUI 통합·다중 프로필은 스펙에서 제외된 항목이라 계획에도 없음(의도).
- rod 세부 시그니처(Expose 반환값, Shape/Box, EachEvent bool)는 버전에 따라 다를 수 있어 Global Constraints에 `go doc` 확인 지침 명시.
- 타입 일관성: RunLsof·PickContext·DevServer·send 함수형이 태스크 간 동일함 확인.
