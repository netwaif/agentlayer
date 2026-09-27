# 총괄 수신함 채널화 (agentlayer channel serve) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 총괄 세션이 수신함 편지를 Monitor 폴링 대신 Claude Code 채널(MCP 알림)로 받게 하고, `--help` 오해석·자식 세션 오보고 버그를 고친다.

**Architecture:** agentlayer에 stdio MCP 서버 `channel serve <inbox>`를 추가한다(수신원 = 기존 `task.Watch`+`RunRemotePolling`, 전달기 = `notifications/claude/channel`). folder-bot이 봇 기동 명령에 개발 채널 플래그를 붙이고 확인창을 Enter 한 번으로 넘긴다. ai-company가 회사 폴더 local 스코프에 서버를 등록하고 총괄 지침의 Monitor 절을 채널 절로 바꾼다.

**Tech Stack:** Go 1.25(표준 라이브러리만, 외부 의존 추가 없음) · bash(bot-up.sh) · Python 3(botctl.py·companyctl.py, 표준 라이브러리) · pytest.

**Spec:** `docs/superpowers/specs/2026-09-27-inbox-channel-design.md`

## Global Constraints

- agentlayer 새 의존성 금지(`go.mod` 변경 없음). `go test ./...` 전체 실행 금지 — 패키지별로 한 번에 하나(`go test ./internal/channel/` 등). 이유: 메모리 규칙(ui·usage·wiring·wt가 멈춤).
- 채널 알림 규격: method `notifications/claude/channel`, `params.content`는 string, `params.meta`는 string→string, meta 키는 `^[a-zA-Z_][a-zA-Z0-9_]*$`.
- 서버 stdout은 프로토콜 전용(JSON 한 줄씩). 사람이 읽는 로그는 전부 stderr, 접두 `agentlayer channel: `.
- 서버 이름은 `agentlayer`(등록명·`source=` 값·`server:agentlayer` 플래그 모두 동일).
- 등록은 local 스코프(`claude mcp add -s local agentlayer -- <agentlayer> channel serve <inbox>`). `.mcp.json` 금지.
- 훅은 에이전트를 막지 않는다: 훅 경로 추가 작업은 수십 ms 안(ps 한 번).
- 세 레포 순서: agentlayer(main 브랜치에서 직접, worktree 금지) → folder-bot → ai-company → 실회사 전환.
- 버전: agentlayer v1.10.0 / folder-bot 0.1.22 / ai-company 0.2.9(`MIN_AGENTLAYER` 1.10.0).
- 커밋 메시지 끝에 세션 attribution 두 줄(Co-Authored-By·Claude-Session)을 붙인다.

## Review Focus

1. **initialize 전에 도착한 편지**: 서버가 뜨자마자 Poll이 pending을 비우는데 Claude는 아직 initialize를 안 보냈다 → 알림이 큐에 쌓였다가 initialize 응답 직후 순서대로 나가야 한다(Task 1 테스트 `TestNotifyBeforeInitializeIsQueued`).
2. **stdout이 닫힌 뒤(세션 종료)**: 쓰기 실패·EOF에서 서버가 즉시 끝나고 goroutine이 남지 않아야 한다(Task 1 `TestRunStopsOnEOF`, Task 2 `TestRunChannelStopsWhenStdinCloses`).
3. **`-h`가 메시지 본문인 경우**: `send`·`broadcast`에는 help 가로채기를 적용하지 않는다(Task 4 `TestHelpFlagOnlyForListedSubcommands`).
4. **재시작으로 session_id만 바뀐 정상 세션**: 조상 사슬에 claude가 하나뿐이면 보고가 계속 나가야 한다(Task 5 `TestNestedClaudeSingleAncestor`).
5. **확인창이 60초 안에 안 뜬 기동**: bot-up이 Enter를 보내지 않고 claude를 그대로 두며 로그만 남긴다(Task 7 셸 테스트 `no_dialog` 케이스).

---

## Part A — agentlayer (v1.10.0)

### Task 1: `internal/channel` — stdio JSON-RPC 채널 서버

**Files:**
- Create: `internal/channel/server.go`
- Create: `internal/channel/server_test.go`

**Interfaces:**
- Produces:
  - `const Name = "agentlayer"`
  - `type Notification struct { Content string; Meta map[string]string }`
  - `func New(in io.Reader, out io.Writer, version, instructions string) *Server`
  - `func (s *Server) Run(ctx context.Context) error` — stdin EOF면 nil, ctx 취소면 ctx.Err()
  - `func (s *Server) Notify(n Notification)` — initialize 전엔 큐, 뒤엔 즉시 전송
  - `var Log func(string)` 대신 필드 `s.Log func(string)`(기본 no-op)

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/channel/server_test.go
package channel

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func readLine(t *testing.T, r *bufio.Reader) map[string]any {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("json %q: %v", line, err)
	}
	return m
}

func TestInitializeDeclaresChannelCapability(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}` + "\n")
	pr, pw := io.Pipe()
	s := New(in, pw, "1.10.0", "편지 안내")
	go func() { _ = s.Run(context.Background()); pw.Close() }()
	m := readLine(t, bufio.NewReader(pr))
	res := m["result"].(map[string]any)
	if res["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion 에코: %v", res["protocolVersion"])
	}
	caps := res["capabilities"].(map[string]any)
	if _, ok := caps["experimental"].(map[string]any)["claude/channel"]; !ok {
		t.Error("experimental[claude/channel] 선언돼야 함")
	}
	if res["serverInfo"].(map[string]any)["name"] != Name || res["instructions"] != "편지 안내" {
		t.Errorf("serverInfo/instructions: %v", res)
	}
}

func TestNotifyBeforeInitializeIsQueued(t *testing.T) {
	inr, inw := io.Pipe()
	pr, pw := io.Pipe()
	s := New(inr, pw, "", "")
	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()
	s.Notify(Notification{Content: "first", Meta: map[string]string{"event": "MESSAGE"}})
	s.Notify(Notification{Content: "second", Meta: map[string]string{}})
	go inw.Write([]byte(`{"jsonrpc":"2.0","id":7,"method":"initialize","params":{}}` + "\n"))
	br := bufio.NewReader(pr)
	if m := readLine(t, br); m["id"].(float64) != 7 {
		t.Fatalf("첫 줄은 initialize 응답: %v", m)
	}
	n1 := readLine(t, br)
	n2 := readLine(t, br)
	if n1["method"] != "notifications/claude/channel" || n1["params"].(map[string]any)["content"] != "first" {
		t.Errorf("큐 순서 1: %v", n1)
	}
	if n2["params"].(map[string]any)["content"] != "second" {
		t.Errorf("큐 순서 2: %v", n2)
	}
	inw.Close()
	if err := <-done; err != nil {
		t.Errorf("EOF는 nil 종료: %v", err)
	}
}

func TestNotifyDropsBadMetaKeys(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	pr, pw := io.Pipe()
	var logged []string
	s := New(in, pw, "", "")
	s.Log = func(m string) { logged = append(logged, m) }
	go func() { _ = s.Run(context.Background()) }()
	br := bufio.NewReader(pr)
	readLine(t, br)
	s.Notify(Notification{Content: "x", Meta: map[string]string{"task": "T-1", "bad-key": "v", "9x": "v"}})
	n := readLine(t, br)
	meta := n["params"].(map[string]any)["meta"].(map[string]any)
	if _, ok := meta["bad-key"]; ok || meta["task"] != "T-1" || len(meta) != 1 {
		t.Errorf("meta 키 검증: %v", meta)
	}
	if len(logged) == 0 {
		t.Error("버린 키는 로그에 남아야 함")
	}
}

func TestPingToolsListAndUnknownMethod(t *testing.T) {
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n" +
			`{"jsonrpc":"2.0","id":3,"method":"tools/list"}` + "\n" +
			`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"x"}}` + "\n" +
			`not json` + "\n")
	pr, pw := io.Pipe()
	s := New(in, pw, "", "")
	go func() { _ = s.Run(context.Background()); pw.Close() }()
	br := bufio.NewReader(pr)
	readLine(t, br) // initialize
	if m := readLine(t, br); m["id"].(float64) != 2 || len(m["result"].(map[string]any)) != 0 {
		t.Errorf("ping: %v", m)
	}
	if m := readLine(t, br); len(m["result"].(map[string]any)["tools"].([]any)) != 0 {
		t.Errorf("tools/list 빈 목록: %v", m)
	}
	m := readLine(t, br)
	if e, ok := m["error"].(map[string]any); !ok || e["code"].(float64) != -32601 {
		t.Errorf("미지 메서드 -32601: %v", m)
	}
	if _, err := br.ReadString('\n'); err != io.EOF {
		t.Errorf("알림·깨진 줄엔 응답 없음, EOF 기대: %v", err)
	}
}

func TestRunStopsOnEOF(t *testing.T) {
	inr, inw := io.Pipe()
	s := New(inr, io.Discard, "", "")
	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()
	inw.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("EOF → nil: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("EOF 뒤 2초 안에 끝나야 함")
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	inr, _ := io.Pipe() // 아무것도 안 쓰는 stdin
	s := New(inr, io.Discard, "", "")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("ctx 취소 → context.Canceled: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("취소 뒤 2초 안에 끝나야 함")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `cd ~/ai-folder/dev/agentlayer && go test ./internal/channel/ 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: New`)

- [ ] **Step 3: 서버 구현**

```go
// internal/channel/server.go
// Package channel은 Claude Code 채널(MCP 서버가 notifications/claude/channel로 세션에 이벤트를 미는 구조)의
// stdio 서버다. 외부 의존 없이 줄 단위 JSON-RPC 2.0만 구현한다 — initialize·ping·tools/list와 알림 전송.
// stdout은 프로토콜 전용이고, 사람용 로그는 Log 콜백(stderr)로만 나간다.
package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Name은 서버 이름 — 등록명·source= 값·`server:agentlayer` 플래그가 전부 이 값이다.
const Name = "agentlayer"

