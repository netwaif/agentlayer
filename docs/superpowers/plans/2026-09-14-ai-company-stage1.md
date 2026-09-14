# AI 회사 1단계 — agentlayer v1.5.0 배관 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 총괄 세션이 직원 세션 하나에 지시를 넣고(`send`), 업무를 등록하면(`task assign`) 직원의 상태 전이(DONE·WAIT·ERR)가 훅에서 자동으로 총괄 수신함에 떨어지고(`task watch`) 상주 수신되게 한다.

**Architecture:** 새 패키지 `internal/task`가 업무 등록 파일(`<state>/tasks/<agent-id>.json`)·보고 파일(`<inbox>/pending/<id>.json`)·수신 루프의 순수 로직을 맡는다. `internal/cli`에 `send`·`task` 명령의 얇은 CLI 층을 두고, `main.go`가 라우팅과 훅 전이 콜백에서의 보고 호출을 잇는다. tmux 전송은 기존 `tmuxx.SendText`를 그대로 쓴다.

**Tech Stack:** Go 1.25, 표준 라이브러리만(uuid는 `crypto/rand` 16바이트 hex). 테스트는 `go test`, 페이크는 인터페이스 주입.

**Spec:** `docs/superpowers/specs/2026-09-14-ai-company-design.md` §1

## Global Constraints

- 상태 판정은 훅·tmux 메타데이터로만. `CapturePane`은 절대 상태 판정에 쓰지 않는다(README 상태 모델).
- 훅은 에이전트를 막지 않는다 — 보고 실패는 stderr 한 줄, 에러 반환 없음.
- 파일 쓰기는 임시 파일 + `os.Rename` 원자적. 디렉터리 권한 0700, 파일 0600.
- 업무ID는 `^[A-Za-z0-9._-]{1,64}$`. 보고 파일 16KiB 초과·심볼릭 링크·깨진 JSON은 `quarantine/`.
- 전송 게이트: `IDLE`·`DONE_UNREAD`만 전송. `WORKING`·`WAITING`은 `--force` 없으면 거부. `DEAD`·`ERROR`는 항상 거부.
- 대상 표기 `<세션>` 또는 `<세션>:<창이름>`. 둘 이상 일치하면 오류.
- 모든 사용자 문구는 한국어. `gofmt` 적용. 커밋 메시지는 `feat(task):`·`feat(send):` 접두 + 마지막에 attribution 두 줄.
- 기존 명령(`broadcast`·`wt send`)은 건드리지 않는다.

---

## File Structure

| 파일 | 책임 |
|---|---|
| `internal/task/assign.go` (신규) | 업무 등록 파일: `Assignment` 타입, `Dir`, `Assign`, `Load`, `List`, `Done`, `ValidID` |
| `internal/task/report.go` (신규) | 보고: `Report` 타입, `ShouldReport`, `ReportFor`, `WriteReport` |
| `internal/task/watch.go` (신규) | 수신: `Poll`(한 바퀴), `Watch`(루프), 격리 규칙 |
| `internal/task/*_test.go` (신규) | 위 셋의 단위 테스트 |
| `internal/cli/sendcmd.go` (신규) | `ResolveTarget`, `SendGate`, `TextSender`, `RunSend` |
| `internal/cli/taskcmd.go` (신규) | `RunTask` — assign·list·done·watch 서브명령 |
| `internal/cli/sendcmd_test.go`·`taskcmd_test.go` (신규) | CLI 층 테스트 |
| `internal/cli/helpcmd.go` (수정) | 도움말 두 줄 추가 |
| `main.go` (수정) | `send`·`task` 라우팅, 훅 콜백에서 `task.ReportFor`+`WriteReport` |
| `README.md` (수정) | "세션 지시·업무 보고" 절 |

---

### Task 1: `internal/task` 업무 등록 파일

**Files:**
- Create: `internal/task/assign.go`
- Test: `internal/task/assign_test.go`

**Interfaces:**
- Produces:
  ```go
  type Assignment struct {
      TaskID     string    `json:"task_id"`
      AgentID    string    `json:"agent_id"`
      Session    string    `json:"session"`
      Window     string    `json:"window,omitempty"`
      Pane       string    `json:"pane"`
      Inbox      string    `json:"inbox"`
      AssignedAt time.Time `json:"assigned_at"`
  }
  func Dir(stateDir string) string                              // <stateDir>/tasks
  func ValidID(id string) bool
  func Assign(stateDir string, as Assignment, replace bool) error
  func Load(stateDir, agentID string) (*Assignment, bool, error)  // 없으면 ok=false
  func List(stateDir string) ([]Assignment, error)                // TaskID 오름차순
  func Done(stateDir, taskID string) (bool, error)                // 없으면 false
  var ErrAlreadyAssigned = errors.New("이 세션에는 이미 업무가 있습니다 (--replace로 교체)")
  ```

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/task/assign_test.go
package task

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sample(id, agent string) Assignment {
	return Assignment{TaskID: id, AgentID: agent, Session: "search-youtube-bot", Window: "t170966",
		Pane: "%16", Inbox: "/tmp/inbox", AssignedAt: time.Date(2026, 9, 14, 15, 0, 0, 0, time.UTC)}
}

func TestValidID(t *testing.T) {
	for _, ok := range []string{"VIDEO-07", "a.b_c", "x"} {
		if !ValidID(ok) {
			t.Errorf("%q는 유효해야 함", ok)
		}
	}
	for _, bad := range []string{"", "한글", "a b", "a/b", string(make([]byte, 65))} {
		if ValidID(bad) {
			t.Errorf("%q는 거부돼야 함", bad)
		}
	}
}

func TestAssignLoadListDone(t *testing.T) {
	dir := t.TempDir()
	if err := Assign(dir, sample("T-1", "claude-%16"), false); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(dir, "claude-%16")
	if err != nil || !ok || got.TaskID != "T-1" || got.Window != "t170966" {
		t.Fatalf("Load: %+v ok=%v err=%v", got, ok, err)
	}
	if _, ok, _ := Load(dir, "없음"); ok {
		t.Error("없는 에이전트는 ok=false")
	}
	if err := Assign(dir, sample("T-2", "claude-%16"), false); !errors.Is(err, ErrAlreadyAssigned) {
		t.Fatalf("중복 배정은 ErrAlreadyAssigned: %v", err)
	}
	if err := Assign(dir, sample("T-2", "claude-%16"), true); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if err := Assign(dir, sample("A-0", "codex-%2"), false); err != nil {
		t.Fatal(err)
	}
	list, err := List(dir)
	if err != nil || len(list) != 2 || list[0].TaskID != "A-0" || list[1].TaskID != "T-2" {
		t.Fatalf("List 정렬(TaskID 오름차순): %+v err=%v", list, err)
	}
	if done, err := Done(dir, "T-2"); err != nil || !done {
		t.Fatalf("Done: %v %v", done, err)
	}
	if done, _ := Done(dir, "T-2"); done {
		t.Error("두 번째 Done은 false")
	}
	if _, err := os.Stat(filepath.Join(Dir(dir), "claude-%16.json")); !os.IsNotExist(err) {
		t.Error("Done 뒤 파일이 남아 있음")
	}
}

func TestAssignRejectsBadID(t *testing.T) {
	if err := Assign(t.TempDir(), sample("bad id", "x"), false); err == nil {
		t.Error("잘못된 업무ID는 거부")
	}
}

