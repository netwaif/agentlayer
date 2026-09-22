# 에이전트 브라우저 제어권·오버레이·행 감시·호출 효율 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 에이전트 브라우저에 소유권(agent/user/idle) 상태 기계와 제어권 버튼을 넣고, 오버레이를 작업 탭에만 ego-lite 수준으로 그리며, 굳은 브라우저를 감지해 자동 재시작하고, 호출당 오버헤드·토큰 낭비를 줄인다.

**Architecture:** 정본은 `<stateDir>/browser-control.json` 파일 하나(프록시가 여럿이라 파일). mcp-serve 프록시(`internal/cli/browsercmd.go`)가 tools/call마다 게이트를 지나며 파일을 갱신하고, 작업 탭(pageId→url 표)에만 CDP로 미러 속성을 쓴다. 확장 콘텐츠 스크립트(`internal/browser/fx/content.js`)가 미러 속성을 읽어 방패·디밍·커서·알약을 그리고, 버튼 클릭은 요청 속성에 남겨 프록시가 회수한다. 행 감시는 훅 경로(`browser autopreview`)에 붙는다.

**Tech Stack:** Go 1.25(`go-rod/rod` CDP), MV3 확장 콘텐츠 스크립트(순수 JS), chrome-devtools MCP(stdio JSON-RPC 프록시), macOS `osascript`·`sample`.

**Spec:** `docs/superpowers/specs/2026-09-22-agent-browser-control-design.md`

## Global Constraints

- 파일 쓰기는 임시 파일 → rename 원자 쓰기, 권한 0600(`board.RememberRoot` 방식).
- 호출당 프록시 추가 지연 목표 50ms 이하. 탭 병렬 Eval 전체 마감 300ms.
- 소유권 만료 20초. 게이트 대기 상한 기본 120초(설정 `browser_control_wait_seconds`). 대기 중 폴링 0.5초.
- 입력 도구 목록(방패 내림·커서 이동): `click hover drag fill fill_form type_text press_key upload_file`.
- 팔레트: 테라코타 `#d97757`, 크림 `#faf9f5`, 잉크 `#1f1e1d`.
- 미러 속성 `data-agentlayer-owner`, 요청 속성 `data-agentlayer-request`, 기존 효과 속성 `data-agentlayer-fx`.
- 미러 값의 label·title은 `:`를 `∶`(U+2236)로 바꿔 쓴다.
- 테스트는 패키지별로 실행한다(`go test ./...` 전체는 멈춤 — 메모리 go-test-full-suite-hangs). rod 실브라우저는 테스트에서 띄우지 않는다.
- 커밋 메시지는 한국어, `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` 줄 포함.

---

## 파일 구조

| 파일 | 책임 |
|---|---|
| `internal/browser/control.go` (신규) | 소유권 상태 구조체·전이 함수·파일 저장/읽기·만료·미러 값 생성·요청 파싱 |
| `internal/browser/pagemap.go` (신규) | chrome-devtools MCP `## Pages` 목록 파싱, pageId→url 표, 호출 줄에서 pageId 추출 |
| `internal/browser/fx.go` (수정) | `FxTracker`가 id별 도구명 보관, `InputTools`, 병렬 `SignalFx`, `SyncTabs`(미러 쓰기+요청 회수 한 왕복) |
| `internal/browser/trim.go` (신규) | 응답의 자동 스냅샷 잘라내기 |
| `internal/browser/front.go` (신규) | macOS 앞 앱 기억·복원, `new_page` background 채우기 |
| `internal/browser/hangwatch.go` (신규) | 행 판정 상태 파일, 연속 실패 판정, 채집·재시작 순서 |
| `internal/browser/instance.go` (수정) | 기동 전후 앞 앱 복원, `--disable-hang-monitor` 제거 |
| `internal/browser/fx/content.js` (수정) | 오버레이: 작업 탭/타 탭 분기, 방패, 디밍, 커서 라벨, 알약·버튼, 요청 속성 쓰기, 자체 만료 |
| `internal/browser/fx/content_test.mjs` (신규) | node로 콘텐츠 스크립트 검사(DOM 최소 스텁) |
| `internal/cli/controlgate.go` (신규) | 프록시 게이트(잡기·대기·응답 합성·에이전트 식별) |
| `internal/cli/browsercmd.go` (수정) | mcp-serve 루프에 게이트·pagemap·trim·background 배선, autopreview에 hangwatch 배선 |
| `internal/config/config.go` (수정) | `browser_control_wait_seconds`, `browser_trim_snapshots` |
| `internal/cli/skills.go` (수정) | agent-browser 스킬 문안(제어권·배경·효율 규칙) |
| `README.md` (수정) | 에이전트 전용 브라우저 절 갱신 |

---

### Task 1: 소유권 상태 기계 `control.go`

**Files:**
- Create: `internal/browser/control.go`
- Test: `internal/browser/control_test.go`

**Interfaces:**
- Produces:
  - `type Owner string` — `OwnerIdle="idle"`, `OwnerAgent="agent"`, `OwnerUser="user"`
  - `type Control struct { Owner Owner; Agent, Label string; Since, LastCall time.Time; Stopped bool; Waiting int }`
  - `type Event int` — `EvCall`, `EvUserTake`, `EvUserReturn`, `EvUserStop`
  - `func Apply(c Control, ev Event, agent, label string, now time.Time) Control`
  - `func (c Control) Expired(now time.Time) bool` — Owner==agent && now-LastCall ≥ 20s
  - `func (c Control) Effective(now time.Time) Control` — 만료면 idle로 바꾼 사본
  - `func LoadControl(dir string) Control` / `func SaveControl(dir string, c Control) error`
  - `func MirrorValue(c Control, target bool, title string) string` — `owner:agent:label:since_ms:last_ms:stopped:waiting:target:title`
  - `func ParseRequest(v string) (Event, int64, bool)` — `"user:<ms>"|"agent:<ms>"|"stop:<ms>"`
  - `const ControlExpiry = 20 * time.Second`

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/browser/control_test.go
package browser

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestApplyTransitions(t *testing.T) {
	t0 := time.Date(2026, 9, 22, 14, 0, 0, 0, time.Local)
	idle := Control{Owner: OwnerIdle}
	a := Apply(idle, EvCall, "claude-%1", "검색", t0)
	if a.Owner != OwnerAgent || a.Agent != "claude-%1" || a.Label != "검색" || !a.Since.Equal(t0) || !a.LastCall.Equal(t0) {
		t.Fatalf("idle→agent: %+v", a)
	}
	// 다른 에이전트 호출도 막지 않고 공유 — since는 유지, last_call·agent 갱신
	b := Apply(a, EvCall, "codex-%2", "정리", t0.Add(3*time.Second))
	if b.Owner != OwnerAgent || b.Agent != "codex-%2" || !b.Since.Equal(t0) || !b.LastCall.Equal(t0.Add(3*time.Second)) {
		t.Fatalf("agent→agent 공유: %+v", b)
	}
	u := Apply(b, EvUserTake, "", "", t0.Add(4*time.Second))
	if u.Owner != OwnerUser || u.Stopped || !u.Since.Equal(t0.Add(4*time.Second)) {
		t.Fatalf("→user: %+v", u)
	}
	s := Apply(u, EvUserStop, "", "", t0.Add(5*time.Second))
	if s.Owner != OwnerUser || !s.Stopped {
		t.Fatalf("→stopped: %+v", s)
	}
	// stopped 중 호출은 상태를 못 바꾼다
	if got := Apply(s, EvCall, "claude-%1", "x", t0.Add(6*time.Second)); got.Owner != OwnerUser || !got.Stopped {
		t.Fatalf("stopped 중 call은 무시: %+v", got)
	}
	r := Apply(s, EvUserReturn, "", "", t0.Add(7*time.Second))
	if r.Owner != OwnerAgent || r.Stopped || r.Agent != "codex-%2" {
		t.Fatalf("user→agent 복귀(마지막 에이전트 유지): %+v", r)
	}
}

func TestExpiredAndEffective(t *testing.T) {
	t0 := time.Now()
	c := Control{Owner: OwnerAgent, Agent: "a", LastCall: t0}
	if c.Expired(t0.Add(19 * time.Second)) {
		t.Fatal("19초는 만료 아님")
	}
	if !c.Expired(t0.Add(ControlExpiry)) {
		t.Fatal("20초는 만료")
	}
	if e := c.Effective(t0.Add(30 * time.Second)); e.Owner != OwnerIdle {
		t.Fatalf("만료면 idle: %+v", e)
	}
	u := Control{Owner: OwnerUser, LastCall: t0}
	if u.Expired(t0.Add(time.Hour)) {
		t.Fatal("user 소유는 만료 없음")
	}
}

func TestSaveLoadControl(t *testing.T) {
	dir := t.TempDir()
	if got := LoadControl(dir); got.Owner != OwnerIdle {
		t.Fatalf("없으면 idle: %+v", got)
	}
	c := Control{Owner: OwnerUser, Agent: "claude-%1", Label: "a:b", Since: time.Unix(1, 0), LastCall: time.Unix(2, 0), Stopped: true, Waiting: 2}
	if err := SaveControl(dir, c); err != nil {
		t.Fatal(err)
	}
	got := LoadControl(dir)
	if got.Owner != OwnerUser || got.Agent != c.Agent || got.Label != c.Label || !got.Stopped || got.Waiting != 2 || !got.Since.Equal(c.Since) {
		t.Fatalf("왕복: %+v", got)
	}
	if _, err := filepath.Abs(dir); err != nil {
		t.Fatal(err)
	}
}