// Notification은 세션에 밀어 넣는 이벤트 한 건. Meta 키는 Claude 규칙(^[a-zA-Z_][a-zA-Z0-9_]*$)을 지켜야
// 하며 어긋난 키는 Notify가 버리고 로그를 남긴다.
type Notification struct {
	Content string
	Meta    map[string]string
}

var metaKey = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

type Server struct {
	in                   io.Reader
	out                  io.Writer
	version              string
	instructions         string
	mu                   sync.Mutex // out 쓰기 + ready/queue 보호
	ready                bool
	queue                []Notification
	Log                  func(string)
}

func New(in io.Reader, out io.Writer, version, instructions string) *Server {
	if version == "" {
		version = "dev"
	}
	return &Server{in: in, out: out, version: version, instructions: instructions, Log: func(string) {}}
}

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type line struct {
	b   []byte
	err error
}

// Run은 stdin을 한 줄씩 읽어 처리한다. EOF면 nil, ctx가 끝나면 ctx.Err().
func (s *Server) Run(ctx context.Context) error {
	ch := make(chan line)
	go func() {
		r := bufioReader(s.in)
		for {
			b, err := r.ReadBytes('\n')
			if len(bytes.TrimSpace(b)) > 0 {
				select {
				case ch <- line{b: b}:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				select {
				case ch <- line{err: err}:
				case <-ctx.Done():
				}
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case l := <-ch:
			if l.err != nil {
				if errors.Is(l.err, io.EOF) {
					return nil
				}
				return l.err
			}
			s.handle(l.b)
		}
	}
}

func (s *Server) handle(raw []byte) {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		s.Log("잘못된 JSON 줄 무시: " + err.Error())
		return
	}
	if len(req.ID) == 0 || string(req.ID) == "null" {
		return // 알림(notifications/initialized 등)은 응답하지 않는다
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2024-11-05"
		}
		s.reply(req.ID, map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities": map[string]any{
				"tools":        map[string]any{},
				"experimental": map[string]any{"claude/channel": map[string]any{}},
			},
			"serverInfo":   map[string]any{"name": Name, "version": s.version},
			"instructions": s.instructions,
		})
		s.mu.Lock()
		s.ready = true
		q := s.queue
		s.queue = nil
		s.mu.Unlock()
		for _, n := range q {
			s.send(n)
		}
	case "ping":
		s.reply(req.ID, map[string]any{})
	case "tools/list":
		s.reply(req.ID, map[string]any{"tools": []any{}})
	default:
		s.write(map[string]any{"jsonrpc": "2.0", "id": req.ID,
			"error": map[string]any{"code": -32601, "message": "method not found: " + req.Method}})
	}
}