func TestListEmptyDir(t *testing.T) {
	list, err := List(t.TempDir())
	if err != nil || len(list) != 0 {
		t.Fatalf("tasks/ 없음 → 빈 목록: %v %v", list, err)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/task -run 'TestValidID|TestAssign|TestList' -count=1`
Expected: FAIL — `undefined: Assignment` 등 컴파일 오류

- [ ] **Step 3: 최소 구현**

```go
// internal/task/assign.go
// Package task는 AI 회사 배관 — 세션에 업무를 등록하고, 상태 전이를 총괄 수신함에 보고하고,
// 수신함을 상주 감시한다. 정본은 파일이며 데몬이 없다(agentlayer 원칙).
package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Assignment는 세션(pane) 하나 ↔ 업무 하나. 파일은 <state>/tasks/<agent-id>.json.
type Assignment struct {
	TaskID     string    `json:"task_id"`
	AgentID    string    `json:"agent_id"`
	Session    string    `json:"session"`
	Window     string    `json:"window,omitempty"`
	Pane       string    `json:"pane"`
	Inbox      string    `json:"inbox"`
	AssignedAt time.Time `json:"assigned_at"`
}

var ErrAlreadyAssigned = errors.New("이 세션에는 이미 업무가 있습니다 (--replace로 교체)")

var idRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// ValidID는 업무ID 규칙(영숫자·점·밑줄·하이픈, 1~64자).
func ValidID(id string) bool { return idRe.MatchString(id) }

// Dir은 업무 등록 파일 디렉터리.
func Dir(stateDir string) string { return filepath.Join(stateDir, "tasks") }

func path(stateDir, agentID string) string {
	return filepath.Join(Dir(stateDir), agentID+".json")
}

// Assign은 등록 파일을 원자적으로 쓴다. 같은 에이전트에 이미 있으면 replace가 아닌 한 거부.
func Assign(stateDir string, as Assignment, replace bool) error {
	if !ValidID(as.TaskID) {
		return fmt.Errorf("업무ID 형식 오류: %q (영숫자·점·밑줄·하이픈 1~64자)", as.TaskID)
	}
	if as.AgentID == "" {
		return errors.New("에이전트 ID가 비었습니다")
	}
	if _, ok, err := Load(stateDir, as.AgentID); err != nil {
		return err
	} else if ok && !replace {
		return ErrAlreadyAssigned
	}
	if err := os.MkdirAll(Dir(stateDir), 0o700); err != nil {
		return err
	}
	return writeAtomic(path(stateDir, as.AgentID), as)
}

// Load는 에이전트의 등록을 읽는다. 없으면 ok=false, 에러 없음.
func Load(stateDir, agentID string) (*Assignment, bool, error) {
	b, err := os.ReadFile(path(stateDir, agentID))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var as Assignment
	if err := json.Unmarshal(b, &as); err != nil {
		return nil, false, fmt.Errorf("등록 파일 손상 %s: %w", path(stateDir, agentID), err)
	}
	return &as, true, nil
}

// List는 모든 등록을 TaskID 오름차순으로. 디렉터리가 없으면 빈 목록.
func List(stateDir string) ([]Assignment, error) {
	entries, err := os.ReadDir(Dir(stateDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Assignment
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(Dir(stateDir), e.Name()))
		if err != nil {
			continue
		}
		var as Assignment
		if json.Unmarshal(b, &as) == nil {
			out = append(out, as)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TaskID < out[j].TaskID })
	return out, nil
}

// Done은 업무ID로 등록을 해제한다. 보고 파일은 건드리지 않는다.
func Done(stateDir, taskID string) (bool, error) {
	list, err := List(stateDir)
	if err != nil {
		return false, err
	}
	found := false
	for _, as := range list {
		if as.TaskID != taskID {
			continue
		}
		if err := os.Remove(path(stateDir, as.AgentID)); err != nil && !os.IsNotExist(err) {
			return false, err
		}
		found = true
	}
	return found, nil
}

// writeAtomic은 temp→rename. 여러 프로세스(hook·CLI)가 동시에 써도 반쪽 파일이 안 생긴다.
func writeAtomic(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*.tmp")
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
```

- [ ] **Step 4: 통과 확인**

Run: `gofmt -l internal/task && go vet ./internal/task && go test ./internal/task -count=1`
Expected: `ok`

- [ ] **Step 5: 커밋**

```bash
git add internal/task/assign.go internal/task/assign_test.go
git commit -m "feat(task): 업무 등록 파일 — Assign/Load/List/Done, 업무ID 검증"
```

---

### Task 2: `internal/task` 보고 파일

**Files:**
- Create: `internal/task/report.go`
- Test: `internal/task/report_test.go`

**Interfaces:**
- Consumes: `Assignment`, `Load` (Task 1); `state.Agent`, `state.AgentState`
- Produces:
  ```go
  type Report struct {
      Version int       `json:"version"`   // 항상 1
      ID      string    `json:"id"`        // 32자 hex
      TaskID  string    `json:"task_id"`
      Session string    `json:"session"`
      Window  string    `json:"window,omitempty"`
      Kind    string    `json:"kind"`
      From    string    `json:"from"`      // 이전 상태
      To      string    `json:"to"`        // DONE_UNREAD | WAITING | ERROR
      Task    string    `json:"task,omitempty"` // Agent.Task 요약
      Ask     string    `json:"ask,omitempty"`  // Agent.Ask (WAITING일 때)
      CWD     string    `json:"cwd,omitempty"`
      At      time.Time `json:"at"`
      Inbox   string    `json:"-"`         // 쓸 곳
  }
  func ShouldReport(prev, to state.AgentState) bool
  func ReportFor(stateDir string, a *state.Agent, prev, to state.AgentState, now time.Time) (*Report, bool)
  func WriteReport(r *Report) (string, error)   // 쓴 파일 경로
  func NewID() string
  ```

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/task/report_test.go
package task

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

func TestShouldReportTable(t *testing.T) {
	cases := []struct {
		prev, to state.AgentState
		want     bool
	}{
		{state.StateWorking, state.StateDoneUnread, true},
		{state.StateWorking, state.StateWaiting, true},
		{state.StateWorking, state.StateError, true},
		{state.StateWorking, state.StateWorking, false}, // heartbeat
		{state.StateWaiting, state.StateWorking, false}, // 승인됨 — 총괄이 알 필요 없음
		{state.StateDoneUnread, state.StateIdle, false},
		{state.StateIdle, state.StateDead, false}, // dead는 scan 몫, v1 범위 밖
		{state.StateDoneUnread, state.StateDoneUnread, false},
	}
	for _, c := range cases {
		if got := ShouldReport(c.prev, c.to); got != c.want {
			t.Errorf("%s→%s = %v, want %v", c.prev, c.to, got, c.want)
		}
	}
}

func TestReportForUnassignedIsSilent(t *testing.T) {
	a := &state.Agent{ID: "claude-%1", Kind: "claude", Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}}
	if _, ok := ReportFor(t.TempDir(), a, state.StateWorking, state.StateDoneUnread); ok {
		t.Error("등록 안 된 세션은 보고하지 않음")
	}
}

func TestReportForAssignedAndWrite(t *testing.T) {
	stateDir, inbox := t.TempDir(), filepath.Join(t.TempDir(), "inbox")
	if err := Assign(stateDir, Assignment{TaskID: "T-1", AgentID: "claude-%16", Session: "search-youtube-bot",
		Window: "t170966", Pane: "%16", Inbox: inbox}, false); err != nil {
		t.Fatal(err)
	}
	a := &state.Agent{ID: "claude-%16", Kind: "claude", Task: "주제 3개 정리", Ask: "Claude needs your permission",
		CWD: "/tmp/w", Tmux: state.TmuxRef{Session: "search-youtube-bot", WindowName: "t170966", PaneID: "%16"}}
	now := time.Date(2026, 9, 14, 15, 7, 12, 0, time.UTC)
	r, ok := ReportFor(stateDir, a, state.StateWorking, state.StateWaiting, now)
	if !ok {
		t.Fatal("등록된 세션의 WAITING 전이는 보고")
	}
	if r.TaskID != "T-1" || r.To != "WAITING" || r.From != "WORKING" || r.Ask == "" || r.Inbox != inbox || len(r.ID) != 32 {
		t.Fatalf("보고 내용: %+v", r)
	}
	p, err := WriteReport(r)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(p) != filepath.Join(inbox, "pending") || filepath.Base(p) != r.ID+".json" {
		t.Errorf("경로: %s", p)
	}
	b, _ := os.ReadFile(p)
	var back Report
	if err := json.Unmarshal(b, &back); err != nil || back.Version != 1 || back.Task != "주제 3개 정리" || !back.At.Equal(now) {
		t.Fatalf("파일 내용: %s err=%v", b, err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("파일 권한 0600이어야 함: %v", fi.Mode().Perm())
	}
	if _, ok := ReportFor(stateDir, a, state.StateWorking, state.StateWorking, now); ok {
		t.Error("heartbeat는 보고 안 함")
	}
}

func TestNewIDIsHex32Unique(t *testing.T) {
	a, b := NewID(), NewID()
	if len(a) != 32 || a == b {
		t.Errorf("id: %s %s", a, b)
	}
	for _, c := range a {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			t.Fatalf("hex 아님: %s", a)
		}
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/task -run 'TestShouldReport|TestReportFor|TestNewID' -count=1`
Expected: FAIL — `undefined: ShouldReport`

- [ ] **Step 3: 최소 구현**

```go
// internal/task/report.go
package task

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

// Report는 총괄 수신함에 떨어지는 이벤트 한 건. 파일은 <inbox>/pending/<id>.json.
type Report struct {
	Version int       `json:"version"`
	ID      string    `json:"id"`
	TaskID  string    `json:"task_id"`
	Session string    `json:"session"`
	Window  string    `json:"window,omitempty"`
	Kind    string    `json:"kind"`
	From    string    `json:"from"`
	To      string    `json:"to"`
	Task    string    `json:"task,omitempty"`
	Ask     string    `json:"ask,omitempty"`
	CWD     string    `json:"cwd,omitempty"`
	At      time.Time `json:"at"`
	Inbox   string    `json:"-"`
}

// ShouldReport — 총괄이 알아야 하는 것은 "멈췄다"뿐: 끝남·승인 대기·에러.
// heartbeat, 승인됨(WAITING→WORKING), 읽음(DONE→IDLE), dead(scan 몫)는 보고하지 않는다.
func ShouldReport(prev, to state.AgentState) bool {
	if prev == to {
		return false
	}
	switch to {
	case state.StateDoneUnread, state.StateWaiting, state.StateError:
		return true
	}
	return false
}

// ReportFor는 등록된 에이전트의 보고 대상 전이면 Report를 만든다.
func ReportFor(stateDir string, a *state.Agent, prev, to state.AgentState, now time.Time) (*Report, bool) {
	if !ShouldReport(prev, to) {
		return nil, false
	}
	as, ok, err := Load(stateDir, a.ID)
	if err != nil || !ok {
		return nil, false
	}
	r := &Report{Version: 1, ID: NewID(), TaskID: as.TaskID, Session: a.Tmux.Session, Window: a.Tmux.WindowName,
		Kind: a.Kind, From: string(prev), To: string(to), Task: a.Task, CWD: a.CWD, At: now, Inbox: as.Inbox}
	if to == state.StateWaiting {
		r.Ask = a.Ask
	}
	return r, true
}

// WriteReport는 <inbox>/pending/<id>.json을 원자적으로 쓴다. 디렉터리는 0700으로 만든다.
func WriteReport(r *Report) (string, error) {
	dir := filepath.Join(r.Inbox, "pending")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, r.ID+".json")
	return p, writeAtomic(p, r)
}

// NewID는 32자 hex(128비트 난수). 외부 uuid 의존 없이 충분히 유일하다.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 난수 실패는 사실상 없지만, 시각 기반으로라도 유일하게
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000"))[:16])
	}
	return hex.EncodeToString(b[:])
}
```

- [ ] **Step 4: 통과 확인**

Run: `gofmt -l internal/task && go vet ./internal/task && go test ./internal/task -count=1`
Expected: `ok`

- [ ] **Step 5: 커밋**

```bash
git add internal/task/report.go internal/task/report_test.go
git commit -m "feat(task): 상태 전이 보고 파일 — ShouldReport/ReportFor/WriteReport"
```

---

### Task 3: `internal/task` 수신함 감시

**Files:**
- Create: `internal/task/watch.go`
- Test: `internal/task/watch_test.go`

**Interfaces:**
- Consumes: `Report`(Task 2), `writeAtomic`(Task 1)
- Produces:
  ```go
  const MaxReportBytes = 16384
  func Poll(inbox string) (*Report, bool, error)     // pending 한 건 처리(정상→received, 불량→quarantine). 없으면 ok=false
  func Watch(ctx context.Context, inbox string, interval time.Duration, once bool, emit func(*Report)) error
  ```

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/task/watch_test.go
package task

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writePending(t *testing.T, inbox, name, body string) string {
	t.Helper()
	dir := filepath.Join(inbox, "pending")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func goodReport(id string) string {
	b, _ := json.Marshal(Report{Version: 1, ID: id, TaskID: "T-1", Session: "s", Kind: "claude",
		From: "WORKING", To: "DONE_UNREAD", At: time.Now()})
	return string(b)
}

func TestPollMovesGoodToReceived(t *testing.T) {
	inbox := t.TempDir()
	id := strings.Repeat("a", 32)
	writePending(t, inbox, id+".json", goodReport(id))
	r, ok, err := Poll(inbox)
	if err != nil || !ok || r.ID != id || r.To != "DONE_UNREAD" {
		t.Fatalf("Poll: %+v ok=%v err=%v", r, ok, err)
	}
	if _, err := os.Stat(filepath.Join(inbox, "received", id+".json")); err != nil {
		t.Error("received/로 이동돼야 함")
	}
	if _, ok, _ := Poll(inbox); ok {
		t.Error("두 번째 Poll은 비어 있음")
	}
}

func TestPollQuarantinesBadFiles(t *testing.T) {
	inbox := t.TempDir()
	id := strings.Repeat("b", 32)
	writePending(t, inbox, "broken.json", "{not json")
	writePending(t, inbox, strings.Repeat("c", 32)+".json", `{"version":2,"id":"`+strings.Repeat("c", 32)+`"}`)
	writePending(t, inbox, "mismatch.json", goodReport(id)) // 파일명≠id
	writePending(t, inbox, strings.Repeat("d", 32)+".json", strings.Repeat("x", MaxReportBytes+1))
	target := writePending(t, inbox, "real.txt", goodReport(id))
	os.Symlink(target, filepath.Join(inbox, "pending", id+".json"))
	for i := 0; i < 5; i++ {
		if _, ok, err := Poll(inbox); ok || err != nil {
			t.Fatalf("불량 파일은 ok=false·에러 없음 (%d): ok=%v err=%v", i, ok, err)
		}
	}
	q, _ := os.ReadDir(filepath.Join(inbox, "quarantine"))
	if len(q) != 5 {
		t.Errorf("quarantine 5건이어야 함: %d", len(q))
	}
	if left, _ := filepath.Glob(filepath.Join(inbox, "pending", "*.json")); len(left) != 0 {
		t.Errorf("pending에 json이 남으면 안 됨: %v", left)
	}
}

func TestWatchOnceEmitsAndReturns(t *testing.T) {
	inbox := t.TempDir()
	id := strings.Repeat("e", 32)
	var got []*Report
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		time.Sleep(100 * time.Millisecond)
		writePending(t, inbox, id+".json", goodReport(id))
	}()
	if err := Watch(ctx, inbox, 20*time.Millisecond, true, func(r *Report) { got = append(got, r) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != id {
		t.Fatalf("한 건 수신 후 종료: %+v", got)
	}
}

func TestWatchStopsOnContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	err := Watch(ctx, t.TempDir(), 10*time.Millisecond, false, func(*Report) {})
	if err != context.DeadlineExceeded {
		t.Fatalf("컨텍스트 만료로 종료: %v", err)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/task -run 'TestPoll|TestWatch' -count=1`
Expected: FAIL — `undefined: Poll`

- [ ] **Step 3: 최소 구현**

```go
// internal/task/watch.go
package task

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// MaxReportBytes — 보고 한 건 상한. 넘으면 격리.
const MaxReportBytes = 16384

// Poll은 pending/의 json 파일을 이름순으로 하나씩 검사해 첫 정상 건을 received/로 옮기고 반환한다.
// 불량(심볼릭 링크·과대·깨진 JSON·version≠1·파일명≠id)은 quarantine/로 옮기고 계속 본다.
func Poll(inbox string) (*Report, bool, error) {
	for _, sub := range []string{"pending", "received", "quarantine"} {
		if err := os.MkdirAll(filepath.Join(inbox, sub), 0o700); err != nil {
			return nil, false, err
		}
	}
	files, err := filepath.Glob(filepath.Join(inbox, "pending", "*.json"))
	if err != nil {
		return nil, false, err
	}
	sort.Strings(files)
	for _, f := range files {
		r, verr := readReport(f)
		if verr != nil {
			_ = os.Rename(f, filepath.Join(inbox, "quarantine", filepath.Base(f)))
			continue
		}
		if err := os.Rename(f, filepath.Join(inbox, "received", filepath.Base(f))); err != nil {
			return nil, false, err
		}
		return r, true, nil
	}
	return nil, false, nil
}

func readReport(f string) (*Report, error) {
	fi, err := os.Lstat(f)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("symlink")
	}
	if fi.Size() > MaxReportBytes {
		return nil, errors.New("oversize")
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}
	var r Report
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	base := filepath.Base(f)
	if r.Version != 1 || len(r.ID) != 32 || base != r.ID+".json" || r.TaskID == "" || r.To == "" {
		return nil, errors.New("identity")
	}
	return &r, nil
}

// Watch는 interval마다 Poll하고 정상 건마다 emit을 부른다. once면 첫 건 뒤 nil로 종료,
// 아니면 ctx가 끝날 때까지 돈다(ctx.Err() 반환).
func Watch(ctx context.Context, inbox string, interval time.Duration, once bool, emit func(*Report)) error {
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		for {
			r, ok, err := Poll(inbox)
			if err != nil {
				return err
			}
			if !ok {
				break
			}
			emit(r)
			if once {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
```

- [ ] **Step 4: 통과 확인**

Run: `gofmt -l internal/task && go vet ./internal/task && go test ./internal/task -count=1`
Expected: `ok`

- [ ] **Step 5: 커밋**

```bash
git add internal/task/watch.go internal/task/watch_test.go
git commit -m "feat(task): 수신함 감시 — Poll(격리 규칙)·Watch(상주/once)"
```

---

### Task 4: `agentlayer send` CLI 층

**Files:**
- Create: `internal/cli/sendcmd.go`
- Test: `internal/cli/sendcmd_test.go`

**Interfaces:**
- Consumes: `state.Agent`, `state.Store.List`, `tmuxx.Tmux.SendText`
- Produces:
  ```go
  type TextSender interface{ SendText(paneID, text string) error }   // tmuxx.Tmux가 만족
  func ResolveTarget(agents []*state.Agent, spec string) (*state.Agent, error)  // "<세션>" | "<세션>:<창>"
  func SendGate(s state.AgentState, force bool) (ok bool, reason string)
  type SendOptions struct{ Force, JSON bool }
  func ParseSendFlags(args []string) (SendOptions, []string, error)
  func RunSend(w io.Writer, stdin io.Reader, st *state.Store, tm TextSender, args []string) error
  ```

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/cli/sendcmd_test.go
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/netwaif/agentlayer/internal/state"
)

type fakeSender struct{ pane, text string; calls int; fail bool }

func (f *fakeSender) SendText(paneID, text string) error {
	f.calls++
	f.pane, f.text = paneID, text
	if f.fail {
		return errString("boom")
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }

func withWindow(a *state.Agent, w string) *state.Agent { a.Tmux.WindowName = w; return a }

func TestResolveTarget(t *testing.T) {
	agents := []*state.Agent{
		mkAgent("claude", "search-youtube-bot", "%1", state.StateIdle),
		withWindow(mkAgent("claude", "search-youtube-bot", "%16", state.StateIdle), "t170966"),
		mkAgent("claude", "dead-bot", "%4", state.StateDead),
		mkAgent("codex", "codex-live", "%2", state.StateIdle),
	}
	if a, err := ResolveTarget(agents, "codex-live"); err != nil || a.Tmux.PaneID != "%2" {
		t.Fatalf("세션만: %v %v", a, err)
	}
	if a, err := ResolveTarget(agents, "search-youtube-bot:t170966"); err != nil || a.Tmux.PaneID != "%16" {
		t.Fatalf("세션:창: %v %v", a, err)
	}
	if _, err := ResolveTarget(agents, "search-youtube-bot"); err == nil || !strings.Contains(err.Error(), "둘 이상") {
		t.Fatalf("같은 세션에 pane 둘 → 창 명시 요구: %v", err)
	}
	if _, err := ResolveTarget(agents, "없는세션"); err == nil {
		t.Error("없는 세션은 오류")
	}
	if a, err := ResolveTarget(agents, "dead-bot"); err != nil || a.State != state.StateDead {
		t.Fatalf("dead도 해석은 됨(게이트가 거부): %v %v", a, err)
	}
}

func TestSendGate(t *testing.T) {
	cases := []struct {
		s     state.AgentState
		force bool
		ok    bool
	}{
		{state.StateIdle, false, true}, {state.StateDoneUnread, false, true},
		{state.StateWorking, false, false}, {state.StateWorking, true, true},
		{state.StateWaiting, false, false}, {state.StateWaiting, true, true},
		{state.StateDead, true, false}, {state.StateError, true, false},
	}
	for _, c := range cases {
		if ok, _ := SendGate(c.s, c.force); ok != c.ok {
			t.Errorf("%s force=%v: %v", c.s, c.force, ok)
		}
	}
}

func TestParseSendFlags(t *testing.T) {
	o, rest, err := ParseSendFlags([]string{"--force", "--json", "sess", "안녕"})
	if err != nil || !o.Force || !o.JSON || len(rest) != 2 {
		t.Fatalf("%+v %v %v", o, rest, err)
	}
	if _, _, err := ParseSendFlags([]string{"--nope"}); err == nil {
		t.Error("모르는 플래그는 오류")
	}
}

func TestRunSendDeliversAndGates(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	_ = st.Save(mkAgent("claude", "collab-bot", "%1", state.StateIdle))
	_ = st.Save(mkAgent("claude", "busy-bot", "%2", state.StateWorking))
	var out bytes.Buffer
	f := &fakeSender{}
	if err := RunSend(&out, nil, st, f, []string{"collab-bot", "주제", "3개"}); err != nil {
		t.Fatal(err)
	}
	if f.pane != "%1" || f.text != "주제 3개" {
		t.Errorf("전송: %q %q", f.pane, f.text)
	}
	out.Reset()
	f = &fakeSender{}
	err := RunSend(&out, nil, st, f, []string{"busy-bot", "x"})
	if err == nil || f.calls != 0 {
		t.Fatalf("WORKING은 거부(전송 0회): err=%v calls=%d", err, f.calls)
	}
	if err := RunSend(&out, nil, st, f, []string{"--force", "busy-bot", "x"}); err != nil || f.calls != 1 {
		t.Fatalf("--force면 전송: %v %d", err, f.calls)
	}
	// stdin 본문
	f = &fakeSender{}
	if err := RunSend(&out, strings.NewReader("첫 줄\n둘째 줄\n"), st, f, []string{"collab-bot", "-"}); err != nil {
		t.Fatal(err)
	}
	if f.text != "첫 줄\n둘째 줄" {
		t.Errorf("stdin 본문(끝 개행 제거): %q", f.text)
	}
	// --json
	out.Reset()
	if err := RunSend(&out, nil, st, &fakeSender{}, []string{"--json", "collab-bot", "x"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"sent":true`) || !strings.Contains(out.String(), `"pane":"%1"`) {
		t.Errorf("json 출력: %s", out.String())
	}
	if err := RunSend(&out, nil, st, &fakeSender{fail: true}, []string{"collab-bot", "x"}); err == nil {
		t.Error("전송 실패는 에러")
	}
	if err := RunSend(&out, nil, st, f, []string{"collab-bot"}); err == nil {
		t.Error("메시지 없으면 오류")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/cli -run 'TestResolveTarget|TestSendGate|TestParseSendFlags|TestRunSend' -count=1`
Expected: FAIL — `undefined: ResolveTarget`

- [ ] **Step 3: 최소 구현**

```go
// internal/cli/sendcmd.go
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/netwaif/agentlayer/internal/state"
)

// TextSender는 pane에 지시를 넣는 최소 인터페이스 — tmuxx.Tmux가 만족하고 테스트는 페이크.
type TextSender interface {
	SendText(paneID, text string) error
}

// ResolveTarget은 "<세션>" 또는 "<세션>:<창이름>"을 산 pane 하나로 해석한다.
// 창 이름은 folder-bot 스레드 창(t+6자리)을 가리키는 용도. 후보가 둘 이상이면 창 명시를 요구한다.
func ResolveTarget(agents []*state.Agent, spec string) (*state.Agent, error) {
	session, window := spec, ""
	if i := strings.LastIndex(spec, ":"); i > 0 {
		session, window = spec[:i], spec[i+1:]
	}
	var found []*state.Agent
	for _, a := range agents {
		if a.Tmux.Session != session {
			continue
		}
		if window != "" && a.Tmux.WindowName != window {
			continue
		}
		found = append(found, a)
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("세션 %q을 찾지 못했습니다 ('agentlayer status'로 이름 확인)", spec)
	case 1:
		return found[0], nil
	}
	names := make([]string, 0, len(found))
	for _, a := range found {
		names = append(names, fmt.Sprintf("%s:%s(%s)", a.Tmux.Session, a.Tmux.WindowName, a.Tmux.PaneID))
	}
	return nil, fmt.Errorf("세션 %q에 pane이 둘 이상입니다 — 창을 명시하세요: %s", session, strings.Join(names, ", "))
}

// SendGate — idle·DONE만 보낸다. WORK는 현재 턴 뒤에 처리되고 WAIT(승인창)는 입력이 승인창을
// 깨뜨리므로 --force 없이는 거부. dead·ERR는 받을 곳이 없다.
func SendGate(s state.AgentState, force bool) (bool, string) {
	switch s {
	case state.StateIdle, state.StateDoneUnread:
		return true, ""
	case state.StateWorking:
		if force {
			return true, "작업 중 — 현재 턴 뒤에 처리됩니다"
		}
		return false, "작업 중입니다 — 끝난 뒤 보내거나 --force"
	case state.StateWaiting:
		if force {
			return true, "승인 대기 중 — 입력이 승인창에 들어갑니다"
		}
		return false, "승인 대기 중입니다(입력이 승인창을 깨뜨림) — 승인 뒤 보내거나 --force"
	case state.StateDead:
		return false, "세션이 죽었습니다 ('agentlayer restore')"
	default:
		return false, "비정상 종료 상태입니다"
	}
}

type SendOptions struct {
	Force, JSON bool
}

// ParseSendFlags는 --force·--json만 받고 나머지를 위치 인자로 돌려준다.
func ParseSendFlags(args []string) (SendOptions, []string, error) {
	var o SendOptions
	var rest []string
	for _, a := range args {
		switch {
		case a == "--force":
			o.Force = true
		case a == "--json":
			o.JSON = true
		case strings.HasPrefix(a, "--") && len(rest) == 0:
			return o, nil, fmt.Errorf("알 수 없는 플래그: %s", a)
		default:
			rest = append(rest, a)
		}
	}
	return o, rest, nil
}

// RunSend: agentlayer send [--force] [--json] <세션[:창]> <메시지…|->
func RunSend(w io.Writer, stdin io.Reader, st *state.Store, tm TextSender, args []string) error {
	o, rest, err := ParseSendFlags(args)
	if err != nil {
		return err
	}
	if len(rest) < 2 {
		return errors.New("사용법: agentlayer send [--force] [--json] <세션[:창]> <메시지> (여러 줄은 '-'로 stdin)")
	}
	message := strings.Join(rest[1:], " ")
	if message == "-" {
		if stdin == nil {
			return errors.New("stdin이 없습니다")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		message = strings.TrimRight(string(b), "\n")
	}
	if strings.TrimSpace(message) == "" {
		return errors.New("메시지가 비었습니다")
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	a, err := ResolveTarget(agents, rest[0])
	if err != nil {
		return err
	}
	ok, reason := SendGate(a.State, o.Force)
	if !ok {
		return fmt.Errorf("%s(%s): %s", a.Tmux.Session, a.State, reason)
	}
	if err := tm.SendText(a.Tmux.PaneID, message); err != nil {
		return fmt.Errorf("%s 전송 실패: %w", a.Tmux.Session, err)
	}
	if o.JSON {
		return json.NewEncoder(w).Encode(map[string]any{"session": a.Tmux.Session, "window": a.Tmux.WindowName,
			"pane": a.Tmux.PaneID, "state": a.State, "sent": true})
	}
	note := ""
	if reason != "" {
		note = "  ⚠ " + reason
	}
	fmt.Fprintf(w, "전송 완료 → %s %s [%s]%s\n", a.Tmux.Session, a.Tmux.PaneID, a.State, note)
	return nil
}
```

- [ ] **Step 4: 통과 확인**

Run: `gofmt -l internal/cli && go vet ./internal/cli && go test ./internal/cli -count=1`
Expected: `ok`

- [ ] **Step 5: 커밋**

```bash
git add internal/cli/sendcmd.go internal/cli/sendcmd_test.go
git commit -m "feat(send): 세션 하나에 지시 전송 — 대상 해석·상태 게이트·stdin 본문"
```

---

### Task 5: `agentlayer task` CLI 층

**Files:**
- Create: `internal/cli/taskcmd.go`
- Test: `internal/cli/taskcmd_test.go`

**Interfaces:**
- Consumes: `task.Assign/List/Done/Watch/Assignment/Report`(Task 1~3), `ResolveTarget`(Task 4), `cli.PadRight`, `cli.Since`
- Produces:
  ```go
  func RunTask(ctx context.Context, w io.Writer, st *state.Store, stateDir string, args []string, now time.Time) error
  ```
  서브명령: `assign <업무ID> <세션[:창]> --inbox <폴더> [--replace]` / `list [--json]` / `done <업무ID>` / `watch <inbox> [--once] [--interval 200ms]`

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/cli/taskcmd_test.go
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

func TestRunTaskAssignListDone(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.NewStore(dir)
	a := mkAgent("claude", "search-youtube-bot", "%16", state.StateIdle)
	a.Tmux.WindowName = "t170966"
	_ = st.Save(a)
	inbox := filepath.Join(t.TempDir(), "inbox")
	now := time.Now()
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, dir, []string{"assign", "T-1", "search-youtube-bot:t170966", "--inbox", inbox}, now); err != nil {
		t.Fatal(err)
	}
	as, ok, _ := task.Load(dir, a.ID)
	if !ok || as.TaskID != "T-1" || as.Pane != "%16" || as.Window != "t170966" {
		t.Fatalf("등록: %+v ok=%v", as, ok)
	}
	if abs, _ := filepath.Abs(inbox); as.Inbox != abs {
		t.Errorf("inbox는 절대경로로 저장: %s", as.Inbox)
	}
	if err := RunTask(context.Background(), &out, st, dir, []string{"assign", "T-2", "search-youtube-bot:t170966", "--inbox", inbox}, now); err == nil {
		t.Error("중복 배정은 오류")
	}
	if err := RunTask(context.Background(), &out, st, dir, []string{"assign", "T-2", "search-youtube-bot:t170966", "--inbox", inbox, "--replace"}, now); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := RunTask(context.Background(), &out, st, dir, []string{"list"}, now); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "T-2") || !strings.Contains(out.String(), "search-youtube-bot") || !strings.Contains(out.String(), "idle") {
		t.Errorf("list 출력: %s", out.String())
	}
	out.Reset()
	if err := RunTask(context.Background(), &out, st, dir, []string{"list", "--json"}, now); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0]["task_id"] != "T-2" || rows[0]["state"] != "IDLE" {
		t.Errorf("list --json: %s err=%v", out.String(), err)
	}
	out.Reset()
	if err := RunTask(context.Background(), &out, st, dir, []string{"done", "T-2"}, now); err != nil {
		t.Fatal(err)
	}
	if err := RunTask(context.Background(), &out, st, dir, []string{"done", "T-2"}, now); err == nil {
		t.Error("없는 업무 done은 오류")
	}
	if err := RunTask(context.Background(), &out, st, dir, []string{"assign", "T-3", "search-youtube-bot:t170966"}, now); err == nil {
		t.Error("--inbox 없으면 오류")
	}
	if err := RunTask(context.Background(), &out, st, dir, []string{"nope"}, now); err == nil {
		t.Error("모르는 서브명령은 오류")
	}
}

func TestRunTaskWatchOncePrintsJSONLine(t *testing.T) {
	inbox := t.TempDir()
	id := strings.Repeat("f", 32)
	b, _ := json.Marshal(task.Report{Version: 1, ID: id, TaskID: "T-9", Session: "s", Kind: "claude",
		From: "WORKING", To: "DONE_UNREAD", At: time.Now()})
	os.MkdirAll(filepath.Join(inbox, "pending"), 0o700)
	os.WriteFile(filepath.Join(inbox, "pending", id+".json"), b, 0o600)
	var out bytes.Buffer
	st, _ := state.NewStore(t.TempDir())
	if err := RunTask(context.Background(), &out, st, t.TempDir(), []string{"watch", inbox, "--once"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(out.String())
	if strings.Count(line, "\n") != 0 || !strings.Contains(line, `"task_id":"T-9"`) || !strings.Contains(line, `"to":"DONE_UNREAD"`) {
		t.Errorf("한 줄 JSON: %q", line)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/cli -run 'TestRunTask' -count=1`
Expected: FAIL — `undefined: RunTask`

- [ ] **Step 3: 최소 구현**

```go
// internal/cli/taskcmd.go
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

const taskUsage = `사용법:
  agentlayer task assign <업무ID> <세션[:창]> --inbox <폴더> [--replace]
  agentlayer task list [--json]
  agentlayer task done <업무ID>
  agentlayer task watch <inbox> [--once] [--interval 200ms]`

// RunTask — 업무 ↔ 세션 등록과 수신함 감시. 보고 자체는 hook이 쓴다(main.go).
func RunTask(ctx context.Context, w io.Writer, st *state.Store, stateDir string, args []string, now time.Time) error {
	if len(args) == 0 {
		return errors.New(taskUsage)
	}
	switch args[0] {
	case "assign":
		return taskAssign(w, st, stateDir, args[1:], now)
	case "list":
		return taskList(w, st, stateDir, args[1:], now)
	case "done":
		if len(args) < 2 {
			return errors.New(taskUsage)
		}
		found, err := task.Done(stateDir, args[1])
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("업무 %q이 등록돼 있지 않습니다", args[1])
		}
		fmt.Fprintf(w, "업무 %s 등록 해제\n", args[1])
		return nil
	case "watch":
		return taskWatch(ctx, w, args[1:])
	default:
		return fmt.Errorf("알 수 없는 task 명령: %s\n%s", args[0], taskUsage)
	}
}

func taskAssign(w io.Writer, st *state.Store, stateDir string, args []string, now time.Time) error {
	var pos []string
	var inbox string
	replace := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--inbox":
			if i+1 >= len(args) {
				return errors.New("--inbox 뒤에 폴더가 필요합니다")
			}
			inbox = args[i+1]
			i++
		case "--replace":
			replace = true
		default:
			pos = append(pos, args[i])
		}
	}
	if len(pos) != 2 || inbox == "" {
		return errors.New(taskUsage)
	}
	abs, err := filepath.Abs(inbox)
	if err != nil {
		return err
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	a, err := ResolveTarget(agents, pos[1])
	if err != nil {
		return err
	}
	as := task.Assignment{TaskID: pos[0], AgentID: a.ID, Session: a.Tmux.Session, Window: a.Tmux.WindowName,
		Pane: a.Tmux.PaneID, Inbox: abs, AssignedAt: now}
	if err := task.Assign(stateDir, as, replace); err != nil {
		return err
	}
	fmt.Fprintf(w, "업무 %s → %s %s [%s] 등록. 보고는 %s/pending/ 에 떨어집니다.\n",
		as.TaskID, SessionLabel(a), a.Tmux.PaneID, a.State, ShortenHome(abs))
	return nil
}

func taskList(w io.Writer, st *state.Store, stateDir string, args []string, now time.Time) error {
	list, err := task.List(stateDir)
	if err != nil {
		return err
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	byID := map[string]*state.Agent{}
	for _, a := range agents {
		byID[a.ID] = a
	}
	type row struct {
		task.Assignment
		State string `json:"state"`
	}
	var rows []row
	for _, as := range list {
		s := "gone"
		if a, ok := byID[as.AgentID]; ok {
			s = string(a.State)
		}
		rows = append(rows, row{as, s})
	}
	if len(args) > 0 && args[0] == "--json" {
		if rows == nil {
			rows = []row{}
		}
		return json.NewEncoder(w).Encode(rows)
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "등록된 업무 없음.")
		return nil
	}
	fmt.Fprintln(w, PadRight("업무ID", 24)+PadRight("세션", 30)+PadRight("상태", 8)+"경과")
	for _, r := range rows {
		label := r.Session
		if r.Window != "" {
			label += ":" + r.Window
		}
		fmt.Fprintln(w, PadRight(r.TaskID, 24)+PadRight(label, 30)+PadRight(stateWord(r.State), 8)+Since(r.AssignedAt, now))
	}
	return nil
}

// stateWord는 status 표의 단어와 맞춘다(idle·WAIT·DONE·WORK·ERR·dead).
func stateWord(s string) string {
	switch state.AgentState(s) {
	case state.StateIdle:
		return "idle"
	case state.StateWaiting:
		return "WAIT"
	case state.StateDoneUnread:
		return "DONE"
	case state.StateWorking:
		return "WORK"
	case state.StateError:
		return "ERR"
	case state.StateDead:
		return "dead"
	}
	return s
}

func taskWatch(ctx context.Context, w io.Writer, args []string) error {
	if len(args) == 0 {
		return errors.New(taskUsage)
	}
	inbox, once, interval := args[0], false, 200*time.Millisecond
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--once":
			once = true
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
	enc := json.NewEncoder(w)
	err = task.Watch(ctx, abs, interval, once, func(r *task.Report) { _ = enc.Encode(r) })
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

// 미사용 경고 방지용 — strings는 stateWord 확장 때 쓸 수 있으니 유지하지 않는다면 import에서 제거할 것.
var _ = strings.TrimSpace
```

주의: 마지막 `var _ = strings.TrimSpace` 줄과 `strings` import는 실제로 쓰지 않으면 둘 다 지운다(`go vet` 통과 기준).

- [ ] **Step 4: 통과 확인**

Run: `gofmt -l internal/cli && go vet ./internal/cli && go test ./internal/cli -count=1`
Expected: `ok`

- [ ] **Step 5: 커밋**

```bash
git add internal/cli/taskcmd.go internal/cli/taskcmd_test.go
git commit -m "feat(task): CLI — assign/list/done/watch"
```

---

### Task 6: main.go 라우팅·훅 보고·도움말·README

**Files:**
- Modify: `main.go` (`run` switch, `runHook`의 `SetTransitionHook` 콜백)
- Modify: `internal/cli/helpcmd.go` (명령 목록)
- Test: `internal/cli/helpcmd_test.go`(기존 도움말 테스트가 있으면 두 줄 포함 여부 추가)
- Modify: `README.md` ("알림·Discord" 절 뒤에 "세션 지시·업무 보고" 절)

**Interfaces:**
- Consumes: `cli.RunSend`, `cli.RunTask`, `task.ReportFor`, `task.WriteReport`

- [ ] **Step 1: 도움말 테스트 추가**

`internal/cli/helpcmd_test.go`를 열어 기존 테스트 함수 스타일을 확인한 뒤 추가:

```go
func TestHelpMentionsSendAndTask(t *testing.T) {
	h := HelpText()
	for _, want := range []string{"send <세션[:창]>", "task assign"} {
		if !strings.Contains(h, want) {
			t.Errorf("도움말에 %q 누락", want)
		}
	}
}
```

Run: `go test ./internal/cli -run TestHelpMentions -count=1` → Expected: FAIL

- [ ] **Step 2: 도움말 두 줄 추가**

`internal/cli/helpcmd.go`의 `broadcast <메시지>` 줄 바로 아래에:

```
  send <세션[:창]> <메시지|->  세션 하나에 지시 전달 (idle·DONE만; --force로 강제, '-'는 stdin 본문)
  task assign|list|done|watch  업무↔세션 등록·상주 수신 — 등록된 세션의 DONE·WAIT·ERR 전이가 hook에서 <inbox>/pending/ 에 자동 보고됨
```

Run: `go test ./internal/cli -run TestHelp -count=1` → Expected: PASS

- [ ] **Step 3: main.go 라우팅**

`run`의 switch에 `case "wake-all", …` 앞에 추가:

```go
	case "send":
		st, err := state.NewStore(state.DefaultDir())
		if err != nil {
			return err
		}
		if panes, err := (tmuxx.Tmux{}).ListPanes(); err == nil {
			_ = scan.Sync(st, panes, time.Now())
		}
		return cli.RunSend(os.Stdout, os.Stdin, st, tmuxx.Tmux{}, args[1:])
	case "task":
		st, err := state.NewStore(state.DefaultDir())
		if err != nil {
			return err
		}
		if panes, err := (tmuxx.Tmux{}).ListPanes(); err == nil {
			_ = scan.Sync(st, panes, time.Now())
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return cli.RunTask(ctx, os.Stdout, st, state.DefaultDir(), args[1:], time.Now())
```

import에 `context`, `os/signal`, `syscall`, `github.com/netwaif/agentlayer/internal/task` 추가(task는 Step 4에서 씀).

- [ ] **Step 4: 훅 콜백에 보고 연결**

`main.go`의 `runHook`에서 `hookcmd.SetTransitionHook(func(a *state.Agent, prev, to state.AgentState) { … })` 본문 끝(`notify.Notify(...)` 다음)에 추가. 이 콜백보다 위에서 만든 저장소 변수 이름을 `main.go:400-431`에서 확인해 `<store>.Dir`로 쓴다(없으면 `state.DefaultDir()`):

```go
		if rep, ok := task.ReportFor(state.DefaultDir(), a, prev, to, time.Now()); ok {
			if _, err := task.WriteReport(rep); err != nil {
				fmt.Fprintln(os.Stderr, "agentlayer task report:", err) // hook은 에이전트를 막지 않는다
			}
		}
```

- [ ] **Step 5: 빌드·전체 테스트**

Run: `gofmt -l . && go vet ./... && go build ./... && go test ./internal/task ./internal/cli ./internal/hookcmd . -count=1`
Expected: 전부 `ok` (전체 `go test ./...`는 병렬로 돌리면 usage·wiring·wt가 환경 타임아웃을 낼 수 있으니 패키지별로 확인)

- [ ] **Step 6: README 절 추가**

"알림·Discord" 절(README.md 104행 부근) 뒤에:

```markdown
## 세션 지시·업무 보고 (AI 회사 배관)

총괄 세션이 직원 세션에 일을 주고, 직원이 멈추면(끝남·승인 대기·에러) 자동으로 보고받는 최소 배관이다.
직원 지침에 보고 명령이 필요 없다 — 상태 전이가 곧 보고다.

```bash
agentlayer send collab-bot "협업 제안 3건 요약해줘"          # idle·DONE 세션에만 들어간다
agentlayer send search-youtube-bot:t170966 - < 업무요청.md   # 스레드 창(t+6자리)에 여러 줄 본문
agentlayer task assign VIDEO-07 search-youtube-bot:t170966 --inbox ~/ai-folder/company/runtime/inbox
agentlayer task list
agentlayer task watch ~/ai-folder/company/runtime/inbox     # 총괄이 Monitor로 띄워 두는 상주 수신
agentlayer task done VIDEO-07
```

- `send`는 `WORK`(작업 중)·`WAIT`(승인창)에는 넣지 않는다. `--force`로 강제. `dead`는 거부.
- 등록된 세션의 `DONE`·`WAIT`·`ERR` 전이를 hook이 `<inbox>/pending/<id>.json`으로 쓴다
  (`task_id`·세션·창·이전/현재 상태·요약·승인 문구·cwd·시각). heartbeat·승인됨·읽음은 무음.
- `task watch`는 정상 건을 한 줄 JSON으로 출력하고 `received/`로 옮긴다. 깨진 파일·심볼릭 링크·16KiB 초과는 `quarantine/`.
- 세션 소실(dead)은 hook이 아니라 `status`·TUI 실행 때 판정되므로 보고되지 않는다 — `agentlayer status`로 본다.
- 회사 폴더·총괄 절차·직원 등록은 별도 플러그인 `ai-company`가 만든다(이 바이너리는 배관만).
```

- [ ] **Step 7: 커밋**

```bash
git add main.go internal/cli/helpcmd.go internal/cli/helpcmd_test.go README.md
git commit -m "feat: send·task 명령 라우팅, hook 전이 자동 보고, 도움말·README"
```

---

### Task 7: 실측 한 바퀴 → SESSION.md → v1.5.0 릴리즈

**Files:**
- Modify: `SESSION.md` (현재 상태·다음 단계 덮어쓰기, 결정 기록·파일 흔적 추가)
- 릴리즈 노트: scratchpad `notes-1.5.0.md`

- [ ] **Step 1: 로컬 설치**

Run: `make install && ~/.local/bin/agentlayer version && ~/.local/bin/agentlayer help | grep -E 'send|task'`
Expected: 버전 줄 + 도움말 두 줄

- [ ] **Step 2: 실측 — 임시 tmux 세션으로 한 바퀴**

```bash
INBOX=$(mktemp -d)/inbox
tmux new-session -d -s lab-worker -c "$HOME/ai-folder/demo" 'claude'
sleep 8; ~/.local/bin/agentlayer status | grep lab-worker          # idle 확인
~/.local/bin/agentlayer task assign LAB-1 lab-worker --inbox "$INBOX"
~/.local/bin/agentlayer task list
~/.local/bin/agentlayer send lab-worker "도구 없이 'LAB_OK'라고만 답해"
~/.local/bin/agentlayer task watch "$INBOX" --once                  # Stop 훅 → DONE 보고 한 줄
~/.local/bin/agentlayer send lab-worker "x"   # DONE 상태에서 전송되는지(게이트 통과)
tmux kill-session -t lab-worker
~/.local/bin/agentlayer task done LAB-1
ls "$INBOX"/received
```

Expected: `watch --once`가 `{"version":1,…,"task_id":"LAB-1",…,"to":"DONE_UNREAD",…}` 한 줄 출력, `received/`에 파일 1개.
승인 대기(WAIT) 경로도 한 번: 두 번째 지시로 폴더 밖 파일 쓰기를 시키면 권한 프롬프트 → `to:"WAITING"` + `ask` 문구가 보고되는지 확인 후 tmux에서 거부.

- [ ] **Step 3: SESSION.md 갱신**

현재 상태를 "v1.5.0 릴리즈 완료"로 덮어쓰고, 다음 단계 1번을 "2단계 ai-company 플러그인(`netwaif/ai-company`) — 스펙 §2"로. 결정 기록에 오늘 결정 4건(직원층=기존 봇 / 총괄=`~/ai-folder/company` 새 봇 / 배선=agentlayer+플러그인 / dead 보고 범위 밖)과 실측 결과를, 파일 흔적에 신규 파일 목록을 추가.

- [ ] **Step 4: 릴리즈**

```bash
git add SESSION.md && git commit -m "chore: 세션 기록 — v1.5.0(send·task·hook 보고) 실측"
git push origin main
git tag -a v1.5.0 -m "v1.5.0 — send·task·hook 자동 보고 (AI 회사 배관)"
git push origin v1.5.0
export GITHUB_TOKEN=$(gh auth token)
goreleaser release --clean --release-notes <scratchpad>/notes-1.5.0.md   # 분류기 차단 시 사용자 /permissions 승인 후 재실행
gh api repos/netwaif/agentlayer/releases/latest --jq .tag_name           # v1.5.0
```

릴리즈 노트(`notes-1.5.0.md`): 새 명령 `send`·`task assign/list/done/watch`, hook 자동 보고, 업그레이드 명령 `brew upgrade netwaif/tap/agentlayer && agentlayer init`.

Expected: latest v1.5.0, 자산 5개, tap `Casks/agentlayer.rb` 1.5.0. 그 뒤 `make install`로 로컬도 태그 빌드로 교체.

---

## Self-Review 결과

- 스펙 §1.1 send: Task 4·6 ✓ (대상 표기·게이트·`--force`·stdin·`--json`)
- §1.2 task assign/list/done/watch: Task 1·3·5 ✓ (세션당 하나·`--replace`·ID 규칙·watch 격리·`--once`)
- §1.3 훅 보고: Task 2·6 ✓ (DONE·WAIT·ERR만, 미등록 무음, 원자적, dead 범위 밖 명시)
- §1.4 표시: 변경 없음 ✓
- §1.5 테스트·실측: 각 Task 테스트 + Task 7 실측 ✓
- 타입 일관성: `task.Assignment{TaskID, AgentID, Session, Window, Pane, Inbox, AssignedAt}` / `task.Report{…, Inbox}` / `cli.ResolveTarget(agents, spec)` / `cli.TextSender` / `cli.RunTask(ctx, w, st, stateDir, args, now)` — Task 4·5·6에서 같은 시그니처 사용 ✓
- Task 5 구현의 `strings` 미사용 주의 명시 ✓