func TestMirrorValueAndParseRequest(t *testing.T) {
	c := Control{Owner: OwnerAgent, Agent: "claude-%1", Label: "제목: 검색", Since: time.UnixMilli(1000), LastCall: time.UnixMilli(2000), Waiting: 1}
	v := MirrorValue(c, true, "JustWatch: 신작")
	want := "agent:claude-%1:제목∶ 검색:1000:2000:0:1:1:JustWatch∶ 신작"
	if v != want {
		t.Fatalf("mirror = %q, want %q", v, want)
	}
	if strings.Count(v, ":") != 8 {
		t.Fatalf("구분자 수: %q", v)
	}
	for in, want := range map[string]Event{"user:123": EvUserTake, "agent:5": EvUserReturn, "stop:9": EvUserStop} {
		ev, ms, ok := ParseRequest(in)
		if !ok || ev != want || ms == 0 {
			t.Errorf("ParseRequest(%q) = %v %d %v", in, ev, ms, ok)
		}
	}
	if _, _, ok := ParseRequest("bogus"); ok {
		t.Error("잘못된 요청은 false")
	}
	if _, _, ok := ParseRequest(""); ok {
		t.Error("빈 요청은 false")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/browser -run 'TestApplyTransitions|TestExpiredAndEffective|TestSaveLoadControl|TestMirrorValueAndParseRequest' -v`
Expected: FAIL — `undefined: Control` 등 컴파일 오류.

- [ ] **Step 3: 구현**

```go
// internal/browser/control.go
package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// 소유권 = 브라우저를 지금 누가 쓰는가. 정본은 <stateDir>/browser-control.json 하나 —
// mcp-serve 프록시가 클로드 세션마다 하나씩 떠서 파일이어야 전부 같은 값을 본다.
// 설계: docs/superpowers/specs/2026-09-22-agent-browser-control-design.md 1절.
type Owner string

const (
	OwnerIdle  Owner = "idle"
	OwnerAgent Owner = "agent"
	OwnerUser  Owner = "user"
)

// ControlExpiry — 마지막 도구 호출 뒤 이 시간 동안 호출이 없으면 agent 소유가 풀린다.
const ControlExpiry = 20 * time.Second

type Control struct {
	Owner    Owner     `json:"owner"`
	Agent    string    `json:"agent,omitempty"`    // 마지막으로 도구를 부른 에이전트 ID
	Label    string    `json:"label,omitempty"`    // 알약에 보일 작업명
	Since    time.Time `json:"since"`              // 현재 owner가 된 시각
	LastCall time.Time `json:"last_call"`          // 마지막 도구 호출
	Stopped  bool      `json:"stopped,omitempty"`  // 사용자가 「중단」을 눌렀는지
	Waiting  int       `json:"waiting,omitempty"`  // 게이트에 잡혀 있는 호출 수(마지막으로 쓴 프록시 값)
}

type Event int

const (
	EvCall       Event = iota // 도구 호출 통과
	EvUserTake                // 「내가 조작하기」
	EvUserReturn              // 「AI에게 돌려주기」
	EvUserStop                // 「중단」
)

// Apply는 전이 표(스펙 1절)를 그대로 옮긴 순수 함수. stopped 상태에서 EvCall은 무시한다 —
// 게이트가 그 호출을 통과시키지 않기 때문에 상태도 바뀌면 안 된다.
func Apply(c Control, ev Event, agent, label string, now time.Time) Control {
	switch ev {
	case EvCall:
		if c.Owner == OwnerUser {
			return c
		}
		if c.Owner != OwnerAgent {
			c.Since = now
		}
		c.Owner = OwnerAgent
		c.Agent, c.Label, c.LastCall = agent, label, now
	case EvUserTake:
		c.Owner, c.Since, c.Stopped = OwnerUser, now, false
	case EvUserStop:
		c.Owner, c.Since, c.Stopped = OwnerUser, now, true
	case EvUserReturn:
		c.Owner, c.Since, c.Stopped, c.LastCall = OwnerAgent, now, false, now
	}
	return c
}

// Expired — agent 소유가 호출 없이 ControlExpiry를 넘겼는지. user 소유는 만료가 없다.
func (c Control) Expired(now time.Time) bool {
	return c.Owner == OwnerAgent && now.Sub(c.LastCall) >= ControlExpiry
}

// Effective는 만료를 반영한 사본.
func (c Control) Effective(now time.Time) Control {
	if c.Expired(now) {
		c.Owner = OwnerIdle
	}
	return c
}

func controlPath(dir string) string { return filepath.Join(dir, "browser-control.json") }

// LoadControl — 파일이 없거나 깨졌으면 idle.
func LoadControl(dir string) Control {
	b, err := os.ReadFile(controlPath(dir))
	if err != nil {
		return Control{Owner: OwnerIdle}
	}
	var c Control
	if json.Unmarshal(b, &c) != nil || c.Owner == "" {
		return Control{Owner: OwnerIdle}
	}
	return c
}

// SaveControl — 임시 파일 → rename, 0600(board.RememberRoot와 같은 규칙).
func SaveControl(dir string, c Control) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	p := controlPath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".browser-control.*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// mirrorSafe — 미러 값은 ':'로 나누므로 본문의 ':'는 비슷한 글자(U+2236)로 바꾼다.
func mirrorSafe(s string) string { return strings.ReplaceAll(s, ":", "∶") }

// MirrorValue는 <html data-agentlayer-owner>에 쓸 값.
// owner:agent:label:since_ms:last_ms:stopped(0/1):waiting:target(0/1):title
func MirrorValue(c Control, target bool, title string) string {
	b := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	return strings.Join([]string{
		string(c.Owner), mirrorSafe(c.Agent), mirrorSafe(c.Label),
		strconv.FormatInt(c.Since.UnixMilli(), 10), strconv.FormatInt(c.LastCall.UnixMilli(), 10),
		b(c.Stopped), strconv.Itoa(c.Waiting), b(target), mirrorSafe(title),
	}, ":")
}

// ParseRequest는 콘텐츠 스크립트가 <html data-agentlayer-request>에 남긴 버튼 요청.
func ParseRequest(v string) (Event, int64, bool) {
	kind, msStr, ok := strings.Cut(v, ":")
	if !ok {
		return 0, 0, false
	}
	ms, err := strconv.ParseInt(msStr, 10, 64)
	if err != nil || ms == 0 {
		return 0, 0, false
	}
	switch kind {
	case "user":
		return EvUserTake, ms, true
	case "agent":
		return EvUserReturn, ms, true
	case "stop":
		return EvUserStop, ms, true
	}
	return 0, 0, false
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/browser -run 'TestApplyTransitions|TestExpiredAndEffective|TestSaveLoadControl|TestMirrorValueAndParseRequest' -v`
Expected: PASS 4건.

- [ ] **Step 5: 커밋**

```bash
git add internal/browser/control.go internal/browser/control_test.go
git commit -m "feat(browser): 소유권 상태 기계 control.go — agent/user/idle 전이·파일 정본·미러 값

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: pageId→url 표 `pagemap.go`

**Files:**
- Create: `internal/browser/pagemap.go`
- Test: `internal/browser/pagemap_test.go`

**Interfaces:**
- Produces:
  - `type PageMap struct{ mu sync.Mutex; urls map[int]string; selected int }`
  - `func (m *PageMap) Update(text string) bool` — 응답 텍스트에 `## Pages` 목록이 있으면 표를 통째로 바꾸고 true
  - `func (m *PageMap) URLFor(pageID int) (string, bool)`
  - `func (m *PageMap) SelectedURL() (string, bool)`
  - `func (m *PageMap) TitleFor(pageID int) string`
  - `func PageIDFromCall(line []byte) (int, bool)` — tools/call `params.arguments.pageId`
  - `func ResultText(line []byte) string` — JSON-RPC 응답의 `result.content[*].text`를 이어 붙임(없으면 "")

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/browser/pagemap_test.go
package browser

import "testing"

const pagesSample = "Note: the previously selected page was closed. Page 1 is now selected.\n## Pages\n1: 계약서 미리보기 (file:///tmp/a.html)\n2: JustWatch - 신작 (https://www.justwatch.com/kr/new) [selected]\n"

func TestPageMapUpdateAndLookup(t *testing.T) {
	var m PageMap
	if m.Update("Successfully clicked on the element") {
		t.Fatal("목록 없는 응답은 false")
	}
	if !m.Update(pagesSample) {
		t.Fatal("목록 있는 응답은 true")
	}
	if u, ok := m.URLFor(2); !ok || u != "https://www.justwatch.com/kr/new" {
		t.Fatalf("URLFor(2) = %q %v", u, ok)
	}
	if u, ok := m.SelectedURL(); !ok || u != "https://www.justwatch.com/kr/new" {
		t.Fatalf("SelectedURL = %q %v", u, ok)
	}
	if m.TitleFor(1) != "계약서 미리보기" {
		t.Fatalf("TitleFor(1) = %q", m.TitleFor(1))
	}
	if _, ok := m.URLFor(9); ok {
		t.Fatal("없는 id")
	}
	// 재번호: 다음 목록이 오면 이전 표는 버린다
	m.Update("## Pages\n1: only (https://example.com) [selected]\n")
	if _, ok := m.URLFor(2); ok {
		t.Fatal("재번호 뒤 옛 id는 사라져야 함")
	}
}

func TestPageIDFromCallAndResultText(t *testing.T) {
	line := []byte(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"click","arguments":{"pageId":2,"uid":"1_1"}}}`)
	if id, ok := PageIDFromCall(line); !ok || id != 2 {
		t.Fatalf("PageIDFromCall = %d %v", id, ok)
	}
	if _, ok := PageIDFromCall([]byte(`{"method":"tools/call","params":{"name":"list_pages","arguments":{}}}`)); ok {
		t.Fatal("pageId 없으면 false")
	}
	resp := []byte(`{"jsonrpc":"2.0","id":7,"result":{"content":[{"type":"text","text":"a\n"},{"type":"text","text":"## Pages\n1: x (https://x) [selected]\n"}]}}`)
	if got := ResultText(resp); got != "a\n## Pages\n1: x (https://x) [selected]\n" {
		t.Fatalf("ResultText = %q", got)
	}
	if ResultText([]byte(`{"id":1,"result":{}}`)) != "" {
		t.Fatal("content 없으면 빈 문자열")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/browser -run 'TestPageMap|TestPageIDFromCall' -v`
Expected: FAIL — `undefined: PageMap`.

- [ ] **Step 3: 구현**

```go
// internal/browser/pagemap.go
package browser

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// PageMap — chrome-devtools MCP의 pageId(서버 내부 번호)를 url·제목으로 잇는 표.
// 서버는 list_pages·new_page·select_page·close_page·navigate_page 응답에 "## Pages" 목록을
// 실어 보내므로 그때마다 표를 통째로 바꾼다(닫히면 재번호되기 때문).
type PageMap struct {
	mu       sync.Mutex
	urls     map[int]string
	titles   map[int]string
	selected int
}

// pageLineRe: "2: JustWatch - 신작 (https://www.justwatch.com/kr/new) [selected]"
var pageLineRe = regexp.MustCompile(`^(\d+): (.*?) \(([^()\s]+)\)( \[selected\])?\s*$`)

func (m *PageMap) Update(text string) bool {
	i := strings.Index(text, "## Pages")
	if i < 0 {
		return false
	}
	urls, titles := map[int]string{}, map[int]string{}
	selected := 0
	for _, ln := range strings.Split(text[i:], "\n")[1:] {
		mm := pageLineRe.FindStringSubmatch(ln)
		if mm == nil {
			continue
		}
		id, _ := strconv.Atoi(mm[1])
		urls[id], titles[id] = mm[3], mm[2]
		if mm[4] != "" {
			selected = id
		}
	}
	if len(urls) == 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.urls, m.titles, m.selected = urls, titles, selected
	return true
}

func (m *PageMap) URLFor(id int) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.urls[id]
	return u, ok
}

func (m *PageMap) SelectedURL() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.urls[m.selected]
	return u, ok
}

func (m *PageMap) TitleFor(id int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.titles[id]
}

// PageIDFromCall — tools/call의 params.arguments.pageId. 없으면 false.
func PageIDFromCall(line []byte) (int, bool) {
	var msg struct {
		Params struct {
			Arguments struct {
				PageID *int `json:"pageId"`
			} `json:"arguments"`
		} `json:"params"`
	}
	if json.Unmarshal(line, &msg) != nil || msg.Params.Arguments.PageID == nil {
		return 0, false
	}
	return *msg.Params.Arguments.PageID, true
}

// ResultText — JSON-RPC 응답의 result.content[].text를 이어 붙인다. 없으면 "".
func ResultText(line []byte) string {
	var msg struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if json.Unmarshal(line, &msg) != nil {
		return ""
	}
	var sb strings.Builder
	for _, c := range msg.Result.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	return sb.String()
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/browser -run 'TestPageMap|TestPageIDFromCall' -v`
Expected: PASS 2건.

- [ ] **Step 5: 커밋**

```bash
git add internal/browser/pagemap.go internal/browser/pagemap_test.go
git commit -m "feat(browser): pageId→url 표 pagemap.go — MCP 응답의 ## Pages 목록 파싱

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: FX 트래커 도구명 보관 · 입력 도구 · 병렬 신호 · `SyncTabs`

**Files:**
- Modify: `internal/browser/fx.go`
- Test: `internal/browser/fx_test.go`

**Interfaces:**
- Consumes: Task 1 `Control`, `MirrorValue`, `ParseRequest`, `Event`.
- Produces:
  - `func (t *FxTracker) End(line []byte) (tool string, last bool)` — **시그니처 변경**: 응답이 추적 중이면 그 호출의 도구명, 마지막이면 last=true
  - `func IsInputTool(name string) bool`
  - `const fxBudget = 300 * time.Millisecond`
  - `func forEachPage(pages []*rod.Page, budget time.Duration, fn func(*rod.Page))` — 병렬 실행, 전체 마감
  - `func SignalFx(b *rod.Browser, tool string, on bool, targetURL string) error` — targetURL이 비어 있지 않으면 그 url 탭들에만
  - `type TabRequest struct{ Event Event; At int64 }`
  - `func SyncTabs(b *rod.Browser, c Control, targetURL, targetTitle string) []TabRequest` — 미러 쓰기+요청 회수 한 왕복, 병렬
  - `func LatestRequest(reqs []TabRequest) (Event, bool)` — 가장 최신 요청

- [ ] **Step 1: 실패하는 테스트 작성** (기존 `fx_test.go`에 추가)

```go
// internal/browser/fx_test.go 에 추가
func TestFxTrackerKeepsToolName(t *testing.T) {
	var tr FxTracker
	if tool, ok := tr.Start([]byte(`{"id":1,"method":"tools/call","params":{"name":"wait_for"}}`)); !ok || tool != "wait_for" {
		t.Fatal("start")
	}
	if tool, ok := tr.Start([]byte(`{"id":2,"method":"tools/call","params":{"name":"click"}}`)); !ok || tool != "click" {
		t.Fatal("start2")
	}
	tool, last := tr.End([]byte(`{"id":2,"result":{}}`))
	if tool != "click" || last {
		t.Fatalf("End(2) = %q last=%v", tool, last)
	}
	tool, last = tr.End([]byte(`{"id":1,"result":{}}`))
	if tool != "wait_for" || !last {
		t.Fatalf("End(1) = %q last=%v", tool, last)
	}
	if tool, last := tr.End([]byte(`{"id":99,"result":{}}`)); tool != "" || last {
		t.Fatal("모르는 id")
	}
}

func TestIsInputTool(t *testing.T) {
	for _, n := range []string{"click", "hover", "drag", "fill", "fill_form", "type_text", "press_key", "upload_file"} {
		if !IsInputTool(n) {
			t.Errorf("%s는 입력 도구", n)
		}
	}
	for _, n := range []string{"take_snapshot", "wait_for", "navigate_page", "evaluate_script", ""} {
		if IsInputTool(n) {
			t.Errorf("%s는 입력 도구 아님", n)
		}
	}
}

func TestForEachPageRunsInParallelWithinBudget(t *testing.T) {
	// rod.Page 없이 병렬성만 본다 — fn이 각각 200ms 자면 순차면 600ms, 병렬이면 ~200ms
	pages := make([]*rod.Page, 3)
	start := time.Now()
	var n int32
	forEachPage(pages, 300*time.Millisecond, func(*rod.Page) {
		time.Sleep(200 * time.Millisecond)
		atomic.AddInt32(&n, 1)
	})
	if d := time.Since(start); d > 450*time.Millisecond {
		t.Fatalf("병렬이 아님: %v", d)
	}
	if atomic.LoadInt32(&n) != 3 {
		t.Fatalf("3개 다 실행돼야 함: %d", n)
	}
	// 마감을 넘기는 fn은 기다리지 않는다
	start = time.Now()
	forEachPage(pages, 100*time.Millisecond, func(*rod.Page) { time.Sleep(2 * time.Second) })
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Fatalf("마감을 안 지킴: %v", d)
	}
}

func TestLatestRequest(t *testing.T) {
	if _, ok := LatestRequest(nil); ok {
		t.Fatal("없으면 false")
	}
	ev, ok := LatestRequest([]TabRequest{{EvUserTake, 10}, {EvUserStop, 30}, {EvUserReturn, 20}})
	if !ok || ev != EvUserStop {
		t.Fatalf("최신은 stop: %v %v", ev, ok)
	}
}
```

import에 `"sync/atomic"`, `"time"`, `"github.com/go-rod/rod"` 추가.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/browser -run 'TestFxTracker|TestIsInputTool|TestForEachPage|TestLatestRequest' -v`
Expected: FAIL — `End` 반환값 개수 불일치, `IsInputTool` 미정의.

- [ ] **Step 3: 구현** — `fx.go`에서 `FxTracker`·`SignalFx`를 아래처럼 바꾸고 함수를 추가한다.

```go
// FxTracker — inflight는 id → 도구명.
type FxTracker struct {
	mu       sync.Mutex
	inflight map[string]string
}

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

// End — 추적 중인 호출의 응답이면 (도구명, 마지막 응답인지). 아니면 ("", false).
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
var inputTools = map[string]bool{"click": true, "hover": true, "drag": true, "fill": true,
	"fill_form": true, "type_text": true, "press_key": true, "upload_file": true}

func IsInputTool(name string) bool { return inputTools[name] }

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

// webPages — 웹 URL인 탭만. targetURL이 있으면 그 url인 탭만.
func webPages(b *rod.Browser, targetURL string) ([]*rod.Page, error) {
	pages, err := b.Pages()
	if err != nil {
		return nil, err
	}
	var out []*rod.Page
	for _, p := range pages {
		info, err := p.Info()
		if err != nil || !IsWebURL(info.URL) {
			continue
		}
		if targetURL != "" && info.URL != targetURL {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// SignalFx — 시작/끝 신호. targetURL이 비어 있으면 모든 웹 탭(작업 탭을 모를 때).
func SignalFx(b *rod.Browser, tool string, on bool, targetURL string) error {
	pages, err := webPages(b, targetURL)
	if err != nil {
		return err
	}
	v := fxValue(tool, on)
	forEachPage(pages, fxBudget, func(p *rod.Page) {
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

// SyncTabs — 모든 웹 탭에 현재 소유권을 미러하고(작업 탭은 target=1), 버튼 요청을 회수한다.
func SyncTabs(b *rod.Browser, c Control, targetURL, targetTitle string) []TabRequest {
	pages, err := webPages(b, "")
	if err != nil {
		return nil
	}
	var mu sync.Mutex
	var reqs []TabRequest
	forEachPage(pages, fxBudget, func(p *rod.Page) {
		info, err := p.Info()
		if err != nil {
			return
		}
		target := targetURL != "" && info.URL == targetURL
		res, err := p.Timeout(fxBudget).Eval(syncJS, MirrorValue(c, target, targetTitle), ownerAttr, requestAttr)
		if err != nil {
			return
		}
		if ev, at, ok := ParseRequest(res.Value.Str()); ok {
			mu.Lock()
			reqs = append(reqs, TabRequest{Event: ev, At: at})
			mu.Unlock()
		}
	})
	return reqs
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
```

기존 `SignalFx(b, tool, on)` 호출처(`internal/cli/browsercmd.go fxSignaler.signal`)는 Task 4에서 새 시그니처로 바꾼다. 이 Task에서 컴파일이 깨지지 않도록 `browsercmd.go`의 호출을 임시로 `browser.SignalFx(f.b, tool, on, "")`로 고치고, `f.tracker.End(line)` 반환을 `if _, last := f.tracker.End(line); last {`로 맞춘다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/browser -run 'TestFxTracker|TestIsInputTool|TestForEachPage|TestLatestRequest|TestInstallFx' -v && go build ./...`
Expected: PASS, 빌드 성공.

- [ ] **Step 5: 커밋**

```bash
git add internal/browser/fx.go internal/browser/fx_test.go internal/cli/browsercmd.go
git commit -m "feat(browser): FX 트래커 도구명 보관·입력 도구 목록·탭 병렬 신호(300ms 마감)·SyncTabs 미러/요청 회수

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: 프록시 게이트 `controlgate.go` + mcp-serve 배선

**Files:**
- Create: `internal/cli/controlgate.go`
- Modify: `internal/cli/browsercmd.go` (mcp-serve 루프, `fxSignaler`)
- Modify: `internal/config/config.go` (`BrowserControlWaitSeconds`)
- Test: `internal/cli/controlgate_test.go`, `internal/config/config_test.go`

**Interfaces:**
- Consumes: Task 1 `browser.Control/Apply/LoadControl/SaveControl/Event`, Task 2 `browser.PageMap/PageIDFromCall/ResultText`, Task 3 `browser.SyncTabs/LatestRequest/SignalFx/IsInputTool`.
- Produces:
  - `type controlGate struct` with fields `dir string; now func() time.Time; sleep func(time.Duration); wait, poll time.Duration; sync func(c browser.Control) []browser.TabRequest; agent func() (id, label string)`
  - `func newControlGate(dir string, wait time.Duration, sync func(browser.Control) []browser.TabRequest, agent func() (string, string)) *controlGate`
  - `func (g *controlGate) Pass(line []byte) (forward bool, reply []byte)` — tools/call 한 줄을 통과시킬지. reply가 있으면 클라이언트에 직접 쓴다
  - `func gateErrorReply(id json.RawMessage, text string) []byte` — MCP `isError` 결과 합성
  - `func resolveAgent(st *state.Store, pane string) (id, label string)` — pane으로 레코드 찾기
  - `func (c *Config) BrowserControlWait() time.Duration` — 기본 120s, 0 이하·파싱 불가는 기본

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/cli/controlgate_test.go
package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/browser"
	"github.com/netwaif/agentlayer/internal/state"
)

func gateLine(id int, tool string) []byte {
	return []byte(`{"jsonrpc":"2.0","id":` + itoa(id) + `,"method":"tools/call","params":{"name":"` + tool + `","arguments":{"pageId":1}}}` + "\n")
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func newTestGate(t *testing.T, dir string, sync func(browser.Control) []browser.TabRequest) (*controlGate, *time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 22, 14, 0, 0, 0, time.Local)
	g := newControlGate(dir, 3*time.Second, sync, func() (string, string) { return "claude-%1", "검색" })
	g.now = func() time.Time { return now }
	g.sleep = func(d time.Duration) { now = now.Add(d) }
	g.poll = time.Second
	return g, &now
}

func TestGatePassIdleBecomesAgent(t *testing.T) {
	dir := t.TempDir()
	g, _ := newTestGate(t, dir, func(browser.Control) []browser.TabRequest { return nil })
	fwd, reply := g.Pass(gateLine(1, "click"))
	if !fwd || reply != nil {
		t.Fatalf("idle이면 통과: %v %s", fwd, reply)
	}
	c := browser.LoadControl(dir)
	if c.Owner != browser.OwnerAgent || c.Agent != "claude-%1" || c.Label != "검색" {
		t.Fatalf("파일 갱신: %+v", c)
	}
}

func TestGateNonToolCallPasses(t *testing.T) {
	g, _ := newTestGate(t, t.TempDir(), func(browser.Control) []browser.TabRequest { return nil })
	fwd, reply := g.Pass([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"))
	if !fwd || reply != nil {
		t.Fatal("tools/call 아니면 그대로 통과")
	}
	if c := browser.LoadControl(g.dir); c.Owner != browser.OwnerIdle {
		t.Fatal("상태도 안 건드림")
	}
}

func TestGateUserHoldsThenReleases(t *testing.T) {
	dir := t.TempDir()
	_ = browser.SaveControl(dir, browser.Control{Owner: browser.OwnerUser, Agent: "claude-%1"})
	calls := 0
	g, _ := newTestGate(t, dir, func(browser.Control) []browser.TabRequest {
		calls++
		if calls == 2 { // 두 번째 폴링에서 「돌려주기」
			return []browser.TabRequest{{Event: browser.EvUserReturn, At: 5}}
		}
		return nil
	})
	fwd, reply := g.Pass(gateLine(2, "click"))
	if !fwd || reply != nil {
		t.Fatalf("돌려주면 통과해야 함: %v %s", fwd, reply)
	}
	if c := browser.LoadControl(dir); c.Owner != browser.OwnerAgent {
		t.Fatalf("복귀 뒤 agent: %+v", c)
	}
}

func TestGateUserTimeoutReplies(t *testing.T) {
	dir := t.TempDir()
	_ = browser.SaveControl(dir, browser.Control{Owner: browser.OwnerUser})
	g, _ := newTestGate(t, dir, func(browser.Control) []browser.TabRequest { return nil })
	fwd, reply := g.Pass(gateLine(3, "click"))
	if fwd || reply == nil {
		t.Fatal("타임아웃이면 전달 안 하고 응답 합성")
	}
	var msg struct {
		ID     int `json:"id"`
		Result struct {
			IsError bool `json:"isError"`
			Content []struct{ Text string } `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(reply, &msg); err != nil || msg.ID != 3 || !msg.Result.IsError {
		t.Fatalf("응답 형식: %s (%v)", reply, err)
	}
	if !strings.Contains(msg.Result.Content[0].Text, "직접 조작 중") || !strings.Contains(msg.Result.Content[0].Text, "재시도하지") {
		t.Fatalf("문구: %s", msg.Result.Content[0].Text)
	}
	if !strings.HasSuffix(string(reply), "\n") {
		t.Fatal("stdio 한 줄이므로 개행으로 끝나야 함")
	}
	if c := browser.LoadControl(dir); c.Waiting != 0 {
		t.Fatalf("대기 해제 뒤 waiting 0: %+v", c)
	}
}

func TestGateStoppedRepliesImmediately(t *testing.T) {
	dir := t.TempDir()
	_ = browser.SaveControl(dir, browser.Control{Owner: browser.OwnerUser, Stopped: true})
	slept := 0
	g, _ := newTestGate(t, dir, func(browser.Control) []browser.TabRequest { return nil })
	g.sleep = func(time.Duration) { slept++ }
	fwd, reply := g.Pass(gateLine(4, "click"))
	if fwd || reply == nil || slept != 0 {
		t.Fatalf("stopped는 즉시 거절: %v %s slept=%d", fwd, reply, slept)
	}
	if !strings.Contains(string(reply), "중단") {
		t.Fatalf("문구: %s", reply)
	}
}

func TestGateExpiredAgentIsIdle(t *testing.T) {
	dir := t.TempDir()
	g, now := newTestGate(t, dir, func(browser.Control) []browser.TabRequest { return nil })
	_ = browser.SaveControl(dir, browser.Control{Owner: browser.OwnerAgent, Agent: "codex-%9", LastCall: now.Add(-time.Minute)})
	g.Pass(gateLine(5, "take_snapshot"))
	c := browser.LoadControl(dir)
	if c.Agent != "claude-%1" || !c.Since.Equal(*now) {
		t.Fatalf("만료 뒤 새 소유: %+v", c)
	}
}

func TestGateTakeRequestDuringAgent(t *testing.T) {
	// 통과 시점의 sync가 「내가 조작하기」를 회수하면 그 호출은 잡힌다(다음 폴링에서 돌려주면 풀림)
	dir := t.TempDir()
	n := 0
	g, _ := newTestGate(t, dir, func(browser.Control) []browser.TabRequest {
		n++
		switch n {
		case 1:
			return []browser.TabRequest{{Event: browser.EvUserTake, At: 1}}
		case 2:
			return []browser.TabRequest{{Event: browser.EvUserReturn, At: 2}}
		}
		return nil
	})
	fwd, reply := g.Pass(gateLine(6, "click"))
	if !fwd || reply != nil || n != 2 {
		t.Fatalf("take→return: %v %s n=%d", fwd, reply, n)
	}
}

func TestResolveAgent(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.NewStore(dir)
	_ = st.Save(&state.Agent{ID: "claude-%7", Kind: "claude", Task: "JustWatch 신작", Tmux: state.TmuxRef{PaneID: "%7"}})
	if id, label := resolveAgent(st, "%7"); id != "claude-%7" || label != "JustWatch 신작" {
		t.Fatalf("resolveAgent = %q %q", id, label)
	}
	if id, label := resolveAgent(st, "%8"); id != "agent" || label != "브라우저 작업" {
		t.Fatalf("없으면 기본값: %q %q", id, label)
	}
	if id, _ := resolveAgent(nil, "%7"); id != "agent" {
		t.Fatal("store nil이면 기본값")
	}
}
```

`internal/config/config_test.go`에 추가:

```go
func TestBrowserControlWait(t *testing.T) {
	var c Config
	if c.BrowserControlWait() != 120*time.Second {
		t.Fatal("기본 120초")
	}
	c.BrowserControlWaitSeconds = 30
	if c.BrowserControlWait() != 30*time.Second {
		t.Fatal("설정 반영")
	}
	c.BrowserControlWaitSeconds = -1
	if c.BrowserControlWait() != 120*time.Second {
		t.Fatal("0 이하는 기본")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/cli -run 'TestGate|TestResolveAgent' -v; go test ./internal/config -run TestBrowserControlWait -v`
Expected: FAIL — 미정의.

- [ ] **Step 3: 구현**

`internal/config/config.go` — 필드와 접근자:

```go
	// BrowserControlWaitSeconds — 사용자가 「내가 조작하기」로 제어권을 가진 동안 프록시가
	// 도구 호출을 잡고 기다리는 상한(초). 0 이하·미설정은 120.
	BrowserControlWaitSeconds int `json:"browser_control_wait_seconds,omitempty"`
```

```go
const defaultBrowserControlWait = 120 * time.Second

// BrowserControlWait는 browser_control_wait_seconds를 반영한 대기 상한.
func (c *Config) BrowserControlWait() time.Duration {
	if c.BrowserControlWaitSeconds <= 0 {
		return defaultBrowserControlWait
	}
	return time.Duration(c.BrowserControlWaitSeconds) * time.Second
}
```

`internal/cli/controlgate.go`:

```go
// internal/cli/controlgate.go
package cli

import (
	"encoding/json"
	"time"

	"github.com/netwaif/agentlayer/internal/browser"
	"github.com/netwaif/agentlayer/internal/state"
)

// controlGate — mcp-serve 프록시의 소유권 게이트. tools/call 한 줄마다 파일 정본을 읽고
// 탭의 버튼 요청을 회수해 전이한 뒤, 사용자 소유면 잡고 기다린다.
// 설계: docs/superpowers/specs/2026-09-22-agent-browser-control-design.md 2절.
type controlGate struct {
	dir   string
	now   func() time.Time
	sleep func(time.Duration)
	wait  time.Duration // 사용자 소유 대기 상한
	poll  time.Duration // 대기 중 재확인 주기
	sync  func(c browser.Control) []browser.TabRequest // 미러 쓰기 + 요청 회수(Task 3 SyncTabs를 감싼 것)
	agent func() (id, label string)
}

func newControlGate(dir string, wait time.Duration, sync func(browser.Control) []browser.TabRequest, agent func() (string, string)) *controlGate {
	return &controlGate{dir: dir, now: time.Now, sleep: time.Sleep, wait: wait, poll: 500 * time.Millisecond, sync: sync, agent: agent}
}

const (
	gateUserBusyText = "사용자가 브라우저를 직접 조작 중입니다 — 돌려줄 때까지 기다린 뒤 다시 시도하세요. 재시도하지 말고 사용자에게 알린 뒤 지시를 기다리세요."
	gateStoppedText  = "사용자가 브라우저 조작을 중단시켰습니다 — 재시도하지 말고 사용자 지시를 기다리세요."
)

// step — 파일을 읽고 만료를 반영하고, 탭 요청을 회수해 전이한 뒤 저장한다. 결과 상태를 돌려준다.
func (g *controlGate) step(waiting int) browser.Control {
	now := g.now()
	c := browser.LoadControl(g.dir).Effective(now)
	c.Waiting = waiting
	if ev, ok := browser.LatestRequest(g.sync(c)); ok {
		c = browser.Apply(c, ev, "", "", now)
	}
	_ = browser.SaveControl(g.dir, c)
	return c
}

// Pass — 이 줄을 서버로 넘길지. reply가 있으면 클라이언트에 그대로 쓴다(서버로 안 보냄).
func (g *controlGate) Pass(line []byte) (bool, []byte) {
	if !IsMCPToolCall(line) {
		return true, nil
	}
	c := g.step(0)
	if c.Owner == browser.OwnerUser {
		if c.Stopped {
			return false, gateErrorReply(rpcID(line), gateStoppedText)
		}
		deadline := g.now().Add(g.wait)
		for c.Owner == browser.OwnerUser && !c.Stopped {
			if !g.now().Before(deadline) {
				g.step(0)
				return false, gateErrorReply(rpcID(line), gateUserBusyText)
			}
			g.sleep(g.poll)
			c = g.step(1)
		}
		if c.Stopped {
			g.step(0)
			return false, gateErrorReply(rpcID(line), gateStoppedText)
		}
	}
	id, label := g.agent()
	c = browser.Apply(c, browser.EvCall, id, label, g.now())
	c.Waiting = 0
	_ = browser.SaveControl(g.dir, c)
	return true, nil
}

func rpcID(line []byte) json.RawMessage {
	var msg struct {
		ID json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal(line, &msg)
	return msg.ID
}

// gateErrorReply — MCP tools/call 결과(isError)를 stdio 한 줄로 합성.
func gateErrorReply(id json.RawMessage, text string) []byte {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]any{
			"isError": true,
			"content": []map[string]string{{"type": "text", "text": text}},
		},
	})
	return append(b, '\n')
}

// resolveAgent — 프록시가 물려받은 TMUX_PANE으로 관제탑 레코드를 찾아 (ID, 최근 작업).
func resolveAgent(st *state.Store, pane string) (string, string) {
	const defID, defLabel = "agent", "브라우저 작업"
	if st == nil || pane == "" {
		return defID, defLabel
	}
	agents, err := st.List()
	if err != nil {
		return defID, defLabel
	}
	for _, a := range agents {
		if a.Tmux.PaneID == pane {
			label := a.Task
			if label == "" {
				label = defLabel
			}
			return a.ID, label
		}
	}
	return defID, defLabel
}
```

`internal/cli/browsercmd.go` — `browserMCPServe` 루프 배선. `fx := newFxSignaler(...)` 아래에:

```go
	pm := &browser.PageMap{}
	var st *state.Store
	if s, err := state.NewStore(state.DefaultDir()); err == nil {
		st = s
	}
	pane := os.Getenv("TMUX_PANE")
	gate := newControlGate(state.DefaultDir(), cfg.BrowserControlWait(),
		func(c browser.Control) []browser.TabRequest {
			b := fx.browser()
			if b == nil {
				return nil
			}
			url, title := fx.target()
			return browser.SyncTabs(b, c, url, title)
		},
		func() (string, string) { return resolveAgent(st, pane) })
```

클라이언트→서버 루프 안, `fx.OnClientLine(line)` 앞에:

```go
			if IsMCPToolCall(line) {
				if id, ok := browser.PageIDFromCall(line); ok {
					fx.setTarget(pm.URLFor(id))
					fx.setTargetTitle(pm.TitleFor(id))
				} else {
					fx.setTarget(pm.SelectedURL())
				}
				if fwd, reply := gate.Pass(line); !fwd {
					_, _ = os.Stdout.Write(reply)
					continue
				}
			}
```

(`continue`가 되려면 그 자리의 `if len(line) > 0 { ... }` 블록을 `for` 본문에서 함수로 빼거나 `goto` 없이 아래 전달 코드를 `else`로 감싼다 — 간단히 `handled := false` 플래그로 전달 코드를 건너뛴다.)

서버→클라이언트 고루틴, `fx.OnServerLine(line)` 앞에: `pm.Update(browser.ResultText(line))`.

`fxSignaler`에 추가:

```go
	tmu         sync.Mutex
	targetURL   string
	targetTitle string

func (f *fxSignaler) setTarget(url string, ok bool) {
	f.tmu.Lock(); defer f.tmu.Unlock()
	if ok { f.targetURL = url } else { f.targetURL = "" }
}
func (f *fxSignaler) setTargetTitle(t string) { f.tmu.Lock(); f.targetTitle = t; f.tmu.Unlock() }
func (f *fxSignaler) target() (string, string) { f.tmu.Lock(); defer f.tmu.Unlock(); return f.targetURL, f.targetTitle }

// browser — 연결을 맺어 돌려준다(없으면 nil). enabled와 무관 — 게이트는 효과가 꺼져도 돈다.
func (f *fxSignaler) browser() *rod.Browser {
	f.mu.Lock(); defer f.mu.Unlock()
	if f.b == nil {
		b, err := f.connect()
		if err != nil { return nil }
		f.b = b
	}
	return f.b
}
```

`signal`은 `browser.SignalFx(f.b, tool, on, url)`로 (url은 `f.target()`의 첫 값). `OnServerLine`은 `if _, last := f.tracker.End(line); last { f.signal("", false) }`.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/cli -run 'TestGate|TestResolveAgent|TestIsMCPToolCall' -v && go test ./internal/config -run TestBrowserControlWait -v && go build ./...`
Expected: PASS, 빌드 성공.

- [ ] **Step 5: 커밋**

```bash
git add internal/cli/controlgate.go internal/cli/controlgate_test.go internal/cli/browsercmd.go internal/config/config.go internal/config/config_test.go
git commit -m "feat(cli): mcp-serve 소유권 게이트 — 사용자 제어 중 호출 대기(기본 120초)·중단 즉시 응답·작업 탭 특정 배선

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 5: 배경 동작 — 앞 앱 복원 · `new_page` background 채우기

**Files:**
- Create: `internal/browser/front.go`
- Modify: `internal/browser/instance.go:148-156` (Launch 전후)
- Modify: `internal/cli/browsercmd.go` (클라이언트 루프)
- Test: `internal/browser/front_test.go`

**Interfaces:**
- Produces:
  - `type FrontOps struct{ Frontmost func() (string, error); Activate func(name string) error }`
  - `func DefaultFrontOps(goos string) FrontOps` — darwin은 osascript, 그 외 no-op
  - `const EngineAppName = "Google Chrome for Testing"`
  - `func RestoreFront(ops FrontOps, launch func() error) error` — 앞 앱 기억 → launch → 3초 안에 창이 뜨면 복원
  - `func RewriteNewPage(line []byte, browserInFront bool) []byte` — `new_page`·`background` 비어 있음·브라우저가 앞이 아님 → `background:true`

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/browser/front_test.go
package browser

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRestoreFrontRemembersAndActivates(t *testing.T) {
	var activated []string
	ops := FrontOps{
		Frontmost: func() (string, error) { return "iTerm2", nil },
		Activate:  func(n string) error { activated = append(activated, n); return nil },
	}
	launched := false
	if err := RestoreFront(ops, func() error { launched = true; return nil }); err != nil || !launched {
		t.Fatal(err)
	}
	if len(activated) != 1 || activated[0] != "iTerm2" {
		t.Fatalf("복원: %v", activated)
	}
}

func TestRestoreFrontSkipsWhenBrowserWasFront(t *testing.T) {
	var activated []string
	ops := FrontOps{
		Frontmost: func() (string, error) { return EngineAppName, nil },
		Activate:  func(n string) error { activated = append(activated, n); return nil },
	}
	_ = RestoreFront(ops, func() error { return nil })
	if len(activated) != 0 {
		t.Fatal("브라우저가 앞이었으면 복원 안 함")
	}
}

func TestRestoreFrontSwallowsOpsErrors(t *testing.T) {
	ops := FrontOps{
		Frontmost: func() (string, error) { return "", errors.New("no osascript") },
		Activate:  func(string) error { return errors.New("x") },
	}
	if err := RestoreFront(ops, func() error { return nil }); err != nil {
		t.Fatal("ops 실패는 삼킨다")
	}
	if err := RestoreFront(ops, func() error { return errors.New("launch") }); err == nil {
		t.Fatal("launch 실패는 그대로")
	}
}

func TestRewriteNewPage(t *testing.T) {
	in := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"new_page","arguments":{"url":"https://x"}}}` + "\n")
	out := RewriteNewPage(in, false)
	var msg struct {
		Params struct {
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
	}
	if err := json.Unmarshal(out, &msg); err != nil || msg.Params.Arguments["background"] != true {
		t.Fatalf("background 채움: %s (%v)", out, err)
	}
	if out[len(out)-1] != '\n' {
		t.Fatal("개행 유지")
	}
	if got := RewriteNewPage(in, true); string(got) != string(in) {
		t.Fatal("브라우저가 앞이면 그대로")
	}
	explicit := []byte(`{"method":"tools/call","params":{"name":"new_page","arguments":{"url":"https://x","background":false}}}` + "\n")
	if got := RewriteNewPage(explicit, false); string(got) != string(explicit) {
		t.Fatal("명시된 값은 존중")
	}
	other := []byte(`{"method":"tools/call","params":{"name":"click","arguments":{"pageId":1}}}` + "\n")
	if got := RewriteNewPage(other, false); string(got) != string(other) {
		t.Fatal("다른 도구는 그대로")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/browser -run 'TestRestoreFront|TestRewriteNewPage' -v`
Expected: FAIL — 미정의.

- [ ] **Step 3: 구현**

```go
// internal/browser/front.go
package browser

import (
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// 배경 동작(스펙 2절): 에이전트가 검색을 시켜도 브라우저가 앞으로 튀어나오지 않게.
// macOS 한정 — 리눅스는 no-op.

const EngineAppName = "Google Chrome for Testing"

type FrontOps struct {
	Frontmost func() (string, error) // 지금 앞에 있는 앱 이름
	Activate  func(name string) error
}

func DefaultFrontOps(goos string) FrontOps {
	if goos != "darwin" {
		return FrontOps{Frontmost: func() (string, error) { return "", nil }, Activate: func(string) error { return nil }}
	}
	return FrontOps{
		Frontmost: func() (string, error) {
			out, err := exec.Command("osascript", "-e",
				`tell application "System Events" to get name of first application process whose frontmost is true`).Output()
			return strings.TrimSpace(string(out)), err
		},
		Activate: func(name string) error {
			return exec.Command("osascript", "-e",
				`tell application "System Events" to set frontmost of process "`+strings.ReplaceAll(name, `"`, ``)+`" to true`).Run()
		},
	}
}

// RestoreFront — launch 전 앞 앱을 기억했다가 launch 뒤 되돌린다. ops 실패는 삼키고 launch 오류만 돌려준다.
// Chrome은 창을 만들며 스스로 활성화하므로 launch 반환 뒤 잠깐(최대 3초, 0.5초 간격) 앞 앱이
// 브라우저로 바뀌는 것을 기다린 뒤 복원한다 — 너무 일찍 복원하면 Chrome이 다시 앞으로 온다.
func RestoreFront(ops FrontOps, launch func() error) error {
	prev, err := ops.Frontmost()
	if err != nil || prev == "" || prev == EngineAppName {
		return launch()
	}
	if err := launch(); err != nil {
		return err
	}
	for i := 0; i < 6; i++ {
		if cur, err := ops.Frontmost(); err == nil && cur == EngineAppName {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	_ = ops.Activate(prev)
	return nil
}

// RewriteNewPage — new_page 호출에 background가 없고 브라우저가 앞이 아니면 background:true.
func RewriteNewPage(line []byte, browserInFront bool) []byte {
	if browserInFront {
		return line
	}
	var msg map[string]any
	if json.Unmarshal(line, &msg) != nil || msg["method"] != "tools/call" {
		return line
	}
	params, _ := msg["params"].(map[string]any)
	if params == nil || params["name"] != "new_page" {
		return line
	}
	args, _ := params["arguments"].(map[string]any)
	if args == nil {
		args = map[string]any{}
		params["arguments"] = args
	}
	if _, set := args["background"]; set {
		return line
	}
	args["background"] = true
	out, err := json.Marshal(msg)
	if err != nil {
		return line
	}
	return append(out, '\n')
}
```

테스트의 `RestoreFront` 복원 케이스는 `Frontmost`가 "iTerm2"를 계속 돌려주므로 6회 × 0.5초 = 3초 걸린다. 테스트를 빠르게 하려면 `time.Sleep`을 패키지 변수 `frontSleep = time.Sleep`로 빼고 테스트에서 `frontSleep = func(time.Duration) {}`로 바꾼다(테스트 첫 줄에 `frontSleep = func(time.Duration) {}; t.Cleanup(func() { frontSleep = time.Sleep })`).

`instance.go`의 `ws, err = newLauncher(bin, profile, port, fxDir).Launch()`를:

```go
	var wsURL string
	err = RestoreFront(DefaultFrontOps(runtime.GOOS), func() error {
		u, lerr := newLauncher(bin, profile, port, fxDir).Launch()
		wsURL = u
		return lerr
	})
	ws = wsURL
```

`browsercmd.go` 클라이언트 루프, 게이트 통과 뒤 전달 직전:

```go
			if IsMCPToolCall(line) {
				front, _ := frontOps.Frontmost()
				line = browser.RewriteNewPage(line, front == browser.EngineAppName)
			}
```

`frontOps := browser.DefaultFrontOps(runtime.GOOS)`를 루프 앞에 둔다. `new_page`가 아닌 호출에서 osascript를 부르지 않도록 `RewriteNewPage` 호출 전에 도구명이 `new_page`인지 `FxTracker.Start`가 돌려준 이름으로 확인한다(그 이름이 `new_page`일 때만 `Frontmost()` 실행).

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/browser -run 'TestRestoreFront|TestRewriteNewPage' -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
git add internal/browser/front.go internal/browser/front_test.go internal/browser/instance.go internal/cli/browsercmd.go
git commit -m "feat(browser): 배경 동작 — 기동 뒤 앞 앱 복원, 브라우저가 앞이 아니면 new_page를 배경 탭으로

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: 응답 스냅샷 잘라내기 `trim.go`

**Files:**
- Create: `internal/browser/trim.go`
- Modify: `internal/config/config.go` (`BrowserTrimSnapshots *bool`, `BrowserTrimSnapshotsEnabled()`), `internal/cli/browsercmd.go` (서버→클라이언트 고루틴)
- Test: `internal/browser/trim_test.go`, `internal/config/config_test.go`

**Interfaces:**
- Consumes: Task 3 `FxTracker.End` 도구명, Task 2 `ResultText`.
- Produces:
  - `func TrimSnapshot(line []byte, tool string) []byte` — tool이 `wait_for`·`navigate_page`이고 텍스트에 `## Latest page snapshot`이 있으면 그 이후를 `(스냅샷 생략 — 필요하면 take_snapshot)`으로 대체. 다른 도구·마커 없음은 그대로.
  - `const trimTools`… `var trimTools = map[string]bool{"wait_for": true, "navigate_page": true}`

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/browser/trim_test.go
package browser

import (
	"strings"
	"testing"
)

func TestTrimSnapshot(t *testing.T) {
	body := "Element found\n## Latest page snapshot\nuid=1_0 RootWebArea\n  uid=1_1 button\n"
	line := []byte(`{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":` + jsonStr(body) + `}]}}` + "\n")
	out := TrimSnapshot(line, "wait_for")
	got := ResultText(out)
	if strings.Contains(got, "uid=1_1") || !strings.Contains(got, "Element found") || !strings.Contains(got, "스냅샷 생략") {
		t.Fatalf("잘라내기: %q", got)
	}
	if out[len(out)-1] != '\n' {
		t.Fatal("개행 유지")
	}
	if got := TrimSnapshot(line, "take_snapshot"); string(got) != string(line) {
		t.Fatal("take_snapshot은 그대로")
	}
	noMarker := []byte(`{"id":4,"result":{"content":[{"type":"text","text":"ok"}]}}` + "\n")
	if got := TrimSnapshot(noMarker, "wait_for"); string(got) != string(noMarker) {
		t.Fatal("마커 없으면 그대로")
	}
	if got := TrimSnapshot(line, "navigate_page"); strings.Contains(ResultText(got), "uid=1_1") {
		t.Fatal("navigate_page도 잘라냄")
	}
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
```

(import에 `"encoding/json"` 추가.) `config_test.go`에:

```go
func TestBrowserTrimSnapshotsEnabled(t *testing.T) {
	var c Config
	if !c.BrowserTrimSnapshotsEnabled() {
		t.Fatal("기본 true")
	}
	f := false
	c.BrowserTrimSnapshots = &f
	if c.BrowserTrimSnapshotsEnabled() {
		t.Fatal("false 반영")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/browser -run TestTrimSnapshot -v; go test ./internal/config -run TestBrowserTrimSnapshotsEnabled -v`
Expected: FAIL.

- [ ] **Step 3: 구현**

```go
// internal/browser/trim.go
package browser

import (
	"encoding/json"
	"strings"
)

// 호출 효율(스펙 5절): wait_for·navigate_page 응답에 딸려오는 페이지 전체 스냅샷을 잘라낸다.
// 한 번에 1만 5천 토큰이 들어온 실측(2026-09-22). 필요하면 에이전트가 take_snapshot을 따로 부른다.

var trimTools = map[string]bool{"wait_for": true, "navigate_page": true}

const (
	snapshotMarker = "## Latest page snapshot"
	trimNotice     = "(스냅샷 생략 — 필요하면 take_snapshot)"
)

func TrimSnapshot(line []byte, tool string) []byte {
	if !trimTools[tool] {
		return line
	}
	var msg map[string]any
	if json.Unmarshal(line, &msg) != nil {
		return line
	}
	result, _ := msg["result"].(map[string]any)
	content, _ := result["content"].([]any)
	changed := false
	for _, it := range content {
		item, _ := it.(map[string]any)
		text, _ := item["text"].(string)
		if i := strings.Index(text, snapshotMarker); item["type"] == "text" && i >= 0 {
			item["text"] = text[:i] + trimNotice + "\n"
			changed = true
		}
	}
	if !changed {
		return line
	}
	out, err := json.Marshal(msg)
	if err != nil {
		return line
	}
	return append(out, '\n')
}
```

`config.go`:

```go
	// BrowserTrimSnapshots — wait_for·navigate_page 응답의 자동 스냅샷을 잘라낼지. 기본 true.
	BrowserTrimSnapshots *bool `json:"browser_trim_snapshots,omitempty"`

func (c *Config) BrowserTrimSnapshotsEnabled() bool {
	if c.BrowserTrimSnapshots == nil {
		return true
	}
	return *c.BrowserTrimSnapshots
}
```

`browsercmd.go` 서버→클라이언트 고루틴: `fx.OnServerLine(line)`이 도구명을 알아야 하므로 `OnServerLine`을 `func (f *fxSignaler) OnServerLine(line []byte) (tool string)`로 바꿔 `End`의 도구명을 돌려주게 하고, 그 뒤 `if cfg.BrowserTrimSnapshotsEnabled() { fwd = browser.TrimSnapshot(fwd, tool) }`를 stdout 쓰기 전에 둔다. 순서: `pm.Update(ResultText(line))` → `tool := fx.OnServerLine(line)` → trim → stdout.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/browser -run TestTrimSnapshot -v && go test ./internal/config -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
git add internal/browser/trim.go internal/browser/trim_test.go internal/config/config.go internal/config/config_test.go internal/cli/browsercmd.go
git commit -m "feat(browser): wait_for·navigate_page 응답의 자동 스냅샷 잘라내기(browser_trim_snapshots, 기본 켬)

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 7: 오버레이 콘텐츠 스크립트

**Files:**
- Modify: `internal/browser/fx/content.js` (전면 개정)
- Create: `internal/browser/fx/content_test.mjs`
- Modify: `internal/browser/fx_test.go` (node 있으면 mjs 실행)

**Interfaces:**
- Consumes: 미러 속성 `data-agentlayer-owner` 값 형식(Task 1 `MirrorValue`), 효과 속성 `data-agentlayer-fx` 값 `on:<tool>:<ts>|off:<ts>`(기존), 요청 속성 `data-agentlayer-request`.
- Produces: 콘텐츠 스크립트가 `window.__agentlayerFx = { parseMirror, state }`를 노출(테스트용, 페이지 코드에 영향 없음). 버튼 클릭은 `data-agentlayer-request="user:<ms>|agent:<ms>|stop:<ms>"`.

- [ ] **Step 1: 실패하는 테스트 작성**

```js
// internal/browser/fx/content_test.mjs — node로 DOM을 최소 흉내 내어 콘텐츠 스크립트를 검사한다.
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import assert from 'node:assert/strict';

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'content.js'), 'utf8');

// ---- 최소 DOM 스텁: 속성·자식·스타일·이벤트만
function el(tag) {
  const e = { tagName: tag.toUpperCase(), style: {}, children: [], attrs: {}, listeners: {}, textContent: '', innerHTML: '' };
  e.setAttribute = (k, v) => { e.attrs[k] = String(v); observers.forEach(o => o.target === e && o.cb()); };
  e.getAttribute = (k) => (k in e.attrs ? e.attrs[k] : null);
  e.removeAttribute = (k) => { delete e.attrs[k]; };
  e.appendChild = (c) => { e.children.push(c); c.parentElement = e; return c; };
  e.append = (...cs) => cs.forEach(e.appendChild);
  e.remove = () => { if (e.parentElement) e.parentElement.children = e.parentElement.children.filter(c => c !== e); };
  e.addEventListener = (t, cb) => { (e.listeners[t] ||= []).push(cb); };
  e.click = () => (e.listeners.click || []).forEach(cb => cb({ stopPropagation() {}, preventDefault() {} }));
  e.getBoundingClientRect = () => ({ left: 0, top: 0, width: 10, height: 10 });
  Object.defineProperty(e.style, 'cssText', { set(v) { v.split(';').forEach(kv => { const [k, val] = kv.split(':'); if (k) e.style[k.trim()] = (val || '').trim(); }); }, get() { return ''; } });
  return e;
}
const observers = [];
globalThis.MutationObserver = class { constructor(cb) { this.cb = cb; } observe(t) { this.target = t; observers.push(this); } };
const html = el('html'), body = el('body'); html.appendChild(body);
globalThis.document = { documentElement: html, body, createElement: el, getElementById: () => null };
globalThis.window = { top: null, addEventListener: (t, cb) => { (window.listeners ||= {})[t] ||= []; window.listeners[t].push(cb); }, innerWidth: 1000, innerHeight: 800 };
window.top = window;
globalThis.requestAnimationFrame = (cb) => cb();
globalThis.setTimeout = (cb) => { cb(); return 0; }; // 타이머는 즉시 — LINGER·만료는 별도 함수로 검사
globalThis.clearTimeout = () => {};
globalThis.Date = class extends Date { static now() { return 1_000_000; } };

new Function(src)();
const fx = window.__agentlayerFx;
assert.ok(fx, 'content.js가 window.__agentlayerFx를 노출해야 함');

// ---- parseMirror
const m = fx.parseMirror('agent:claude-%1:JustWatch∶ 신작:1000:2000:0:1:1:탭 제목');
assert.equal(m.owner, 'agent'); assert.equal(m.agent, 'claude-%1'); assert.equal(m.label, 'JustWatch∶ 신작');
assert.equal(m.lastMs, 2000); assert.equal(m.stopped, false); assert.equal(m.waiting, 1); assert.equal(m.target, true); assert.equal(m.title, '탭 제목');
assert.equal(fx.parseMirror('garbage'), null);

// ---- 작업 탭: agent 소유 → 방패·디밍·알약 켜짐, 버튼 문구
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999000:0:0:1:제목');
let s = fx.state();
assert.equal(s.owner, 'agent'); assert.equal(s.target, true);
assert.equal(s.shield, 'auto', '입력 도구 구간 밖에서는 방패가 실제 마우스를 삼킨다');
assert.equal(s.dim, true);
assert.deepEqual(s.buttons, ['내가 조작하기', '중단']);
assert.equal(s.pillStatus, 'AI가 조작 중 · claude-%1');

// ---- 입력 도구 구간: 방패 내림, 커서 라벨
html.setAttribute('data-agentlayer-fx', 'on:click:1');
s = fx.state();
assert.equal(s.shield, 'none', '클릭 구간엔 방패를 내려 CDP 입력이 닿게');
assert.equal(s.cursorLabel, '클릭');
html.setAttribute('data-agentlayer-fx', 'on:take_snapshot:2');
s = fx.state();
assert.equal(s.shield, 'auto', '읽기 도구 구간엔 방패 유지');
assert.equal(s.cursorLabel, '읽는 중');
html.setAttribute('data-agentlayer-fx', 'off:3');

// ---- 커서는 입력 구간의 마우스 이벤트만 따라감
html.setAttribute('data-agentlayer-fx', 'on:click:4');
window.listeners.mousemove.forEach(cb => cb({ clientX: 50, clientY: 60 }));
assert.deepEqual(fx.state().cursor, { x: 50, y: 60 });
html.setAttribute('data-agentlayer-fx', 'off:5');
window.listeners.mousemove.forEach(cb => cb({ clientX: 500, clientY: 600 }));
assert.deepEqual(fx.state().cursor, { x: 50, y: 60 }, '구간 밖(사용자 마우스)은 무시');

// ---- 버튼 클릭 → 요청 속성
fx.clickButton('내가 조작하기');
assert.match(html.getAttribute('data-agentlayer-request'), /^user:\d+$/);
fx.clickButton('중단');
assert.match(html.getAttribute('data-agentlayer-request'), /^stop:\d+$/);

// ---- user 소유: 방패 없음, 돌려주기 버튼, 대기 수
html.setAttribute('data-agentlayer-owner', 'user:claude-%1:검색:1000:999000:0:2:1:제목');
s = fx.state();
assert.equal(s.shield, 'none'); assert.equal(s.dim, false);
assert.deepEqual(s.buttons, ['AI에게 돌려주기']);
assert.equal(s.pillStatus, '내가 조작 중 · 대기 중인 호출 2');
html.setAttribute('data-agentlayer-owner', 'user:claude-%1:검색:1000:999000:1:0:1:제목');
assert.equal(fx.state().pillStatus, '중단됨');
fx.clickButton('AI에게 돌려주기');
assert.match(html.getAttribute('data-agentlayer-request'), /^agent:\d+$/);

// ---- 타 탭(target=0): 띠만
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:999000:0:0:0:JustWatch 신작');
s = fx.state();
assert.equal(s.shield, 'none'); assert.equal(s.dim, false); assert.equal(s.pill, false);
assert.equal(s.banner, 'AI가 다른 탭에서 작업 중 · JustWatch 신작');

// ---- 자체 만료: last_ms + 20초 지나면 idle로 본다(프록시가 다 꺼진 경우)
html.setAttribute('data-agentlayer-owner', 'agent:claude-%1:검색:1000:900000:0:0:1:제목'); // now=1,000,000 → 100초 경과
s = fx.state();
assert.equal(s.owner, 'idle'); assert.equal(s.shield, 'none'); assert.equal(s.pill, false);

// ---- idle
html.setAttribute('data-agentlayer-owner', 'idle:::0:0:0:0:0:');
s = fx.state();
assert.equal(s.owner, 'idle'); assert.equal(s.banner, '');

console.log('content_test.mjs OK');
```

`fx_test.go`에 추가:

```go
func TestContentScriptWithNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node 없음")
	}
	cmd := exec.Command(node, "fx/content_test.mjs")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("content_test.mjs 실패: %v\n%s", err, out)
	}
}
```

(import `"os/exec"`.)

- [ ] **Step 2: 실패 확인**

Run: `cd internal/browser && node fx/content_test.mjs; cd ../.. && go test ./internal/browser -run TestContentScriptWithNode -v`
Expected: FAIL — `window.__agentlayerFx` 없음.

- [ ] **Step 3: 구현** — `content.js`를 아래로 교체.

```js
// AgentLayer FX — 에이전트 브라우저 오버레이(스펙 3절).
// 신호 셋: data-agentlayer-fx(도구 구간 on/off, 프록시가 씀), data-agentlayer-owner(소유권 미러, 프록시가 씀),
// data-agentlayer-request(버튼 요청, 여기서 씀 → 프록시가 회수).
(() => {
  if (window.top !== window) return;
  const FX = 'data-agentlayer-fx', OWNER = 'data-agentlayer-owner', REQ = 'data-agentlayer-request';
  const ACCENT = '217,119,87', TERRA = '#d97757', CREAM = '#faf9f5', INK = '#1f1e1d';
  const LINGER = 2500, EXPIRY = 20000;
  const INPUT = new Set(['click', 'hover', 'drag', 'fill', 'fill_form', 'type_text', 'press_key', 'upload_file']);
  const LABELS = { click: '클릭', hover: '가리키는 중', fill: '입력 중', fill_form: '입력 중', type_text: '입력 중', press_key: '입력 중',
    upload_file: '입력 중', drag: '끌기', navigate_page: '이동 중', new_page: '이동 중', take_snapshot: '읽는 중', take_screenshot: '읽는 중',
    wait_for: '기다리는 중', evaluate_script: '확인 중' };

  const st = { owner: 'idle', agent: '', label: '', lastMs: 0, stopped: false, waiting: 0, target: false, title: '',
    fxOn: false, tool: '', cursor: null, offTimer: 0 };

  const parseMirror = (v) => {
    if (!v) return null;
    const p = v.split(':');
    if (p.length < 9) return null;
    const [owner, agent, label, sinceMs, lastMs, stopped, waiting, target] = p;
    if (!['agent', 'user', 'idle'].includes(owner)) return null;
    return { owner, agent, label, sinceMs: +sinceMs, lastMs: +lastMs, stopped: stopped === '1', waiting: +waiting || 0, target: target === '1', title: p.slice(8).join(':') };
  };

  // 유효 소유권 — 프록시가 전부 꺼지면 아무도 미러를 안 고치므로 스스로 만료를 계산한다.
  const effOwner = () => (st.owner === 'agent' && Date.now() - st.lastMs >= EXPIRY) ? 'idle' : st.owner;

  // ---- DOM
  let host, shield, dim, glow, hl, cursor, cursorLabel, pill, pillTitle, pillStatus, pillBtns, banner;
  const css = (e, s) => { e.style.cssText = s; return e; };
  const btn = (text) => {
    const b = css(document.createElement('button'),
      `pointer-events:auto;cursor:pointer;border:0;border-radius:999px;padding:6px 12px;margin-left:6px;` +
      `font:600 12px/16px -apple-system,system-ui,sans-serif;background:${text === '중단' ? 'rgba(255,255,255,.12)' : CREAM};color:${text === '중단' ? CREAM : INK}`);
    b.textContent = text;
    b.setAttribute('type', 'button');
    b.addEventListener('click', (e) => { e.stopPropagation(); e.preventDefault(); request(text); });
    return b;
  };
  const ensure = () => {
    if (host) return;
    host = css(document.createElement('div'), 'position:fixed;inset:0;pointer-events:none;z-index:2147483647;contain:strict;');
    host.id = '__agentlayer_fx';
    host.setAttribute('aria-hidden', 'true'); // take_snapshot에 안 섞이게. inert는 붙이지 않는다(버튼이 눌려야 함)
    shield = css(document.createElement('div'), 'position:absolute;inset:0;pointer-events:none;');
    dim = css(document.createElement('div'),
      `position:absolute;inset:0;opacity:0;transition:opacity .35s ease;background:` +
      `radial-gradient(rgba(255,255,255,.06) 1px, transparent 1.2px) 0 0/14px 14px, rgba(0,0,0,.35);`);
    glow = css(document.createElement('div'),
      `position:absolute;inset:0;opacity:0;transition:opacity .35s ease;` +
      `box-shadow:inset 0 0 0 3px rgba(${ACCENT},.95),inset 0 0 60px rgba(${ACCENT},.45),inset 0 0 160px rgba(${ACCENT},.2);`);
    hl = css(document.createElement('div'),
      `position:absolute;opacity:0;transition:opacity .25s ease;border-radius:6px;box-shadow:0 0 0 2px rgba(${ACCENT},.9),0 0 14px rgba(${ACCENT},.45);`);
    cursor = css(document.createElement('div'),
      'position:absolute;left:0;top:0;width:0;height:0;opacity:0;transition:transform .25s cubic-bezier(.2,.8,.2,1),opacity .3s ease;will-change:transform;');
    cursor.innerHTML =
      `<svg width="30" height="36" viewBox="0 0 22 26" style="position:absolute;left:-3px;top:-3px;filter:drop-shadow(0 1px 2px rgba(0,0,0,.45))">` +
      `<path d="M3 2 L19 13 L11.5 14.5 L15.5 23 L12.5 24.2 L8.5 15.8 L3 21 Z" fill="${CREAM}" stroke="${INK}" stroke-width="1.4" stroke-linejoin="round"/></svg>`;
    cursorLabel = css(document.createElement('div'),
      `position:absolute;left:24px;top:28px;padding:3px 9px;border-radius:999px;background:${TERRA};color:${CREAM};` +
      `font:700 11px/15px -apple-system,system-ui,sans-serif;box-shadow:0 1px 3px rgba(0,0,0,.4);white-space:nowrap`);
    cursor.appendChild(cursorLabel);
    pill = css(document.createElement('div'),
      `position:absolute;left:50%;bottom:22px;transform:translate(-50%,8px);opacity:0;transition:opacity .3s ease,transform .3s ease;` +
      `display:flex;align-items:center;gap:10px;padding:10px 12px 10px 16px;border-radius:16px;background:rgba(31,30,29,.92);color:${CREAM};` +
      `font:500 12px/16px -apple-system,system-ui,sans-serif;box-shadow:0 8px 28px rgba(0,0,0,.45);white-space:nowrap;backdrop-filter:blur(8px)`);
    const spin = css(document.createElement('div'),
      `width:10px;height:10px;border-radius:50%;background:${TERRA};box-shadow:0 0 0 4px rgba(${ACCENT},.25);animation:__alpulse 1.2s ease-in-out infinite`);
    const txt = css(document.createElement('div'), 'display:flex;flex-direction:column;gap:2px');
    pillTitle = css(document.createElement('div'), 'font-weight:700;font-size:13px');
    pillStatus = css(document.createElement('div'), `color:${TERRA};font-weight:600`);
    txt.append(pillTitle, pillStatus);
    pillBtns = css(document.createElement('div'), 'display:flex;align-items:center;margin-left:6px');
    pill.append(spin, txt, pillBtns);
    banner = css(document.createElement('div'),
      `position:absolute;left:0;right:0;top:0;height:28px;opacity:0;transition:opacity .3s ease;display:flex;align-items:center;justify-content:center;` +
      `background:${TERRA};color:${CREAM};font:600 12px/16px -apple-system,system-ui,sans-serif;box-shadow:0 2px 8px rgba(0,0,0,.35)`);
    const style = document.createElement('style');
    style.textContent = '@keyframes __alpulse{0%,100%{transform:scale(1)}50%{transform:scale(1.35)}}';
    host.append(style, shield, dim, glow, hl, cursor, pill, banner);
    (document.body || document.documentElement).appendChild(host);
  };

  const request = (text) => {
    const kind = text === '내가 조작하기' ? 'user' : text === '중단' ? 'stop' : 'agent';
    document.documentElement.setAttribute(REQ, `${kind}:${Date.now()}`);
  };

  // ---- 표시 계산(테스트가 같은 함수를 본다)
  const compute = () => {
    const owner = effOwner();
    const active = owner === 'agent' && st.target;
    const inputPhase = st.fxOn && INPUT.has(st.tool);
    const s = { owner, target: st.target, shield: 'none', dim: false, pill: false, banner: '', buttons: [], pillStatus: '', cursorLabel: '', cursor: st.cursor };
    if (active) {
      s.shield = inputPhase ? 'none' : 'auto';
      s.dim = true; s.pill = true;
      s.buttons = ['내가 조작하기', '중단'];
      s.pillStatus = `AI가 조작 중 · ${st.agent}`;
      s.cursorLabel = st.fxOn ? (LABELS[st.tool] || '작업 중') : '';
    } else if (owner === 'user' && st.target) {
      s.pill = true;
      s.buttons = ['AI에게 돌려주기'];
      s.pillStatus = st.stopped ? '중단됨' : `내가 조작 중 · 대기 중인 호출 ${st.waiting}`;
    } else if (owner !== 'idle' && !st.target) {
      s.banner = `AI가 다른 탭에서 작업 중 · ${st.title}`;
    }
    return s;
  };

  const render = () => {
    ensure();
    const s = compute();
    shield.style.pointerEvents = s.shield;
    dim.style.opacity = s.dim ? '1' : '0';
    glow.style.opacity = s.dim ? '1' : '0';
    pill.style.opacity = s.pill ? '1' : '0';
    pill.style.transform = s.pill ? 'translate(-50%,0)' : 'translate(-50%,8px)';
    pillTitle.textContent = st.label || '브라우저 작업';
    pillStatus.textContent = s.pillStatus;
    pillBtns.innerHTML = '';
    pillBtns.children.length = 0;
    s.buttons.forEach(b => pillBtns.appendChild(btn(b)));
    banner.textContent = s.banner;
    banner.style.opacity = s.banner ? '1' : '0';
    cursorLabel.textContent = s.cursorLabel;
    cursor.style.opacity = (s.owner === 'agent' && st.target && st.cursor) ? '1' : '0';
    if (!s.dim) hl.style.opacity = '0';
  };

  const moveTo = (x, y) => { st.cursor = { x, y }; ensure(); cursor.style.transform = `translate(${x}px,${y}px)`; render(); };
  const ripple = (x, y) => {
    ensure();
    const r = css(document.createElement('div'),
      `position:absolute;left:${x - 18}px;top:${y - 18}px;width:36px;height:36px;border-radius:50%;border:3px solid rgba(${ACCENT},.95);` +
      'transform:scale(.3);opacity:.95;transition:transform .6s ease-out,opacity .6s ease-out;');
    host.appendChild(r);
    requestAnimationFrame(() => { r.style.transform = 'scale(1.7)'; r.style.opacity = '0'; });
    setTimeout(() => r.remove(), 700);
  };
  const highlight = (el) => {
    if (!(el instanceof Element) || el === document.body || el === document.documentElement) return;
    const b = el.getBoundingClientRect();
    if (!b.width || !b.height) return;
    ensure();
    hl.style.left = b.left + 'px'; hl.style.top = b.top + 'px'; hl.style.width = b.width + 'px'; hl.style.height = b.height + 'px';
    hl.style.opacity = '1';
    setTimeout(() => { hl.style.opacity = '0'; }, 1500);
  };

  const root = document.documentElement;
  const readOwner = () => {
    const m = parseMirror(root.getAttribute(OWNER));
    if (!m) { st.owner = 'idle'; st.target = false; return; }
    Object.assign(st, m);
  };
  const readFx = () => {
    const v = root.getAttribute(FX) || '';
    clearTimeout(st.offTimer);
    if (v.startsWith('on:')) { st.fxOn = true; st.tool = v.split(':')[1] || ''; }
    else if (v.startsWith('off:')) { st.offTimer = setTimeout(() => { st.fxOn = false; st.tool = ''; render(); }, LINGER); }
  };
  new MutationObserver(() => { readOwner(); readFx(); render(); }).observe(root, { attributes: true, attributeFilter: [OWNER, FX] });
  readOwner(); readFx();

  const inputPhase = () => st.fxOn && INPUT.has(st.tool) && effOwner() === 'agent' && st.target;
  window.addEventListener('mousemove', (e) => { if (inputPhase()) moveTo(e.clientX, e.clientY); }, true);
  window.addEventListener('mousedown', (e) => { if (inputPhase()) { moveTo(e.clientX, e.clientY); ripple(e.clientX, e.clientY); } }, true);
  window.addEventListener('focusin', (e) => { if (inputPhase()) highlight(e.target); }, true);
  window.addEventListener('scroll', () => { if (hl) hl.style.opacity = '0'; }, true);
  // 만료 자체 계산 — 프록시가 없을 때도 20초 뒤 내려가게 5초마다 다시 그린다
  setInterval(() => { if (st.owner !== 'idle') render(); }, 5000);

  window.__agentlayerFx = { parseMirror, state: () => (readOwner(), readFx(), compute()), clickButton: (t) => request(t) };
})();
```

주의: 테스트 스텁에서 `setInterval`이 없으므로 `if (typeof setInterval === 'function')`로 감싼다. `Element`도 스텁에 없으니 `highlight`의 `instanceof Element`는 `typeof Element !== 'undefined' && el instanceof Element`로 쓴다. `pillBtns.innerHTML=''; pillBtns.children.length = 0`은 스텁·실DOM 모두에서 자식 비우기가 되게 한 것이다(실DOM은 innerHTML로 충분, 스텁은 children 배열).

- [ ] **Step 4: 통과 확인**

Run: `cd internal/browser && node fx/content_test.mjs && cd ../.. && go test ./internal/browser -run 'TestContentScriptWithNode|TestInstallFx' -v`
Expected: `content_test.mjs OK`, PASS.

- [ ] **Step 5: 실브라우저 눈 확인** — `make install` 뒤 에이전트 브라우저를 종료했다가(다음 기동부터 새 확장) 클로드 세션에서 `new_page`·`click`을 한 번 돌리며 작업 탭에 방패·디밍·알약·커서 라벨이, 다른 탭에 띠만 뜨는지 본다. 어긋나면 `content.js`만 고치고 Step 4를 다시 돈다.

- [ ] **Step 6: 커밋**

```bash
git add internal/browser/fx/content.js internal/browser/fx/content_test.mjs internal/browser/fx_test.go
git commit -m "feat(browser-fx): 오버레이 개정 — 작업 탭만 방패·디밍·커서 라벨·제어권 알약, 타 탭은 띠, 자체 만료

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 8: 행 감시 `hangwatch.go` + 자동 재시작 + `--disable-hang-monitor` 제거

**Files:**
- Create: `internal/browser/hangwatch.go`
- Modify: `internal/browser/instance.go:161-175` (`Delete("disable-hang-monitor")`), `internal/browser/instance_test.go`
- Modify: `internal/cli/browsercmd.go` (`browserAutoPreview` 끝)
- Test: `internal/browser/hangwatch_test.go`

**Interfaces:**
- Produces:
  - `type HangOps struct{ Ping func() error; Sample func(pid int, out string) error; Kill func(pid int) error; Relaunch func() error; Notify func(msg string) }`
  - `type hangState struct{ Fails int; LastAt time.Time }` (파일 `<dir>/hangwatch.json`)
  - `func HangWatch(dir string, pid int, ops HangOps, now time.Time) (restarted bool, diag string)` — 판정·채집·재시작
  - `func PingUI(b *rod.Browser, timeout time.Duration) error` — 첫 웹 탭에 `Browser.getWindowForTarget`
  - `func DefaultHangOps(stateDir string, port int, goos string, notify func(string)) HangOps`
  - `const hangPing = 3 * time.Second`, `hangMinGap = 10 * time.Second`, `hangFailsNeeded = 2`

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/browser/hangwatch_test.go
package browser

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type hangLog struct{ sampled, killed, relaunched int; notes []string }

func fakeHangOps(l *hangLog, pingErr error) HangOps {
	return HangOps{
		Ping:     func() error { return pingErr },
		Sample:   func(pid int, out string) error { l.sampled++; return nil },
		Kill:     func(pid int) error { l.killed++; return nil },
		Relaunch: func() error { l.relaunched++; return nil },
		Notify:   func(m string) { l.notes = append(l.notes, m) },
	}
}

func TestHangWatchHealthyResetsFails(t *testing.T) {
	dir := t.TempDir()
	var l hangLog
	now := time.Now()
	saveHangState(dir, hangState{Fails: 1, LastAt: now.Add(-time.Minute)})
	restarted, _ := HangWatch(dir, 100, fakeHangOps(&l, nil), now)
	if restarted || l.killed != 0 || loadHangState(dir).Fails != 0 {
		t.Fatal("정상이면 실패 횟수 0으로")
	}
}

func TestHangWatchNeedsTwoFailsTenSecondsApart(t *testing.T) {
	dir := t.TempDir()
	var l hangLog
	now := time.Now()
	ops := fakeHangOps(&l, errors.New("timeout"))
	if r, _ := HangWatch(dir, 100, ops, now); r || l.killed != 0 {
		t.Fatal("1회 실패로 재시작하면 안 됨")
	}
	if r, _ := HangWatch(dir, 100, ops, now.Add(3*time.Second)); r || l.killed != 0 {
		t.Fatal("10초 안 지난 재판정은 세지 않음")
	}
	if loadHangState(dir).Fails != 1 {
		t.Fatalf("fails=%d", loadHangState(dir).Fails)
	}
	r, diag := HangWatch(dir, 100, ops, now.Add(11*time.Second))
	if !r || l.sampled != 1 || l.killed != 1 || l.relaunched != 1 {
		t.Fatalf("2회 연속 실패면 채집·종료·재기동: r=%v %+v", r, l)
	}
	if !strings.Contains(diag, "hang/") || len(l.notes) != 1 || !strings.Contains(l.notes[0], "재시작") {
		t.Fatalf("진단 경로·알림: %q %v", diag, l.notes)
	}
	if loadHangState(dir).Fails != 0 {
		t.Fatal("재시작 뒤 실패 횟수 0")
	}
}

func TestHangWatchNoPid(t *testing.T) {
	var l hangLog
	if r, _ := HangWatch(t.TempDir(), 0, fakeHangOps(&l, errors.New("x")), time.Now()); r || l.killed != 0 {
		t.Fatal("pid 0이면 아무것도 안 함")
	}
}
```

`instance_test.go`에 추가:

```go
func TestLaunchArgsDropHangMonitorFlag(t *testing.T) {
	for _, a := range launchArgs("/bin/chrome", "/tmp/p", 9222, "") {
		if strings.Contains(a, "disable-hang-monitor") {
			t.Fatalf("hang monitor는 켜 둔다: %s", a)
		}
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/browser -run 'TestHangWatch|TestLaunchArgsDropHangMonitorFlag' -v`
Expected: FAIL.

- [ ] **Step 3: 구현**

```go
// internal/browser/hangwatch.go
package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// 행 감시(스펙 4절): 창 전체가 굳어 강제 종료만 통하던 증상. 원인 미상이라 감지·채집·재시작까지.
const (
	hangPing        = 3 * time.Second
	hangMinGap      = 10 * time.Second
	hangFailsNeeded = 2
)

type HangOps struct {
	Ping     func() error              // UI 스레드를 타는 CDP 호출(타임아웃 포함)
	Sample   func(pid int, out string) error
	Kill     func(pid int) error
	Relaunch func() error
	Notify   func(msg string)
}

type hangState struct {
	Fails  int       `json:"fails"`
	LastAt time.Time `json:"last_at"`
}

func hangStatePath(dir string) string { return filepath.Join(dir, "hangwatch.json") }

func loadHangState(dir string) hangState {
	var s hangState
	if b, err := os.ReadFile(hangStatePath(dir)); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

func saveHangState(dir string, s hangState) {
	b, _ := json.Marshal(s)
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(hangStatePath(dir), b, 0o600)
}

// HangWatch — ping 실패가 hangMinGap 이상 떨어져 hangFailsNeeded번 이어지면 행으로 확정하고
// 채집 → 강제 종료 → 재기동 → 알림. 돌려주는 diag는 채집 파일 경로(없으면 "").
func HangWatch(dir string, pid int, ops HangOps, now time.Time) (bool, string) {
	if pid <= 0 {
		return false, ""
	}
	s := loadHangState(dir)
	if ops.Ping() == nil {
		if s.Fails != 0 {
			saveHangState(dir, hangState{})
		}
		return false, ""
	}
	if s.Fails > 0 && now.Sub(s.LastAt) < hangMinGap {
		return false, "" // 너무 이른 재판정은 세지 않는다(훅이 잦다)
	}
	s.Fails++
	s.LastAt = now
	if s.Fails < hangFailsNeeded {
		saveHangState(dir, s)
		return false, ""
	}
	diag := filepath.Join(dir, "hang", now.Format("20060102-150405")+".txt")
	_ = os.MkdirAll(filepath.Dir(diag), 0o755)
	if err := ops.Sample(pid, diag); err != nil {
		diag = ""
	}
	_ = ops.Kill(pid)
	_ = ops.Relaunch()
	saveHangState(dir, hangState{})
	msg := "에이전트 브라우저가 멈춰 재시작했습니다"
	if diag != "" {
		msg += " · 진단: " + diag
	}
	if ops.Notify != nil {
		ops.Notify(msg)
	}
	return true, diag
}

// PingUI — 첫 웹 탭의 창 정보를 묻는다(UI 스레드 경유). 웹 탭이 없으면 Browser.getVersion.
func PingUI(b *rod.Browser, timeout time.Duration) error {
	bt := b.Timeout(timeout)
	pages, err := bt.Pages()
	if err != nil {
		return err
	}
	for _, p := range pages {
		if info, err := p.Info(); err == nil && IsWebURL(info.URL) {
			_, err = proto.BrowserGetWindowForTarget{TargetID: p.TargetID}.Call(p.Timeout(timeout))
			return err
		}
	}
	_, err = proto.BrowserGetVersion{}.Call(bt)
	return err
}

// DefaultHangOps — 실제 수단. Sample은 macOS `sample`만(리눅스는 생략해 diag "").
func DefaultHangOps(stateDir string, port int, goos string, notify func(string)) HangOps {
	return HangOps{
		Ping: func() error {
			b, err := Connect(stateDir, port)
			if err != nil {
				return err
			}
			return PingUI(b, hangPing)
		},
		Sample: func(pid int, out string) error {
			if goos != "darwin" {
				return fmt.Errorf("sample 없음")
			}
			return exec.Command("sample", fmt.Sprint(pid), "2", "-file", out).Run()
		},
		Kill: func(pid int) error {
			RemoveInstance(stateDir)
			_ = syscall.Kill(-pid, syscall.SIGKILL) // 프로세스 그룹
			return syscall.Kill(pid, syscall.SIGKILL)
		},
		Relaunch: func() error { _, err := Connect(stateDir, port); return err },
		Notify:   notify,
	}
}
```

`instance.go newLauncher`에 `.Delete("disable-hang-monitor"). // 렌더러 행 때 "페이지 응답 없음" 안내가 뜨게(스펙 4절)` 추가.

`browsercmd.go browserAutoPreview` 끝(capture-janitor 블록 뒤)에:

```go
	// 행 감시(스펙 4절) — 브라우저 프로세스는 있는데 UI 스레드가 3초 안에 답이 없는 게 10초 간격으로
	// 두 번이면 채집·강제 종료·재기동·알림.
	if browser.ThrottleOK(state.DefaultDir(), "hangwatch", 10*time.Second, time.Now()) {
		if pid := browser.ChromePID(port, browser.ExecLsof); pid > 0 {
			notify := func(msg string) {
				if url := cfg.NotifyURL(); url != "" && cfg.NotifyDiscord {
					payload, _ := json.Marshal(map[string]any{"username": "agentlayer", "content": msg})
					_ = notifypkg.DefaultSender().PostJSON(url, payload)
				}
			}
			if restarted, diag := browser.HangWatch(state.DefaultDir(), pid, browser.DefaultHangOps(state.DefaultDir(), port, runtime.GOOS, notify), time.Now()); restarted {
				fmt.Fprintf(out, "에이전트 브라우저 재시작(행 감지) 진단: %s\n", diag)
			}
		}
	}
```

(`notifypkg "github.com/netwaif/agentlayer/internal/notify"` import; `ChromePID`는 `capturelock.go`에 이미 있음.) 주의: `Ping`이 `Connect`로 실브라우저에 붙는데 브라우저가 굳어 있으면 `Connect` 자체가 rod 기본 타임아웃까지 걸릴 수 있다 — `Ping` 안에서 `Connect`는 `LoadInstance`의 WS로 `newBrowser(ws).Timeout(hangPing).Connect()`를 직접 써서 3초를 넘기지 않게 한다(위 `Connect` 호출을 그렇게 바꾼다).

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/browser -run 'TestHangWatch|TestLaunchArgs' -v && go build ./... && go vet ./internal/browser ./internal/cli`
Expected: PASS.

- [ ] **Step 5: 실측** — `make install` 뒤 에이전트 브라우저 pid에 `kill -STOP <pid>`, 아무 클로드 세션에서 훅이 두 번 돌 만큼(예: 프롬프트 두 번, 10초 이상 간격) 기다린 뒤 브라우저가 새 pid로 떠 있고 `~/.local/state/agentlayer/hang/` 에 파일이 있고 알림이 왔는지 확인. 이후 `kill -CONT`는 필요 없다(이미 죽임).

- [ ] **Step 6: 커밋**

```bash
git add internal/browser/hangwatch.go internal/browser/hangwatch_test.go internal/browser/instance.go internal/browser/instance_test.go internal/cli/browsercmd.go
git commit -m "feat(browser): 행 감시 — UI ping 3초×연속 2회면 sample 채집·강제 종료·재기동·알림, hang monitor 플래그 복원

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 9: 스킬 문안 · README · 릴리즈

**Files:**
- Modify: `internal/cli/skills.go` (`agentBrowserSkill` 상수)
- Modify: `README.md:219-275` (에이전트 전용 브라우저 절)
- Test: 기존 `internal/cli` 스킬 테스트(`go test ./internal/cli -run Skill`)

- [ ] **Step 1: 스킬 문안에 절 추가** — `agentBrowserSkill`의 「공통 규칙」 아래에:

```markdown
## 제어권·작업 탭 (v1.7.0+)

- 도구를 부르면 그 탭이 **작업 탭**이 되어 어둡게 덮이고 AI 커서·하단 알약이 뜬다. 다른 탭에는 얇은 띠만 보이고 사용자가 자유롭게 쓴다. 20초 동안 호출이 없으면 저절로 내려간다.
- 사용자가 알약의 「내가 조작하기」를 누르면 도구 호출이 **잡혀서 기다린다**(최대 2분). 「AI에게 돌려주기」를 누르면 이어서 실행된다. 기다림은 프록시가 하므로 에이전트는 아무것도 안 해도 된다.
- 응답에 "사용자가 브라우저를 직접 조작 중" 또는 "중단시켰습니다"가 오면 **재시도하지 말고** 사용자에게 한 줄로 알린 뒤 지시를 기다린다.
- 브라우저가 앞에 있지 않으면 `new_page`는 배경 탭으로 열린다(사용자 화면을 뺏지 않는다). 사용자가 보게 하려면 URL을 한 줄로 찍어 준다.

## 호출 효율

- 여러 단계로 데이터를 뽑을 땐 `evaluate_script` 하나 안에서 기다렸다가(폴링) 추출한다. `wait_for`·`navigate_page` 응답의 페이지 스냅샷은 잘려서 오므로(토큰 절약) 구조가 필요하면 `take_snapshot`을 따로 부른다.
```

- [ ] **Step 2: README 갱신** — 「에이전트 전용 브라우저」 절의 `browser_fx` 문단 뒤에 제어권·작업 탭·배경 동작·행 감시·스냅샷 잘라내기 다섯 항목을 각 두세 문장으로 추가하고, 설정 표에 `browser_control_wait_seconds`(기본 120)·`browser_trim_snapshots`(기본 true)를 넣는다. 문안은 Step 1 스킬 문안을 사용자 관점으로 옮긴다(버튼 이름 그대로).

- [ ] **Step 3: 테스트·빌드 전체(패키지별)**

Run: `go vet ./... && for p in browser cli config hookcmd state scan task board; do go test ./internal/$p || exit 1; done`
Expected: 전부 ok.

- [ ] **Step 4: 커밋**

```bash
git add internal/cli/skills.go README.md
git commit -m "docs: agent-browser 스킬·README — 제어권 버튼·작업 탭·배경 동작·행 감시·스냅샷 잘라내기

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

- [ ] **Step 5: 실측 체크리스트(스펙 6절)** — `make install` → 에이전트 브라우저 종료 후 재기동 → 항목 ⓪~⑦을 순서대로 확인하고 결과를 SESSION.md 결정 기록에 한 줄로 남긴다. 어긋난 항목은 해당 Task로 돌아가 고친다.

- [ ] **Step 6: 릴리즈** — `git push origin main && git tag -a v1.7.0 -m "v1.7.0: 에이전트 브라우저 제어권·오버레이·행 감시·호출 효율" && git push origin v1.7.0`, 릴리즈 노트를 scratchpad에 Write로 만든 뒤 `export GITHUB_TOKEN=$(gh auth token); goreleaser release --clean --release-notes <경로>` (분류기 차단 시 사용자 승인 요청). `agentlayer init`을 한 번 돌려 스킬 파일이 갱신되는지 확인.

---

## 자체 검토

**스펙 대조**: 1절 상태 기계→Task 1 / 2절 게이트·작업 탭 특정→Task 2·3·4, 배경 동작→Task 5, 오버헤드 예산(병렬·300ms·한 왕복)→Task 3 / 3절 오버레이→Task 7 / 4절 행 감시·플래그→Task 8 / 5절 스냅샷 잘라내기·스킬 규칙→Task 6·9 / 6절 테스트→각 Task Step 1 + Task 9 Step 5 / 7절 범위 밖은 계획에 없음(맞음). `Waiting` 표시(스펙 3절 "대기 중인 호출 N")→Task 4 `step(1)`이 파일에 1을 쓰고 Task 7이 표시.

**형 일관성**: `browser.SignalFx(b, tool, on, targetURL)` 4인자(Task 3 정의, Task 4 사용) / `FxTracker.End` → `(string, bool)`(Task 3 정의, Task 4·6 사용) / `browser.Control`·`Event` 이름(Task 1 정의, Task 3·4 사용) / `PageMap.URLFor` → `(string, bool)`이며 `fxSignaler.setTarget(url string, ok bool)`이 그 두 값을 받음(Task 4) / `HangOps` 필드명(Task 8 정의·테스트 일치) / `RewriteNewPage(line, browserInFront bool)`(Task 5).

**자리표시자**: 없음. Task 4 `continue` 처리는 "handled 플래그"로 명시. Task 5 `frontSleep` 변수는 본문에 명시.