// Notify는 편지 1건을 알림으로 보낸다. initialize 전이면 큐에 쌓아 두었다가 응답 직후 순서대로 보낸다.
func (s *Server) Notify(n Notification) {
	n = s.cleanMeta(n)
	s.mu.Lock()
	if !s.ready {
		s.queue = append(s.queue, n)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.send(n)
}

func (s *Server) cleanMeta(n Notification) Notification {
	meta := map[string]string{}
	var bad []string
	for k, v := range n.Meta {
		if metaKey.MatchString(k) {
			meta[k] = v
		} else {
			bad = append(bad, k)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		s.Log("meta 키 규칙 위반으로 버림: " + strings.Join(bad, ", "))
	}
	n.Meta = meta
	return n
}

func (s *Server) send(n Notification) {
	s.write(map[string]any{"jsonrpc": "2.0", "method": "notifications/claude/channel",
		"params": map[string]any{"content": n.Content, "meta": n.Meta}})
}

func (s *Server) reply(id json.RawMessage, result any) {
	s.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *Server) write(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		s.Log("직렬화 실패: " + err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.out.Write(append(b, '\n')); err != nil {
		s.Log(fmt.Sprintf("stdout 쓰기 실패(세션 종료?): %v", err))
	}
}
```

`bufioReader`는 같은 파일 하단에 둔다(1MiB 버퍼 — 편지는 16KiB 상한이라 넉넉):

```go
import "bufio"

func bufioReader(r io.Reader) *bufio.Reader { return bufio.NewReaderSize(r, 1<<20) }
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/channel/ -v 2>&1 | tail -12`
Expected: 6 PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/channel/
git commit -m "feat(channel): stdio JSON-RPC 채널 서버 — initialize·ping·tools/list·notifications/claude/channel, initialize 전 큐"
```

### Task 2: `agentlayer channel serve <inbox>` CLI

**Files:**
- Create: `internal/cli/channelcmd.go`
- Create: `internal/cli/channelcmd_test.go`
- Modify: `main.go` (`run()` switch에 `case "channel"`, `case "task"` 바로 뒤)
- Modify: `internal/cli/helpcmd.go` (task 줄 아래 한 줄)

**Interfaces:**
- Consumes: `channel.New/Run/Notify`(Task 1), `task.Watch`, `task.RunRemotePolling`, `remote.Load`, `OpenRemote`, `refreshBoard`(모두 기존, `internal/cli/taskcmd.go`의 `taskWatch`와 같은 사용법)
- Produces:
  - `const channelUsage`
  - `func LetterNotification(r *task.Report) channel.Notification`
  - `func RunChannel(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, st *state.Store, stateDir, version string, args []string) error`

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/cli/channelcmd_test.go
package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

func TestLetterNotificationMeta(t *testing.T) {
	local := &task.Report{Version: 1, ID: strings.Repeat("a", 32), TaskID: "T-1", Session: "collab-bot", Kind: "claude", From: "WORKING", To: "DONE_UNREAD"}
	n := LetterNotification(local)
	want := map[string]string{"task": "T-1", "event": "DONE_UNREAD", "letter_id": local.ID, "origin": "local", "session": "collab-bot", "kind": "claude"}
	for k, v := range want {
		if n.Meta[k] != v {
			t.Errorf("meta[%s]=%q want %q", k, n.Meta[k], v)
		}
	}
	var back task.Report
	if err := json.Unmarshal([]byte(n.Content), &back); err != nil || back.TaskID != "T-1" {
		t.Errorf("content는 편지 JSON: %q", n.Content)
	}
	for _, r := range []*task.Report{
		{Kind: "hermes", Session: "hermes-qa", To: "DONE_UNREAD"},
		{Kind: "message", Session: "hermes-qa", To: "MESSAGE", Letter: "t_1234"},
	} {
		if got := LetterNotification(r).Meta["origin"]; got != "remote" {
			t.Errorf("%+v origin=%q want remote", r, got)
		}
	}
	if got := LetterNotification(&task.Report{Kind: "message", To: "MESSAGE"}).Meta["origin"]; got != "local" {
		t.Errorf("로컬 편지 origin=%q", got)
	}
}

func TestRunChannelUsageAndArgs(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	var out, errb bytes.Buffer
	if err := RunChannel(context.Background(), strings.NewReader(""), &out, &errb, st, t.TempDir(), "1.10.0", nil); err == nil || !strings.Contains(err.Error(), "사용법") {
		t.Errorf("인자 없음 → 사용법: %v", err)
	}
	if err := RunChannel(context.Background(), strings.NewReader(""), &out, &errb, st, t.TempDir(), "1.10.0", []string{"serve", t.TempDir(), "--bogus"}); err == nil || !strings.Contains(err.Error(), "알 수 없는 인자") {
		t.Errorf("모르는 인자 거부: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("오류 경로에서 stdout에 아무것도 쓰면 안 됨: %q", out.String())
	}
}

func TestRunChannelDeliversPendingLetter(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.NewStore(dir)
	inbox := filepath.Join(t.TempDir(), "inbox")
	id := strings.Repeat("b", 32)
	rep := &task.Report{Version: 1, ID: id, TaskID: "T-9", Session: "s", Kind: "claude", From: "WORKING", To: "WAITING", Ask: "승인?", At: time.Now(), Inbox: inbox}
	if _, err := task.WriteReport(rep); err != nil {
		t.Fatal(err)
	}
	inr, inw := io.Pipe()
	outr, outw := io.Pipe()
	var errb bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- RunChannel(context.Background(), inr, outw, &errb, st, dir, "1.10.0", []string{"serve", inbox, "--interval", "20ms"})
		outw.Close()
	}()
	go inw.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"))
	br := bufio.NewReader(outr)
	first, _ := br.ReadString('\n')
	if !strings.Contains(first, `"id":1`) {
		t.Fatalf("첫 줄은 initialize 응답: %s", first)
	}
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(line), &m)
	if m["method"] != "notifications/claude/channel" {
		t.Fatalf("알림 기대: %s", line)
	}
	meta := m["params"].(map[string]any)["meta"].(map[string]any)
	if meta["event"] != "WAITING" || meta["task"] != "T-9" || meta["letter_id"] != id {
		t.Errorf("meta: %v", meta)
	}
	if _, err := os.Stat(filepath.Join(inbox, "received", id+".json")); err != nil {
		t.Error("전달된 편지는 received/로 이동")
	}
	inw.Close() // 세션 종료
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("stdin EOF → nil: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stdin EOF 뒤 3초 안에 종료돼야 함")
	}
}

func TestRunChannelStopsWhenStdinCloses(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.NewStore(dir)
	inr, inw := io.Pipe()
	var errb bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- RunChannel(context.Background(), inr, io.Discard, &errb, st, dir, "", []string{"serve", filepath.Join(t.TempDir(), "inbox")})
	}()
	time.Sleep(50 * time.Millisecond)
	inw.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("nil 기대: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("EOF 뒤 종료돼야 함(감시 goroutine이 붙들면 안 됨)")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/cli/ -run 'TestLetterNotification|TestRunChannel' 2>&1 | head -5`
Expected: 컴파일 실패 (`undefined: RunChannel`)

- [ ] **Step 3: 구현**

```go
// internal/cli/channelcmd.go
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/netwaif/agentlayer/internal/channel"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

const channelUsage = `사용법:
  agentlayer channel serve <inbox> [--interval 200ms]
    총괄 세션에 수신함 편지를 Claude Code 채널(MCP 알림)로 밀어 넣는다 — Monitor 폴링 대체.
    등록: (회사 폴더에서) claude mcp add -s local agentlayer -- agentlayer channel serve <inbox>
    기동: claude --dangerously-load-development-channels server:agentlayer  (확인창은 봇 기동 스크립트가 넘긴다)`

// channelInstructions는 initialize 응답에 실려 세션이 채널 메시지를 무엇으로 볼지 알려준다.
const channelInstructions = `<channel source="agentlayer">로 오는 메시지는 총괄 수신함 편지다. content가 편지 JSON, ` +
	`meta.event가 종류(DONE_UNREAD·WAITING·ERROR·MESSAGE·READY), meta.origin이 local|remote. ` +
	`답은 agentlayer task reply(원격 편지) / send(로컬 세션) / task done으로 한다.`

// LetterNotification은 편지 한 건을 채널 알림으로 바꾼다. content = 편지 JSON 한 줄, meta 6키 고정.
func LetterNotification(r *task.Report) channel.Notification {
	b, _ := json.Marshal(r)
	origin := "local"
	switch r.Kind {
	case "hermes", "exec", "remote":
		origin = "remote"
	}
	if r.Letter != "" {
		origin = "remote"
	}
	return channel.Notification{Content: string(b), Meta: map[string]string{
		"task": r.TaskID, "event": r.To, "letter_id": r.ID, "origin": origin, "session": r.Session, "kind": r.Kind}}
}

// RunChannel — `agentlayer channel serve <inbox>`. stdin/stdout은 MCP 프로토콜, stderr는 로그.
func RunChannel(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, st *state.Store, stateDir, version string, args []string) error {
	if len(args) < 2 || args[0] != "serve" {
		return errors.New(channelUsage)
	}
	inbox, interval := args[1], 200*time.Millisecond
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--interval":
			if i+1 >= len(args) {
				return errors.New("--interval 뒤에 기간이 필요합니다 (예: 500ms)")
			}
			d, err := time.ParseDuration(args[i+1])
			if err != nil {
				return err
			}
			interval = d
			i++
		default:
			return fmt.Errorf("알 수 없는 인자: %s", args[i])
		}
	}
	abs, err := filepath.Abs(inbox)
	if err != nil {
		return err
	}
	logf := func(f string, a ...any) { fmt.Fprintf(stderr, "agentlayer channel: "+f+"\n", a...) }
	srv := channel.New(stdin, stdout, version, channelInstructions)
	srv.Log = func(m string) { logf("%s", m) }

	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// 원격 직원 폴링은 taskWatch와 같은 방식 — 전이가 inbox 파일로 떨어져 아래 Watch가 같은 길로 흘린다.
	opener := func(name string) (remote.Adapter, *remote.Remote, error) {
		r, ok, err := remote.Load(stateDir, name)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			return nil, nil, fmt.Errorf("원격 %q 등록 없음", name)
		}
		ad, err := OpenRemote(*r, stateDir)
		return ad, r, err
	}
	go func() {
		_ = task.RunRemotePolling(wctx, stateDir, abs, opener, func(s string) { logf("remote: %s", s) }, time.Now)
	}()
	werr := make(chan error, 1)
	go func() {
		werr <- task.Watch(wctx, abs, interval, false, func(r *task.Report) {
			srv.Notify(LetterNotification(r))
			refreshBoard(io.Discard, st, stateDir, time.Now())
		})
	}()
	serr := make(chan error, 1)
	go func() { serr <- srv.Run(wctx) }()
	logf("수신함 감시 시작: %s", abs)
	select {
	case err = <-serr:
	case err = <-werr:
		if err == nil {
			err = errors.New("수신함 감시가 예기치 않게 끝났습니다")
		}
	}
	cancel()
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}
```

`main.go`의 `case "task":` 블록 바로 뒤에 추가:

```go
	case "channel":
		st, err := storeWithSync()
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return cli.RunChannel(ctx, os.Stdin, os.Stdout, os.Stderr, st, state.DefaultDir(), buildVersion().Version, args[1:])
```

`internal/cli/helpcmd.go`의 `task assign|list|done|watch|message` 줄 아래에:

```
  channel serve <inbox>  총괄 수신함을 Claude Code 채널(MCP 알림)로 전달 — Monitor 폴링 대체. 'agentlayer channel --help'
```

- [ ] **Step 4: storeWithSync가 stdout에 쓰지 않는지 확인**

Run: `grep -n 'Fprint\|Println' main.go | sed -n '1,40p' | grep -v Stderr`
Expected: `storeWithSync` 함수 범위(main.go의 `func storeWithSync` 본문)에 stdout 출력이 없다. 있으면 그 줄을 `os.Stderr`로 바꾼다(채널 프로토콜 오염 방지).

- [ ] **Step 5: 통과 확인**

Run: `go test ./internal/cli/ -run 'TestLetterNotification|TestRunChannel' -v 2>&1 | tail -8 && go build ./... && go vet ./internal/cli/ ./internal/channel/`
Expected: 4 PASS, build·vet 통과

- [ ] **Step 6: 커밋**

```bash
git add internal/cli/channelcmd.go internal/cli/channelcmd_test.go internal/cli/helpcmd.go main.go
git commit -m "feat(channel): agentlayer channel serve <inbox> — task.Watch+원격 폴링을 채널 알림으로, stdin EOF 종료"
```

### Task 3: 수동 통합 확인(스파이크 절차 재사용)

**Files:** 없음(실행만). 결과는 Task 9의 릴리즈 노트에 한 줄.

- [ ] **Step 1: 빌드·임시 폴더 준비**

```bash
cd ~/ai-folder/dev/agentlayer && make install   # ~/.local/bin/agentlayer
S=/tmp/al-channel-check && rm -rf $S && mkdir -p $S/inbox/pending && cd $S && echo "# check" > CLAUDE.md
claude mcp add -s local agentlayer -- ~/.local/bin/agentlayer channel serve $S/inbox
```

- [ ] **Step 2: tmux 임시 세션 기동 + 확인창 Enter + 편지 투입**

```bash
tmux new-session -d -s al-check -x 140 -y 40 -c $S "cd $S && claude --dangerously-load-development-channels server:agentlayer; sleep 300"
sleep 9; tmux capture-pane -p -t al-check: | grep -c 'Loading development channels'   # 1 기대
tmux send-keys -t al-check: Enter; sleep 8
ID=$(python3 -c 'import secrets;print(secrets.token_hex(16))')
printf '{"version":1,"id":"%s","task_id":"CHK-1","session":"s","kind":"claude","from":"WORKING","to":"DONE_UNREAD","task":"reply with the single word CHANNELOK","at":"2026-09-27T23:00:00+09:00"}\n' "$ID" > $S/inbox/pending/$ID.json
sleep 15; tmux capture-pane -p -t al-check: | grep -E 'agentlayer:|CHANNELOK'
```

Expected: `← agentlayer: {"version":1,…}` 줄과 `CHANNELOK` 응답. `ls $S/inbox/received`에 편지가 있다.

- [ ] **Step 3: 정리**

```bash
tmux send-keys -t al-check: '/exit' Enter; sleep 3; tmux kill-session -t al-check
cd $S && claude mcp remove -s local agentlayer; rm -rf $S
```

### Task 4: 하위 명령 `--help` 처리

**Files:**
- Create: `internal/cli/usage.go`
- Create: `internal/cli/usage_test.go`
- Create: `main_test.go`
- Modify: `main.go` `run()` — switch 앞

**Interfaces:**
- Produces: `func SubUsage(cmd string) (string, bool)`, `func HasHelpFlag(args []string) bool`, `const browserUsage`

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/cli/usage_test.go
package cli

import "testing"

func TestSubUsageCoversHelpCommands(t *testing.T) {
	for _, c := range []string{"task", "remote", "wt", "board", "browser", "channel"} {
		u, ok := SubUsage(c)
		if !ok || u == "" {
			t.Errorf("%s: 사용법 있어야 함", c)
		}
	}
	if _, ok := SubUsage("send"); ok {
		t.Error("send는 --help 가로채기 대상이 아님(본문이 -h일 수 있다)")
	}
}

func TestHasHelpFlag(t *testing.T) {
	if !HasHelpFlag([]string{"watch", "--help"}) || !HasHelpFlag([]string{"-h"}) {
		t.Error("--help/-h 감지")
	}
	if HasHelpFlag([]string{"watch", "/tmp/inbox", "--once"}) || HasHelpFlag([]string{"--helpme"}) {
		t.Error("정확 일치만")
	}
}
```

```go
// main_test.go
package main

import (
	"os"
	"testing"
)

func TestSubcommandHelpDoesNotTouchDisk(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, args := range [][]string{{"task", "watch", "--help"}, {"task", "-h"}, {"remote", "add", "--help"}, {"wt", "--help"}, {"channel", "serve", "-h"}} {
		if err := run(args); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
	if _, err := os.Stat("--help"); err == nil {
		t.Error("'--help' 디렉터리가 생기면 안 됨")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("help는 디스크에 아무것도 만들지 않는다: %v", entries)
	}
}

func TestHelpFlagOnlyForListedSubcommands(t *testing.T) {
	// send/broadcast는 가로채지 않는다 — 여기서는 usage 조회만 확인(실제 send는 tmux가 필요)
	if err := run([]string{"send", "--help"}); err == nil {
		t.Error("send --help는 SubUsage 대상이 아니므로 send 자체 인자 검증 오류가 나야 한다")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/cli/ -run 'TestSubUsage|TestHasHelpFlag' 2>&1 | head -3; go test . -run 'TestSubcommandHelp|TestHelpFlagOnly' 2>&1 | head -3`
Expected: 컴파일 실패(`undefined: SubUsage`), main_test는 `--help` 디렉터리 생성으로 실패 또는 감시 시작으로 멈춤(멈추면 Ctrl-C — 그래서 구현 전엔 main_test를 돌리지 말고 cli 테스트만 확인해도 된다)

- [ ] **Step 3: 구현**

```go
// internal/cli/usage.go
package cli

// browserUsage — browser는 서브커맨드별 usage가 흩어져 있어 요약만 낸다.
const browserUsage = `사용법: agentlayer browser <open|pick|shot|errors|preview|cookies|mcp|control reset|restart> [...]
  상세는 'agentlayer help'의 browser 줄과 각 서브커맨드의 오류 메시지 참고`

// SubUsage는 --help 가로채기 대상 하위 명령의 사용법. send·broadcast·wake-all·close-all은 본문 인자에
// "-h"가 올 수 있어 대상에서 뺀다.
func SubUsage(cmd string) (string, bool) {
	switch cmd {
	case "task":
		return taskUsage, true
	case "remote":
		return remoteUsage, true
	case "wt":
		return wtUsage, true
	case "board":
		return boardUsage, true
	case "browser":
		return browserUsage, true
	case "channel":
		return channelUsage, true
	}
	return "", false
}

// HasHelpFlag는 인자 중 정확히 "--help" 또는 "-h"가 있으면 true.
func HasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}
```

`main.go` `run()`의 `switch args[0] {` 바로 앞에:

```go
	// 하위 명령 --help: task watch --help가 "--help"를 inbox 경로로 삼아 디렉터리를 만들던 버그의 일반 처리.
	if len(args) >= 2 && cli.HasHelpFlag(args[1:]) {
		if u, ok := cli.SubUsage(args[0]); ok {
			fmt.Println(u)
			return nil
		}
	}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/cli/ -run 'TestSubUsage|TestHasHelpFlag' -v 2>&1 | tail -4 && go test . -run 'TestSubcommandHelp|TestHelpFlagOnly' -v 2>&1 | tail -4`
Expected: 4 PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/cli/usage.go internal/cli/usage_test.go main.go main_test.go
git commit -m "fix(cli): 하위 명령 --help/-h 처리 — task watch --help가 './--help/' 수신함을 만들던 버그"
```

### Task 5: 자식 Claude 세션 훅 무시(조상 사슬 판정)

**Files:**
- Create: `internal/hookcmd/nested.go`
- Create: `internal/hookcmd/nested_test.go`
- Modify: `internal/hookcmd/claude.go:29-33` (`RunClaude` 첫 부분)
- Modify: `internal/hookcmd/claude_test.go` (테스트 1개 추가)

**Interfaces:**
- Consumes: `scan.ProcTable`, `scan.ParseProcTable`, `scan.LoadProcTable`, `scan.KindFromArgs` (기존 `internal/scan/proc.go`)
- Produces: `func NestedClaude(pt scan.ProcTable, pid int) bool`, 패키지 변수 `nestedCheck func() bool`(테스트 교체용)

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/hookcmd/nested_test.go
package hookcmd

import (
	"testing"

	"github.com/netwaif/agentlayer/internal/scan"
)

// 실측 사슬(2026-09-27): hook ← sh ← claude ← zsh ← tmux. 자식 세션은 hook ← sh ← claude(자식) ← zsh -c(Bash 도구) ← claude(부모).
const procFixture = `1 0 /sbin/launchd
1193 1 /usr/local/bin/tmux new-session -d -s company-bot
62376 1193 -zsh
62539 62376 claude --resume fe4078c8
20700 62539 /bin/sh -c agentlayer hook claude --event stop
20655 62539 /bin/zsh -c source /Users/x/.claude/shell-snapshots/snap.sh
70000 20655 claude -p hi
70001 70000 /bin/sh -c agentlayer hook claude --event stop
`

func TestNestedClaudeSingleAncestor(t *testing.T) {
	pt := scan.ParseProcTable(procFixture)
	if NestedClaude(pt, 20700) {
		t.Error("부모 세션의 훅(claude 조상 1개)은 자식이 아니다 — 재시작으로 session_id가 바뀌어도 보고는 계속돼야 한다")
	}
}

func TestNestedClaudeChildSession(t *testing.T) {
	pt := scan.ParseProcTable(procFixture)
	if !NestedClaude(pt, 70001) {
		t.Error("Bash 도구로 띄운 자식 claude의 훅은 자식으로 판정돼야 한다")
	}
}

func TestNestedClaudeUnknownPidIsNotNested(t *testing.T) {
	pt := scan.ParseProcTable(procFixture)
	if NestedClaude(pt, 99999) {
		t.Error("표에 없는 pid는 판정 불가 → 자식 아님(훅을 막지 않는다)")
	}
}
```

`internal/hookcmd/claude_test.go` 끝에 추가:

```go
func TestNestedSessionHookIsIgnored(t *testing.T) {
	st := newStore(t)
	// 부모 세션이 WORK 상태
	if err := RunClaude(st, "post-tool-use", strings.NewReader(payload), env("%3"), t0); err != nil {
		t.Fatal(err)
	}
	old := nestedCheck
	nestedCheck = func() bool { return true }
	defer func() { nestedCheck = old }()
	calls := 0
	SetTransitionHook(func(a *state.Agent, prev, to state.AgentState) { calls++ })
	defer SetTransitionHook(nil)
	child := `{"session_id":"child-1","cwd":"/tmp/scratch","hook_event_name":"Stop"}`
	if err := RunClaude(st, "stop", strings.NewReader(child), env("%3"), t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	a, _ := st.Load(scan.IDForPane("claude", "%3"))
	if a.State != state.StateWorking || a.SessionID != "10ec8033-ca55" || calls != 0 {
		t.Errorf("자식 세션 훅은 레코드·전이·보고를 건드리지 않는다: state=%s sid=%s calls=%d", a.State, a.SessionID, calls)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/hookcmd/ -run 'TestNested' 2>&1 | head -3`
Expected: 컴파일 실패(`undefined: NestedClaude`)

- [ ] **Step 3: 구현**

```go
// internal/hookcmd/nested.go
package hookcmd

import (
	"os"

	"github.com/netwaif/agentlayer/internal/scan"
)

// NestedClaude는 pid의 조상 사슬에 claude 프로세스가 둘 이상이면 true — 직원 세션이 Bash 도구로 띄운
// 자식 claude(예: `claude -p`)의 훅은 같은 TMUX_PANE으로 들어와 부모 업무ID로 DONE_UNREAD를 보고한다
// (WAKE-PATH-RESEARCH-3 실측 4회). 표에 없는 pid·사슬 끊김은 false(훅을 막지 않는다).
func NestedClaude(pt scan.ProcTable, pid int) bool {
	n := 0
	for p := pid; p > 1; {
		e, ok := pt[p]
		if !ok || e.PPID == p {
			return false
		}
		if scan.KindFromArgs(e.Args) == "claude" {
			n++
			if n >= 2 {
				return true
			}
		}
		p = e.PPID
	}
	return false
}

// nestedCheck는 RunClaude가 부른다(ps 한 번, 수십 ms). 테스트가 바꿔 끼운다.
var nestedCheck = func() bool { return NestedClaude(scan.LoadProcTable(), os.Getppid()) }
```

`internal/hookcmd/claude.go` `RunClaude`의 `pane == ""` 검사 직후(payload 파싱 전)에:

```go
	if nestedCheck() {
		return nil // 직원이 띄운 자식 claude 세션 — 부모 pane의 기록·전이·보고를 오염시키지 않는다
	}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/hookcmd/ -v 2>&1 | tail -8 && go vet ./internal/hookcmd/`
Expected: 기존 테스트 포함 전부 PASS (기존 테스트는 실제 `ps`를 부르지만 테스트 프로세스 위에 claude가 하나뿐이거나 없어 false — 이 세션 안에서 돌리면 claude 조상이 1개라 false. 만약 테스트 러너가 자식 claude 안에서 돈다면 `nestedCheck`를 TestMain에서 false로 고정한다)

- [ ] **Step 5: 커밋**

```bash
git add internal/hookcmd/nested.go internal/hookcmd/nested_test.go internal/hookcmd/claude.go internal/hookcmd/claude_test.go
git commit -m "fix(hook): 자식 claude 세션(Bash로 띄운 claude -p 등)의 훅 무시 — 조상 사슬에 claude 2개면 건너뜀"
```

### Task 6: README·릴리즈 v1.10.0

**Files:**
- Modify: `README.md:153-178` ("세션 지시·업무 보고" 절)
- Modify: `SESSION.md`(파일 흔적·결정 기록 추가)

- [ ] **Step 1: README 갱신**

`agentlayer task watch ~/ai-folder/company/runtime/inbox` 줄을 다음으로 교체:

```
agentlayer channel serve ~/ai-folder/company/runtime/inbox  # 총괄 세션의 MCP 채널 서버(claude mcp add -s local로 등록) — 편지가 세션에 직접 온다
agentlayer task watch ~/ai-folder/company/runtime/inbox     # 같은 수신을 한 줄 JSON으로(디버그·전환기용)
```

불릿 목록의 `task watch` 항목 뒤에 추가:

```
- **채널 수신(1.10.0+)**: 총괄은 `claude mcp add -s local agentlayer -- agentlayer channel serve <inbox>`로 서버를 등록하고
  `--dangerously-load-development-channels server:agentlayer`로 기동한다(research preview 플래그 — 기동마다 확인창이 한 번 뜨며
  folder-bot 0.1.22+의 bot-up이 넘긴다). 편지는 `<channel source="agentlayer" event="DONE_UNREAD" task="…" letter_id="…" origin="local|remote">`로
  세션에 직접 들어오고 Monitor·재무장이 필요 없다. 채널이 없으면 `task watch`가 그대로 대체 경로다.
- 훅은 직원이 Bash로 띄운 자식 claude 세션(조상 사슬에 claude 2개)의 이벤트를 무시한다(1.10.0+).
```

- [ ] **Step 2: 패키지별 테스트 재확인**

Run: `for p in ./internal/channel/ ./internal/cli/ ./internal/hookcmd/ ./internal/task/ .; do go test $p 2>&1 | tail -1; done`
Expected: 전부 `ok`

- [ ] **Step 3: 커밋·태그·릴리즈**

```bash
git add README.md
git commit -m "docs(readme): 채널 수신(channel serve)·자식 세션 무시 기록"
git tag v1.10.0 && git push && git push --tags
# goreleaser는 태그 푸시로 돈다(기존 절차). 완료 확인:
gh release view v1.10.0 --json tagName,assets -q '.tagName, (.assets|length)'
```

Expected: `v1.10.0`, 자산 수 > 0. brew tap 갱신은 goreleaser가 한다(기존).

---

## Part B — folder-bot (0.1.22)

작업 폴더 `~/VSCodeWorkspace/folder-bot`. agentlayer 세션이 worktree가 아니므로 이 레포 git 작업이 가능하다.

### Task 7: botctl `--dev-channel` + bot-up 확인창 자동 통과

**Files:**
- Modify: `plugins/folder-bot/skills/configure-bot/generator/botctl.py` (`resolve_bot`, `build_cmd`, `cmd_add`, argparse `add`, `cmd_doctor` claude 분기)
- Modify: `plugins/folder-bot/skills/configure-bot/assets/bot-up.sh` (`exec "$CLAUDE_BIN"` 직전)
- Modify: `plugins/folder-bot/skills/configure-bot/SKILL.md:54` (add 플래그 안내)
- Modify: `tests/test_botctl.py`
- Create: `tests/test_bot_up_devchannel.sh`

**Interfaces:**
- Produces: bots.json 항목 필드 `dev_channels: list[str]`(선택), CLI `add --dev-channel <server:이름>`(반복)·`--no-dev-channels`, bot-up.sh 환경 오버라이드 `TMUX_BIN`·`DEV_CHANNEL_TIMEOUT`(초, 기본 60)·`DEV_CHANNEL_POLL`(초, 기본 3)·`BOT_UP_LOG`(기본 `$HOME/.claude/logs/bot-up.log`)

- [ ] **Step 1: 실패하는 pytest 작성** (`tests/test_botctl.py` 끝에)

```python
def test_dev_channel_flag_in_plist_and_roundtrip(tmp_path):
    folder = tmp_path / "company"; folder.mkdir()
    r = run(tmp_path, "add", "--name", "company", "--folder", str(folder), "--session", "company-bot",
            "--no-directive-block", "--dev-channel", "server:agentlayer", "--dev-channel", "server:agentlayer")
    assert r.returncode == 0, r.stderr
    data = json.loads((tmp_path / ".config/folder-bot/bots.json").read_text())
    assert data["company"]["dev_channels"] == ["server:agentlayer"]          # 중복 제거
    plist = tmp_path / "Library/LaunchAgents/com.folder-bot.company.plist"
    assert plist.exists()
    text = plist.read_text()
    assert "--channels plugin:discord@claude-plugins-official --dangerously-load-development-channels server:agentlayer" in text
    # 플래그 없이 재등록해도 유지된다
    r = run(tmp_path, "add", "--name", "company", "--folder", str(folder), "--session", "company-bot", "--no-directive-block")
    assert r.returncode == 0, r.stderr
    data = json.loads((tmp_path / ".config/folder-bot/bots.json").read_text())
    assert data["company"]["dev_channels"] == ["server:agentlayer"]
    # --no-dev-channels로 제거
    r = run(tmp_path, "add", "--name", "company", "--folder", str(folder), "--session", "company-bot",
            "--no-directive-block", "--no-dev-channels")
    assert r.returncode == 0, r.stderr
    data = json.loads((tmp_path / ".config/folder-bot/bots.json").read_text())
    assert "dev_channels" not in data["company"]
    assert "--dangerously-load-development-channels" not in plist.read_text()
```

- [ ] **Step 2: 실패 확인**

Run: `cd ~/VSCodeWorkspace/folder-bot && python3 -m pytest tests/test_botctl.py -k dev_channel -q 2>&1 | tail -3`
Expected: FAIL (`unrecognized arguments: --dev-channel`)

- [ ] **Step 3: botctl.py 구현**

`resolve_bot` — `b.setdefault("directive_block", True)` 다음 줄에:

```python
    b.setdefault("dev_channels", [])
```

`build_cmd` — `parts.append(...)` 부분을 다음으로 교체:

```python
    dev = ""
    if bot.get("dev_channels"):
        # research preview 채널(예: agentlayer channel serve). 기동마다 확인창이 뜨며 bot-up.sh가 Enter로 넘긴다.
        dev = " --dangerously-load-development-channels " + " ".join(bot["dev_channels"])
    parts.append(f"exec {home()}/.local/bin/bot-up{flags}"
                 " --channels plugin:discord@claude-plugins-official" + dev)
```

`cmd_add` — `if a.no_autostart:` 블록 다음에:

```python
    if a.no_dev_channels:
        entry.pop("dev_channels", None)
    elif a.dev_channel:
        entry["dev_channels"] = list(dict.fromkeys(a.dev_channel))
    elif prior.get("dev_channels"):
        entry["dev_channels"] = prior["dev_channels"]  # 플래그 없이 재등록해도 유지
```

argparse `add` — `--no-autostart` 다음에:

```python
    ap.add_argument("--dev-channel", action="append", default=[],
                    help="개발 채널(예: server:agentlayer) — 기동 인자 --dangerously-load-development-channels에 추가, 반복 가능")
    ap.add_argument("--no-dev-channels", action="store_true", help="등록된 개발 채널 제거")
```

`cmd_doctor` claude 분기 — 워크스페이스 미신뢰 WARN 다음에:

```python
            if b.get("dev_channels"):
                want = "--dangerously-load-development-channels " + " ".join(b["dev_channels"])
                p = plist_path(b)
                if p.exists() and want not in p.read_text():
                    rep("WARN", f"개발 채널 플래그가 기동 정의에 없음({want}) — botctl add 재실행")
```

- [ ] **Step 4: pytest 통과 확인**

Run: `python3 -m pytest tests/test_botctl.py -q 2>&1 | tail -3`
Expected: 전부 passed

- [ ] **Step 5: 실패하는 셸 테스트 작성** (`tests/test_bot_up_devchannel.sh`, 실행 권한)

```bash
#!/usr/bin/env bash
# bot-up.sh 개발 채널 확인창 자동 통과 — tmux 스텁으로 capture-pane 3번째 호출에 문구를 내고 send-keys가 1회만 불리는지.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
BOTUP="$HERE/../plugins/folder-bot/skills/configure-bot/assets/bot-up.sh"
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
mkdir -p "$T/bin" "$T/home/.claude/logs"
cat > "$T/bin/tmux" <<'EOF'
#!/usr/bin/env bash
cnt_file="$STUB_DIR/capture.count"
case "$1" in
  capture-pane)
    n=$(( $(cat "$cnt_file" 2>/dev/null || echo 0) + 1 )); echo "$n" > "$cnt_file"
    if [[ "${STUB_SHOW:-1}" == 1 && $n -ge 3 ]]; then printf '  WARNING: Loading development channels\n  Channels: server:agentlayer\n'; else printf '  loading...\n'; fi ;;
  send-keys) echo "send-keys $*" >> "$STUB_DIR/keys.log" ;;
esac
EOF
chmod +x "$T/bin/tmux"
# claude 스텁: 6초 살아 있다 종료(감시자가 그동안 폴링)
cat > "$T/bin/claude" <<'EOF'
#!/usr/bin/env bash
sleep 6
EOF
chmod +x "$T/bin/claude"
run_case() {  # $1=케이스명 $2=STUB_SHOW $3=인자...
  rm -f "$T/capture.count" "$T/keys.log"
  ( cd "$T" && STUB_DIR="$T" STUB_SHOW="$2" HOME="$T/home" CLAUDE_BIN="$T/bin/claude" TMUX_BIN="$T/bin/tmux" TMUX_PANE="%9" \
      CLAUDE_BOT_LOCK="$T/lock.d" CONNECT_TIMEOUT=1 DEV_CHANNEL_POLL=1 DEV_CHANNEL_TIMEOUT=4 BOT_UP_LOG="$T/home/.claude/logs/bot-up.log" \
      bash "$BOTUP" "${@:3}" >/dev/null 2>&1 ) || true
  sleep 1
}
run_case dialog 1 -n x --channels plugin:discord@claude-plugins-official --dangerously-load-development-channels server:agentlayer
[[ $(grep -c 'send-keys -t %9 Enter' "$T/keys.log" 2>/dev/null || echo 0) == 1 ]] || { echo "FAIL dialog: Enter 1회 기대"; cat "$T/keys.log" 2>/dev/null; exit 1; }
grep -q 'dev-channel: 확인창 통과' "$T/home/.claude/logs/bot-up.log" || { echo "FAIL dialog: 로그 없음"; exit 1; }
run_case no_dialog 0 -n x --channels plugin:discord@claude-plugins-official --dangerously-load-development-channels server:agentlayer
[[ ! -s "$T/keys.log" ]] || { echo "FAIL no_dialog: Enter를 보내면 안 됨"; exit 1; }
grep -q 'dev-channel: 확인창 미출현' "$T/home/.claude/logs/bot-up.log" || { echo "FAIL no_dialog: 미출현 로그 없음"; exit 1; }
run_case no_flag 1 -n x --channels plugin:discord@claude-plugins-official
[[ ! -f "$T/capture.count" ]] || { echo "FAIL no_flag: 플래그 없으면 감시자도 없어야 함"; exit 1; }
echo "PASS test_bot_up_devchannel"
```

- [ ] **Step 6: 실패 확인**

Run: `chmod +x tests/test_bot_up_devchannel.sh && tests/test_bot_up_devchannel.sh`
Expected: `FAIL dialog: Enter 1회 기대`

- [ ] **Step 7: bot-up.sh 구현** — `log "claude 기동: ..."` 줄 바로 앞에 삽입

```bash
# --- 개발 채널 확인창 자동 통과 (agentlayer channel serve 등 server: 채널은 research preview라 기동마다 확인창이 뜬다) ---
# 확인창("WARNING: Loading development channels" / "I am using this for local development")은 매번 같은 고정 화면이므로
# Enter 한 번을 부팅 절차로 보낸다 — 메시지 주입이 아니다. 로그는 pane이 아니라 파일로(감시자는 fd를 분리한다).
if [[ " $* " == *" --dangerously-load-development-channels "* && -n "${TMUX_PANE:-}" ]]; then
  TMUX_BIN="${TMUX_BIN:-$(command -v tmux || true)}"
  [[ -z "$TMUX_BIN" && -x /opt/homebrew/bin/tmux ]] && TMUX_BIN=/opt/homebrew/bin/tmux
  DEV_LOG="${BOT_UP_LOG:-$HOME/.claude/logs/bot-up.log}"; mkdir -p "$(dirname "$DEV_LOG")"
  if [[ -n "$TMUX_BIN" ]]; then
    (
      poll="${DEV_CHANNEL_POLL:-3}"; deadline=$((SECONDS + ${DEV_CHANNEL_TIMEOUT:-60}))
      while (( SECONDS < deadline )); do
        sleep "$poll"
        if "$TMUX_BIN" capture-pane -p -t "$TMUX_PANE" 2>/dev/null | grep -q 'WARNING: Loading development channels'; then
          "$TMUX_BIN" send-keys -t "$TMUX_PANE" Enter
          echo "[bot-up $(date '+%F %T')] dev-channel: 확인창 통과 (pane $TMUX_PANE)" >> "$DEV_LOG"
          exit 0
        fi
      done
      echo "[bot-up $(date '+%F %T')] dev-channel: 확인창 미출현(${DEV_CHANNEL_TIMEOUT:-60}s) — 플래그·UI 변경 여부 확인" >> "$DEV_LOG"
    ) >/dev/null 2>&1 &
  fi
fi
```

- [ ] **Step 8: 셸 테스트·기존 테스트 통과 확인**

Run: `tests/test_bot_up_devchannel.sh && python3 -m pytest tests -q 2>&1 | tail -2`
Expected: `PASS test_bot_up_devchannel`, pytest 전부 passed

- [ ] **Step 9: SKILL.md 한 줄** — 54행 괄호 안에 추가: `개발 채널(agentlayer channel 등)을 쓰는 봇은 --dev-channel server:agentlayer(반복 가능, 제거는 --no-dev-channels)`

- [ ] **Step 10: 커밋**

```bash
git add plugins/folder-bot/skills/configure-bot/generator/botctl.py plugins/folder-bot/skills/configure-bot/assets/bot-up.sh plugins/folder-bot/skills/configure-bot/SKILL.md tests/test_botctl.py tests/test_bot_up_devchannel.sh
git commit -m "feat(bot): --dev-channel — 개발 채널 플래그를 기동 명령에, bot-up이 확인창을 Enter 1회로 통과(0.1.22)"
```

### Task 8: folder-bot 0.1.22 릴리즈

**Files:**
- Modify: `plugins/folder-bot/.claude-plugin/plugin.json:3`, `.claude-plugin/marketplace.json:9` (0.1.21 → 0.1.22)
- Modify: `README.md` (사용 절에 한 줄)

- [ ] **Step 1: 버전·README**

```bash
sed -i '' 's/"version": "0.1.21"/"version": "0.1.22"/' plugins/folder-bot/.claude-plugin/plugin.json .claude-plugin/marketplace.json
```

README `## 사용` 아래 자동 목록에 한 줄: `- 개발 채널 플래그(`dev_channels`, 예: agentlayer 수신함 채널)를 기동 명령에 붙이고, 기동 때 뜨는 확인창을 bot-up이 Enter 한 번으로 넘긴다(0.1.22)`

- [ ] **Step 2: 커밋·태그·푸시**

```bash
git add -A && git commit -m "chore: 0.1.22 — dev_channels·확인창 자동 통과" && git tag v0.1.22 && git push && git push --tags
```

- [ ] **Step 3: 이 맥에 플러그인 갱신**

`claude` 안에서 `/plugin` → folder-bot 업데이트(0.1.22). 확인: `ls ~/.claude/plugins/cache/folder-bot/folder-bot/ | tail -1` → `0.1.22`. 그다음 `python3 ~/.claude/plugins/cache/folder-bot/folder-bot/0.1.22/skills/configure-bot/generator/botctl.py list`가 돈다.

---

## Part C — ai-company (0.2.9)

작업 폴더 `~/VSCodeWorkspace/ai-company`, 엔진 `plugins/ai-company/skills/configure-company/generator/companyctl.py`.

### Task 9: 채널 등록 + 총괄 봇 플래그 (install)

**Files:**
- Modify: `plugins/ai-company/skills/configure-company/generator/companyctl.py` (`MIN_AGENTLAYER`, 새 함수 2개, `cmd_install` 끝)
- Modify: `tests/conftest.py` (`FAKES`에 `claude` 추가, agentlayer 버전 1.10.0)
- Modify: `tests/test_companyctl.py`

**Interfaces:**
- Consumes: agentlayer `channel serve`(Task 2), botctl `--dev-channel`(Task 7)
- Produces: `register_channel(root: Path, inbox: str) -> list[str]`, `set_manager_dev_channel(root: Path) -> list[str]`, `find_botctl() -> str | None`

- [ ] **Step 1: conftest 갱신**

`FAKES["agentlayer"]`의 `v1.6.0`을 `v1.10.0`으로. `FAKES`에 추가:

```python
    "claude": '#!/bin/sh\necho "claude $*" >> "$HOME/claude-calls.log"\n'
              'if [ "$1" = mcp ] && [ "$2" = get ]; then\n'
              '  if [ -f "$HOME/mcp-registered" ]; then printf "agentlayer:\\n  Scope: Local config\\n  Command: /x/agentlayer\\n  Args: channel serve %s\\n" "$(cat "$HOME/mcp-registered")"; exit 0; fi\n'
              '  echo "No MCP server named \\"$3\\"." >&2; exit 1\nfi\n'
              'if [ "$1" = mcp ] && [ "$2" = add ]; then shift 7; echo "$4" > "$HOME/mcp-registered"; fi\n'
              'if [ "$1" = mcp ] && [ "$2" = remove ]; then rm -f "$HOME/mcp-registered"; fi\nexit 0\n',
```

(가짜 `claude`의 인자: `mcp add -s local agentlayer -- <bin> channel serve <inbox>` → `shift 7` 뒤 `$1`=`<bin>`, `$2`=`channel`, `$3`=`serve`, `$4`=`<inbox>`.)

- [ ] **Step 2: 실패하는 테스트 작성** (`tests/test_companyctl.py` 끝에)

```python
def test_install_registers_channel_and_manager_dev_flag(env, tmp_path):
    root = tmp_path / "company"
    assert run(env, "init", "--root", str(root), "--name", "C").returncode == 0
    # 총괄 봇이 등록돼 있고 botctl.py가 플러그인 캐시 자리에 있는 상황
    write_bots_json(env, {"company": {"engine": "claude", "folder": str(root), "session": "company-bot"}})
    botctl = Path(env["HOME"]) / ".claude/plugins/cache/folder-bot/folder-bot/0.1.22/skills/configure-bot/generator/botctl.py"
    botctl.parent.mkdir(parents=True)
    botctl.write_text('import sys, os\nopen(os.path.expanduser("~/botctl-calls.log"), "a").write(" ".join(sys.argv[1:]) + "\\n")\n')
    r = run(env, "install", "--root", str(root), "--no-restart")
    assert r.returncode == 0, r.stderr
    calls = (Path(env["HOME"]) / "claude-calls.log").read_text()
    assert f"claude mcp add -s local agentlayer -- " in calls and f"channel serve {root / 'runtime' / 'inbox'}" in calls
    assert "채널 등록" in r.stdout
    bcalls = (Path(env["HOME"]) / "botctl-calls.log").read_text()
    assert f"add --name company --folder {root} --session company-bot --engine claude --dev-channel server:agentlayer" in bcalls
    # 재실행도 성공(멱등: remove 후 add)
    r = run(env, "install", "--root", str(root), "--no-restart")
    assert r.returncode == 0, r.stderr


def test_install_without_manager_bot_only_registers_channel(env, tmp_path):
    root = tmp_path / "company"
    assert run(env, "init", "--root", str(root), "--name", "C").returncode == 0
    r = run(env, "install", "--root", str(root), "--no-restart")
    assert r.returncode == 0, r.stderr
    assert "채널 등록" in r.stdout and "총괄 봇 플래그" not in r.stdout
```

- [ ] **Step 3: 실패 확인**

Run: `cd ~/VSCodeWorkspace/ai-company && python3 -m pytest tests -k 'registers_channel or only_registers' -q 2>&1 | tail -3`
Expected: FAIL (`claude-calls.log` 없음)

- [ ] **Step 4: 구현**

`MIN_AGENTLAYER = (1, 10, 0)`, `MIN_AGENTLAYER_STR = "1.10.0"`.

`restart_manager_bot` 아래에 추가:

```python
def find_botctl():
    """folder-bot 플러그인 캐시의 botctl.py(최신 버전). 없으면 None."""
    base = Path.home() / ".claude" / "plugins" / "cache" / "folder-bot" / "folder-bot"
    cands = sorted(base.glob("*/skills/configure-bot/generator/botctl.py"),
                   key=lambda p: tuple(int(x) if x.isdigit() else 0 for x in p.parts[-5].split(".")))
    return str(cands[-1]) if cands else None


def register_channel(root: Path, inbox: str) -> list:
    """총괄 수신함 채널 — 회사 폴더 local 스코프에 `agentlayer channel serve <inbox>`를 MCP 서버로 등록.
    .mcp.json은 쓰지 않는다(미신뢰 폴더에서 매 기동 승인창). 멱등: remove(실패 무시) 후 add."""
    claude = which("claude")
    al = which("agentlayer")
    if not claude or not al:
        return ["WARN 채널 등록 생략 — claude/agentlayer 명령을 PATH에서 못 찾음"]
    subprocess.run([claude, "mcp", "remove", "-s", "local", "agentlayer"], cwd=root, capture_output=True, text=True)
    r = subprocess.run([claude, "mcp", "add", "-s", "local", "agentlayer", "--", al, "channel", "serve", inbox],
                       cwd=root, capture_output=True, text=True)
    if r.returncode != 0:
        return [f"WARN 채널 등록 실패: {(r.stderr or r.stdout).strip()}"]
    return [f"채널 등록: claude mcp(local, {root}) agentlayer → {al} channel serve {inbox}"]


def set_manager_dev_channel(root: Path) -> list:
    """총괄 봇 기동 명령에 --dangerously-load-development-channels server:agentlayer를 붙인다(botctl add 재실행, 멱등)."""
    name, b = manager_bot_of(root)
    if not b:
        return []
    botctl = find_botctl()
    if not botctl:
        return [f"WARN 총괄 봇 플래그 생략 — folder-bot 0.1.22+ botctl.py를 못 찾음. 수동: botctl.py add --name {name} "
                f"--folder {b['folder']} --session {b.get('session', name + '-bot')} --dev-channel server:agentlayer"]
    args = [sys.executable, botctl, "add", "--name", name, "--folder", b["folder"], "--session", b.get("session", f"{name}-bot"),
            "--engine", b.get("engine", "claude"), "--dev-channel", "server:agentlayer"]
    if b.get("autostart") is False:
        args.append("--no-autostart")
    if b.get("directive_block") is False:
        args.append("--no-directive-block")
    r = subprocess.run(args, capture_output=True, text=True)
    if r.returncode != 0:
        return [f"WARN 총괄 봇 플래그 실패({name}): {(r.stderr or r.stdout).strip()}"]
    return [f"총괄 봇 플래그: {name} dev_channels=[server:agentlayer]"]
```

`cmd_install` — `manager_changed = bool(msg)` 다음 줄에:

```python
    done.extend(register_channel(root, inbox))
    done.extend(set_manager_dev_channel(root))
```

- [ ] **Step 5: 통과 확인**

Run: `python3 -m pytest tests -q 2>&1 | tail -3`
Expected: 전부 passed (기존 테스트 중 `agentlayer v1.6.0` 문자열을 검사하는 것이 있으면 1.10.0으로 갱신)

- [ ] **Step 6: 커밋**

```bash
git add plugins/ai-company/skills/configure-company/generator/companyctl.py tests/conftest.py tests/test_companyctl.py
git commit -m "feat(install): 총괄 수신함 채널 등록(claude mcp add -s local) + 총괄 봇 --dev-channel(botctl 재실행)"
```

### Task 10: 총괄 지침 블록·SKILL·doctor

**Files:**
- Modify: `plugins/ai-company/skills/configure-company/assets/company-block.md:9,17,18`
- Modify: `plugins/ai-company/skills/configure-company/SKILL.md:12,46,64`
- Modify: `plugins/ai-company/skills/configure-company/generator/companyctl.py` (`cmd_doctor`)
- Modify: `tests/test_companyctl.py`

- [ ] **Step 1: 실패하는 테스트 작성**

```python
def test_block_uses_channel_not_monitor(env, tmp_path):
    root = tmp_path / "company"
    assert run(env, "init", "--root", str(root), "--name", "C").returncode == 0
    assert run(env, "install", "--root", str(root), "--no-restart").returncode == 0
    text = (root / "CLAUDE.md").read_text()
    assert '<channel source="agentlayer"' in text
    assert "Monitor" not in text and "timeout_ms" not in text


def test_doctor_reports_channel_state(env, tmp_path):
    root = tmp_path / "company"
    assert run(env, "init", "--root", str(root), "--name", "C").returncode == 0
    write_bots_json(env, {"company": {"engine": "claude", "folder": str(root), "session": "company-bot"}})
    r = run(env, "doctor", "--root", str(root))
    out = r.stdout
    assert "WARN 수신함 채널 미등록" in out and "WARN 총괄 봇 개발 채널 플래그 없음" in out
    (Path(env["HOME"]) / "mcp-registered").write_text(str(root / "runtime" / "inbox"))
    write_bots_json(env, {"company": {"engine": "claude", "folder": str(root), "session": "company-bot", "dev_channels": ["server:agentlayer"]}})
    r = run(env, "doctor", "--root", str(root))
    assert "OK   수신함 채널: agentlayer channel serve" in r.stdout and "OK   총괄 봇 개발 채널 플래그" in r.stdout
```

- [ ] **Step 2: 실패 확인**

Run: `python3 -m pytest tests -k 'channel_not_monitor or reports_channel' -q 2>&1 | tail -3`
Expected: FAIL

- [ ] **Step 3: company-block.md 교체**

9행 `감시는 **상시** — 재정박 때 켠다(아래 **감시** 규칙).` → `편지는 채널 메시지로 이 세션에 직접 온다(아래 **편지 수신(채널)** 규칙) — 따로 켤 것이 없다.`

17행(`- **감시(상시)**: …`)을 다음으로 교체:

```
   - **편지 수신(채널)**: 편지는 `<channel source="agentlayer" event="…" task="…" letter_id="…" origin="local|remote">` 메시지로 이 세션에 직접 온다(agentlayer 1.10.0+, 등록·기동 플래그는 `companyctl install`이 넣는다). content가 편지 JSON이고 `event`가 종류다. 감시를 켜거나 재무장할 것이 없다. 채널 메시지가 안 오면 `companyctl doctor --root {ROOT}`로 등록·봇 플래그를 점검한다(대체 경로: `agentlayer task watch {INBOX}`).
```

18행 `4. **수신**:`의 `Monitor 이벤트의 \`to\`가` → `채널 메시지의 \`to\`(=meta event)가`.

- [ ] **Step 4: SKILL.md**

12행 `총괄 역할은 Claude Code 전용이다(Monitor·SendMessage 의존).` → `총괄 역할은 Claude Code 전용이다(채널 수신·SendMessage 의존. Claude 데스크톱 앱 Code 탭은 채널을 받지 못한다 — tmux의 CLI 세션이어야 한다).`

46행 `보고는 \`task watch\`의 폴링이 같은 inbox 형식으로 만든다(감시 상시)` → `보고는 채널 서버의 원격 폴링이 같은 inbox 형식으로 만들어 채널로 온다`.

64행 `Monitor 이벤트 \`to: DONE_UNREAD\`가 오면 성공` → `채널 메시지 \`event="DONE_UNREAD"\`가 오면 성공`, `Monitor에 \`to: READY, task_id: LAB-2\`` → `채널에 \`event="READY" task="LAB-2"\``.

- [ ] **Step 5: doctor 구현** — `cmd_doctor`에서 총괄 봇 `ok(...)`/`warn(...)` 다음에:

```python
    inbox = str(root / "runtime" / "inbox")
    claude = which("claude")
    got = subprocess.run([claude, "mcp", "get", "agentlayer"], cwd=root, capture_output=True, text=True) if claude else None
    if got and got.returncode == 0 and f"channel serve {inbox}" in got.stdout:
        ok(f"수신함 채널: agentlayer channel serve {inbox} (claude mcp local)")
    else:
        warn(f"수신함 채널 미등록 — companyctl install --root {root} (claude mcp add -s local agentlayer -- agentlayer channel serve {inbox})")
    if mgr:
        if "server:agentlayer" in (bots[mgr].get("dev_channels") or []):
            ok(f"총괄 봇 개발 채널 플래그: {mgr} dev_channels=server:agentlayer")
        else:
            warn(f"총괄 봇 개발 채널 플래그 없음 — companyctl install --root {root} (botctl add … --dev-channel server:agentlayer)")
```

- [ ] **Step 6: 통과 확인**

Run: `python3 -m pytest tests -q 2>&1 | tail -3`
Expected: 전부 passed

- [ ] **Step 7: 커밋**

```bash
git add plugins/ai-company/skills/configure-company/assets/company-block.md plugins/ai-company/skills/configure-company/SKILL.md plugins/ai-company/skills/configure-company/generator/companyctl.py tests/test_companyctl.py
git commit -m "feat(block): 총괄 감시(Monitor) 절을 채널 수신 절로 교체, doctor에 채널 등록·봇 플래그 점검"
```

### Task 11: ai-company 0.2.9 릴리즈

**Files:**
- Modify: `plugins/ai-company/.claude-plugin/plugin.json:3`, `.claude-plugin/marketplace.json:9` (0.2.8 → 0.2.9)
- Modify: `README.md` (요구 사항: agentlayer ≥ 1.10.0, folder-bot ≥ 0.1.22)

- [ ] **Step 1: 버전·README·커밋·태그·GitHub 릴리즈**

```bash
sed -i '' 's/"version": "0.2.8"/"version": "0.2.9"/' plugins/ai-company/.claude-plugin/plugin.json .claude-plugin/marketplace.json
# README 요구 사항 절: agentlayer 1.10.0+, folder-bot 0.1.22+ 로 갱신
git add -A && git commit -m "chore: 0.2.9 — 채널 수신(agentlayer 1.10.0+, folder-bot 0.1.22+)" && git tag v0.2.9 && git push && git push --tags
gh release create v0.2.9 --title "v0.2.9" --notes "총괄 수신함을 Claude Code 채널로 — Monitor 재무장 제거. 요구: agentlayer 1.10.0+, folder-bot 0.1.22+"
```

- [ ] **Step 2: 이 맥에 플러그인 갱신** — `/plugin`에서 ai-company 0.2.9.

---

## Part D — 실회사 전환·실측

### Task 12: 전환 (블록 교체 + 등록 + 재기동을 한 번에)

**Files:** 없음(실회사 `~/ai-folder/company` 상태 변경). 기록: agentlayer `SESSION.md` 결정 기록.

- [ ] **Step 1: 사전 점검**

```bash
agentlayer version | head -1            # v1.10.0
ls ~/.claude/plugins/cache/folder-bot/folder-bot/ | tail -1   # 0.1.22
C=$(ls -d ~/.claude/plugins/cache/ai-company/ai-company/*/ | tail -1); echo $C   # 0.2.9
agentlayer status | grep company-bot     # 현재 상태(WORK면 끝날 때까지 기다림)
```

- [ ] **Step 2: install(등록·플래그·블록·재시작 한 번에)**

```bash
python3 $C/skills/configure-company/generator/companyctl.py install --root ~/ai-folder/company
```

Expected 출력에 `채널 등록: …`, `총괄 봇 플래그: company …`, `블록 갱신…`, `총괄 봇 재시작: company-bot — ✅ …`.

- [ ] **Step 3: 재기동 확인**

```bash
sleep 20; tmux capture-pane -p -t company-bot: | grep -E 'Channels \(experimental\)|Loading development channels' | head -2
tail -3 ~/.claude/logs/bot-up.log     # "dev-channel: 확인창 통과"
cd ~/ai-folder/company && claude mcp get agentlayer | grep -E 'Status|Args'
```

Expected: pane에 `Channels (experimental) messages from server:agentlayer …` 배너, 로그에 통과, `Status: ✔ Connected`, `Args: channel serve /Users/soonho/ai-folder/company/runtime/inbox`.

- [ ] **Step 4: 재정박 + 편지 실측**

총괄 채널(디스코드)에 "이어서하자". 총괄이 Monitor를 띄우지 않는지 pane에서 확인(`Monitor` 도구 호출 없음). 직원 세션 하나(예: collab-bot pane)에서:

```bash
agentlayer task message "채널 전환 테스트 편지 — 받으면 '편지 수신 확인'이라고만 답해 주세요"
```

Expected: 총괄 pane에 `← agentlayer: {"version":1,…"to":"MESSAGE"…}` 도착 → 총괄이 디스코드에 답. `ls ~/ai-folder/company/runtime/inbox/received | tail -1`이 방금 편지.

- [ ] **Step 5: 원격(Hermes) 편지 실측(선택, 서버가 살아 있을 때)**

Hermes 스킬 `imac-letter`로 편지 1통 → 총괄 채널 메시지 `origin="remote"` 도착 → `task reply` 답장까지.

- [ ] **Step 6: 다음 날 비용 확인**

총괄 transcript(`~/.claude/projects/-Users-soonho-ai-folder-company/*.jsonl` 최신)에서 `Monitor` 호출·"만료" 재무장 턴이 0회인지, 편지 0건 구간에 턴이 없는지 확인. 결과를 agentlayer `SESSION.md` 결정 기록에 한 줄.

- [ ] **Step 7: 총괄 → agentlayer-dev 세션 보고**

총괄 세션(company-bot)에 결과를 알린다(`agentlayer send company-bot "채널 전환 완료 — Monitor 재무장 절차 삭제됨, 이후 편지는 <channel source=\"agentlayer\">로 옴"`). 총괄 쪽 후속(기동 인자·CLAUDE.md·재무장 삭제)은 install이 이미 처리했으므로 총괄이 따로 할 일은 없다고 함께 적는다.
