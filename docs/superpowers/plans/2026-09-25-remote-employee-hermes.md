# 원격 직원 어댑터 + 편지함 (첫 구현 Hermes) — 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 로컬 AI 회사 총괄이 기존 절차(`task assign`·`send`·Monitor 이벤트·`task done`) 그대로 호스팅어 Hermes 전담 프로필을 직원으로 부리고, 직원(로컬·원격)이 총괄에게 먼저 편지(`MESSAGE`)를 보낼 수 있게 한다.

**Architecture:** `internal/remote`에 어댑터 계약(`Adapter`)과 두 구현(`hermes` 내장, `exec` 범용)을 두고, `send`·`task assign`·`task watch`·`task done`이 대상이 원격이면 어댑터로 분기한다. 보고는 훅 대신 `task watch` 안의 폴링 루프가 어댑터 `Poll`/`Mailbox` 결과를 **기존** `ApplyTransition`·`ReportFor`·`WriteReport`로 흘려 inbox 형식을 그대로 유지한다. 원격 레코드는 에이전트 저장소(`agents/`)에 넣지 않는다.

**Tech Stack:** Go 1.25 표준 라이브러리만(`os/exec`, `archive/tar`, `encoding/json`). ssh는 로컬 `ssh` 바이너리(ControlMaster). 테스트는 `testing` + 페이크(`FakeRunner`·`fakeAdapter`).

**Spec:** `docs/superpowers/specs/2026-09-25-remote-employee-hermes-design.md`

## Global Constraints

- 데몬 없음: 폴링은 총괄이 켜 둔 `task watch` 프로세스 안에서만 돈다.
- 원격 레코드를 `~/.local/state/agentlayer/agents/`에 쓰지 않는다(`scan.Sync`가 DEAD로 만든다).
- 원격으로 가는 텍스트는 로컬 셸을 거치지 않는다(argv). 원격 셸 문자열은 `ShellQuote` 하나로만 만든다.
- 보고 파일 형식은 기존 `task.Report`(version 1) 그대로. watch 검증(version·id 32자·파일명·task_id·to 비어 있지 않음)을 통과해야 한다.
- 상태 값 문자열은 기존 그대로: `WORKING`·`WAITING`·`DONE_UNREAD`·`IDLE`·`ERROR`. 새 `to` 값은 `MESSAGE` 하나.
- 타임아웃: 조회 30초, 지시·답변 60초, 산출물 회수 5분. 산출물 상한 50MiB.
- 테스트는 패키지별로 실행한다(`go test ./internal/remote/`, `./internal/task/`, `./internal/cli/`). `go test ./...` 전체 금지(멈춤).
- 호스팅어 실측값: ssh 호스트 `hostinger`, exec 접두어 `docker exec -i -u hermes hermes-agent-iqxn-hermes-agent-1`, 프로필 `tech-qa`, workspace root `/opt/data/ai-company/결과물`, 편지함 담당자 `imac-manager`.
- 커밋 메시지 끝에 `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` 와 `Claude-Session: https://claude.ai/code/session_01JWnLtcFr2Aoaq9RRuyQzCZ` 두 줄.

## Review Focus

1. 본문에 `'`·`$`·백틱·개행·한글이 섞인 업무요청을 `send`했을 때 원격 `--body`가 글자 그대로 들어가야 한다 → Task 2 `TestShellQuoteRoundTrip`.
2. `dispatch`가 우리 카드가 아닌 다른 ready 카드를 띄우고 우리 카드는 안 띄운 경우, `send`는 카드를 남긴 채 실패해야 하고 다음 `send`가 같은 카드를 재사용(idempotency)해야 한다 → Task 4 `TestHermesDispatchNotSpawned`, Task 9 `TestSendRemoteRetryAfterDispatchFailure`.
3. 감시가 꺼져 있던 동안 blocked→done까지 지나간 카드는 다시 켰을 때 `DONE_UNREAD` 한 번만 보고해야 한다(중간 WAITING 생략) → Task 7 `TestApplyRemoteStatusSkipsIntermediate`.
4. 회수한 tar에 `../` 경로나 절대 경로·심볼릭 링크가 섞여 있으면 `결과물/` 밖에 쓰면 안 된다 → Task 5 `TestUntarRejectsEscape`.
5. tmux 밖(예: 총괄이 실수로 자기 pane이 아닌 곳)에서 `task message`를 실행하면 거부해야 하고, `--task`가 이 세션에 등록되지 않은 업무면 `[MESSAGE]` 로그 없이 이벤트만 써야 한다 → Task 11 `TestTaskMessageOutsideTmux`, `TestTaskMessageUnassignedTaskNoLog`.

---

## 파일 구조

| 파일 | 책임 |
|---|---|
| `internal/remote/registry.go` | 원격 직원 등록 파일(`remotes/<이름>.json`) 읽기·쓰기·검증 |
| `internal/remote/shell.go` | `Runner` 인터페이스, `ShellQuote`/`ShellJoin`, `SSHRunner` |
| `internal/remote/adapter.go` | `Adapter` 계약, `Status`·`Letter`·`Info`, `Open` 팩토리 |
| `internal/remote/hermes.go` | Hermes 칸반 어댑터(카드 JSON 파싱·상태 대응 포함) |
| `internal/remote/untar.go` | 안전한 tar 풀기(상한·경로 탈출 거부) |
| `internal/remote/exec.go` | 범용 `exec` 어댑터(명령 템플릿 + JSON 응답) |
| `internal/task/assign.go` | `Assignment.Remote` 필드, `Save` |
| `internal/task/remote.go` | 원격 상태 → 기존 전이/보고 적용, `MESSAGE` 보고, 폴링 루프 |
| `internal/cli/remotecmd.go` | `agentlayer remote add/list/check/rm` |
| `internal/cli/sendremote.go` | `send`의 원격 분기 |
| `internal/cli/messagecmd.go` | `agentlayer task message` |
| `internal/cli/taskcmd.go` | assign/list/done/watch의 원격 분기 |
| `internal/hookcmd/guard.go` | `PaneFromEnv` 내보내기 |
| `docs/remote-exec-example/hermes-exec.sh` | exec 규격 예시 스크립트 |

---

### Task 1: 원격 직원 등록 파일 (`internal/remote/registry.go`)

**Files:**
- Create: `internal/remote/registry.go`
- Test: `internal/remote/registry_test.go`

**Interfaces:**
- Produces: `type Remote struct{Name, Kind, Poll string; AddedAt time.Time; SSH string; Exec []string; Profile, Board, WorkspaceRoot, Mailbox, MaxRuntime string; Commands map[string][]string}`, `func Dir(stateDir string) string`, `func ValidName(name string) bool`, `func (r Remote) Validate() error`, `func (r Remote) PollInterval() time.Duration`, `func Save(stateDir string, r Remote) error`, `func Load(stateDir, name string) (*Remote, bool, error)`, `func List(stateDir string) ([]Remote, error)`, `func Delete(stateDir, name string) (bool, error)`.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/remote/registry_test.go
package remote

import (
	"testing"
	"time"
)

func hermesRemote() Remote {
	return Remote{Name: "hermes-qa", Kind: "hermes", SSH: "hostinger",
		Exec: []string{"docker", "exec", "-i", "-u", "hermes", "hermes-agent-iqxn-hermes-agent-1"},
		Profile: "tech-qa", WorkspaceRoot: "/opt/data/ai-company/결과물"}
}

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{"hermes-qa": true, "a": true, "with:colon": false, "../x": false, "": false, "a b": false} {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q)=%v want %v", name, got, want)
		}
	}
}

func TestValidateAndDefaults(t *testing.T) {
	r := hermesRemote()
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.PollInterval() != 5*time.Second {
		t.Errorf("기본 poll 5s, got %v", r.PollInterval())
	}
	bad := hermesRemote()
	bad.WorkspaceRoot = ""
	if bad.Validate() == nil {
		t.Error("hermes는 workspace_root 필수")
	}
	ex := Remote{Name: "oc", Kind: "exec", Commands: map[string][]string{"dispatch": {"./d.sh"}, "poll": {"./p.sh"}}}
	if err := ex.Validate(); err != nil {
		t.Errorf("exec는 dispatch·poll만 있으면 유효: %v", err)
	}
	ex.Commands = map[string][]string{"poll": {"./p.sh"}}
	if ex.Validate() == nil {
		t.Error("exec는 dispatch 필수")
	}
}

func TestSaveLoadListDelete(t *testing.T) {
	dir := t.TempDir()
	r := hermesRemote()
	if err := Save(dir, r); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(dir, "hermes-qa")
	if err != nil || !ok || got.Profile != "tech-qa" || got.Mailbox != "imac-manager" {
		t.Fatalf("Load: %+v ok=%v err=%v (Mailbox 기본값 imac-manager)", got, ok, err)
	}
	if _, ok, _ := Load(dir, "none"); ok {
		t.Error("없는 이름은 ok=false")
	}
	if _, _, err := Load(dir, "bad:name"); err == nil {
		t.Error("잘못된 이름은 에러")
	}
	list, _ := List(dir)
	if len(list) != 1 || list[0].Name != "hermes-qa" {
		t.Errorf("List=%+v", list)
	}
	if found, _ := Delete(dir, "hermes-qa"); !found {
		t.Error("Delete found=false")
	}
	if list, _ := List(dir); len(list) != 0 {
		t.Error("삭제 뒤 비어야 함")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/remote/ -run 'TestValidName|TestValidateAndDefaults|TestSaveLoadListDelete' -v`
Expected: FAIL (패키지 없음 / 정의 없음)

- [ ] **Step 3: 구현**

```go
// internal/remote/registry.go
// Package remote는 다른 실행기(호스팅어 Hermes 등)를 AI 회사 직원으로 붙이는 배관이다.
// 등록 파일은 <state>/remotes/<이름>.json — 에이전트 저장소(agents/)와 분리해 scan.Sync의 DEAD 처리를 피한다.
package remote

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

// Remote는 원격 직원 하나의 등록. Kind가 hermes면 SSH·Exec·Profile·WorkspaceRoot·Mailbox·MaxRuntime을,
// exec면 Commands를 쓴다.
type Remote struct {
	Name    string    `json:"name"`
	Kind    string    `json:"kind"` // hermes | exec
	Poll    string    `json:"poll,omitempty"`
	AddedAt time.Time `json:"added_at"`

	SSH           string   `json:"ssh,omitempty"`
	Exec          []string `json:"exec,omitempty"`
	Profile       string   `json:"profile,omitempty"`
	Board         string   `json:"board,omitempty"`
	WorkspaceRoot string   `json:"workspace_root,omitempty"`
	Mailbox       string   `json:"mailbox,omitempty"`
	MaxRuntime    string   `json:"max_runtime,omitempty"`

	Commands map[string][]string `json:"commands,omitempty"` // dispatch·poll·reply·pull·mailbox·finish·check
}

const (
	DefaultMailbox    = "imac-manager"
	DefaultMaxRuntime = "2h"
	DefaultPoll       = 5 * time.Second
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// ValidName — 파일명 한 조각이고 ':'가 없다('<세션>:<창>' 파싱과 충돌 금지).
func ValidName(name string) bool {
	return nameRe.MatchString(name) && name != "." && name != ".."
}

func (r Remote) PollInterval() time.Duration {
	if d, err := time.ParseDuration(r.Poll); err == nil && d >= time.Second {
		return d
	}
	return DefaultPoll
}

func (r Remote) Validate() error {
	if !ValidName(r.Name) {
		return fmt.Errorf("이름 형식 오류: %q (영숫자·점·밑줄·하이픈 1~64자, ':' 불가)", r.Name)
	}
	switch r.Kind {
	case "hermes":
		if r.SSH == "" || r.Profile == "" || r.WorkspaceRoot == "" {
			return errors.New("hermes 원격은 --ssh, --profile, --workspace-root가 필수")
		}
		if !strings.HasPrefix(r.WorkspaceRoot, "/") {
			return errors.New("--workspace-root는 원격 절대경로")
		}
	case "exec":
		if len(r.Commands["dispatch"]) == 0 || len(r.Commands["poll"]) == 0 {
			return errors.New("exec 원격은 commands.dispatch와 commands.poll이 필수")
		}
	default:
		return fmt.Errorf("알 수 없는 kind: %q (hermes | exec)", r.Kind)
	}
	if r.Poll != "" {
		if _, err := time.ParseDuration(r.Poll); err != nil {
			return fmt.Errorf("poll 형식 오류: %w", err)
		}
	}
	return nil
}

func Dir(stateDir string) string { return filepath.Join(stateDir, "remotes") }

func path(stateDir, name string) string { return filepath.Join(Dir(stateDir), name+".json") }

// Save는 기본값(Mailbox·MaxRuntime)을 채우고 원자적으로 쓴다.
func Save(stateDir string, r Remote) error {
	if r.Kind == "hermes" {
		if r.Mailbox == "" {
			r.Mailbox = DefaultMailbox
		}
		if r.MaxRuntime == "" {
			r.MaxRuntime = DefaultMaxRuntime
		}
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if r.AddedAt.IsZero() {
		r.AddedAt = time.Now()
	}
	if err := os.MkdirAll(Dir(stateDir), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	p := path(stateDir, r.Name)
	tmp, err := os.CreateTemp(Dir(stateDir), "."+r.Name+".*.tmp")
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

// Load — 없으면 ok=false, 에러 없음. 이름이 규칙에 어긋나면 에러.
func Load(stateDir, name string) (*Remote, bool, error) {
	if !ValidName(name) {
		return nil, false, fmt.Errorf("원격 이름 형식 오류: %q", name)
	}
	b, err := os.ReadFile(path(stateDir, name))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var r Remote
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, false, fmt.Errorf("등록 파일 손상 %s: %w", path(stateDir, name), err)
	}
	return &r, true, nil
}

func List(stateDir string) ([]Remote, error) {
	entries, err := os.ReadDir(Dir(stateDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Remote
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(Dir(stateDir), e.Name()))
		if err != nil {
			continue
		}
		var r Remote
		if json.Unmarshal(b, &r) == nil {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func Delete(stateDir, name string) (bool, error) {
	if !ValidName(name) {
		return false, fmt.Errorf("원격 이름 형식 오류: %q", name)
	}
	err := os.Remove(path(stateDir, name))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/remote/ -v`
Expected: PASS (3 tests)

- [ ] **Step 5: 커밋**

```bash
git add internal/remote/registry.go internal/remote/registry_test.go
git commit -m "feat(remote): 원격 직원 등록 파일(remotes/<이름>.json) 읽기·쓰기·검증"
```

---

### Task 2: 셸 인용과 ssh 실행기 (`internal/remote/shell.go`)

**Files:**
- Create: `internal/remote/shell.go`
- Test: `internal/remote/shell_test.go`

**Interfaces:**
- Produces: `type Runner interface{ Run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) }`, `func ShellQuote(s string) string`, `func ShellJoin(args []string) string`, `type SSHRunner struct{Host string; Exec []string; ControlDir string}` (Runner 구현), `func (s SSHRunner) Argv(args ...string) []string`, `type FakeRunner struct{Calls [][]string; Stdins []string; Reply func(args []string) ([]byte, error)}` (테스트용, `fake.go`가 아니라 `shell.go`에 두어 다른 패키지 테스트도 쓴다).

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/remote/shell_test.go
package remote

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestShellQuoteRoundTrip(t *testing.T) {
	// 실제 sh로 되돌려 본다 — 이스케이프 규칙의 정본은 셸이다.
	inputs := []string{"plain", "한글 본문", "it's", "a$b `c` \"d\"", "line1\nline2\n", "  spaces  ", ""}
	for _, in := range inputs {
		out, err := exec.Command("sh", "-c", "printf '%s' "+ShellQuote(in)).Output()
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if string(out) != in {
			t.Errorf("ShellQuote(%q) → %q", in, out)
		}
	}
	if got := ShellJoin([]string{"hermes", "kanban", "create", "제목 A"}); got != "hermes kanban create '제목 A'" {
		t.Errorf("ShellJoin=%q", got)
	}
}

func TestSSHRunnerArgv(t *testing.T) {
	s := SSHRunner{Host: "hostinger", Exec: []string{"docker", "exec", "-i", "-u", "hermes", "c1"}, ControlDir: "/tmp/ctl"}
	argv := s.Argv("hermes", "kanban", "show", "t_1", "--json")
	joined := strings.Join(argv, " ")
	for _, want := range []string{"ssh", "-o BatchMode=yes", "-o ControlMaster=auto", "-o ControlPath=/tmp/ctl/ssh-%C", "hostinger"} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv에 %q 없음: %v", want, argv)
		}
	}
	if argv[len(argv)-1] != "docker exec -i -u hermes c1 hermes kanban show t_1 --json" {
		t.Errorf("원격 명령 문자열=%q", argv[len(argv)-1])
	}
}

func TestFakeRunnerRecords(t *testing.T) {
	f := &FakeRunner{Reply: func(args []string) ([]byte, error) { return []byte(`{"ok":true}`), nil }}
	out, err := f.Run(context.Background(), strings.NewReader("in"), "hermes", "--version")
	if err != nil || string(out) != `{"ok":true}` || len(f.Calls) != 1 || f.Calls[0][1] != "--version" || f.Stdins[0] != "in" {
		t.Errorf("FakeRunner: out=%s err=%v calls=%v stdins=%v", out, err, f.Calls, f.Stdins)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/remote/ -run 'TestShellQuote|TestSSHRunner|TestFakeRunner' -v`
Expected: FAIL (정의 없음)

- [ ] **Step 3: 구현**

```go
// internal/remote/shell.go
package remote

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Runner는 원격(또는 로컬)에서 argv 하나를 실행하고 표준출력을 돌려준다. 원격 텍스트는 전부 인자로만 간다.
type Runner interface {
	Run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error)
}

// ShellQuote는 POSIX sh용 단일따옴표 인용. 원격 셸 문자열을 만드는 유일한 함수다.
func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ShellJoin — 안전한 토큰([A-Za-z0-9_./:=@%+-])은 그대로, 나머지는 ShellQuote.
func ShellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if a != "" && strings.Trim(a, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_./:=@%+-") == "" {
			parts[i] = a
		} else {
			parts[i] = ShellQuote(a)
		}
	}
	return strings.Join(parts, " ")
}

// SSHRunner는 `ssh <host> <Exec…> <args…>`. ControlMaster로 연결을 60초 재사용해 폴링을 싸게 한다.
type SSHRunner struct {
	Host       string
	Exec       []string
	ControlDir string
}

func (s SSHRunner) Argv(args ...string) []string {
	argv := []string{"ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ControlMaster=auto",
		"-o", "ControlPath=" + s.ControlDir + "/ssh-%C", "-o", "ControlPersist=60s", s.Host}
	remote := append(append([]string{}, s.Exec...), args...)
	return append(argv, ShellJoin(remote))
}

func (s SSHRunner) Run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	if s.ControlDir != "" {
		_ = os.MkdirAll(s.ControlDir, 0o700)
	}
	argv := s.Argv(args...)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = stdin
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return out.Bytes(), fmt.Errorf("ssh %s: %w: %s", s.Host, err, msg)
	}
	return out.Bytes(), nil
}

// FakeRunner — 테스트용. 호출 argv·stdin을 기록하고 Reply로 응답한다.
type FakeRunner struct {
	Calls  [][]string
	Stdins []string
	Reply  func(args []string) ([]byte, error)
}

func (f *FakeRunner) Run(_ context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	in := ""
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		in = string(b)
	}
	f.Calls = append(f.Calls, append([]string{}, args...))
	f.Stdins = append(f.Stdins, in)
	if f.Reply == nil {
		return nil, nil
	}
	return f.Reply(args)
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/remote/ -v`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/remote/shell.go internal/remote/shell_test.go
git commit -m "feat(remote): ShellQuote·SSHRunner(ControlMaster)·FakeRunner"
```

---

### Task 3: 어댑터 계약 (`internal/remote/adapter.go`)

**Files:**
- Create: `internal/remote/adapter.go`

**Interfaces:**
- Produces:
  ```go
  type Handle = string
  type Status struct{ State state.AgentState; Summary, Ask, Error string; Seen int64 }
  type Letter struct{ ID, From, Text, TaskID string; At time.Time }
  type Info struct{ Version string; ProfileOK bool; RoundTrip time.Duration; Detail string }
  type Adapter interface {
      Dispatch(ctx context.Context, taskID, title, body string, parent Handle) (Handle, error)
      Poll(ctx context.Context, h Handle) (Status, error)
      Reply(ctx context.Context, h Handle, text string) error
      Pull(ctx context.Context, h Handle, destDir string) error
      Mailbox(ctx context.Context) ([]Letter, error)
      Finish(ctx context.Context, h Handle) error
      Check(ctx context.Context) (Info, error)
  }
  func Open(r Remote, stateDir string) (Adapter, error)   // Task 6에서 exec 분기 추가
  const (TimeoutQuery = 30*time.Second; TimeoutDispatch = 60*time.Second; TimeoutPull = 5*time.Minute)
  ```
- 테스트 없음(인터페이스·상수만). Task 4의 테스트가 컴파일로 검증한다.

- [ ] **Step 1: 작성**

```go
// internal/remote/adapter.go
package remote

import (
	"context"
	"fmt"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

// Handle은 실행기 쪽 식별자(Hermes = 카드 ID).
type Handle = string

// Status는 Poll 결과. State는 agentlayer 상태 값 그대로. Seen은 마지막 관측 이벤트 시각(초) — 같은 상태의
// 재관측을 걸러 낸다.
type Status struct {
	State   state.AgentState
	Summary string
	Ask     string
	Error   string
	Seen    int64
}

// Letter는 실행기 쪽에서 총괄 앞으로 온 편지. Mailbox가 돌려준 편지는 수신 확인된 것이다.
type Letter struct {
	ID     string
	From   string
	Text   string
	TaskID string
	At     time.Time
}

type Info struct {
	Version   string
	ProfileOK bool
	RoundTrip time.Duration
	Detail    string
}

// Adapter는 send·task watch·task done이 실행기에 대해 아는 전부다.
type Adapter interface {
	Dispatch(ctx context.Context, taskID, title, body string, parent Handle) (Handle, error)
	Poll(ctx context.Context, h Handle) (Status, error)
	Reply(ctx context.Context, h Handle, text string) error
	Pull(ctx context.Context, h Handle, destDir string) error
	Mailbox(ctx context.Context) ([]Letter, error)
	Finish(ctx context.Context, h Handle) error
	Check(ctx context.Context) (Info, error)
}

const (
	TimeoutQuery    = 30 * time.Second
	TimeoutDispatch = 60 * time.Second
	TimeoutPull     = 5 * time.Minute
	MaxPullBytes    = 50 << 20
)

// Open은 등록에 맞는 어댑터를 만든다.
func Open(r Remote, stateDir string) (Adapter, error) {
	switch r.Kind {
	case "hermes":
		return &Hermes{R: SSHRunner{Host: r.SSH, Exec: r.Exec, ControlDir: Dir(stateDir)}, Profile: r.Profile, Board: r.Board,
			WorkspaceRoot: r.WorkspaceRoot, Mailbox: r.Mailbox, MaxRuntime: r.MaxRuntime, Now: time.Now}, nil
	case "exec":
		return &Exec{Commands: r.Commands}, nil
	}
	return nil, fmt.Errorf("알 수 없는 kind: %q", r.Kind)
}
```

(`Hermes`는 Task 4, `Exec`는 Task 6에서 정의한다 — Task 3 단독으로는 컴파일되지 않으므로 Task 4까지 한 커밋으로 묶어도 된다.)

---

### Task 4: Hermes 어댑터 (`internal/remote/hermes.go`)

**Files:**
- Create: `internal/remote/hermes.go`
- Test: `internal/remote/hermes_test.go`

**Interfaces:**
- Consumes: Task 2 `Runner`·`FakeRunner`, Task 3 `Adapter`·`Status`·`Letter`.
- Produces: `type Hermes struct{ R Runner; Profile, Board, WorkspaceRoot, Mailbox, MaxRuntime string; Now func() time.Time }` (Adapter 구현), `func StatusOfCard(c Card) Status`, `type Card struct{...}` (show --json 파싱).

Hermes CLI 대응(실측 v0.20.0):
- Dispatch: `mkdir -p <ws>` → `hermes kanban create <title> --assignee P --idempotency-key agentlayer:<taskID>[:<parent>] --created-by agentlayer --workspace dir:<ws> --max-runtime <mr> [--parent <h>] --body <body> --json` → `id` → `hermes kanban dispatch --max 3 --json` → `spawned[].task_id`에 있어야 성공.
- Poll: `hermes kanban show <h> --json`.
- Reply: `hermes kanban unblock <h> --reason <text>` → `dispatch --max 3 --json`(spawned에 있어야 성공).
- Pull: show로 `workspace_path` → `tar -C <ws> -cf - .`(stdout) → `Untar`(Task 5) → `RESULT.md`.
- Mailbox: `hermes kanban list --json` → `assignee == Mailbox && status ∈ {todo, ready}` → 각각 `claim <id>` → `complete <id> --result "received by agentlayer <RFC3339>"`.
- Finish: `hermes kanban archive <h>`.
- Check: `hermes --version`(첫 줄) + `hermes kanban assignees`(프로필 줄에 `yes`).
- `--board`가 있으면 `hermes kanban --board <b> …`.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/remote/hermes_test.go
package remote

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

const showDone = `{"task":{"id":"t_40b3eb2f","title":"PING","status":"done","result":"PONG-OK","workspace_path":"/opt/data/kanban/workspaces/t_40b3eb2f","created_by":"agentlayer"},
"latest_summary":"PONG-OK","comments":[],
"events":[{"kind":"created","payload":{},"created_at":1790320928},{"kind":"claimed","payload":{},"created_at":1790320929},{"kind":"completed","payload":{"summary":"PONG-OK"},"created_at":1790320946}],
"runs":[{"id":7,"profile":"tech-qa","status":"done","outcome":"completed","summary":"PONG-OK","error":null}]}`

const showBlocked = `{"task":{"id":"t_1","title":"Q","status":"blocked","result":null,"workspace_path":"/w"},
"latest_summary":null,"comments":[{"author":"tech-qa","body":"BLOCKED: 파일 이름을 알려주세요","created_at":1790320270}],
"events":[{"kind":"created","payload":{},"created_at":1790320100},{"kind":"blocked","payload":{"reason":"파일 이름을 알려주세요","kind":"needs_input"},"created_at":1790320270}],"runs":[]}`

const showCrashed = `{"task":{"id":"t_2","title":"X","status":"crashed","result":null,"workspace_path":"/w"},"latest_summary":null,"comments":[],
"events":[{"kind":"crashed","payload":{},"created_at":1790320400}],"runs":[{"id":1,"outcome":"crashed","error":"worker exited 1"}]}`

func TestStatusOfCard(t *testing.T) {
	cases := []struct {
		raw  string
		want state.AgentState
		ask  string
		sum  string
		seen int64
	}{
		{showDone, state.StateDoneUnread, "", "PONG-OK", 1790320946},
		{showBlocked, state.StateWaiting, "파일 이름을 알려주세요", "", 1790320270},
		{showCrashed, state.StateError, "", "", 1790320400},
		{`{"task":{"status":"ready"},"events":[]}`, state.StateIdle, "", "", 0},
		{`{"task":{"status":"running"},"events":[]}`, state.StateWorking, "", "", 0},
	}
	for _, c := range cases {
		var card Card
		if err := json.Unmarshal([]byte(c.raw), &card); err != nil {
			t.Fatal(err)
		}
		s := StatusOfCard(card)
		if s.State != c.want || s.Ask != c.ask || s.Summary != c.sum || s.Seen != c.seen {
			t.Errorf("%s: got %+v", c.raw[:30], s)
		}
	}
	var crashed Card
	_ = json.Unmarshal([]byte(showCrashed), &crashed)
	if StatusOfCard(crashed).Error != "worker exited 1" {
		t.Error("crashed는 runs[].error를 Error에")
	}
}

// replyTable은 argv 접두어 → 응답. FakeRunner.Reply에서 쓴다.
func replyTable(t *testing.T, table map[string]string) func([]string) ([]byte, error) {
	return func(args []string) ([]byte, error) {
		joined := strings.Join(args, " ")
		for prefix, out := range table {
			if strings.HasPrefix(joined, prefix) {
				return []byte(out), nil
			}
		}
		t.Logf("응답 없는 호출: %s", joined)
		return nil, nil
	}
}

func newHermes(f *FakeRunner) *Hermes {
	return &Hermes{R: f, Profile: "tech-qa", WorkspaceRoot: "/opt/data/ai-company/결과물", Mailbox: "imac-manager", MaxRuntime: "2h",
		Now: func() time.Time { return time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC) }}
}

func TestHermesDispatch(t *testing.T) {
	f := &FakeRunner{}
	f.Reply = replyTable(t, map[string]string{
		"hermes kanban create":   `{"id":"t_new"}`,
		"hermes kanban dispatch": `{"spawned":[{"task_id":"t_new","assignee":"tech-qa"}],"skipped_nonspawnable":[]}`,
	})
	h, err := newHermes(f).Dispatch(context.Background(), "PING-2", "연결 시험", "본문 it's", "")
	if err != nil || h != "t_new" {
		t.Fatalf("Dispatch: %q %v", h, err)
	}
	if f.Calls[0][0] != "mkdir" || f.Calls[0][2] != "/opt/data/ai-company/결과물/PING-2" {
		t.Errorf("첫 호출은 mkdir -p <ws>: %v", f.Calls[0])
	}
	create := strings.Join(f.Calls[1], " ")
	for _, want := range []string{"create PING-2 연결 시험", "--assignee tech-qa", "--idempotency-key agentlayer:PING-2", "--created-by agentlayer",
		"--workspace dir:/opt/data/ai-company/결과물/PING-2", "--max-runtime 2h", "--body 본문 it's", "--json"} {
		if !strings.Contains(create, want) {
			t.Errorf("create에 %q 없음: %s", want, create)
		}
	}
	if strings.Contains(create, "--parent") {
		t.Error("parent 없으면 --parent 없음")
	}
	// 후속 카드: parent와 키 접미
	f.Calls = nil
	if _, err := newHermes(f).Dispatch(context.Background(), "PING-2", "연결 시험", "후속", "t_prev"); err != nil {
		t.Fatal(err)
	}
	create = strings.Join(f.Calls[1], " ")
	if !strings.Contains(create, "--parent t_prev") || !strings.Contains(create, "--idempotency-key agentlayer:PING-2:t_prev") {
		t.Errorf("후속 카드 옵션: %s", create)
	}
}

func TestHermesDispatchNotSpawned(t *testing.T) {
	f := &FakeRunner{Reply: replyTable(t, map[string]string{
		"hermes kanban create":   `{"id":"t_new"}`,
		"hermes kanban dispatch": `{"spawned":[],"skipped_per_profile_capped":["t_new"]}`,
	})}
	h, err := newHermes(f).Dispatch(context.Background(), "PING-2", "", "본문", "")
	if err == nil || !strings.Contains(err.Error(), "skipped_per_profile_capped") {
		t.Fatalf("기동 실패는 사유를 담은 에러: %v", err)
	}
	if h != "t_new" {
		t.Errorf("카드는 남긴다(재시도용 handle 반환): %q", h)
	}
}

func TestHermesPollReplyFinish(t *testing.T) {
	f := &FakeRunner{Reply: replyTable(t, map[string]string{
		"hermes kanban show t_1":     showBlocked,
		"hermes kanban unblock":      "",
		"hermes kanban dispatch":     `{"spawned":[{"task_id":"t_1"}]}`,
		"hermes kanban archive":      "",
	})}
	h := newHermes(f)
	s, err := h.Poll(context.Background(), "t_1")
	if err != nil || s.State != state.StateWaiting {
		t.Fatalf("Poll: %+v %v", s, err)
	}
	if err := h.Reply(context.Background(), "t_1", "이름은 a.txt"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.Calls[1], " "); got != "hermes kanban unblock t_1 --reason 이름은 a.txt" {
		t.Errorf("unblock argv=%q", got)
	}
	if err := h.Finish(context.Background(), "t_1"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.Calls[len(f.Calls)-1], " "); got != "hermes kanban archive t_1" {
		t.Errorf("archive argv=%q", got)
	}
}

func TestHermesBoardFlag(t *testing.T) {
	f := &FakeRunner{Reply: replyTable(t, map[string]string{"hermes kanban --board b1 show": showDone})}
	h := newHermes(f)
	h.Board = "b1"
	if _, err := h.Poll(context.Background(), "t_40b3eb2f"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.Calls[0], " "); !strings.HasPrefix(got, "hermes kanban --board b1 show t_40b3eb2f --json") {
		t.Errorf("argv=%q", got)
	}
}

func TestHermesMailbox(t *testing.T) {
	list := `[{"id":"t_m1","title":"[VIDEO-07] 정리본","body":"정리 내용","assignee":"imac-manager","status":"ready","created_by":"default","created_at":1790320000},
	{"id":"t_m2","title":"다른 직원 카드","body":"x","assignee":"tech-qa","status":"ready","created_by":"agentlayer","created_at":1790320001},
	{"id":"t_m3","title":"이미 받은 편지","body":"y","assignee":"imac-manager","status":"done","created_by":"default","created_at":1790320002}]`
	f := &FakeRunner{Reply: replyTable(t, map[string]string{"hermes kanban list --json": list, "hermes kanban claim": "/w", "hermes kanban complete": ""})}
	letters, err := newHermes(f).Mailbox(context.Background())
	if err != nil || len(letters) != 1 {
		t.Fatalf("letters=%+v err=%v", letters, err)
	}
	l := letters[0]
	if l.ID != "t_m1" || l.From != "default" || l.Text != "정리 내용" || l.TaskID != "VIDEO-07" || l.At.Unix() != 1790320000 {
		t.Errorf("letter=%+v", l)
	}
	if got := strings.Join(f.Calls[1], " "); got != "hermes kanban claim t_m1" {
		t.Errorf("claim argv=%q", got)
	}
	if got := strings.Join(f.Calls[2], " "); got != "hermes kanban complete t_m1 --result received by agentlayer 2026-09-25T10:00:00Z" {
		t.Errorf("complete argv=%q", got)
	}
}

func TestHermesMailboxSkipsOnAckFailure(t *testing.T) {
	list := `[{"id":"t_m1","title":"편지","body":"b","assignee":"imac-manager","status":"ready","created_by":"default","created_at":1}]`
	f := &FakeRunner{Reply: func(args []string) ([]byte, error) {
		if args[2] == "list" {
			return []byte(list), nil
		}
		return nil, os.ErrPermission
	}}
	letters, _ := newHermes(f).Mailbox(context.Background())
	if len(letters) != 0 {
		t.Error("수신 확인 실패한 편지는 돌려주지 않는다(다음 주기 재시도)")
	}
}

func TestHermesPullWritesResult(t *testing.T) {
	tarBytes := tarOf(t, map[string]string{"out.txt": "hello"})
	f := &FakeRunner{Reply: func(args []string) ([]byte, error) {
		if args[0] == "tar" {
			return tarBytes, nil
		}
		return []byte(showDone), nil
	}}
	dest := filepath.Join(t.TempDir(), "remote")
	if err := newHermes(f).Pull(context.Background(), "t_40b3eb2f", dest); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "out.txt")); string(b) != "hello" {
		t.Error("tar 내용이 풀려야 함")
	}
	res, _ := os.ReadFile(filepath.Join(dest, "RESULT.md"))
	if !strings.Contains(string(res), "PONG-OK") || !strings.Contains(string(res), "t_40b3eb2f") {
		t.Errorf("RESULT.md=%s", res)
	}
	if got := strings.Join(f.Calls[1], " "); got != "tar -C /opt/data/kanban/workspaces/t_40b3eb2f -cf - ." {
		t.Errorf("tar argv=%q", got)
	}
}

func TestHermesCheck(t *testing.T) {
	f := &FakeRunner{Reply: replyTable(t, map[string]string{
		"hermes --version":        "Hermes Agent v0.20.0 (2026.8.3)\nInstall directory: /opt/hermes\n",
		"hermes kanban assignees": "NAME      ON DISK   COUNTS\ntech-qa   yes       (idle)\n",
	})}
	info, err := newHermes(f).Check(context.Background())
	if err != nil || info.Version != "Hermes Agent v0.20.0 (2026.8.3)" || !info.ProfileOK {
		t.Errorf("info=%+v err=%v", info, err)
	}
}
```

`tarOf`는 Task 5의 `untar_test.go`에 정의한다(같은 패키지). Task 5를 먼저 하거나, 이 테스트 파일에 임시로 두었다가 옮긴다.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/remote/ -run 'TestStatusOfCard|TestHermes' -v`
Expected: FAIL (정의 없음)

- [ ] **Step 3: 구현**

```go
// internal/remote/hermes.go
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

// Hermes는 `hermes kanban` CLI 위의 어댑터. 카드 하나 = 업무 하나(후속 지시는 --parent로 새 카드).
type Hermes struct {
	R             Runner
	Profile       string
	Board         string
	WorkspaceRoot string
	Mailbox       string
	MaxRuntime    string
	Now           func() time.Time
}

// Card는 `kanban show --json`에서 읽는 부분만.
type Card struct {
	Task struct {
		ID            string  `json:"id"`
		Title         string  `json:"title"`
		Status        string  `json:"status"`
		Result        *string `json:"result"`
		WorkspacePath string  `json:"workspace_path"`
		CreatedBy     string  `json:"created_by"`
	} `json:"task"`
	LatestSummary *string `json:"latest_summary"`
	Comments      []struct {
		Author    string `json:"author"`
		Body      string `json:"body"`
		CreatedAt int64  `json:"created_at"`
	} `json:"comments"`
	Events []struct {
		Kind      string          `json:"kind"`
		Payload   json.RawMessage `json:"payload"`
		CreatedAt int64           `json:"created_at"`
	} `json:"events"`
	Runs []struct {
		Outcome string  `json:"outcome"`
		Error   *string `json:"error"`
		Summary string  `json:"summary"`
	} `json:"runs"`
}

// listItem은 `kanban list --json`의 원소.
type listItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Assignee  string `json:"assignee"`
	Status    string `json:"status"`
	CreatedBy string `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
}

func firstLine120(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return s
}

// StatusOfCard — 스펙 3절의 상태 대응표.
func StatusOfCard(c Card) Status {
	var s Status
	if n := len(c.Events); n > 0 {
		s.Seen = c.Events[n-1].CreatedAt
	}
	switch c.Task.Status {
	case "running", "scheduled":
		s.State = state.StateWorking
	case "blocked":
		s.State = state.StateWaiting
		s.Ask = blockedReason(c)
	case "done":
		s.State = state.StateDoneUnread
		if c.LatestSummary != nil && *c.LatestSummary != "" {
			s.Summary = firstLine120(*c.LatestSummary)
		} else if c.Task.Result != nil {
			s.Summary = firstLine120(*c.Task.Result)
		}
	case "crashed", "timed_out", "gave_up":
		s.State = state.StateError
		s.Error = lastRunError(c)
	default: // todo · ready · triage · archived
		s.State = state.StateIdle
	}
	if s.State != state.StateError {
		if n := len(c.Runs); n > 0 {
			switch c.Runs[n-1].Outcome {
			case "crashed", "timed_out", "gave_up":
				if c.Task.Status != "done" && c.Task.Status != "running" {
					s.State = state.StateError
					s.Error = lastRunError(c)
				}
			}
		}
	}
	return s
}

func blockedReason(c Card) string {
	for i := len(c.Events) - 1; i >= 0; i-- {
		if c.Events[i].Kind != "blocked" {
			continue
		}
		var p struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(c.Events[i].Payload, &p) == nil && p.Reason != "" {
			return p.Reason
		}
		break
	}
	for i := len(c.Comments) - 1; i >= 0; i-- {
		if strings.HasPrefix(c.Comments[i].Body, "BLOCKED:") {
			return strings.TrimSpace(strings.TrimPrefix(c.Comments[i].Body, "BLOCKED:"))
		}
	}
	return "입력 대기"
}

func lastRunError(c Card) string {
	for i := len(c.Runs) - 1; i >= 0; i-- {
		if c.Runs[i].Error != nil && *c.Runs[i].Error != "" {
			return firstLine120(*c.Runs[i].Error)
		}
	}
	return c.Task.Status
}

func (h *Hermes) kanban(args ...string) []string {
	base := []string{"hermes", "kanban"}
	if h.Board != "" {
		base = append(base, "--board", h.Board)
	}
	return append(base, args...)
}

func (h *Hermes) run(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return h.R.Run(ctx, nil, args...)
}

type dispatchResult struct {
	Spawned []struct {
		TaskID string `json:"task_id"`
	} `json:"spawned"`
	SkippedUnassigned      []string `json:"skipped_unassigned"`
	SkippedNonspawnable    []string `json:"skipped_nonspawnable"`
	SkippedPerProfileCapped []string `json:"skipped_per_profile_capped"`
}

// dispatch는 ready 카드를 띄우고 h가 실제로 떴는지 확인한다.
func (h *Hermes) dispatch(ctx context.Context, want Handle) error {
	out, err := h.run(ctx, TimeoutDispatch, h.kanban("dispatch", "--max", "3", "--json")...)
	if err != nil {
		return err
	}
	var d dispatchResult
	if err := json.Unmarshal(bytes.TrimSpace(out), &d); err != nil {
		return fmt.Errorf("dispatch 응답 파싱: %w", err)
	}
	for _, s := range d.Spawned {
		if s.TaskID == want {
			return nil
		}
	}
	reason := "spawned 목록에 없음"
	for name, ids := range map[string][]string{"skipped_unassigned": d.SkippedUnassigned, "skipped_nonspawnable": d.SkippedNonspawnable,
		"skipped_per_profile_capped": d.SkippedPerProfileCapped} {
		for _, id := range ids {
			if id == want {
				reason = name
			}
		}
	}
	return fmt.Errorf("카드 %s 기동 실패: %s (다음 send로 재시도)", want, reason)
}

func (h *Hermes) Dispatch(ctx context.Context, taskID, title, body string, parent Handle) (Handle, error) {
	ws := h.WorkspaceRoot + "/" + taskID
	if _, err := h.run(ctx, TimeoutQuery, "mkdir", "-p", ws); err != nil {
		return "", err
	}
	full := taskID
	if title != "" {
		full = taskID + " " + title
	}
	key := "agentlayer:" + taskID
	if parent != "" {
		key += ":" + parent
	}
	args := []string{"create", full, "--assignee", h.Profile, "--idempotency-key", key, "--created-by", "agentlayer",
		"--workspace", "dir:" + ws, "--max-runtime", h.MaxRuntime}
	if parent != "" {
		args = append(args, "--parent", parent)
	}
	args = append(args, "--body", body, "--json")
	out, err := h.run(ctx, TimeoutDispatch, h.kanban(args...)...)
	if err != nil {
		return "", err
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &created); err != nil || created.ID == "" {
		return "", fmt.Errorf("create 응답에 id 없음: %s", firstLine120(string(out)))
	}
	if err := h.dispatch(ctx, created.ID); err != nil {
		return created.ID, err
	}
	return created.ID, nil
}

func (h *Hermes) show(ctx context.Context, id Handle) (Card, error) {
	out, err := h.run(ctx, TimeoutQuery, h.kanban("show", id, "--json")...)
	if err != nil {
		return Card{}, err
	}
	var c Card
	if err := json.Unmarshal(bytes.TrimSpace(out), &c); err != nil {
		return Card{}, fmt.Errorf("show 응답 파싱: %w", err)
	}
	return c, nil
}

func (h *Hermes) Poll(ctx context.Context, id Handle) (Status, error) {
	c, err := h.show(ctx, id)
	if err != nil {
		return Status{}, err
	}
	return StatusOfCard(c), nil
}

func (h *Hermes) Reply(ctx context.Context, id Handle, text string) error {
	if _, err := h.run(ctx, TimeoutDispatch, h.kanban("unblock", id, "--reason", text)...); err != nil {
		return err
	}
	return h.dispatch(ctx, id)
}

func (h *Hermes) Pull(ctx context.Context, id Handle, destDir string) error {
	c, err := h.show(ctx, id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	if c.Task.WorkspacePath != "" {
		ctx2, cancel := context.WithTimeout(ctx, TimeoutPull)
		defer cancel()
		out, err := h.R.Run(ctx2, nil, "tar", "-C", c.Task.WorkspacePath, "-cf", "-", ".")
		if err != nil {
			return fmt.Errorf("산출물 tar: %w", err)
		}
		if err := Untar(destDir, bytes.NewReader(out), MaxPullBytes); err != nil {
			return err
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n- 카드: %s\n- 상태: %s\n- 작업 폴더(원격): %s\n- 회수 시각: %s\n\n", c.Task.Title, c.Task.ID, c.Task.Status,
		c.Task.WorkspacePath, h.Now().Format(time.RFC3339))
	if c.Task.Result != nil {
		fmt.Fprintf(&b, "## result\n\n%s\n\n", *c.Task.Result)
	}
	if c.LatestSummary != nil {
		fmt.Fprintf(&b, "## summary\n\n%s\n\n", *c.LatestSummary)
	}
	for _, r := range c.Runs {
		e := ""
		if r.Error != nil {
			e = " error=" + *r.Error
		}
		fmt.Fprintf(&b, "- run outcome=%s%s\n", r.Outcome, e)
	}
	return os.WriteFile(filepath.Join(destDir, "RESULT.md"), []byte(b.String()), 0o644)
}

// Mailbox — 예약 담당자 앞으로 온 todo·ready 카드를 편지로 돌려주고 claim→complete로 수신 확인한다.
func (h *Hermes) Mailbox(ctx context.Context) ([]Letter, error) {
	if h.Mailbox == "" {
		return nil, nil
	}
	out, err := h.run(ctx, TimeoutQuery, h.kanban("list", "--json")...)
	if err != nil {
		return nil, err
	}
	var items []listItem
	if err := json.Unmarshal(bytes.TrimSpace(out), &items); err != nil {
		return nil, fmt.Errorf("list 응답 파싱: %w", err)
	}
	var letters []Letter
	for _, it := range items {
		if it.Assignee != h.Mailbox || (it.Status != "todo" && it.Status != "ready") {
			continue
		}
		if _, err := h.run(ctx, TimeoutQuery, h.kanban("claim", it.ID)...); err != nil {
			continue
		}
		ack := "received by agentlayer " + h.Now().UTC().Format(time.RFC3339)
		if _, err := h.run(ctx, TimeoutQuery, h.kanban("complete", it.ID, "--result", ack)...); err != nil {
			continue
		}
		l := Letter{ID: it.ID, From: it.CreatedBy, Text: it.Body, At: time.Unix(it.CreatedAt, 0)}
		if strings.HasPrefix(it.Title, "[") {
			if end := strings.IndexByte(it.Title, ']'); end > 1 {
				l.TaskID = it.Title[1:end]
			}
		}
		if l.Text == "" {
			l.Text = it.Title
		}
		letters = append(letters, l)
	}
	return letters, nil
}

func (h *Hermes) Finish(ctx context.Context, id Handle) error {
	_, err := h.run(ctx, TimeoutQuery, h.kanban("archive", id)...)
	return err
}

func (h *Hermes) Check(ctx context.Context) (Info, error) {
	start := time.Now()
	out, err := h.run(ctx, TimeoutQuery, "hermes", "--version")
	if err != nil {
		return Info{}, err
	}
	info := Info{Version: firstLine120(string(out)), RoundTrip: time.Since(start)}
	out, err = h.run(ctx, TimeoutQuery, h.kanban("assignees")...)
	if err != nil {
		return info, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == h.Profile {
			info.ProfileOK = f[1] == "yes"
			info.Detail = line
		}
	}
	if !info.ProfileOK {
		return info, errors.New("프로필 " + h.Profile + "이 서버에 없습니다(hermes kanban assignees)")
	}
	return info, nil
}
```

- [ ] **Step 4: 통과 확인** (Task 5의 `Untar`·`tarOf`가 필요하다 — Task 5를 먼저 끝낸 뒤 실행)

Run: `go test ./internal/remote/ -v`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/remote/adapter.go internal/remote/hermes.go internal/remote/hermes_test.go
git commit -m "feat(remote): 어댑터 계약 + Hermes 칸반 어댑터(지시·조회·답변·회수·편지함·점검)"
```

---

### Task 5: 안전한 tar 풀기 (`internal/remote/untar.go`)

**Files:**
- Create: `internal/remote/untar.go`
- Test: `internal/remote/untar_test.go`

**Interfaces:**
- Produces: `func Untar(destDir string, r io.Reader, maxBytes int64) error`, 테스트 헬퍼 `func tarOf(t *testing.T, files map[string]string) []byte`.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/remote/untar_test.go
package remote

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tarOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	return buf.Bytes()
}

func TestUntarWritesFilesAndDirs(t *testing.T) {
	dest := t.TempDir()
	if err := Untar(dest, bytes.NewReader(tarOf(t, map[string]string{"./a.txt": "A", "sub/b.md": "B"})), 1<<20); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "a.txt")); string(b) != "A" {
		t.Error("a.txt")
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "sub", "b.md")); string(b) != "B" {
		t.Error("sub/b.md")
	}
}

func TestUntarRejectsEscape(t *testing.T) {
	for _, name := range []string{"../evil", "/abs", "sub/../../x"} {
		dest := t.TempDir()
		err := Untar(dest, bytes.NewReader(tarOf(t, map[string]string{name: "x"})), 1<<20)
		if err == nil || !strings.Contains(err.Error(), "경로") {
			t.Errorf("%q: 탈출 경로는 거부해야 함: %v", name, err)
		}
	}
	// 심볼릭 링크 항목도 거부
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	tw.Close()
	if err := Untar(t.TempDir(), &buf, 1<<20); err == nil {
		t.Error("symlink 거부")
	}
}

func TestUntarSizeCap(t *testing.T) {
	big := strings.Repeat("x", 2048)
	err := Untar(t.TempDir(), bytes.NewReader(tarOf(t, map[string]string{"big": big})), 1024)
	if err == nil || !strings.Contains(err.Error(), "초과") {
		t.Errorf("상한 초과는 에러: %v", err)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/remote/ -run TestUntar -v`
Expected: FAIL (Untar 정의 없음)

- [ ] **Step 3: 구현**

```go
// internal/remote/untar.go
package remote

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Untar는 r의 tar를 destDir 아래에 푼다. 일반 파일·디렉터리만 받고, destDir 밖으로 나가는 경로·심볼릭 링크·
// 상한 초과는 에러. 회수 산출물은 신뢰하지 않는다(원격 에이전트가 만든 것).
func Untar(destDir string, r io.Reader, maxBytes int64) error {
	dest, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	tr := tar.NewReader(r)
	var total int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar 읽기: %w", err)
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		if name == "" || name == "." {
			continue
		}
		if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
			return fmt.Errorf("tar 경로 거부(절대): %q", hdr.Name)
		}
		target := filepath.Join(dest, name)
		if target != dest && !strings.HasPrefix(target, dest+string(os.PathSeparator)) {
			return fmt.Errorf("tar 경로 거부(탈출): %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			total += hdr.Size
			if total > maxBytes {
				return fmt.Errorf("산출물 %dMiB 초과", maxBytes>>20)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.CopyN(f, tr, hdr.Size); err != nil && !errors.Is(err, io.EOF) {
				f.Close()
				return err
			}
			f.Close()
		default:
			return fmt.Errorf("tar 항목 거부(%c): %q", hdr.Typeflag, hdr.Name)
		}
	}
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/remote/ -v`
Expected: PASS (Task 4의 Hermes 테스트 포함)

- [ ] **Step 5: 커밋**

```bash
git add internal/remote/untar.go internal/remote/untar_test.go
git commit -m "feat(remote): 산출물 tar 안전 풀기(경로 탈출·symlink 거부, 50MiB 상한)"
```

---

### Task 6: 범용 exec 어댑터 (`internal/remote/exec.go`)

**Files:**
- Create: `internal/remote/exec.go`
- Test: `internal/remote/exec_test.go`

**Interfaces:**
- Consumes: Task 3 `Adapter`.
- Produces: `type Exec struct{ Commands map[string][]string; Dir string }` (Adapter 구현). 응답 규격: `dispatch` → `{"handle":"…"}`; `poll` → `{"status":"idle|working|waiting|done|error","summary":"","ask":"","error":"","seen":0}`; `mailbox` → `[{"id","from","text","task_id","at"}]`(at = RFC3339); `reply`·`pull`·`finish`·`check`는 종료 코드. 자리표시자 `{task_id}` `{title}` `{body_file}` `{handle}` `{parent}` `{text_file}` `{dest_dir}`. 없는 명령(`reply`·`pull`·`mailbox`·`finish`·`check`)은 no-op(빈 결과).

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/remote/exec_test.go
package remote

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/netwaif/agentlayer/internal/state"
)

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExecAdapterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	dispatch := writeScript(t, dir, "d.sh", `echo "dispatch $1 $2 $(cat "$3") $4" >> `+log+`; echo '{"handle":"h-1"}'`)
	poll := writeScript(t, dir, "p.sh", `echo "poll $1" >> `+log+`; echo '{"status":"waiting","ask":"어느 파일?","seen":42}'`)
	reply := writeScript(t, dir, "r.sh", `echo "reply $1 $(cat "$2")" >> `+log)
	mailbox := writeScript(t, dir, "m.sh", `echo '[{"id":"m1","from":"oc","text":"안녕","task_id":"","at":"2026-09-25T10:00:00Z"}]'`)
	e := &Exec{Commands: map[string][]string{
		"dispatch": {dispatch, "{task_id}", "{title}", "{body_file}", "{parent}"},
		"poll":     {poll, "{handle}"},
		"reply":    {reply, "{handle}", "{text_file}"},
		"mailbox":  {mailbox},
	}}
	ctx := context.Background()
	h, err := e.Dispatch(ctx, "T-1", "제목", "본문 it's", "")
	if err != nil || h != "h-1" {
		t.Fatalf("Dispatch=%q %v", h, err)
	}
	s, err := e.Poll(ctx, "h-1")
	if err != nil || s.State != state.StateWaiting || s.Ask != "어느 파일?" || s.Seen != 42 {
		t.Fatalf("Poll=%+v %v", s, err)
	}
	if err := e.Reply(ctx, "h-1", "a.txt"); err != nil {
		t.Fatal(err)
	}
	letters, err := e.Mailbox(ctx)
	if err != nil || len(letters) != 1 || letters[0].From != "oc" || letters[0].Text != "안녕" || letters[0].At.Year() != 2026 {
		t.Fatalf("Mailbox=%+v %v", letters, err)
	}
	if err := e.Pull(ctx, "h-1", dir); err != nil {
		t.Errorf("pull 미정의는 no-op: %v", err)
	}
	if err := e.Finish(ctx, "h-1"); err != nil {
		t.Errorf("finish 미정의는 no-op: %v", err)
	}
	got, _ := os.ReadFile(log)
	want := "dispatch T-1 제목 본문 it's \npoll h-1\nreply h-1 a.txt\n"
	if string(got) != want {
		t.Errorf("스크립트 호출 기록:\n%s\nwant:\n%s", got, want)
	}
}

func TestExecAdapterBadJSON(t *testing.T) {
	dir := t.TempDir()
	poll := writeScript(t, dir, "p.sh", `echo not-json`)
	e := &Exec{Commands: map[string][]string{"dispatch": {"true"}, "poll": {poll, "{handle}"}}}
	if _, err := e.Poll(context.Background(), "h"); err == nil {
		t.Error("불량 JSON은 에러")
	}
	unknown := writeScript(t, dir, "u.sh", `echo '{"status":"weird"}'`)
	e.Commands["poll"] = []string{unknown}
	if _, err := e.Poll(context.Background(), "h"); err == nil {
		t.Error("알 수 없는 status는 에러")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/remote/ -run TestExecAdapter -v`
Expected: FAIL (Exec 정의 없음)

- [ ] **Step 3: 구현**

```go
// internal/remote/exec.go
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

// Exec는 사용자가 등록 파일에 적은 명령 템플릿으로 아무 실행기나 붙이는 어댑터. 본문·답변은 임시 파일로 넘긴다.
type Exec struct {
	Commands map[string][]string
	Dir      string // 작업 폴더(비면 현재 폴더)
}

func (e *Exec) argv(name string, vars map[string]string) ([]string, bool) {
	tmpl := e.Commands[name]
	if len(tmpl) == 0 {
		return nil, false
	}
	out := make([]string, len(tmpl))
	for i, a := range tmpl {
		for k, v := range vars {
			a = strings.ReplaceAll(a, "{"+k+"}", v)
		}
		out[i] = a
	}
	return out, true
}

func (e *Exec) run(ctx context.Context, timeout time.Duration, name string, vars map[string]string) ([]byte, bool, error) {
	argv, ok := e.argv(name, vars)
	if !ok {
		return nil, false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = e.Dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), true, fmt.Errorf("%s: %w: %s", name, err, firstLine120(errb.String()))
	}
	return out.Bytes(), true, nil
}

func tempText(text string) (string, func(), error) {
	f, err := os.CreateTemp("", "agentlayer-remote-*.txt")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	f.Chmod(0o600)
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

func (e *Exec) Dispatch(ctx context.Context, taskID, title, body string, parent Handle) (Handle, error) {
	p, cleanup, err := tempText(body)
	if err != nil {
		return "", err
	}
	defer cleanup()
	out, ok, err := e.run(ctx, TimeoutDispatch, "dispatch", map[string]string{"task_id": taskID, "title": title, "body_file": p, "parent": parent})
	if !ok {
		return "", errors.New("commands.dispatch가 없습니다")
	}
	if err != nil {
		return "", err
	}
	var r struct {
		Handle string `json:"handle"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &r); err != nil || r.Handle == "" {
		return "", fmt.Errorf("dispatch 응답에 handle 없음: %s", firstLine120(string(out)))
	}
	return r.Handle, nil
}

func (e *Exec) Poll(ctx context.Context, h Handle) (Status, error) {
	out, ok, err := e.run(ctx, TimeoutQuery, "poll", map[string]string{"handle": h})
	if !ok {
		return Status{}, errors.New("commands.poll이 없습니다")
	}
	if err != nil {
		return Status{}, err
	}
	var r struct {
		Status  string `json:"status"`
		Summary string `json:"summary"`
		Ask     string `json:"ask"`
		Error   string `json:"error"`
		Seen    int64  `json:"seen"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &r); err != nil {
		return Status{}, fmt.Errorf("poll 응답 파싱: %w", err)
	}
	s := Status{Summary: firstLine120(r.Summary), Ask: r.Ask, Error: firstLine120(r.Error), Seen: r.Seen}
	switch r.Status {
	case "idle":
		s.State = state.StateIdle
	case "working":
		s.State = state.StateWorking
	case "waiting":
		s.State = state.StateWaiting
	case "done":
		s.State = state.StateDoneUnread
	case "error":
		s.State = state.StateError
	default:
		return Status{}, fmt.Errorf("poll 응답 status 값 불명: %q (idle|working|waiting|done|error)", r.Status)
	}
	return s, nil
}

func (e *Exec) Reply(ctx context.Context, h Handle, text string) error {
	p, cleanup, err := tempText(text)
	if err != nil {
		return err
	}
	defer cleanup()
	_, _, err = e.run(ctx, TimeoutDispatch, "reply", map[string]string{"handle": h, "text_file": p})
	return err
}

func (e *Exec) Pull(ctx context.Context, h Handle, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	_, _, err := e.run(ctx, TimeoutPull, "pull", map[string]string{"handle": h, "dest_dir": destDir})
	return err
}

func (e *Exec) Mailbox(ctx context.Context) ([]Letter, error) {
	out, ok, err := e.run(ctx, TimeoutQuery, "mailbox", nil)
	if !ok || err != nil {
		return nil, err
	}
	var items []struct {
		ID     string `json:"id"`
		From   string `json:"from"`
		Text   string `json:"text"`
		TaskID string `json:"task_id"`
		At     string `json:"at"`
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &items); err != nil {
		return nil, fmt.Errorf("mailbox 응답 파싱: %w", err)
	}
	letters := make([]Letter, 0, len(items))
	for _, it := range items {
		at, _ := time.Parse(time.RFC3339, it.At)
		if at.IsZero() {
			at = time.Now()
		}
		letters = append(letters, Letter{ID: it.ID, From: it.From, Text: it.Text, TaskID: it.TaskID, At: at})
	}
	return letters, nil
}

func (e *Exec) Finish(ctx context.Context, h Handle) error {
	_, _, err := e.run(ctx, TimeoutQuery, "finish", map[string]string{"handle": h})
	return err
}

func (e *Exec) Check(ctx context.Context) (Info, error) {
	start := time.Now()
	out, ok, err := e.run(ctx, TimeoutQuery, "check", nil)
	if !ok {
		return Info{ProfileOK: true, Detail: "check 명령 없음(생략)"}, nil
	}
	if err != nil {
		return Info{}, err
	}
	return Info{ProfileOK: true, RoundTrip: time.Since(start), Detail: firstLine120(string(out))}, nil
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/remote/ -v`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/remote/exec.go internal/remote/exec_test.go
git commit -m "feat(remote): 범용 exec 어댑터(명령 템플릿 + JSON 응답 규격)"
```

---

### Task 7: 원격 상태 → 기존 전이·보고, 편지 보고, 폴링 루프 (`internal/task`)

**Files:**
- Modify: `internal/task/assign.go` (`Assignment`에 `Remote *RemoteRef`, `func (as Assignment) IsRemote() bool`, `func Save(stateDir string, as Assignment) error`)
- Create: `internal/task/remote.go`
- Test: `internal/task/remote_test.go`

**Interfaces:**
- Consumes: Task 3 `remote.Adapter`·`remote.Status`·`remote.Letter`; 기존 `ApplyTransition`·`ReportFor`·`WriteReport`·`Load`·`List`·`board.TaskDir`.
- Produces:
  ```go
  type RemoteRef struct{ Name string `json:"name"`; Handle string `json:"handle,omitempty"`; LastState state.AgentState `json:"last_state,omitempty"`; Seen int64 `json:"seen,omitempty"`; Workspace string `json:"workspace,omitempty"` }
  func RemoteAgentID(name string) string                       // "remote-<name>"
  func RemoteAgent(as *Assignment, s remote.Status, cwd string) *state.Agent
  func ApplyRemoteStatus(stateDir string, as *Assignment, s remote.Status, cwd string, now time.Time) (changed bool, err error)
  func MessageReport(inbox, from, taskID, text string, now time.Time) *Report
  func LetterReport(inbox string, l remote.Letter, now time.Time) *Report
  const MaxMessageRunes = 8192
  type AdapterOpener func(name string) (remote.Adapter, *remote.Remote, error)
  func RunRemotePolling(ctx context.Context, stateDir, inbox string, open AdapterOpener, warn func(string), now func() time.Time) error
  func PollRemotesOnce(ctx context.Context, stateDir, inbox string, open AdapterOpener, warn func(string), now time.Time)   // 루프 한 바퀴(테스트용)
  ```

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/task/remote_test.go
package task

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

// 회사 루트 하나와 업무 하나(tasks/<id>/task.md·log.md)를 만든다.
func remoteFixture(t *testing.T) (stateDir, root, inbox string) {
	t.Helper()
	stateDir, root = t.TempDir(), t.TempDir()
	inbox = filepath.Join(root, "runtime", "inbox")
	dir := board.TaskDir(root, "PING-2")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "task.md"), []byte("# 연결 시험\n\n```yaml\nstatus: in_progress\nparents: []\n```\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "log.md"), []byte(""), 0o644)
	as := Assignment{TaskID: "PING-2", AgentID: RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote", Inbox: inbox,
		TaskDir: dir, AssignedAt: time.Now(), Remote: &RemoteRef{Name: "hermes-qa", Handle: "t_1"}}
	if err := Assign(stateDir, as, false); err != nil {
		t.Fatal(err)
	}
	return
}

func pendingReports(t *testing.T, inbox string) []Report {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(inbox, "pending", "*.json"))
	var out []Report
	for _, f := range files {
		b, _ := os.ReadFile(f)
		var r Report
		if err := json.Unmarshal(b, &r); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func TestApplyRemoteStatusReportsAndUpdatesBoard(t *testing.T) {
	stateDir, root, inbox := remoteFixture(t)
	as, _, _ := Load(stateDir, RemoteAgentID("hermes-qa"))
	now := time.Now()
	changed, err := ApplyRemoteStatus(stateDir, as, remote.Status{State: state.StateWaiting, Ask: "어느 파일?", Seen: 10}, "", now)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	reps := pendingReports(t, inbox)
	if len(reps) != 1 || reps[0].To != "WAITING" || reps[0].Ask != "어느 파일?" || reps[0].Session != "hermes-qa" || reps[0].Kind != "hermes" || reps[0].TaskID != "PING-2" {
		t.Fatalf("reports=%+v", reps)
	}
	tf, _ := board.ReadTaskFile(root, "PING-2")
	if tf.Status != "waiting_hermes-qa" {
		t.Errorf("status=%s", tf.Status)
	}
	if !strings.Contains(board.ReadLastLog(root, "PING-2"), "[ASK]") {
		t.Error("[ASK] 로그")
	}
	as, _, _ = Load(stateDir, RemoteAgentID("hermes-qa"))
	if as.Remote.LastState != state.StateWaiting || as.Remote.Seen != 10 {
		t.Errorf("등록 파일 갱신: %+v", as.Remote)
	}
	// 같은 상태 재관측은 무음
	changed, _ = ApplyRemoteStatus(stateDir, as, remote.Status{State: state.StateWaiting, Ask: "어느 파일?", Seen: 10}, "", now)
	if changed || len(pendingReports(t, inbox)) != 1 {
		t.Error("재관측은 보고하지 않음")
	}
}

func TestApplyRemoteStatusSkipsIntermediate(t *testing.T) {
	// 감시가 꺼져 있던 동안 blocked→done까지 지나감: DONE 한 번만
	stateDir, root, inbox := remoteFixture(t)
	as, _, _ := Load(stateDir, RemoteAgentID("hermes-qa"))
	dest := filepath.Join(root, "결과물", "PING-2", "remote")
	if _, err := ApplyRemoteStatus(stateDir, as, remote.Status{State: state.StateDoneUnread, Summary: "PONG-2", Seen: 99}, dest, time.Now()); err != nil {
		t.Fatal(err)
	}
	reps := pendingReports(t, inbox)
	if len(reps) != 1 || reps[0].To != "DONE_UNREAD" || reps[0].Task != "PONG-2" || reps[0].CWD != dest {
		t.Fatalf("reports=%+v", reps)
	}
	tf, _ := board.ReadTaskFile(root, "PING-2")
	if tf.Status != "reviewing" {
		t.Errorf("status=%s", tf.Status)
	}
}

func TestMessageAndLetterReports(t *testing.T) {
	inbox := filepath.Join(t.TempDir(), "inbox")
	r := MessageReport(inbox, "codex-live", "", strings.Repeat("가", MaxMessageRunes+5), time.Now())
	if r.To != "MESSAGE" || r.TaskID != "-" || r.From != "codex-live" || r.Session != "codex-live" || r.Kind != "message" {
		t.Errorf("%+v", r)
	}
	if n := len([]rune(r.Task)); n != MaxMessageRunes+1 { // 절단 + "…"
		t.Errorf("본문 절단 %d", n)
	}
	if _, err := WriteReport(r); err != nil {
		t.Fatal(err)
	}
	l := LetterReport(inbox, remote.Letter{ID: "t_m1", From: "default", Text: "정리본", TaskID: "VIDEO-07", At: time.Unix(1790320000, 0)}, time.Now())
	if l.TaskID != "VIDEO-07" || l.From != "default" || l.Task != "정리본" || l.To != "MESSAGE" {
		t.Errorf("%+v", l)
	}
	if _, err := WriteReport(l); err != nil {
		t.Fatal(err)
	}
	// 기존 Poll 검증을 통과해야 한다
	got, ok, err := Poll(inbox)
	if err != nil || !ok || got.To != "MESSAGE" {
		t.Fatalf("Poll: %+v ok=%v err=%v", got, ok, err)
	}
}

// fakeAdapter — Poll 결과와 편지를 정해 두고 호출을 기록한다.
type fakeAdapter struct {
	status  remote.Status
	letters []remote.Letter
	pulled  []string
	pollErr error
}

func (f *fakeAdapter) Dispatch(context.Context, string, string, string, remote.Handle) (remote.Handle, error) { return "h", nil }
func (f *fakeAdapter) Poll(context.Context, remote.Handle) (remote.Status, error)      { return f.status, f.pollErr }
func (f *fakeAdapter) Reply(context.Context, remote.Handle, string) error              { return nil }
func (f *fakeAdapter) Pull(_ context.Context, _ remote.Handle, dest string) error {
	f.pulled = append(f.pulled, dest)
	return os.MkdirAll(dest, 0o755)
}
func (f *fakeAdapter) Mailbox(context.Context) ([]remote.Letter, error) {
	l := f.letters
	f.letters = nil
	return l, nil
}
func (f *fakeAdapter) Finish(context.Context, remote.Handle) error { return nil }
func (f *fakeAdapter) Check(context.Context) (remote.Info, error)  { return remote.Info{}, nil }

func TestPollRemotesOncePullsOnDoneAndDeliversLetters(t *testing.T) {
	stateDir, root, inbox := remoteFixture(t)
	if err := remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"}); err != nil {
		t.Fatal(err)
	}
	fa := &fakeAdapter{status: remote.Status{State: state.StateDoneUnread, Summary: "PONG-2", Seen: 5},
		letters: []remote.Letter{{ID: "m1", From: "default", Text: "편지", At: time.Now()}}}
	open := func(name string) (remote.Adapter, *remote.Remote, error) {
		r, _, _ := remote.Load(stateDir, name)
		return fa, r, nil
	}
	var warns []string
	PollRemotesOnce(context.Background(), stateDir, inbox, open, func(s string) { warns = append(warns, s) }, time.Now())
	dest := filepath.Join(root, "결과물", "PING-2", "remote")
	if len(fa.pulled) != 1 || fa.pulled[0] != dest {
		t.Errorf("done이면 결과물/<ID>/remote/로 회수: %v", fa.pulled)
	}
	reps := pendingReports(t, inbox)
	var tos []string
	for _, r := range reps {
		tos = append(tos, r.To)
	}
	if len(reps) != 2 || !strings.Contains(strings.Join(tos, ","), "DONE_UNREAD") || !strings.Contains(strings.Join(tos, ","), "MESSAGE") {
		t.Fatalf("reports=%v warns=%v", tos, warns)
	}
	// 두 번째 바퀴: 변화 없음, 편지 없음 → 보고 추가 없음
	PollRemotesOnce(context.Background(), stateDir, inbox, open, func(s string) { warns = append(warns, s) }, time.Now())
	if len(pendingReports(t, inbox)) != 2 {
		t.Error("변화 없으면 무음")
	}
}

func TestPollRemotesOnceUnreachableIsQuietThenReportsOnce(t *testing.T) {
	stateDir, _, inbox := remoteFixture(t)
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	fa := &fakeAdapter{pollErr: os.ErrDeadlineExceeded}
	open := func(name string) (remote.Adapter, *remote.Remote, error) { r, _, _ := remote.Load(stateDir, name); return fa, r, nil }
	var warns []string
	warn := func(s string) { warns = append(warns, s) }
	t0 := time.Now()
	PollRemotesOnce(context.Background(), stateDir, inbox, open, warn, t0)
	PollRemotesOnce(context.Background(), stateDir, inbox, open, warn, t0.Add(1*time.Minute))
	if len(pendingReports(t, inbox)) != 0 || len(warns) != 2 {
		t.Fatalf("5분 전에는 stderr만: reports=%d warns=%d", len(pendingReports(t, inbox)), len(warns))
	}
	PollRemotesOnce(context.Background(), stateDir, inbox, open, warn, t0.Add(6*time.Minute))
	PollRemotesOnce(context.Background(), stateDir, inbox, open, warn, t0.Add(7*time.Minute))
	reps := pendingReports(t, inbox)
	if len(reps) != 1 || reps[0].To != "ERROR" || !strings.Contains(reps[0].Task, "원격 연결 실패") {
		t.Fatalf("5분 넘으면 ERROR 한 번: %+v", reps)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/task/ -run 'TestApplyRemote|TestMessageAndLetter|TestPollRemotes' -v`
Expected: FAIL (RemoteRef 등 정의 없음)

- [ ] **Step 3: `assign.go` 수정**

`Assignment` 구조체 끝(`AssignedAt` 뒤)에 추가하고, `Save`를 내보낸다:

```go
	// Remote는 원격 직원 등록일 때만. 훅이 아니라 task watch의 폴링이 상태를 채운다.
	Remote *RemoteRef `json:"remote,omitempty"`
}

// RemoteRef는 원격 직원(어댑터) 쪽 실행 식별자와 마지막 관측 상태.
type RemoteRef struct {
	Name      string           `json:"name"`
	Handle    string           `json:"handle,omitempty"`
	LastState state.AgentState `json:"last_state,omitempty"`
	Seen      int64            `json:"seen,omitempty"`
	Workspace string           `json:"workspace,omitempty"`
}

// RemoteAgentID는 원격 직원의 에이전트 ID(agents/에는 저장하지 않는다 — tasks/<id>.json 파일명으로만 쓴다).
func RemoteAgentID(name string) string { return "remote-" + name }

func (as Assignment) IsRemote() bool { return as.Remote != nil }

// Save는 등록 파일을 덮어쓴다(원격 handle·last_state 갱신용).
func Save(stateDir string, as Assignment) error {
	if !validAgentID(as.AgentID) {
		return fmt.Errorf("에이전트 ID 형식 오류: %q", as.AgentID)
	}
	if err := os.MkdirAll(Dir(stateDir), 0o700); err != nil {
		return err
	}
	return writeAtomic(path(stateDir, as.AgentID), as)
}
```

`assign.go`의 import에 `"github.com/netwaif/agentlayer/internal/state"`를 추가한다.

- [ ] **Step 4: `remote.go` 작성**

```go
// internal/task/remote.go
// 원격 직원의 상태는 훅이 아니라 task watch의 폴링이 가져온다. 여기서는 그 결과를 기존 전이·보고 함수에
// "가짜 에이전트"로 태워 inbox 형식·task.md·log.md를 로컬 직원과 똑같이 만든다.
package task

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

// MaxMessageRunes — 편지 본문 상한(넘으면 절단 + …). 보고 파일 16KiB 상한 안에 들게.
const MaxMessageRunes = 8192

func truncRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// RemoteAgent는 ApplyTransition·ReportFor가 요구하는 모양의 에이전트(저장하지 않는다).
func RemoteAgent(as *Assignment, s remote.Status, cwd string) *state.Agent {
	a := &state.Agent{ID: as.AgentID, Kind: "hermes", Task: s.Summary, Ask: s.Ask, CWD: cwd,
		Tmux: state.TmuxRef{Session: as.Session, PaneID: as.Pane}}
	if s.State == state.StateError && s.Error != "" {
		a.Task = s.Error
	}
	if as.Remote != nil && as.Remote.Name != "" {
		a.Kind = remoteKind(as)
	}
	return a
}

// remoteKind — 보고 JSON의 kind. 등록의 kind를 알 수 없으면 "remote".
func remoteKind(as *Assignment) string {
	if as.Remote == nil {
		return "remote"
	}
	return "hermes"
}

// ApplyRemoteStatus는 관측 상태가 마지막 것과 다를 때만 task.md·log.md 갱신과 inbox 보고를 하고 등록 파일을 갱신한다.
func ApplyRemoteStatus(stateDir string, as *Assignment, s remote.Status, cwd string, now time.Time) (bool, error) {
	if as.Remote == nil {
		return false, nil
	}
	prev := as.Remote.LastState
	if prev == "" {
		prev = state.StateIdle
	}
	if prev == s.State && as.Remote.Seen == s.Seen {
		return false, nil
	}
	a := RemoteAgent(as, s, cwd)
	if _, err := ApplyTransition(stateDir, a, prev, s.State, now); err != nil {
		return false, err
	}
	if rep, ok := ReportFor(stateDir, a, prev, s.State, now); ok {
		if _, err := WriteReport(rep); err != nil {
			return false, err
		}
	}
	as.Remote.LastState = s.State
	as.Remote.Seen = s.Seen
	if err := Save(stateDir, *as); err != nil {
		return true, err
	}
	return true, nil
}

// MessageReport — 직원이 총괄에게 먼저 보내는 편지. taskID가 비면 "-"(watch 검증이 빈 값을 거부).
func MessageReport(inbox, from, taskID, text string, now time.Time) *Report {
	if taskID == "" {
		taskID = "-"
	}
	return &Report{Version: 1, ID: NewID(), TaskID: taskID, Session: from, Kind: "message", From: from, To: "MESSAGE",
		Task: truncRunes(text, MaxMessageRunes), At: now, Inbox: inbox}
}

// LetterReport — 원격 편지함에서 온 편지를 MESSAGE 보고로.
func LetterReport(inbox string, l remote.Letter, now time.Time) *Report {
	r := MessageReport(inbox, l.From, l.TaskID, l.Text, now)
	r.Kind = "letter"
	if !l.At.IsZero() {
		r.At = l.At
	}
	return r
}

// AdapterOpener는 이름으로 어댑터를 연다(cli가 remote.Load+remote.Open으로 구현).
type AdapterOpener func(name string) (remote.Adapter, *remote.Remote, error)

// unreachableAfter — 이 시간 넘게 연속 실패하면 ERROR를 한 번 보고한다.
const unreachableAfter = 5 * time.Minute

type outage struct {
	since    time.Time
	reported bool
}

var outages = map[string]*outage{} // 원격 이름 → 연속 실패 기록(프로세스 수명)

// PollRemotesOnce — 등록된 원격 업무마다 Poll → 전이 적용(+done이면 Pull), 등록된 원격마다 Mailbox → MESSAGE.
func PollRemotesOnce(ctx context.Context, stateDir, inbox string, open AdapterOpener, warn func(string), now time.Time) {
	list, err := List(stateDir)
	if err != nil {
		warn("등록 목록: " + err.Error())
		return
	}
	adapters := map[string]remote.Adapter{}
	get := func(name string) remote.Adapter {
		if a, ok := adapters[name]; ok {
			return a
		}
		a, _, err := open(name)
		if err != nil {
			warn(name + ": " + err.Error())
			adapters[name] = nil
			return nil
		}
		adapters[name] = a
		return a
	}
	for i := range list {
		as := &list[i]
		if as.Remote == nil || as.Remote.Handle == "" {
			continue
		}
		ad := get(as.Remote.Name)
		if ad == nil {
			continue
		}
		s, err := ad.Poll(ctx, as.Remote.Handle)
		if err != nil {
			o := outages[as.Remote.Name]
			if o == nil {
				o = &outage{since: now}
				outages[as.Remote.Name] = o
			}
			warn(fmt.Sprintf("%s 조회 실패: %v", as.Remote.Name, err))
			if !o.reported && now.Sub(o.since) >= unreachableAfter {
				o.reported = true
				a := RemoteAgent(as, remote.Status{State: state.StateError}, "")
				a.Task = fmt.Sprintf("원격 연결 실패 %d분 (%s)", int(now.Sub(o.since).Minutes()), as.Remote.Name)
				if rep, ok := ReportFor(stateDir, a, state.StateWorking, state.StateError, now); ok {
					if _, err := WriteReport(rep); err != nil {
						warn("보고 쓰기: " + err.Error())
					}
				}
			}
			continue
		}
		delete(outages, as.Remote.Name)
		cwd := ""
		if s.State == state.StateDoneUnread && as.Remote.LastState != state.StateDoneUnread && as.TaskDir != "" {
			root, id := as.BoardRootID()
			cwd = filepath.Join(root, "결과물", id, "remote")
			if err := ad.Pull(ctx, as.Remote.Handle, cwd); err != nil {
				warn(fmt.Sprintf("%s 산출물 회수 실패: %v", as.TaskID, err))
				cwd = ""
			}
		}
		if _, err := ApplyRemoteStatus(stateDir, as, s, cwd, now); err != nil {
			warn(fmt.Sprintf("%s 전이 적용 실패: %v", as.TaskID, err))
		}
	}
	remotes, err := remote.List(stateDir)
	if err != nil {
		warn("원격 목록: " + err.Error())
		return
	}
	for _, r := range remotes {
		ad := get(r.Name)
		if ad == nil {
			continue
		}
		letters, err := ad.Mailbox(ctx)
		if err != nil {
			warn(r.Name + " 편지함: " + err.Error())
			continue
		}
		for _, l := range letters {
			if l.From == "" {
				l.From = r.Name
			}
			if _, err := WriteReport(LetterReport(inbox, l, now)); err != nil {
				warn("편지 보고 쓰기: " + err.Error())
			}
		}
	}
}

// RunRemotePolling은 ctx가 끝날 때까지 PollRemotesOnce를 돈다. 간격은 등록된 원격의 poll 중 최소(기본 5초).
func RunRemotePolling(ctx context.Context, stateDir, inbox string, open AdapterOpener, warn func(string), now func() time.Time) error {
	for {
		interval := remote.DefaultPoll
		if remotes, _ := remote.List(stateDir); len(remotes) > 0 {
			for _, r := range remotes {
				if d := r.PollInterval(); d < interval {
					interval = d
				}
			}
			PollRemotesOnce(ctx, stateDir, inbox, open, warn, now())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
```

- [ ] **Step 5: 통과 확인**

Run: `go test ./internal/task/ -v`
Expected: PASS (기존 테스트 포함)

- [ ] **Step 6: 커밋**

```bash
git add internal/task/assign.go internal/task/remote.go internal/task/remote_test.go
git commit -m "feat(task): 원격 등록(RemoteRef)·원격 상태→기존 전이/보고·MESSAGE 보고·폴링 루프"
```

---

### Task 8: `agentlayer remote add|list|check|rm` (`internal/cli/remotecmd.go`) + main·help 배선

**Files:**
- Create: `internal/cli/remotecmd.go`
- Test: `internal/cli/remotecmd_test.go`
- Modify: `main.go` (run()의 switch에 `case "remote"`), `internal/cli/helpcmd.go`(명령 줄 추가), `internal/cli/helpcmd_test.go`(목록에 `"remote"`)

**Interfaces:**
- Consumes: Task 1 `remote.Save/Load/List/Delete/ValidName`, Task 3 `remote.Open`, `remote.Adapter.Check`.
- Produces: `func RunRemote(ctx context.Context, w io.Writer, st *state.Store, stateDir string, open func(remote.Remote, string) (remote.Adapter, error), args []string, now time.Time) error`. `open` 인자로 `remote.Open`을 넘긴다(테스트는 페이크).

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/cli/remotecmd_test.go
package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

type checkOnlyAdapter struct {
	remote.Adapter
	err error
}

func (c checkOnlyAdapter) Check(context.Context) (remote.Info, error) {
	return remote.Info{Version: "Hermes Agent v0.20.0", ProfileOK: c.err == nil, RoundTrip: 120 * time.Millisecond}, c.err
}

func newStore(t *testing.T) (*state.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := state.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return st, dir
}

func TestRemoteAddCheckListRm(t *testing.T) {
	st, dir := newStore(t)
	open := func(r remote.Remote, _ string) (remote.Adapter, error) { return checkOnlyAdapter{}, nil }
	var out bytes.Buffer
	args := []string{"add", "hermes-qa", "--kind", "hermes", "--ssh", "hostinger", "--profile", "tech-qa",
		"--exec", "docker exec -i -u hermes c1", "--workspace-root", "/opt/data/ai-company/결과물"}
	if err := RunRemote(context.Background(), &out, st, dir, open, args, time.Now()); err != nil {
		t.Fatal(err)
	}
	r, ok, _ := remote.Load(dir, "hermes-qa")
	if !ok || r.Exec[0] != "docker" || len(r.Exec) != 6 || r.Mailbox != "imac-manager" {
		t.Fatalf("등록: %+v", r)
	}
	if !strings.Contains(out.String(), "v0.20.0") {
		t.Errorf("add는 check 결과를 보여 준다: %s", out.String())
	}
	out.Reset()
	if err := RunRemote(context.Background(), &out, st, dir, open, []string{"list"}, time.Now()); err != nil || !strings.Contains(out.String(), "hermes-qa") {
		t.Errorf("list: %v %s", err, out.String())
	}
	out.Reset()
	if err := RunRemote(context.Background(), &out, st, dir, open, []string{"check", "hermes-qa"}, time.Now()); err != nil || !strings.Contains(out.String(), "tech-qa") {
		t.Errorf("check: %v %s", err, out.String())
	}
	if err := RunRemote(context.Background(), &out, st, dir, open, []string{"rm", "hermes-qa"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := remote.Load(dir, "hermes-qa"); ok {
		t.Error("rm 뒤 없어야 함")
	}
}

func TestRemoteAddRefusesOnCheckFailureUnlessNoCheck(t *testing.T) {
	st, dir := newStore(t)
	open := func(r remote.Remote, _ string) (remote.Adapter, error) { return checkOnlyAdapter{err: errors.New("프로필 없음")}, nil }
	base := []string{"add", "x", "--kind", "hermes", "--ssh", "h", "--profile", "p", "--workspace-root", "/w"}
	if err := RunRemote(context.Background(), &bytes.Buffer{}, st, dir, open, base, time.Now()); err == nil {
		t.Error("check 실패면 저장하지 않고 에러")
	}
	if _, ok, _ := remote.Load(dir, "x"); ok {
		t.Error("저장되면 안 됨")
	}
	if err := RunRemote(context.Background(), &bytes.Buffer{}, st, dir, open, append(base, "--no-check"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := remote.Load(dir, "x"); !ok {
		t.Error("--no-check면 저장")
	}
}

func TestRemoteAddRefusesLiveSessionName(t *testing.T) {
	st, dir := newStore(t)
	st.Save(&state.Agent{ID: "codex-3", Kind: "codex", State: state.StateIdle, Tmux: state.TmuxRef{Session: "codex-live", PaneID: "%3"}})
	open := func(r remote.Remote, _ string) (remote.Adapter, error) { return checkOnlyAdapter{}, nil }
	err := RunRemote(context.Background(), &bytes.Buffer{}, st, dir, open,
		[]string{"add", "codex-live", "--kind", "hermes", "--ssh", "h", "--profile", "p", "--workspace-root", "/w"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "tmux 세션") {
		t.Errorf("산 세션 이름과 같으면 거부: %v", err)
	}
}

func TestRemoteAddExecFromFile(t *testing.T) {
	st, dir := newStore(t)
	f := dir + "/oc.json"
	writeFile(t, f, `{"commands":{"dispatch":["./d.sh","{task_id}","{body_file}"],"poll":["./p.sh","{handle}"]},"poll":"10s"}`)
	open := func(r remote.Remote, _ string) (remote.Adapter, error) { return checkOnlyAdapter{}, nil }
	if err := RunRemote(context.Background(), &bytes.Buffer{}, st, dir, open, []string{"add", "oc", "--kind", "exec", "--file", f}, time.Now()); err != nil {
		t.Fatal(err)
	}
	r, _, _ := remote.Load(dir, "oc")
	if r.Kind != "exec" || len(r.Commands["dispatch"]) != 3 || r.PollInterval() != 10*time.Second {
		t.Errorf("%+v", r)
	}
}
```

`writeFile(t, path, content)`가 `internal/cli` 테스트에 이미 없으면 `remotecmd_test.go` 끝에 추가한다:

```go
func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}
```
(`"os"` import 추가.)

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/cli/ -run TestRemote -v`
Expected: FAIL (RunRemote 정의 없음)

- [ ] **Step 3: 구현**

```go
// internal/cli/remotecmd.go
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

const remoteUsage = `사용법:
  agentlayer remote add <이름> --kind hermes --ssh <호스트> --profile <프로필> --workspace-root <원격절대경로>
                      [--exec "<원격 명령 접두어>"] [--board <보드>] [--mailbox <담당자>] [--max-runtime 2h] [--poll 5s] [--no-check]
  agentlayer remote add <이름> --kind exec --file <어댑터 정의 JSON> [--no-check]
  agentlayer remote list [--json]
  agentlayer remote check <이름>
  agentlayer remote rm <이름>`

// RunRemote — 원격 직원 등록·점검·삭제. open은 remote.Open(테스트는 페이크).
func RunRemote(ctx context.Context, w io.Writer, st *state.Store, stateDir string,
	open func(remote.Remote, string) (remote.Adapter, error), args []string, now time.Time) error {
	if len(args) == 0 {
		return errors.New(remoteUsage)
	}
	switch args[0] {
	case "add":
		return remoteAdd(ctx, w, st, stateDir, open, args[1:], now)
	case "list":
		return remoteList(w, stateDir, args[1:])
	case "check":
		if len(args) != 2 {
			return errors.New(remoteUsage)
		}
		r, ok, err := remote.Load(stateDir, args[1])
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("원격 %q이 등록돼 있지 않습니다", args[1])
		}
		return remoteCheck(ctx, w, *r, stateDir, open)
	case "rm":
		if len(args) != 2 {
			return errors.New(remoteUsage)
		}
		found, err := remote.Delete(stateDir, args[1])
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("원격 %q이 등록돼 있지 않습니다", args[1])
		}
		fmt.Fprintf(w, "원격 %s 삭제\n", args[1])
		return nil
	default:
		return fmt.Errorf("알 수 없는 remote 명령: %s\n%s", args[0], remoteUsage)
	}
}

func remoteAdd(ctx context.Context, w io.Writer, st *state.Store, stateDir string,
	open func(remote.Remote, string) (remote.Adapter, error), args []string, now time.Time) error {
	if len(args) == 0 {
		return errors.New(remoteUsage)
	}
	r := remote.Remote{Name: args[0], AddedAt: now}
	file, noCheck := "", false
	for i := 1; i < len(args); i++ {
		val := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s 뒤에 값이 필요합니다", args[i])
			}
			i++
			return args[i], nil
		}
		var v string
		var err error
		switch args[i] {
		case "--kind":
			v, err = val()
			r.Kind = v
		case "--ssh":
			v, err = val()
			r.SSH = v
		case "--profile":
			v, err = val()
			r.Profile = v
		case "--exec":
			v, err = val()
			r.Exec = strings.Fields(v)
		case "--workspace-root":
			v, err = val()
			r.WorkspaceRoot = v
		case "--board":
			v, err = val()
			r.Board = v
		case "--mailbox":
			v, err = val()
			r.Mailbox = v
		case "--max-runtime":
			v, err = val()
			r.MaxRuntime = v
		case "--poll":
			v, err = val()
			r.Poll = v
		case "--file":
			v, err = val()
			file = v
		case "--no-check":
			noCheck = true
		default:
			return fmt.Errorf("알 수 없는 플래그: %s\n%s", args[i], remoteUsage)
		}
		if err != nil {
			return err
		}
	}
	if r.Kind == "" {
		r.Kind = "hermes"
	}
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		var def remote.Remote
		if err := json.Unmarshal(b, &def); err != nil {
			return fmt.Errorf("어댑터 정의 파일 파싱: %w", err)
		}
		r.Commands = def.Commands
		if r.Poll == "" {
			r.Poll = def.Poll
		}
	}
	if err := r.Validate(); err != nil {
		return err
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	for _, a := range agents {
		if a.Tmux.Session == r.Name && a.State != state.StateDead {
			return fmt.Errorf("이름 %q은 산 tmux 세션과 같습니다 — 다른 이름을 쓰세요(원격이 로컬 세션을 가립니다)", r.Name)
		}
	}
	if !noCheck {
		if err := remoteCheck(ctx, w, r, stateDir, open); err != nil {
			return fmt.Errorf("점검 실패로 저장하지 않음(--no-check로 생략 가능): %w", err)
		}
	}
	if err := remote.Save(stateDir, r); err != nil {
		return err
	}
	fmt.Fprintf(w, "원격 %s 등록 (%s). 배정은 'agentlayer task assign <업무ID> %s …', 지시는 'agentlayer send %s …'\n", r.Name, r.Kind, r.Name, r.Name)
	return nil
}

func remoteCheck(ctx context.Context, w io.Writer, r remote.Remote, stateDir string, open func(remote.Remote, string) (remote.Adapter, error)) error {
	ad, err := open(r, stateDir)
	if err != nil {
		return err
	}
	info, err := ad.Check(ctx)
	if info.Version != "" {
		fmt.Fprintf(w, "  %s · 왕복 %s\n", info.Version, info.RoundTrip.Round(time.Millisecond))
	}
	if info.Detail != "" {
		fmt.Fprintf(w, "  %s\n", info.Detail)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  프로필 %s: 사용 가능\n", r.Profile)
	return nil
}

func remoteList(w io.Writer, stateDir string, args []string) error {
	list, err := remote.List(stateDir)
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "--json" {
		if list == nil {
			list = []remote.Remote{}
		}
		return json.NewEncoder(w).Encode(list)
	}
	if len(list) == 0 {
		fmt.Fprintln(w, "등록된 원격 직원 없음. ('agentlayer remote add …')")
		return nil
	}
	fmt.Fprintln(w, PadRight("이름", 20)+PadRight("종류", 8)+PadRight("대상", 40)+"폴링")
	for _, r := range list {
		target := r.SSH + " " + r.Profile
		if r.Kind == "exec" {
			target = strings.Join(r.Commands["dispatch"], " ")
		}
		fmt.Fprintln(w, PadRight(r.Name, 20)+PadRight(r.Kind, 8)+PadRight(target, 40)+r.PollInterval().String())
	}
	return nil
}
```

- [ ] **Step 4: main.go·help 배선**

`main.go` run()의 switch, `case "task":` 앞에:

```go
	case "remote":
		st, err := state.NewStore(state.DefaultDir())
		if err != nil {
			return err
		}
		if panes, err := (tmuxx.Tmux{}).ListPanes(); err == nil {
			_ = scan.Sync(st, panes, time.Now())
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return cli.RunRemote(ctx, os.Stdout, st, state.DefaultDir(), remote.Open, args[1:], time.Now())
```
(import에 `"github.com/netwaif/agentlayer/internal/remote"` 추가. `storeWithSync()`를 그대로 써도 된다: `st, err := storeWithSync()`.)

`helpcmd.go`의 `task assign|list|done|watch` 줄을 다음 두 줄로 바꾼다:

```
  task assign|list|done|watch|message  업무↔세션 등록·상주 수신·편지 — 등록된 세션의 DONE·WAIT·ERR 전이가 hook에서 <inbox>/pending/ 에 자동 보고됨. message는 직원이 총괄에게 먼저 보내는 편지(MESSAGE)
  remote add|list|check|rm  원격 직원(호스팅어 Hermes 등) 등록 — 등록 뒤에는 task assign·send를 로컬 직원과 똑같이 쓴다
```

`helpcmd_test.go`의 목록에 `"remote"`를 추가한다.

- [ ] **Step 5: 통과 확인**

Run: `go build ./... && go test ./internal/cli/ -run 'TestRemote|TestHelp' -v`
Expected: PASS

- [ ] **Step 6: 커밋**

```bash
git add internal/cli/remotecmd.go internal/cli/remotecmd_test.go internal/cli/helpcmd.go internal/cli/helpcmd_test.go main.go
git commit -m "feat(cli): agentlayer remote add|list|check|rm — 원격 직원 등록·점검"
```

---

### Task 9: `send`의 원격 분기 (`internal/cli/sendremote.go`)

**Files:**
- Create: `internal/cli/sendremote.go`
- Modify: `internal/cli/sendcmd.go:137-160` (`RunSend`에서 대상 해석 전에 원격 조회), 시그니처에 `open`을 넘기지 않기 위해 패키지 변수 `var OpenRemote = remote.Open`을 둔다(테스트가 바꿔 끼움).
- Test: `internal/cli/sendremote_test.go`

**Interfaces:**
- Consumes: Task 7 `task.Assignment.Remote`·`task.Save`·`task.RemoteAgentID`, Task 3 `remote.Adapter`, 기존 `SendGate`·`LogExcerpt`·`board.AppendLog`·`RefreshBoardFile`.
- Produces: `func sendRemote(w io.Writer, st *state.Store, stateDir string, r *remote.Remote, message string, o SendOptions, now time.Time) error`, `func RemoteSendGate(s state.AgentState) (bool, string)`.

원격 게이트(스펙 4절): IDLE·DONE → 지시(Dispatch, DONE이면 parent=직전 handle), WAITING → 답변(Reply), WORKING → 거부(`--force`도 거부: "작업 중 — 끝난 뒤 보내세요"), ERROR → 거부.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/cli/sendremote_test.go
package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

type scriptedAdapter struct {
	remote.Adapter
	status      remote.Status
	dispatched  []string // "taskID|title|body|parent"
	replied     []string
	dispatchErr error
	handle      string
}

func (s *scriptedAdapter) Dispatch(_ context.Context, id, title, body string, parent remote.Handle) (remote.Handle, error) {
	s.dispatched = append(s.dispatched, id+"|"+title+"|"+body+"|"+parent)
	return s.handle, s.dispatchErr
}
func (s *scriptedAdapter) Poll(context.Context, remote.Handle) (remote.Status, error) { return s.status, nil }
func (s *scriptedAdapter) Reply(_ context.Context, _ remote.Handle, text string) error {
	s.replied = append(s.replied, text)
	return nil
}

// 회사 루트 + 원격 등록 + 원격 업무 등록. 어댑터는 ad로 바꿔 끼운다.
func remoteSendFixture(t *testing.T, ad *scriptedAdapter) (st *state.Store, stateDir, root string) {
	t.Helper()
	st, stateDir = newStore(t)
	root = t.TempDir()
	dir := board.TaskDir(root, "PING-2")
	os.MkdirAll(dir, 0o755)
	writeFile(t, filepath.Join(dir, "task.md"), "# 연결 시험\n\n```yaml\nstatus: in_progress\nparents: []\n```\n")
	writeFile(t, filepath.Join(dir, "log.md"), "")
	if err := remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "tech-qa", WorkspaceRoot: "/w"}); err != nil {
		t.Fatal(err)
	}
	if err := task.Assign(stateDir, task.Assignment{TaskID: "PING-2", AgentID: task.RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote",
		Inbox: filepath.Join(root, "runtime", "inbox"), TaskDir: dir, AssignedAt: time.Now(), Remote: &task.RemoteRef{Name: "hermes-qa"}}, false); err != nil {
		t.Fatal(err)
	}
	prev := OpenRemote
	OpenRemote = func(remote.Remote, string) (remote.Adapter, error) { return ad, nil }
	t.Cleanup(func() { OpenRemote = prev })
	return
}

func TestSendRemoteDispatchesFirstTime(t *testing.T) {
	ad := &scriptedAdapter{handle: "t_new", status: remote.Status{State: state.StateIdle}}
	st, stateDir, root := remoteSendFixture(t, ad)
	var out bytes.Buffer
	if err := RunSend(&out, strings.NewReader("본문 첫 줄\n둘째 줄\n"), st, stateDir, nil, []string{"hermes-qa", "-"}); err != nil {
		t.Fatal(err)
	}
	if len(ad.dispatched) != 1 || ad.dispatched[0] != "PING-2|연결 시험|본문 첫 줄\n둘째 줄|" {
		t.Errorf("dispatched=%v", ad.dispatched)
	}
	as, _, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa"))
	if as.Remote.Handle != "t_new" || as.Remote.LastState != state.StateWorking {
		t.Errorf("등록 갱신: %+v", as.Remote)
	}
	if !strings.Contains(board.ReadLastLog(root, "PING-2"), "[SEND]") {
		t.Error("[SEND] 로그")
	}
	if !strings.Contains(out.String(), "hermes-qa") {
		t.Errorf("출력: %s", out.String())
	}
}

func TestSendRemoteRetryAfterDispatchFailure(t *testing.T) {
	ad := &scriptedAdapter{handle: "t_new", dispatchErr: errors.New("카드 t_new 기동 실패: skipped_per_profile_capped"), status: remote.Status{State: state.StateIdle}}
	st, stateDir, _ := remoteSendFixture(t, ad)
	err := RunSend(&bytes.Buffer{}, nil, st, stateDir, nil, []string{"hermes-qa", "지시"})
	if err == nil || !strings.Contains(err.Error(), "skipped_per_profile_capped") {
		t.Fatalf("기동 실패는 에러: %v", err)
	}
	as, _, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa"))
	if as.Remote.Handle != "t_new" {
		t.Errorf("카드는 남긴다(handle 저장): %+v", as.Remote)
	}
	// 두 번째 send: 카드가 ready(IDLE)로 보이면 같은 업무ID로 다시 Dispatch(idempotency는 어댑터 몫)
	ad.dispatchErr = nil
	if err := RunSend(&bytes.Buffer{}, nil, st, stateDir, nil, []string{"hermes-qa", "지시"}); err != nil {
		t.Fatal(err)
	}
	if len(ad.dispatched) != 2 {
		t.Errorf("재시도 Dispatch: %v", ad.dispatched)
	}
}

func TestSendRemoteGate(t *testing.T) {
	cases := []struct {
		state   state.AgentState
		wantErr string
		replies int
		disp    int
		parent  string
	}{
		{state.StateWaiting, "", 1, 0, ""},
		{state.StateWorking, "작업 중", 0, 0, ""},
		{state.StateError, "비정상", 0, 0, ""},
		{state.StateDoneUnread, "", 0, 1, "t_old"},
	}
	for _, c := range cases {
		ad := &scriptedAdapter{handle: "t_next", status: remote.Status{State: c.state}}
		st, stateDir, _ := remoteSendFixture(t, ad)
		as, _, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa"))
		as.Remote.Handle, as.Remote.LastState = "t_old", c.state
		task.Save(stateDir, *as)
		err := RunSend(&bytes.Buffer{}, nil, st, stateDir, nil, []string{"hermes-qa", "답"})
		if c.wantErr == "" && err != nil {
			t.Errorf("%s: %v", c.state, err)
		}
		if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("%s: want %q got %v", c.state, c.wantErr, err)
		}
		if len(ad.replied) != c.replies || len(ad.dispatched) != c.disp {
			t.Errorf("%s: replied=%v dispatched=%v", c.state, ad.replied, ad.dispatched)
		}
		if c.disp == 1 && !strings.HasSuffix(ad.dispatched[0], "|"+c.parent) {
			t.Errorf("%s: DONE 뒤 지시는 parent=%s: %v", c.state, c.parent, ad.dispatched)
		}
	}
}

func TestSendRemoteRequiresAssignment(t *testing.T) {
	st, stateDir := newStore(t)
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	err := RunSend(&bytes.Buffer{}, nil, st, stateDir, nil, []string{"hermes-qa", "지시"})
	if err == nil || !strings.Contains(err.Error(), "task assign") {
		t.Errorf("등록 없으면 task assign 안내: %v", err)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/cli/ -run TestSendRemote -v`
Expected: FAIL (OpenRemote 정의 없음 / 원격 이름을 세션으로 못 찾음)

- [ ] **Step 3: 구현**

`sendcmd.go`의 `RunSend`에서 `agents, err := st.List()` 바로 앞에 삽입:

```go
	if r, ok, err := remote.Load(stateDir, rest[0]); err == nil && ok {
		return sendRemote(w, stateDir, r, message, o, time.Now())
	}
```
(`remote.Load`는 이름 규칙에 어긋나면 에러를 돌려주므로 `<세션>:<창>` 표기는 그대로 기존 경로로 간다. import에 `remote` 추가.)

```go
// internal/cli/sendremote.go
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

// OpenRemote는 어댑터 팩토리 — 테스트가 페이크로 바꿔 끼운다.
var OpenRemote = remote.Open

// RemoteSendGate — 원격은 승인창이 없다. WAIT는 곧 답변 경로라 허용, WORK·ERR은 --force로도 거부.
func RemoteSendGate(s state.AgentState) (bool, string) {
	switch s {
	case state.StateIdle, state.StateDoneUnread, state.StateWaiting:
		return true, ""
	case state.StateWorking:
		return false, "작업 중 — 끝난 뒤 보내세요(원격은 작업 중 지시를 넣을 수 없음)"
	default:
		return false, "비정상 종료 상태 — 'task assign --replace' 뒤 다시 보내세요"
	}
}

func sendRemote(w io.Writer, stateDir string, r *remote.Remote, message string, o SendOptions, now time.Time) error {
	as, ok, err := task.Load(stateDir, task.RemoteAgentID(r.Name))
	if err != nil {
		return err
	}
	if !ok || as.Remote == nil {
		return fmt.Errorf("원격 %s에 등록된 업무가 없습니다 — 먼저 'agentlayer task assign <업무ID> %s --inbox …'", r.Name, r.Name)
	}
	ad, err := OpenRemote(*r, stateDir)
	if err != nil {
		return err
	}
	ctx := context.Background()
	cur := state.StateIdle
	if as.Remote.Handle != "" {
		s, err := ad.Poll(ctx, as.Remote.Handle)
		if err != nil {
			return fmt.Errorf("%s 상태 조회 실패: %w", r.Name, err)
		}
		cur = s.State
	}
	if ok, reason := RemoteSendGate(cur); !ok {
		return fmt.Errorf("%s(%s): %s", r.Name, cur, reason)
	}
	root, id := as.BoardRootID()
	title := ""
	if root != "" {
		if tf, err := board.ReadTaskFile(root, id); err == nil {
			title = tf.Title
		}
	}
	action := ""
	switch cur {
	case state.StateWaiting:
		if err := ad.Reply(ctx, as.Remote.Handle, message); err != nil {
			return fmt.Errorf("%s 답변 전달 실패: %w", r.Name, err)
		}
		action = "답변"
	default: // IDLE(첫 지시 또는 기동 실패 재시도) · DONE(후속 지시 = 새 카드, parent=직전)
		parent := ""
		if cur == state.StateDoneUnread {
			parent = as.Remote.Handle
		}
		h, derr := ad.Dispatch(ctx, as.TaskID, title, message, parent)
		if h != "" {
			as.Remote.Handle = h
			as.Remote.Workspace = r.WorkspaceRoot + "/" + as.TaskID
			if derr != nil {
				as.Remote.LastState = state.StateIdle
				_ = task.Save(stateDir, *as)
				return fmt.Errorf("%s: %w", r.Name, derr)
			}
		} else if derr != nil {
			return fmt.Errorf("%s 지시 실패: %w", r.Name, derr)
		}
		action = "지시"
	}
	as.Remote.LastState = state.StateWorking
	if err := task.Save(stateDir, *as); err != nil {
		return err
	}
	if as.TaskDir != "" {
		warnOut := w
		if o.JSON {
			warnOut = os.Stderr
		}
		if err := board.AppendLog(root, id, "SEND", LogExcerpt(message), now); err != nil {
			fmt.Fprintln(warnOut, "  ⚠ log.md 기록 실패:", err)
		}
		if err := board.SetStatus(root, id, "in_progress", now); err != nil {
			fmt.Fprintln(warnOut, "  ⚠ task.md status 갱신 실패:", err)
		}
	}
	if o.JSON {
		return json.NewEncoder(w).Encode(map[string]any{"session": r.Name, "remote": r.Kind, "handle": as.Remote.Handle,
			"state": cur, "sent": true, "action": action})
	}
	fmt.Fprintf(w, "전송 완료 → %s (%s %s) [%s] %s\n", r.Name, r.Kind, as.Remote.Handle, cur, action)
	return nil
}
```

(보드 파일 갱신은 `refreshBoard`가 `*state.Store`를 요구한다 — `sendRemote`는 store를 받지 않으므로 여기서는 생략하고, Task 10의 watch 폴링이 전이마다 보드를 다시 쓴다.)

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/cli/ -run 'TestSend' -v`
Expected: PASS (기존 send 테스트 포함)

- [ ] **Step 5: 커밋**

```bash
git add internal/cli/sendremote.go internal/cli/sendremote_test.go internal/cli/sendcmd.go
git commit -m "feat(send): 원격 직원 분기 — 첫 지시는 Dispatch, WAIT는 Reply, DONE 뒤는 후속 카드"
```

---

### Task 10: `task assign|list|done|watch`의 원격 분기 (`internal/cli/taskcmd.go`)

**Files:**
- Modify: `internal/cli/taskcmd.go` — `taskAssign`(원격 등록), `taskList`(원격 행), `taskDone`(어댑터 `Finish`), `taskWatch`(폴링 고루틴; 시그니처에 `st`·`stateDir` 추가), `RunTask`의 `case "watch"` 호출부.
- Test: `internal/cli/taskremote_test.go`

**Interfaces:**
- Consumes: Task 7 `task.RemoteRef`·`task.RemoteAgentID`·`task.RunRemotePolling`·`task.AdapterOpener`, Task 9 `OpenRemote`, Task 1 `remote.Load`.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/cli/taskremote_test.go
package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

type finishAdapter struct {
	remote.Adapter
	finished []string
	status   remote.Status
}

func (f *finishAdapter) Finish(_ context.Context, h remote.Handle) error { f.finished = append(f.finished, h); return nil }
func (f *finishAdapter) Poll(context.Context, remote.Handle) (remote.Status, error) { return f.status, nil }
func (f *finishAdapter) Mailbox(context.Context) ([]remote.Letter, error)      { return nil, nil }
func (f *finishAdapter) Pull(_ context.Context, _ remote.Handle, d string) error { return os.MkdirAll(d, 0o755) }

func companyRoot(t *testing.T, id string) string {
	t.Helper()
	root := t.TempDir()
	dir := board.TaskDir(root, id)
	os.MkdirAll(dir, 0o755)
	writeFile(t, filepath.Join(dir, "task.md"), "# 제목\n\n```yaml\nstatus: pending\nparents: []\n```\n")
	writeFile(t, filepath.Join(dir, "log.md"), "")
	return root
}

func TestTaskAssignRemote(t *testing.T) {
	st, stateDir := newStore(t)
	root := companyRoot(t, "PING-2")
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "hostinger", Profile: "tech-qa", WorkspaceRoot: "/w"})
	var out bytes.Buffer
	err := RunTask(context.Background(), &out, st, stateDir, []string{"assign", "PING-2", "hermes-qa", "--inbox", filepath.Join(root, "runtime", "inbox"), "--root", root}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	as, ok, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa"))
	if !ok || as.Remote == nil || as.Remote.Name != "hermes-qa" || as.Session != "hermes-qa" || as.Pane != "remote" || as.TaskDir == "" {
		t.Fatalf("등록: %+v", as)
	}
	tf, _ := board.ReadTaskFile(root, "PING-2")
	if tf.Status != "in_progress" {
		t.Errorf("status=%s", tf.Status)
	}
	if last := board.ReadLastLog(root, "PING-2"); !strings.Contains(last, "[ASSIGN]") || !strings.Contains(last, "hermes:tech-qa@hostinger") {
		t.Errorf("log=%s", last)
	}
	if agents, _ := st.List(); len(agents) != 0 {
		t.Error("원격은 agents/에 저장하지 않는다")
	}
	// 같은 원격에 두 번째 업무는 --replace 필요
	err = RunTask(context.Background(), &out, st, stateDir, []string{"assign", "PING-3", "hermes-qa", "--inbox", filepath.Join(root, "runtime", "inbox")}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "--replace") {
		t.Errorf("중복 등록: %v", err)
	}
}

func TestTaskListShowsRemote(t *testing.T) {
	st, stateDir := newStore(t)
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	task.Assign(stateDir, task.Assignment{TaskID: "PING-2", AgentID: task.RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote", Inbox: "/i",
		AssignedAt: time.Now(), Remote: &task.RemoteRef{Name: "hermes-qa", Handle: "t_1", LastState: state.StateWaiting}}, false)
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"list"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if s := out.String(); !strings.Contains(s, "hermes-qa (hermes)") || !strings.Contains(s, "WAIT") || strings.Contains(s, "gone") {
		t.Errorf("list=%s", s)
	}
	out.Reset()
	RunTask(context.Background(), &out, st, stateDir, []string{"list", "--json"}, time.Now())
	if !strings.Contains(out.String(), `"handle":"t_1"`) || !strings.Contains(out.String(), `"state":"WAITING"`) {
		t.Errorf("json=%s", out.String())
	}
}

func TestTaskDoneFinishesRemote(t *testing.T) {
	st, stateDir := newStore(t)
	root := companyRoot(t, "PING-2")
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"})
	task.Assign(stateDir, task.Assignment{TaskID: "PING-2", AgentID: task.RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote",
		Inbox: filepath.Join(root, "runtime", "inbox"), TaskDir: board.TaskDir(root, "PING-2"), AssignedAt: time.Now(),
		Remote: &task.RemoteRef{Name: "hermes-qa", Handle: "t_1", LastState: state.StateDoneUnread}}, false)
	fa := &finishAdapter{}
	prev := OpenRemote
	OpenRemote = func(remote.Remote, string) (remote.Adapter, error) { return fa, nil }
	t.Cleanup(func() { OpenRemote = prev })
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"done", "PING-2"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(fa.finished) != 1 || fa.finished[0] != "t_1" {
		t.Errorf("Finish 호출: %v", fa.finished)
	}
	if _, ok, _ := task.Load(stateDir, task.RemoteAgentID("hermes-qa")); ok {
		t.Error("등록 해제")
	}
	tf, _ := board.ReadTaskFile(root, "PING-2")
	if tf.Status != "done" {
		t.Errorf("status=%s", tf.Status)
	}
}

func TestTaskWatchPollsRemote(t *testing.T) {
	st, stateDir := newStore(t)
	root := companyRoot(t, "PING-2")
	inbox := filepath.Join(root, "runtime", "inbox")
	remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w", Poll: "1s"})
	task.Assign(stateDir, task.Assignment{TaskID: "PING-2", AgentID: task.RemoteAgentID("hermes-qa"), Session: "hermes-qa", Pane: "remote",
		Inbox: inbox, TaskDir: board.TaskDir(root, "PING-2"), AssignedAt: time.Now(), Remote: &task.RemoteRef{Name: "hermes-qa", Handle: "t_1", LastState: state.StateWorking}}, false)
	fa := &finishAdapter{status: remote.Status{State: state.StateDoneUnread, Summary: "PONG-2", Seen: 3}}
	prev := OpenRemote
	OpenRemote = func(remote.Remote, string) (remote.Adapter, error) { return fa, nil }
	t.Cleanup(func() { OpenRemote = prev })
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var out bytes.Buffer
	_ = RunTask(ctx, &out, st, stateDir, []string{"watch", inbox, "--once"}, time.Now())
	if !strings.Contains(out.String(), `"to":"DONE_UNREAD"`) || !strings.Contains(out.String(), `"session":"hermes-qa"`) {
		t.Errorf("watch가 원격 전이를 흘려야 함: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "결과물", "PING-2", "remote")); err != nil {
		t.Error("done이면 결과물/PING-2/remote/ 회수")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/cli/ -run 'TestTaskAssignRemote|TestTaskListShowsRemote|TestTaskDoneFinishesRemote|TestTaskWatchPollsRemote' -v`
Expected: FAIL

- [ ] **Step 3: `taskAssign` 원격 분기**

`taskAssign`에서 `agents, err := st.List()` 앞에 삽입:

```go
	if r, ok, err := remote.Load(stateDir, pos[1]); err == nil && ok {
		return taskAssignRemote(w, st, stateDir, r, pos[0], abs, root, replace, now)
	}
```

같은 파일에 추가:

```go
// taskAssignRemote — 원격 직원 등록. 카드는 만들지 않는다(첫 send가 만든다).
func taskAssignRemote(w io.Writer, st *state.Store, stateDir string, r *remote.Remote, taskID, inbox, root string, replace bool, now time.Time) error {
	as := task.Assignment{TaskID: taskID, AgentID: task.RemoteAgentID(r.Name), Session: r.Name, Pane: "remote", Inbox: inbox, AssignedAt: now,
		Remote: &task.RemoteRef{Name: r.Name}}
	if root == "" {
		root = board.InferRoot(inbox)
	}
	warn := ""
	if root != "" {
		if _, err := board.ReadTaskFile(root, taskID); err == nil {
			as.TaskDir = board.TaskDir(root, taskID)
		}
	}
	if as.TaskDir == "" {
		warn = "  ⚠ tasks/" + taskID + "/task.md가 없어 보드에 표시되지 않음"
	}
	if err := task.Assign(stateDir, as, replace); err != nil {
		return err
	}
	if as.TaskDir != "" {
		if err := board.RememberRoot(stateDir, root); err != nil {
			fmt.Fprintln(w, "  ⚠ 회사 루트 기억 실패:", err)
		}
		if err := board.SetStatus(root, taskID, "in_progress", now); err != nil {
			fmt.Fprintln(w, "  ⚠ task.md status 갱신 실패:", err)
		}
		label := fmt.Sprintf("%s (%s:%s@%s)", r.Name, r.Kind, r.Profile, r.SSH)
		if r.Kind == "exec" {
			label = fmt.Sprintf("%s (exec)", r.Name)
		}
		if err := board.AppendLog(root, taskID, "ASSIGN", label, now); err != nil {
			fmt.Fprintln(w, "  ⚠ log.md 기록 실패:", err)
		}
	}
	fmt.Fprintf(w, "업무 %s → 원격 %s (%s) 등록. 첫 'agentlayer send %s …'가 실행기에 카드를 만들고, 보고는 %s/pending/ 에 떨어집니다(task watch가 켜져 있을 때).%s\n",
		taskID, r.Name, r.Kind, r.Name, ShortenHome(inbox), warn)
	refreshBoard(w, st, stateDir, now)
	return nil
}
```

- [ ] **Step 4: `taskList` 원격 행**

`taskList`의 `for _, as := range list {` 안, `s := "gone"` 다음에:

```go
		if as.Remote != nil {
			s = string(state.StateIdle)
			if as.Remote.LastState != "" {
				s = string(as.Remote.LastState)
			}
			rows = append(rows, row{as, s})
			continue
		}
```

표 출력의 `label := targetLabel(r.Session, r.Window)` 다음에:

```go
		if r.Remote != nil {
			kind := "remote"
			if rr, ok, _ := remote.Load(stateDir, r.Remote.Name); ok {
				kind = rr.Kind
			}
			label = r.Session + " (" + kind + ")"
		}
```

- [ ] **Step 5: `taskDone` 어댑터 Finish**

`taskDone`의 `for _, as := range list {` 루프 안에서 `as.TaskID == id`일 때 원격이면 기억해 두고, `task.MarkDone` 성공 뒤 `task.Done` 앞에:

```go
	// (루프 안) 
	if as.Remote != nil && as.Remote.Handle != "" {
		remoteRef = as.Remote
	}
	// (MarkDone 뒤)
	if remoteRef != nil {
		if r, ok, _ := remote.Load(stateDir, remoteRef.Name); ok {
			if ad, err := OpenRemote(*r, stateDir); err == nil {
				if err := ad.Finish(context.Background(), remoteRef.Handle); err != nil {
					fmt.Fprintln(w, "  ⚠ 원격 카드 마감(archive) 실패:", err)
				}
			}
		}
	}
```
(`var remoteRef *task.RemoteRef`를 루프 앞에 선언.)

- [ ] **Step 6: `taskWatch` 폴링 고루틴**

시그니처를 `taskWatch(ctx context.Context, w io.Writer, st *state.Store, stateDir string, args []string) error`로 바꾸고 `RunTask`의 호출을 `taskWatch(ctx, w, st, stateDir, args[1:])`로. `task.Watch(...)` 호출 앞에:

```go
	pctx, pcancel := context.WithCancel(ctx)
	defer pcancel()
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
		_ = task.RunRemotePolling(pctx, stateDir, abs, opener, func(s string) { fmt.Fprintln(os.Stderr, "agentlayer remote:", s) }, time.Now)
	}()
```

`task.RunRemotePolling`은 첫 바퀴를 즉시 돈 뒤 간격을 기다린다(Task 7). `--once`는 inbox의 첫 정상 건에서 끝나므로 원격 전이가 파일로 떨어지면 그것을 흘리고 종료한다.

폴링이 전이를 쓸 때 보드 파일도 갱신되게, `RunRemotePolling`의 `warn` 대신 별도 콜백을 두지 않고 `taskWatch`가 emit 콜백 안에서 `refreshBoard(os.Stderr, st, stateDir, time.Now())`를 부른다(원격·로컬 전이 모두 보드가 따라간다):

```go
	err = task.Watch(ctx, abs, interval, once, func(r *task.Report) {
		_ = enc.Encode(r)
		refreshBoard(io.Discard, st, stateDir, time.Now())
	})
```

- [ ] **Step 7: 통과 확인**

Run: `go test ./internal/cli/ -v 2>&1 | tail -30`
Expected: PASS (전체 cli 패키지)

- [ ] **Step 8: 커밋**

```bash
git add internal/cli/taskcmd.go internal/cli/taskremote_test.go
git commit -m "feat(task): assign·list·done·watch 원격 분기 — watch 안에서 어댑터 폴링, done은 archive"
```

---

### Task 11: `agentlayer task message` (`internal/cli/messagecmd.go`) + `hookcmd.PaneFromEnv`

**Files:**
- Modify: `internal/hookcmd/guard.go` (내보내기 래퍼 추가)
- Create: `internal/cli/messagecmd.go`
- Modify: `internal/cli/taskcmd.go` (`RunTask`에 `case "message"`, `taskUsage`에 한 줄)
- Test: `internal/cli/messagecmd_test.go`

**Interfaces:**
- Consumes: Task 7 `task.MessageReport`·`task.WriteReport`, 기존 `board.Root`·`board.RememberedRoot`·`config.Load().CompanyRoot`·`board.AppendLog`.
- Produces: `func PaneFromEnv(env func(string) string) string` (hookcmd), `func taskMessage(w io.Writer, stdin io.Reader, st *state.Store, stateDir, companyRoot string, env func(string) string, args []string, now time.Time) error`. `RunTask`는 `config.Load().CompanyRoot`와 `os.Getenv`를 넘긴다(테스트가 실제 설정에 영향받지 않게).

- [ ] **Step 1: `guard.go`에 추가**

```go
// PaneFromEnv는 hookPane의 공개 이름 — task message 등 "이 pane이 누구인가"를 훅과 같은 규칙으로 판정한다.
func PaneFromEnv(env func(string) string) string { return hookPane(env) }
```

- [ ] **Step 2: 실패하는 테스트 작성**

```go
// internal/cli/messagecmd_test.go
package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

func tmuxEnv(pane string) func(string) string {
	return func(k string) string {
		switch k {
		case "TMUX_PANE":
			return pane
		case "TMUX":
			return "/private/tmp/tmux-501/default,123,0"
		}
		return ""
	}
}

func readPending(t *testing.T, inbox string) []task.Report {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(inbox, "pending", "*.json"))
	var out []task.Report
	for _, f := range files {
		b, _ := os.ReadFile(f)
		var r task.Report
		json.Unmarshal(b, &r)
		out = append(out, r)
	}
	return out
}

func TestTaskMessageOutsideTmux(t *testing.T) {
	st, stateDir := newStore(t)
	err := taskMessage(&bytes.Buffer{}, nil, st, stateDir, "", func(string) string { return "" }, []string{"안녕"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "tmux") {
		t.Errorf("tmux 밖은 거부: %v", err)
	}
}

func TestTaskMessageWithAssignedTaskLogs(t *testing.T) {
	st, stateDir := newStore(t)
	root := companyRoot(t, "VIDEO-07")
	inbox := filepath.Join(root, "runtime", "inbox")
	st.Save(&state.Agent{ID: "codex-3", Kind: "codex", State: state.StateWorking, Tmux: state.TmuxRef{Session: "codex-live", PaneID: "%3"}})
	task.Assign(stateDir, task.Assignment{TaskID: "VIDEO-07", AgentID: "codex-3", Session: "codex-live", Pane: "%3", Inbox: inbox,
		TaskDir: board.TaskDir(root, "VIDEO-07"), AssignedAt: time.Now()}, false)
	var out bytes.Buffer
	if err := taskMessage(&out, strings.NewReader("정리본입니다\n둘째 줄"), st, stateDir, "", tmuxEnv("%3"), []string{"--task", "VIDEO-07", "-"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	reps := readPending(t, inbox)
	if len(reps) != 1 || reps[0].To != "MESSAGE" || reps[0].From != "codex-live" || reps[0].TaskID != "VIDEO-07" || reps[0].Task != "정리본입니다\n둘째 줄" {
		t.Fatalf("%+v", reps)
	}
	if !strings.Contains(board.ReadLastLog(root, "VIDEO-07"), "[MESSAGE]") {
		t.Error("[MESSAGE] 로그")
	}
}

func TestTaskMessageUnassignedTaskNoLog(t *testing.T) {
	st, stateDir := newStore(t)
	root := companyRoot(t, "VIDEO-07")
	inbox := filepath.Join(root, "runtime", "inbox")
	board.RememberRoot(stateDir, root)
	st.Save(&state.Agent{ID: "gemini-5", Kind: "gemini", State: state.StateIdle, Tmux: state.TmuxRef{Session: "community-agy", PaneID: "%5"}})
	if err := taskMessage(&bytes.Buffer{}, nil, st, stateDir, "", tmuxEnv("%5"), []string{"--task", "VIDEO-07", "의견 있음"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	reps := readPending(t, inbox)
	if len(reps) != 1 || reps[0].TaskID != "VIDEO-07" || reps[0].From != "community-agy" {
		t.Fatalf("기억된 루트의 inbox로: %+v", reps)
	}
	if strings.Contains(board.ReadLastLog(root, "VIDEO-07"), "[MESSAGE]") {
		t.Error("이 세션에 등록되지 않은 업무면 로그 없이 이벤트만")
	}
}

func TestTaskMessageNoRootIsError(t *testing.T) {
	st, stateDir := newStore(t)
	st.Save(&state.Agent{ID: "codex-3", Kind: "codex", Tmux: state.TmuxRef{Session: "codex-live", PaneID: "%3"}})
	err := taskMessage(&bytes.Buffer{}, nil, st, stateDir, "", tmuxEnv("%3"), []string{"안녕"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "회사 루트") {
		t.Errorf("루트를 못 찾으면 에러: %v", err)
	}
}
```

- [ ] **Step 3: 실패 확인**

Run: `go test ./internal/cli/ -run TestTaskMessage -v`
Expected: FAIL (taskMessage 정의 없음)

- [ ] **Step 4: 구현**

```go
// internal/cli/messagecmd.go
package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/hookcmd"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

// taskMessage — 직원 pane에서 실행: 총괄 수신함에 MESSAGE 이벤트를 쓴다(배정과 무관하게 먼저 말 걸기).
// 보낸 세션은 훅과 같은 규칙(TMUX_PANE + 기본 tmux 서버)으로 판정한다.
func taskMessage(w io.Writer, stdin io.Reader, st *state.Store, stateDir, companyRoot string, env func(string) string, args []string, now time.Time) error {
	taskID := ""
	var pos []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--task" {
			if i+1 >= len(args) {
				return errors.New("--task 뒤에 업무ID가 필요합니다")
			}
			taskID = args[i+1]
			i++
			continue
		}
		pos = append(pos, args[i])
	}
	if len(pos) == 0 {
		return errors.New("사용법: agentlayer task message [--task <업무ID>] <본문|->  (직원 pane에서 실행)")
	}
	if taskID != "" && !task.ValidID(taskID) {
		return fmt.Errorf("업무ID 형식 오류: %q", taskID)
	}
	text := strings.Join(pos, " ")
	if text == "-" {
		if stdin == nil {
			return errors.New("stdin이 없습니다")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		text = strings.TrimRight(string(b), "\n")
	}
	text = SanitizeMessage(text)
	if strings.TrimSpace(text) == "" {
		return errors.New("본문이 비었습니다")
	}
	pane := hookcmd.PaneFromEnv(env)
	if pane == "" {
		return errors.New("tmux pane 밖에서는 보낼 수 없습니다 (직원 세션 안에서 실행하세요)")
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	var me *state.Agent
	for _, a := range agents {
		if a.Tmux.PaneID == pane {
			me = a
			break
		}
	}
	from := "pane-" + strings.TrimPrefix(pane, "%")
	if me != nil {
		from = me.Tmux.Session
	}
	// inbox: 이 세션에 등록된 그 업무 → 등록의 inbox(+로그). 아니면 회사 루트(설정 → 등록 inbox들 → 기억된 루트).
	inbox, taskDir := "", ""
	list, err := task.List(stateDir)
	if err != nil {
		return err
	}
	var inboxes []string
	for _, as := range list {
		inboxes = append(inboxes, as.Inbox)
		if me != nil && taskID != "" && as.TaskID == taskID && as.AgentID == me.ID && as.Session == me.Tmux.Session && as.Pane == me.Tmux.PaneID {
			inbox, taskDir = as.Inbox, as.TaskDir
		}
	}
	if inbox == "" {
		root := board.Root(companyRoot, inboxes, board.RememberedRoot(stateDir))
		if root == "" {
			return errors.New("회사 루트를 찾지 못했습니다 — 설정 company_root 또는 총괄의 task assign이 먼저 필요합니다")
		}
		inbox = filepath.Join(root, "runtime", "inbox")
	}
	r := task.MessageReport(inbox, from, taskID, text, now)
	p, err := task.WriteReport(r)
	if err != nil {
		return err
	}
	if taskDir != "" {
		root, id := filepath.Dir(filepath.Dir(taskDir)), filepath.Base(taskDir)
		if err := board.AppendLog(root, id, "MESSAGE", LogExcerpt(text), now); err != nil {
			fmt.Fprintln(w, "  ⚠ log.md 기록 실패:", err)
		}
	}
	fmt.Fprintf(w, "편지 전송 → 총괄 수신함 (%s) from %s\n", ShortenHome(p), from)
	return nil
}
```

`RunTask`의 switch에 `case "message": return taskMessage(w, os.Stdin, st, stateDir, config.Load().CompanyRoot, os.Getenv, args[1:], now)`를, `taskUsage`에 `  agentlayer task message [--task <업무ID>] <본문|->   # 직원 pane에서: 총괄에게 편지(MESSAGE)`를 추가한다.

- [ ] **Step 5: 통과 확인**

Run: `go test ./internal/cli/ -run 'TestTaskMessage|TestHelp' -v && go test ./internal/hookcmd/`
Expected: PASS

- [ ] **Step 6: 커밋**

```bash
git add internal/hookcmd/guard.go internal/cli/messagecmd.go internal/cli/messagecmd_test.go internal/cli/taskcmd.go
git commit -m "feat(task): task message — 직원이 총괄에게 먼저 보내는 편지(MESSAGE 이벤트)"
```

---

### Task 12: 문서 + exec 예시 스크립트

**Files:**
- Modify: `README.md` ("세션 지시·업무 보고 (AI 회사 배관)" 절 끝, "업무 보드" 소절 앞)
- Create: `docs/remote-exec-example/hermes-exec.sh`, `docs/remote-exec-example/hermes-exec.json`

- [ ] **Step 1: README 소절 추가** (아래를 "### 업무 보드 (칸반 라이트)" 바로 위에)

```markdown
### 원격 직원 (호스팅어 Hermes 등)

다른 머신의 실행기를 직원으로 붙인다. 등록 뒤에는 총괄 절차가 로컬 직원과 같다 — `task assign` → `send` → Monitor 이벤트 → `task done`.

```bash
agentlayer remote add hermes-qa --kind hermes --ssh hostinger --profile tech-qa \
    --exec "docker exec -i -u hermes hermes-agent-iqxn-hermes-agent-1" --workspace-root /opt/data/ai-company/결과물
agentlayer remote check hermes-qa            # ssh 왕복·버전·프로필 확인
agentlayer task assign PING-2 hermes-qa --inbox ~/ai-folder/company/runtime/inbox
agentlayer send hermes-qa - < 업무요청/PING-2.md   # 첫 send가 칸반 카드를 만들고 dispatch로 띄운다
agentlayer send hermes-qa "답: a.txt로"            # 카드가 blocked(질문)면 unblock --reason = 답변
agentlayer task done PING-2                        # 서버 카드 archive까지
```

- 보고는 훅이 아니라 **`task watch`의 폴링**(기본 5초)이 만든다 — 감시가 켜져 있어야 온다. 카드 상태 대응: running→`WORKING`, blocked→`WAITING`(ask = block 사유), done→`DONE_UNREAD`, crashed/timed_out→`ERROR`.
- 완료 시 원격 작업 폴더를 `결과물/<업무ID>/remote/`로 회수하고 `RESULT.md`(result·summary·카드 ID)를 쓴다(50MiB 상한). 보고 JSON의 `cwd`가 그 폴더다.
- 작업 중(`WORKING`)인 원격에는 `--force`로도 보내지 않는다. 기동 실패(프로필 동시 실행 상한 등)는 `send`가 사유를 그대로 보여 주고, 다음 `send`가 같은 카드로 재시도한다.
- 원격 직원은 `agentlayer status`·대시보드에 나오지 않는다(`task list`·`remote list`로 본다).
- ssh는 아이맥이 연다(원격→로컬 방향 없음). 연결이 5분 넘게 끊기면 `ERROR` 한 번 보고.

**편지함 — 직원이 먼저 말 걸기.** 배정과 무관하게 직원이 총괄에게 자료·의견을 보낸다. 총괄은 `to: "MESSAGE"` 이벤트(`from`·본문·있으면 `task_id`)로 받는다. 편지는 언제 올지 모르므로 총괄 감시는 상시로 둔다.

```bash
agentlayer task message "정리본을 결과물/VIDEO-07/에 두었습니다"      # 로컬 직원 pane에서 (Claude·Codex·Gemini 공통)
agentlayer task message --task VIDEO-07 - < 정리본.md                 # 업무ID를 붙이면 log.md에 [MESSAGE]
hermes kanban create "[VIDEO-07] 정리본" --assignee imac-manager --body "…"   # 원격 Hermes 쪽(예약 담당자 = 편지함)
```

**다른 실행기 붙이기(exec 어댑터).** Hermes가 아니라도 명령 몇 개와 JSON 응답 규격만 맞추면 직원이 된다. `docs/remote-exec-example/`에 Hermes CLI를 이 규격으로 감싼 예시가 있다.

```bash
agentlayer remote add oc --kind exec --file oc-adapter.json
```

`oc-adapter.json`의 `commands`: `dispatch`(→`{"handle":"…"}`), `poll`(→`{"status":"idle|working|waiting|done|error","summary","ask","error","seen"}`), 선택 `reply`·`pull`·`mailbox`(→`[{"id","from","text","task_id","at"}]`)·`finish`·`check`. 자리표시자 `{task_id}` `{title}` `{body_file}` `{parent}` `{handle}` `{text_file}` `{dest_dir}`. 본문·답변은 파일로 넘어온다.
```

- [ ] **Step 2: 예시 스크립트**

```json
// docs/remote-exec-example/hermes-exec.json
{
  "poll": "5s",
  "commands": {
    "dispatch": ["docs/remote-exec-example/hermes-exec.sh", "dispatch", "{task_id}", "{title}", "{body_file}", "{parent}"],
    "poll":     ["docs/remote-exec-example/hermes-exec.sh", "poll", "{handle}"],
    "reply":    ["docs/remote-exec-example/hermes-exec.sh", "reply", "{handle}", "{text_file}"],
    "finish":   ["docs/remote-exec-example/hermes-exec.sh", "finish", "{handle}"]
  }
}
```

```sh
#!/bin/sh
# docs/remote-exec-example/hermes-exec.sh — exec 어댑터 규격으로 Hermes 칸반 CLI를 감싼 예시.
# 환경변수: HERMES_SSH(기본 hostinger), HERMES_EXEC(기본 "docker exec -i -u hermes hermes-agent-iqxn-hermes-agent-1"),
#           HERMES_PROFILE(기본 tech-qa), HERMES_WS(기본 /opt/data/ai-company/결과물)
set -eu
SSH_HOST=${HERMES_SSH:-hostinger}
EXEC=${HERMES_EXEC:-"docker exec -i -u hermes hermes-agent-iqxn-hermes-agent-1"}
PROFILE=${HERMES_PROFILE:-tech-qa}
WS=${HERMES_WS:-/opt/data/ai-company/결과물}
q() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"; }
remote() { ssh -o BatchMode=yes "$SSH_HOST" "$EXEC $*"; }

case "$1" in
  dispatch)
    task=$2; title=$3; body=$(cat "$4"); parent=$5
    remote mkdir -p "$(q "$WS/$task")" >/dev/null
    extra=""; [ -n "$parent" ] && extra="--parent $(q "$parent")"
    id=$(remote hermes kanban create "$(q "$task $title")" --assignee "$(q "$PROFILE")" --idempotency-key "$(q "agentlayer:$task:$parent")" \
         --created-by agentlayer --workspace "$(q "dir:$WS/$task")" --max-runtime 2h $extra --body "$(q "$body")" --json | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
    remote hermes kanban dispatch --max 3 --json | python3 -c "import json,sys; d=json.load(sys.stdin); sys.exit(0 if any(s['task_id']=='$id' for s in d['spawned']) else 1)"
    printf '{"handle":"%s"}\n' "$id" ;;
  poll)
    remote hermes kanban show "$(q "$2")" --json | python3 -c '
import json,sys
c=json.load(sys.stdin); t=c["task"]; st=t["status"]
m={"running":"working","scheduled":"working","blocked":"waiting","done":"done","crashed":"error","timed_out":"error","gave_up":"error"}
out={"status":m.get(st,"idle"),"summary":(c.get("latest_summary") or t.get("result") or "").split("\n")[0][:120],"ask":"","error":"","seen":(c["events"][-1]["created_at"] if c["events"] else 0)}
if st=="blocked":
    ev=[e for e in c["events"] if e["kind"]=="blocked"]
    out["ask"]=(ev[-1]["payload"] or {}).get("reason","") if ev else "입력 대기"
print(json.dumps(out, ensure_ascii=False))' ;;
  reply)
    remote hermes kanban unblock "$(q "$2")" --reason "$(q "$(cat "$3")")" >/dev/null
    remote hermes kanban dispatch --max 3 --json >/dev/null ;;
  finish)
    remote hermes kanban archive "$(q "$2")" >/dev/null ;;
  *) echo "usage: $0 dispatch|poll|reply|finish …" >&2; exit 2 ;;
esac
```

`chmod +x docs/remote-exec-example/hermes-exec.sh`.

- [ ] **Step 3: 확인 후 커밋**

Run: `sh -n docs/remote-exec-example/hermes-exec.sh && go test ./internal/cli/ -run TestHelp`
Expected: 문법 OK, PASS

```bash
git add README.md docs/remote-exec-example/
git commit -m "docs: 원격 직원·편지함·exec 어댑터 규격 + Hermes exec 예시"
```

---

### Task 13: 호스팅어 실측 (세 바퀴)

빌드한 로컬 바이너리로 실제 서버와 한 바퀴씩 돈다. 회사 루트는 `~/ai-folder/company`, 총괄 세션이 아니라 이 터미널에서 직접 명령을 친다(총괄 절차와 같은 명령). 실측 결과는 `docs/superpowers/plans/2026-09-25-remote-employee-hermes.md` 끝 "실측 기록" 절에 그대로 적는다.

- [ ] **Step 1: 빌드·등록**

```bash
go build -o /tmp/agentlayer . && /tmp/agentlayer remote add hermes-qa --kind hermes --ssh hostinger --profile tech-qa \
  --exec "docker exec -i -u hermes hermes-agent-iqxn-hermes-agent-1" --workspace-root /opt/data/ai-company/결과물
```
Expected: `Hermes Agent v0.20.0 … 프로필 tech-qa: 사용 가능`, `원격 hermes-qa 등록 (hermes)`.

- [ ] **Step 2: 1바퀴 — 지시→완료→회수→마감**

```bash
R=~/ai-folder/company
mkdir -p $R/tasks/PING-2 && cp $R/_templates/task.md $R/tasks/PING-2/task.md && sed -i '' 's/^# .*/# 원격 연결 시험 2/' $R/tasks/PING-2/task.md && : > $R/tasks/PING-2/log.md
printf '아무 파일도 만들지 말고 result에 정확히 PONG-2 라고 적어 complete 하세요.\n' > $R/업무요청/PING-2.md
/tmp/agentlayer task assign PING-2 hermes-qa --inbox $R/runtime/inbox --root $R
/tmp/agentlayer send hermes-qa - < $R/업무요청/PING-2.md
/tmp/agentlayer task watch $R/runtime/inbox --once      # 30초 안에 {"to":"DONE_UNREAD","session":"hermes-qa",…}
cat $R/결과물/PING-2/remote/RESULT.md                   # PONG-2
/tmp/agentlayer task done PING-2                        # 서버 카드 archived
ssh hostinger 'docker exec -u hermes hermes-agent-iqxn-hermes-agent-1 hermes kanban list' | grep PING-2   # 없어야 함(archived)
```

- [ ] **Step 3: 2바퀴 — 질문(blocked)→답→완료**

업무요청 본문: `먼저 이 카드를 hermes kanban block --kind needs_input "어떤 단어로 답할까요?" 로 막고 기다리세요. unblock 사유로 단어가 오면 그 단어를 result에 적어 complete 하세요. 파일은 만들지 마세요.`

```bash
/tmp/agentlayer task assign PING-3 hermes-qa --inbox $R/runtime/inbox --root $R --replace
/tmp/agentlayer send hermes-qa - < $R/업무요청/PING-3.md
/tmp/agentlayer task watch $R/runtime/inbox --once      # {"to":"WAITING","ask":"어떤 단어로 답할까요?"}
/tmp/agentlayer send hermes-qa "PONG-3"                 # unblock --reason + dispatch
/tmp/agentlayer task watch $R/runtime/inbox --once      # {"to":"DONE_UNREAD"} , RESULT.md에 PONG-3
/tmp/agentlayer task done PING-3
```
Hermes 작업자가 `block`을 안 쓰고 바로 완료하면 이 경로는 "작업자 지침" 문제다 — `docs/KANBAN_PROTOCOL.md`(서버 회사 루트)에 needs_input 규칙이 있는지 확인하고 실측 기록에 적는다.

- [ ] **Step 4: 3바퀴 — 편지함 양쪽**

```bash
ssh hostinger 'docker exec -u hermes hermes-agent-iqxn-hermes-agent-1 hermes kanban create "[PING-3] 편지 시험" --assignee imac-manager --body "서버에서 보내는 편지" --created-by default'
/tmp/agentlayer task watch $R/runtime/inbox --once      # {"to":"MESSAGE","from":"default","task_id":"PING-3","task":"서버에서 보내는 편지"}
# 로컬: 아무 직원 pane(예: codex-live)에서
agentlayer task message "로컬 직원 편지 시험"
/tmp/agentlayer task watch $R/runtime/inbox --once      # {"to":"MESSAGE","from":"codex-live",…}
```

- [ ] **Step 5: 정리·기록**

시험 카드 archive: `ssh hostinger 'docker exec -u hermes hermes-agent-iqxn-hermes-agent-1 hermes kanban archive t_40b3eb2f'`. `tasks/PING-*`·`업무요청/PING-*`·`결과물/PING-*`는 회사 루트에 남겨도 되지만 보드에 시험 카드가 보이므로 `task done` 뒤 폴더째 삭제한다. 실측 기록 절을 계획서 끝에 추가하고 커밋:

```bash
git add docs/superpowers/plans/2026-09-25-remote-employee-hermes.md
git commit -m "docs(plan): 호스팅어 실측 기록 — 지시·질문·편지함 세 바퀴"
```

---

### Task 14: 릴리즈 v1.9.0

- [ ] **Step 1: 회귀** — `go vet ./... && go test ./internal/remote/ ./internal/task/ ./internal/cli/ ./internal/hookcmd/ ./internal/board/` (패키지별, 전체 `./...` 금지). 모두 PASS.
- [ ] **Step 2: 릴리즈 노트** `/tmp/notes-1.9.0.md`:

```markdown
## v1.9.0 — 원격 직원(호스팅어 Hermes) + 편지함

- `agentlayer remote add|list|check|rm`: 다른 머신의 실행기를 AI 회사 직원으로. 첫 구현은 Hermes 칸반(전담 프로필, dispatch 자동 기동). 등록 뒤 총괄 절차는 로컬 직원과 동일(`task assign`·`send`·Monitor·`task done`).
- 보고는 `task watch`의 폴링이 카드 상태를 기존 inbox 형식(WAITING/DONE_UNREAD/ERROR)으로 번역. 완료 시 산출물을 `결과물/<업무ID>/remote/`로 회수(RESULT.md).
- `agentlayer task message`: 직원(Claude·Codex·Gemini)이 총괄에게 먼저 보내는 편지 → `to: MESSAGE` 이벤트. 원격 Hermes는 예약 담당자(`imac-manager`) 카드가 편지함.
- `exec` 어댑터: 명령 템플릿 + JSON 응답 규격으로 OpenClaw 등 다른 실행기를 코드 없이 연결. 예시 `docs/remote-exec-example/`.
- 총괄 지침 변경(ai-company 플러그인 후속): 감시 상시, `MESSAGE` 처리.
```

- [ ] **Step 3: 태그·푸시·goreleaser** (자동 모드 분류기가 막으면 `/permissions`로 승인 뒤 따로 실행)

```bash
git tag -a v1.9.0 -m "v1.9.0 원격 직원(Hermes)·편지함·exec 어댑터" && git push && git push origin v1.9.0
GITHUB_TOKEN=$(gh auth token) goreleaser release --clean --release-notes /tmp/notes-1.9.0.md
```
Expected: GitHub 릴리즈 v1.9.0 자산 2종 + checksums, tap `Casks/agentlayer.rb` 1.9.0.

- [ ] **Step 4: 설치 확인** — `brew update && brew upgrade netwaif/tap/agentlayer && agentlayer version && agentlayer init` (업그레이드 뒤 init 필수). `agentlayer remote list`에 `hermes-qa`가 남아 있는지(state dir은 그대로).

---

### Task 15: ai-company 플러그인 후속 (별도 레포 `~/VSCodeWorkspace/ai-company`)

worktree 세션이 아닌 이 세션에서 직접 한다(메모리: EnterWorktree는 타 레포 git을 막는다).

**Files:**
- Modify: `plugins/ai-company/skills/configure-company/generator/companyctl.py` (`cmd_employee`: `--remote <이름>` 모드)
- Modify: `plugins/ai-company/skills/configure-company/assets/company-block.md` (배정 항목·수신 항목·감시 규칙)
- Modify: `plugins/ai-company/skills/configure-company/assets/employee-block.md` (편지 한 줄)
- Modify: `plugins/ai-company/skills/configure-company/SKILL.md` (원격 직원 등록 절차), 플러그인 버전 0.2.5 → 0.2.6

- [ ] **Step 1: `companyctl.py` 등록 모드**

인자 파서에 `--remote` (원격 이름), `--engine`은 원격이면 kind로 쓴다(기본 `hermes`). `cmd_employee`의 배타 검사와 분기:

```python
    if sum(map(bool, [a.bot, a.folder, a.on_demand, getattr(a, "remote", None)])) != 1:
        die("--bot <이름> | --folder <폴더> | --on-demand | --remote <원격이름> 중 하나만 지정")
    ...
    elif getattr(a, "remote", None):
        # agentlayer remote add로 등록된 원격 직원. 총괄은 스레드 없이 task assign·send를 그 이름으로 보낸다.
        out = subprocess.run(["agentlayer", "remote", "list", "--json"], capture_output=True, text=True)
        names = [r["name"] for r in json.loads(out.stdout or "[]")] if out.returncode == 0 else []
        if a.remote not in names:
            die(f"agentlayer에 원격 {a.remote}이 없습니다 — 먼저 `agentlayer remote add {a.remote} …`")
        kind = next((r["kind"] for r in json.loads(out.stdout) if r["name"] == a.remote), "hermes")
        e.update(tool=kind, mode="remote", session=a.remote, folder="", bot="", channel_id="")
```

`doctor`(`companyctl.py:546-612`)에 원격 직원마다 `agentlayer remote check <session>` 실행 결과(OK/FAIL) 한 줄.

- [ ] **Step 2: 지침 문안**

`company-block.md` 3(배정)에 항목 추가:
```
   - 원격 직원(`mode: remote`, Hermes 등): 스레드 없이 `agentlayer task assign <업무ID> <session> --inbox … --root …` → `agentlayer send <session> - < 업무요청/<업무ID>.md`. 첫 send가 실행기에 카드를 만든다. 산출물은 완료 보고 뒤 `결과물/<업무ID>/remote/`(RESULT.md 포함)에 회수돼 있다. 작업 중에는 `send`가 거부된다 — 기다린다.
```
3의 **감시** 항목을 교체:
```
   - **감시(상시)**: 재정박 때 Monitor 도구로 `agentlayer task watch <root>/runtime/inbox`(`timeout_ms` 최대 30분)를 켜고, 만료 알림마다 다시 켠다. 편지(`MESSAGE`)와 원격 직원 보고는 감시가 켜져 있을 때만 온다.
```
4(수신)에 추가:
```
   `to`가 `MESSAGE`면 직원이 먼저 보낸 편지다 — `from`(보낸 세션·원격 이름)과 본문(`task`)을 읽고 업무 등록·대표 전달·무시를 판단한다. `task_id`가 있으면 그 업무의 맥락이다.
```
5(마감)의 "활성 업무가 0건이면 감시를 끈다(TaskStop)"를 삭제한다.

`employee-block.md`에 한 줄:
```
- 배정과 무관하게 총괄에게 전할 자료·의견이 있으면 `agentlayer task message [--task <업무ID>] "<본문>"`(여러 줄은 `-`로 stdin). 총괄이 편지로 받는다. 파일은 `결과물/`에 두고 편지에는 경로를 적는다.
```

- [ ] **Step 3: 적용·확인**

```bash
cd ~/VSCodeWorkspace/ai-company && python3 -m pytest -q   # 또는 레포의 테스트 명령
# 사용자 회사에 재설치(마커 블록 갱신) — SKILL.md의 설치 명령대로, 예:
python3 plugins/ai-company/skills/configure-company/generator/companyctl.py install --root ~/ai-folder/company
python3 plugins/ai-company/skills/configure-company/generator/companyctl.py employee add --root ~/ai-folder/company --dept 기술검증팀 --name "원격 QA(Hermes)" --remote hermes-qa
python3 plugins/ai-company/skills/configure-company/generator/companyctl.py doctor --root ~/ai-folder/company
```
Expected: `~/ai-folder/company/CLAUDE.md`의 `store:ai-company` 블록에 원격·MESSAGE·감시 상시 문안, `직원명부.json`에 `mode: "remote"` 직원, doctor에 `remote check` OK.

- [ ] **Step 4: 커밋·릴리즈** — ai-company 레포 커밋(`feat: 원격 직원 등록(--remote)·편지함(MESSAGE)·감시 상시`), 태그 v0.2.6, 그 레포의 릴리즈 절차대로.

---

## 실측 기록

(Task 13에서 채운다.)

- 2026-09-25 17:19~17:26, 로컬 빌드 `/tmp/agentlayer`, 서버 Hermes v0.20.0, 프로필 `tech-qa`, ssh 왕복 1.6s(첫 연결).
- 등록: `remote add hermes-qa …` → check OK. 첫 시도는 ControlPath가 state dir 아래라 macOS 소켓 경로 상한(104B)에 걸려 `unix_listener: path too long` → `/tmp/agentlayer-ssh-<uid>`로 옮겨 해결(테스트 `TestOpenHermesControlPathFitsUnixSocket`).
- 1바퀴(지시→완료→회수→마감): `send` → 카드 `t_7fae8b4f` 생성·dispatch → 약 30초 뒤 `{"to":"DONE_UNREAD","task":"PONG-2","cwd":".../결과물/PING-2/remote"}` → `RESULT.md`에 `PONG-2` → `task done PING-2` → 서버 목록에서 PING-2 사라짐(archived). log.md: [ASSIGN]→[SEND]→[REPORT] DONE→[COMPLETE].
- 2바퀴(질문→답→완료): 카드 `t_3f2a4e0e`. Hermes가 지시대로 `block --kind needs_input` → `{"to":"WAITING","ask":"어떤 단어로 답할까요?"}`(task.md `waiting_hermes-qa`, [ASK]) → `send hermes-qa "PONG-3"` → `[WAITING] 답변`(unblock --reason + dispatch) → `{"to":"DONE_UNREAD","task":"PONG-3"}`. 서버 이벤트: blocked→commented→unblocked→claimed→spawned→completed, 코멘트 `UNBLOCK: PONG-3`(author default).
- 3바퀴(편지함): 서버 `kanban create "[PING-3] 편지 시험" --assignee imac-manager --created-by default` → `{"to":"MESSAGE","from":"default","task_id":"PING-3","kind":"letter"}`, 카드는 done(수신 확인). 로컬 `task message "로컬 직원 편지 시험"`(pane %17, 세션 agentlayer-dev) → `{"to":"MESSAGE","from":"agentlayer-dev","task_id":"-","kind":"message"}`.
- 발견·수정: 카드 제목에 업무ID 중복(`PING-2 PING-2 …`) → 제목이 ID로 시작하면 안 붙임(`TestHermesDispatchTitleAlreadyHasID`).
- 정리: 시험 카드 `t_40b3eb2f`·`t_7019c9ff` archive, 회사 루트의 PING-2·PING-3(tasks·업무요청·결과물·received 보고) 삭제. 원격 등록 `hermes-qa`는 유지.
- 2026-09-25 17:40 리뷰 반영 뒤 재검증(PING-4, 카드 `t_0e3bb4ac`): 하이픈으로 시작하는 본문(`--body=`) → WAITING(ask "단어?") → `send "-PONG-4"`(`--reason=`) → DONE_UNREAD, 작업 폴더의 `out.txt`(내용 `-PONG-4`)가 `결과물/PING-4/remote/`로 회수됨(원격 `tar | head -c` 경로). `task done` 뒤 서버 카드 archived. 시험 흔적 삭제.
