# 칸반 라이트 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 회사 루트의 `tasks/<ID>/task.md`·`log.md`를 agentlayer가 자동으로 전이·기록하고, 디스코드 카드와 `agentlayer board` HTML로 보드를 보여 준다(agentlayer v1.6.0), 그리고 ai-company 플러그인 v0.2가 그 절차를 총괄에게 가르친다.

**Architecture:** 새 패키지 `internal/board`가 task.md/log.md 읽기·쓰기·열 파생·HTML의 유일한 소유자다. `internal/task`는 등록(Assignment)에 `TaskDir`를 얹고, 훅 전이·send·done에서 board를 호출한다. 표시는 `internal/discord/card.go`에 컨테이너 하나, CLI `board` 명령 하나. 데몬·HTTP 없음, 파일이 정본.

**Tech Stack:** Go 1.22+(표준 라이브러리만, yaml 파서 없이 줄 단위 파싱), 기존 테스트 관례(`go test ./... -count=1`, `t.TempDir()`), Python 3 + pytest(ai-company).

**Spec:** `docs/superpowers/specs/2026-09-15-kanban-lite-design.md`

## Global Constraints

- 상태값은 `pending / in_progress / waiting_<tmux세션> / reviewing / done`만 쓴다. 새 값 금지. `ready`는 파생.
- 열 이름은 `todo ready running blocked review done` 여섯 개, 이 순서.
- task.md 쓰기는 `status:` 줄(있으면 `updated:` 줄도) 교체만, temp→rename 원자적. 다른 내용은 바이트 단위로 보존.
- log.md 한 줄 형식: `[YYYY-MM-DD HH:MM] [TAG] 내용`, TAG ∈ `ASSIGN ASK SEND REPORT ERROR COMPLETE`.
- `send` 기록은 1000 rune 절단 + `…`, 개행은 `⏎`.
- 방치 기준 `board_stale_ready` 기본 `30m`, 하한 `1m`.
- 훅은 절대 에이전트를 막지 않는다: 실패는 stderr 한 줄, exit 0, 2초 상한.
- 카드 보드 행: blocked → review → running → ready 순, 최대 8행, 넘으면 `외 n`. 루트 없으면 컨테이너 생략.
- READY 이벤트 JSON: `{"version":1,"id":<32hex>,"task_id":<자식>,"kind":"board","from":"pending","to":"READY","task":<제목>,"task_dir":<경로>,"at":<시각>}`.
- 모든 사용자 문구는 한국어, 기존 파일 주석 스타일(한국어 문단 주석) 유지.
- 커밋 메시지 끝에 세션 attribution 두 줄을 붙인다(대화의 system-reminder 참조).

---

## 파일 구조

| 파일 | 책임 |
|---|---|
| `internal/board/taskfile.go` (신규) | task.md 한 장 읽기(`TaskFile`)·`SetStatus`·`AppendLog`·`ReadLastLog` |
| `internal/board/board.go` (신규) | `Card`·`Column`·`Load`·`Children`·`StaleReady`·`InferRoot`·`Root` |
| `internal/board/html.go` (신규) | `HTML(cards, now)` 정적 페이지 |
| `internal/board/*_test.go` | 위 세 파일 테스트 |
| `internal/task/assign.go` (수정) | `Assignment.TaskDir` |
| `internal/task/report.go` (수정) | `Report.TaskDir`, `ReadyReport` |
| `internal/task/board.go` (신규) | 훅 전이 → board 쓰기(`ApplyTransition`), `MarkDone`, `ReadyEvents` |
| `internal/cli/taskcmd.go` (수정) | assign `--root`·TaskDir·status·log, done의 status·READY, `--root` |
| `internal/cli/sendcmd.go` (수정) | 전송 뒤 `[SEND]` 기록 |
| `internal/cli/boardcmd.go` (신규) | `agentlayer board [--out] [--json]` |
| `internal/cli/helpcmd.go` (수정) | board 행 |
| `internal/config/config.go` (수정) | `company_root`·`board_stale_ready` |
| `internal/discord/card.go` (수정) | `CardData.Board`, `boardContainer` |
| `main.go` (수정) | runHook 콜백에서 ApplyTransition, `board` 라우팅, card에 Board 주입 |
| `README.md` (수정) | 업무 보드 절 |
| ai-company: `assets/task.md`·`assets/company-block.md`·`SKILL.md`·`generator/companyctl.py`·`tests/test_companyctl.py`·매니페스트 | v0.2 |

---

### Task 1: `internal/board/taskfile.go` — task.md·log.md 읽기/쓰기

**Files:**
- Create: `internal/board/taskfile.go`
- Test: `internal/board/taskfile_test.go`

**Interfaces:**
- Produces:
  ```go
  type TaskFile struct { ID, Title, Status string; Parents []string; Updated time.Time }
  func ReadTaskFile(root, id string) (TaskFile, error)          // task.md 없음 → os.ErrNotExist 래핑
  func SetStatus(root, id, status string, now time.Time) error   // yaml 블록 없거나 status: 없음 → ErrNoStatus
  func AppendLog(root, id, tag, text string, now time.Time) error
  func ReadLastLog(root, id string) string                        // 마지막 비어 있지 않은 줄, 없으면 ""
  var ErrNoStatus = errors.New("task.md에 ```yaml 블록의 status: 줄이 없습니다")
  func TaskDir(root, id string) string                            // filepath.Join(root, "tasks", id)
  ```

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/board/taskfile_test.go
package board

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 15, 14, 30, 0, 0, time.FixedZone("KST", 9*3600))

func writeTask(t *testing.T, root, id, body string) {
	t.Helper()
	dir := filepath.Join(root, "tasks", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const sample = "# VIDEO-07 — 주제 선정\n\n## 메타\n\n```yaml\nstatus: pending\n# 주석 줄\ncreated: 2026-09-15\nupdated: 2026-09-15\nparents: [VIDEO-06, VIDEO-05]\npriority: medium\n```\n\n## 목표\n한 문장.\n"

func TestReadTaskFileParsesTitleStatusParents(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "VIDEO-07", sample)
	tf, err := ReadTaskFile(root, "VIDEO-07")
	if err != nil {
		t.Fatal(err)
	}
	if tf.ID != "VIDEO-07" || tf.Title != "VIDEO-07 — 주제 선정" || tf.Status != "pending" {
		t.Errorf("got %+v", tf)
	}
	if strings.Join(tf.Parents, ",") != "VIDEO-06,VIDEO-05" {
		t.Errorf("parents = %v", tf.Parents)
	}
	if tf.Updated.IsZero() {
		t.Error("Updated(mtime)가 비었다")
	}
}

func TestReadTaskFileParentsMultilineList(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "B", "# B\n```yaml\nstatus: pending\nparents:\n  - A\n  - C\npriority: low\n```\n")
	tf, err := ReadTaskFile(root, "B")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tf.Parents, ",") != "A,C" {
		t.Errorf("parents = %v", tf.Parents)
	}
}

func TestReadTaskFileNoYAMLIsUnknownAndTitleFallsBackToID(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "X", "no heading here\n")
	tf, err := ReadTaskFile(root, "X")
	if err != nil {
		t.Fatal(err)
	}
	if tf.Status != "unknown" || tf.Title != "X" {
		t.Errorf("got %+v", tf)
	}
}

func TestReadTaskFileMissingIsNotExist(t *testing.T) {
	if _, err := ReadTaskFile(t.TempDir(), "NOPE"); !os.IsNotExist(err) {
		t.Errorf("err = %v, want not-exist", err)
	}
}

func TestSetStatusReplacesOnlyStatusAndUpdatedLines(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "VIDEO-07", sample)
	if err := SetStatus(root, "VIDEO-07", "in_progress", now); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "VIDEO-07", "task.md"))
	got := string(b)
	want := strings.Replace(sample, "status: pending", "status: in_progress", 1)
	if got != want {
		t.Errorf("파일 내용이 status/updated 외에 바뀜:\n%s", got)
	}
	if !strings.Contains(got, "updated: 2026-09-15") {
		t.Error("updated: 줄이 사라짐")
	}
	// updated 날짜가 다른 경우 갱신
	writeTask(t, root, "OLD", strings.Replace(sample, "updated: 2026-09-15", "updated: 2026-09-01", 1))
	if err := SetStatus(root, "OLD", "done", now); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(root, "tasks", "OLD", "task.md"))
	if !strings.Contains(string(b), "updated: 2026-09-15") || !strings.Contains(string(b), "status: done") {
		t.Errorf("updated/status 미갱신:\n%s", b)
	}
	// 임시 파일이 남지 않는다
	entries, _ := os.ReadDir(filepath.Join(root, "tasks", "OLD"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".task.md") {
			t.Errorf("임시 파일 잔재: %s", e.Name())
		}
	}
}

func TestSetStatusWithoutYAMLFails(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "X", "# X\nstatus: pending (블록 밖)\n")
	if err := SetStatus(root, "X", "done", now); err != ErrNoStatus {
		t.Errorf("err = %v, want ErrNoStatus", err)
	}
}

func TestAppendLogFormatAndCreatesFile(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "A", sample)
	if err := AppendLog(root, "A", "ASK", "폴더 밖을 읽어도 될까요?", now); err != nil {
		t.Fatal(err)
	}
	if err := AppendLog(root, "A", "SEND", "네, 됩니다", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "A", "log.md"))
	want := "[2026-09-15 14:30] [ASK] 폴더 밖을 읽어도 될까요?\n[2026-09-15 14:31] [SEND] 네, 됩니다\n"
	if !strings.HasSuffix(string(b), want) {
		t.Errorf("log.md:\n%s", b)
	}
	if ReadLastLog(root, "A") != "[2026-09-15 14:31] [SEND] 네, 됩니다" {
		t.Errorf("ReadLastLog = %q", ReadLastLog(root, "A"))
	}
	if ReadLastLog(root, "NOPE") != "" {
		t.Error("없는 log는 빈 문자열")
	}
}

func TestAppendLogFlattensNewlines(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "A", sample)
	if err := AppendLog(root, "A", "SEND", "첫 줄\n둘째 줄", now); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ReadLastLog(root, "A"), "첫 줄⏎둘째 줄") {
		t.Errorf("개행이 ⏎로 안 바뀜: %q", ReadLastLog(root, "A"))
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/board/ -run 'TestReadTaskFile|TestSetStatus|TestAppendLog' -count=1`
Expected: 컴파일 실패(패키지 없음).

- [ ] **Step 3: 구현**

```go
// internal/board/taskfile.go
// Package board는 AI 회사 루트의 tasks/<ID>/task.md·log.md를 읽고 쓰는 유일한 곳이다.
// yaml 파서를 쓰지 않고 줄 단위로 status:·parents:만 다룬다(starter.readStatus와 같은 원칙 —
// 파싱 실패로 관제탑이 멈추면 안 된다). starter 패키지는 다른 루트(MultiAgent)·다른 목적이라 건드리지 않는다.
package board

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrNoStatus — task.md에 ```yaml 블록의 status: 줄이 없어 상태를 쓸 수 없다.
var ErrNoStatus = errors.New("task.md에 ```yaml 블록의 status: 줄이 없습니다")

// TaskFile은 task.md 한 장의 요약.
type TaskFile struct {
	ID      string
	Title   string    // 첫 "# " 제목, 없으면 ID
	Status  string    // yaml status:, 없으면 "unknown"
	Parents []string  // yaml parents: [A, B] 또는 여러 줄 리스트
	Updated time.Time // task.md mtime
}

// TaskDir은 업무 폴더 경로.
func TaskDir(root, id string) string { return filepath.Join(root, "tasks", id) }

func taskPath(root, id string) string { return filepath.Join(TaskDir(root, id), "task.md") }
func logPath(root, id string) string  { return filepath.Join(TaskDir(root, id), "log.md") }

// ReadTaskFile은 task.md를 읽는다. 없으면 os.IsNotExist가 참인 에러.
func ReadTaskFile(root, id string) (TaskFile, error) {
	p := taskPath(root, id)
	st, err := os.Stat(p)
	if err != nil {
		return TaskFile{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return TaskFile{}, err
	}
	tf := TaskFile{ID: id, Title: id, Status: "unknown", Updated: st.ModTime()}
	inYAML, inParents, titleSet := false, false, false
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case !titleSet && strings.HasPrefix(trimmed, "# "):
			tf.Title = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			titleSet = true
		case strings.HasPrefix(trimmed, "```yaml"):
			inYAML = true
		case strings.HasPrefix(trimmed, "```"):
			inYAML, inParents = false, false
		case !inYAML:
		case inParents && strings.HasPrefix(trimmed, "- "):
			tf.Parents = append(tf.Parents, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		case strings.HasPrefix(trimmed, "status:"):
			inParents = false
			tf.Status = strings.TrimSpace(strings.TrimPrefix(trimmed, "status:"))
		case strings.HasPrefix(trimmed, "parents:"):
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "parents:"))
			tf.Parents = parseInlineList(rest)
			inParents = rest == "" // 여러 줄 리스트가 이어진다
		default:
			inParents = false
		}
	}
	return tf, nil
}

// parseInlineList는 "[A, B]" 또는 "A, B"를 항목 목록으로. 빈 값·"[]"는 nil.
func parseInlineList(s string) []string {
	s = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s), "["), "]")
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.Trim(strings.TrimSpace(p), `"'`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SetStatus는 ```yaml 블록의 status: 줄만 바꾸고(updated: 줄이 있으면 오늘 날짜로) 원자적으로 쓴다.
// 다른 바이트는 그대로 — 총괄이 쓴 본문을 훅이 망치면 안 된다.
func SetStatus(root, id, status string, now time.Time) error {
	p := taskPath(root, id)
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	inYAML, found := false, false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		switch {
		case strings.HasPrefix(trimmed, "```yaml"):
			inYAML = true
		case strings.HasPrefix(trimmed, "```"):
			inYAML = false
		case inYAML && strings.HasPrefix(trimmed, "status:"):
			lines[i] = indent + "status: " + status
			found = true
		case inYAML && strings.HasPrefix(trimmed, "updated:"):
			lines[i] = indent + "updated: " + now.Format("2006-01-02")
		}
	}
	if !found {
		return ErrNoStatus
	}
	return writeAtomic(p, []byte(strings.Join(lines, "\n")))
}

// AppendLog는 "[YYYY-MM-DD HH:MM] [TAG] text" 한 줄을 log.md 끝에 붙인다. 개행은 ⏎로 접는다.
func AppendLog(root, id, tag, text string, now time.Time) error {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "⏎")
	f, err := os.OpenFile(logPath(root, id), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("[" + now.Format("2006-01-02 15:04") + "] [" + tag + "] " + text + "\n")
	return err
}

// ReadLastLog는 log.md의 마지막 비어 있지 않은 줄. 없으면 "".
func ReadLastLog(root, id string) string {
	b, err := os.ReadFile(logPath(root, id))
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}

// writeAtomic은 temp→rename. 훅·총괄이 동시에 써도 반쪽 파일이 없다.
func writeAtomic(p string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/board/ -count=1 -v`
Expected: 8개 PASS.

- [ ] **Step 5: 커밋**

```bash
git add internal/board/taskfile.go internal/board/taskfile_test.go
git commit -m "feat(board): task.md·log.md 읽기/쓰기 — status 줄만 교체, log append"
```

---

### Task 2: `internal/board/board.go` — 카드·열 파생·루트 유추 + 설정 키

**Files:**
- Create: `internal/board/board.go`
- Modify: `internal/config/config.go` (Config 구조체 끝, 접근자 추가)
- Test: `internal/board/board_test.go`, `internal/config/config_test.go`(기존 파일에 추가)

**Interfaces:**
- Consumes: Task 1의 `ReadTaskFile`·`ReadLastLog`·`TaskDir`; `task.Assignment{TaskID, Session, Window, AgentID}`; `state.Agent{ID, State}`.
- Produces:
  ```go
  const (ColTodo = "todo"; ColReady = "ready"; ColRunning = "running"; ColBlocked = "blocked"; ColReview = "review"; ColDone = "done")
  var Columns = []string{ColTodo, ColReady, ColRunning, ColBlocked, ColReview, ColDone}
  type Card struct { ID, Title, Status, Column, Session, State string; Parents []string; Updated, Ready time.Time; LastLog string; Unknown bool }
  type Link struct { TaskID, Session, Window, AgentID string }   // task.Assignment의 board용 축약(import cycle 회피)
  func Load(root string, links []Link, states map[string]string, now time.Time) ([]Card, error) // states: AgentID → 상태 단어(idle·WORK…)
  func Children(cards []Card, parentID string) []Card
  func StaleReady(c Card, now time.Time, limit time.Duration) bool   // ready·blocked 열만
  func InferRoot(inbox string) string                                 // ".../runtime/inbox" → root, 아니면 ""
  func Root(configured string, inboxes []string) string
  func Counts(cards []Card) map[string]int
  func CompanyName(root string) string   // 직원명부.json의 name, 없으면 filepath.Base(root)
  ```
  config: `Config.CompanyRoot string` (`company_root`), `Config.BoardStaleReady string` (`board_stale_ready`), `func (c *Config) BoardStaleLimit() time.Duration`(기본 30m, 하한 1m, 파싱 실패 기본값).

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/board/board_test.go
package board

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func task(status, parents string) string {
	return "# T\n```yaml\nstatus: " + status + "\nparents: " + parents + "\n```\n"
}

func TestLoadDerivesColumns(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "A", task("done", "[]"))
	writeTask(t, root, "B", task("pending", "[A]"))        // ready (부모 done)
	writeTask(t, root, "C", task("pending", "[A, B]"))     // todo (B 미완)
	writeTask(t, root, "D", task("in_progress", "[]"))     // running
	writeTask(t, root, "E", task("waiting_collab-bot", "[]")) // blocked
	writeTask(t, root, "F", task("reviewing", "[]"))       // review
	writeTask(t, root, "G", task("weird", "[]"))           // todo + Unknown
	writeTask(t, root, "H", task("pending", "[ZZZ]"))      // 부모 없음(끊긴 참조) → todo
	cards, err := Load(root, nil, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range cards {
		got[c.ID] = c.Column
	}
	want := map[string]string{"A": ColDone, "B": ColReady, "C": ColTodo, "D": ColRunning, "E": ColBlocked, "F": ColReview, "G": ColTodo, "H": ColTodo}
	for id, col := range want {
		if got[id] != col {
			t.Errorf("%s: %s, want %s", id, got[id], col)
		}
	}
	for _, c := range cards {
		if c.ID == "G" && !c.Unknown {
			t.Error("알 수 없는 status는 Unknown=true")
		}
	}
}

func TestLoadAttachesSessionStateAndLastLog(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "A", task("in_progress", "[]"))
	_ = AppendLog(root, "A", "ASK", "질문?", now)
	links := []Link{{TaskID: "A", Session: "collab-bot", Window: "t123456", AgentID: "claude-%3"}}
	cards, err := Load(root, links, map[string]string{"claude-%3": "WAIT"}, now)
	if err != nil {
		t.Fatal(err)
	}
	c := cards[0]
	if c.Session != "collab-bot:t123456" || c.State != "WAIT" || c.LastLog != "[2026-09-15 14:30] [ASK] 질문?" {
		t.Errorf("got %+v", c)
	}
}

func TestLoadReadyTimeIsLatestParentDone(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "A", task("done", "[]"))
	writeTask(t, root, "B", task("pending", "[A]"))
	old := now.Add(-2 * time.Hour)
	_ = os.Chtimes(filepath.Join(root, "tasks", "A", "task.md"), old, old)
	_ = os.Chtimes(filepath.Join(root, "tasks", "B", "task.md"), now.Add(-3*time.Hour), now.Add(-3*time.Hour))
	cards, _ := Load(root, nil, nil, now)
	for _, c := range cards {
		if c.ID == "B" && !c.Ready.Equal(old) {
			t.Errorf("Ready = %v, want 부모 done 시각 %v", c.Ready, old)
		}
	}
}

func TestLoadSortsByIDAndSkipsNonDirs(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "B", task("pending", "[]"))
	writeTask(t, root, "A", task("pending", "[]"))
	_ = os.WriteFile(filepath.Join(root, "tasks", "README.md"), []byte("x"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "tasks", "empty"), 0o755) // task.md 없음 → 건너뜀
	cards, _ := Load(root, nil, nil, now)
	if len(cards) != 2 || cards[0].ID != "A" || cards[1].ID != "B" {
		t.Errorf("cards = %+v", cards)
	}
}

func TestLoadNoTasksDirIsEmptyNotError(t *testing.T) {
	cards, err := Load(t.TempDir(), nil, nil, now)
	if err != nil || len(cards) != 0 {
		t.Errorf("cards=%v err=%v", cards, err)
	}
}

func TestChildrenAndStaleReady(t *testing.T) {
	cards := []Card{
		{ID: "B", Parents: []string{"A"}, Column: ColReady, Ready: now.Add(-31 * time.Minute)},
		{ID: "C", Parents: []string{"A", "X"}, Column: ColBlocked, Updated: now.Add(-5 * time.Minute)},
		{ID: "D", Column: ColRunning, Updated: now.Add(-3 * time.Hour)},
	}
	if ch := Children(cards, "A"); len(ch) != 2 || ch[0].ID != "B" || ch[1].ID != "C" {
		t.Errorf("Children = %+v", ch)
	}
	if !StaleReady(cards[0], now, 30*time.Minute) {
		t.Error("ready 31분은 ⚠")
	}
	if StaleReady(cards[1], now, 30*time.Minute) {
		t.Error("blocked 5분은 아직")
	}
	if StaleReady(cards[2], now, 30*time.Minute) {
		t.Error("running은 대상 아님")
	}
}

func TestInferRootAndRoot(t *testing.T) {
	if got := InferRoot("/Users/x/company/runtime/inbox"); got != "/Users/x/company" {
		t.Errorf("InferRoot = %q", got)
	}
	if got := InferRoot("/Users/x/company/runtime/inbox/"); got != "/Users/x/company" {
		t.Errorf("InferRoot(trailing slash) = %q", got)
	}
	if InferRoot("/Users/x/somewhere") != "" {
		t.Error("규약 밖 경로는 빈 문자열")
	}
	if Root("/cfg", []string{"/a/runtime/inbox"}) != "/cfg" {
		t.Error("설정이 우선")
	}
	if Root("", []string{"/nope", "/a/runtime/inbox"}) != "/a" {
		t.Error("첫 유추 성공 값")
	}
	if Root("", nil) != "" {
		t.Error("아무것도 없으면 빈 문자열")
	}
}

func TestCompanyName(t *testing.T) {
	root := t.TempDir()
	if CompanyName(root) != filepath.Base(root) {
		t.Error("명부 없으면 폴더명")
	}
	_ = os.WriteFile(filepath.Join(root, "직원명부.json"), []byte(`{"name":"AI 치트키 회사"}`), 0o644)
	if CompanyName(root) != "AI 치트키 회사" {
		t.Errorf("CompanyName = %q", CompanyName(root))
	}
}

func TestCounts(t *testing.T) {
	c := Counts([]Card{{Column: ColReady}, {Column: ColReady}, {Column: ColDone}})
	if c[ColReady] != 2 || c[ColDone] != 1 || c[ColTodo] != 0 {
		t.Errorf("Counts = %v", c)
	}
}
```

config 테스트(`internal/config/config_test.go` 끝에 추가):

```go
func TestBoardStaleLimit(t *testing.T) {
	cases := map[string]time.Duration{"": 30 * time.Minute, "bad": 30 * time.Minute, "10s": time.Minute, "2h": 2 * time.Hour}
	for in, want := range cases {
		c := &Config{BoardStaleReady: in}
		if got := c.BoardStaleLimit(); got != want {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/board/ ./internal/config/ -count=1`
Expected: 컴파일 실패(Load·Link·BoardStaleLimit 미정의).

- [ ] **Step 3: 구현**

`internal/config/config.go` — Config 구조체의 `BrowserFx` 필드 뒤에 추가:

```go
	// AI 회사 루트(tasks/·runtime/inbox/가 있는 폴더). 비면 등록된 업무의 inbox 경로에서 유추한다.
	CompanyRoot string `json:"company_root,omitempty"`
	// 업무 보드에서 ready·blocked 카드가 이 시간 넘게 방치되면 ⚠ (Go duration, 기본 30m, 하한 1m).
	BoardStaleReady string `json:"board_stale_ready,omitempty"`
```

접근자(BrowserFxEnabled 뒤):

```go
const (
	defaultBoardStale = 30 * time.Minute
	minBoardStale     = time.Minute
)

// BoardStaleLimit는 board_stale_ready를 반영한 방치 기준. 파싱 불가·0 이하는 기본값, 하한 미만은 하한.
func (c *Config) BoardStaleLimit() time.Duration {
	d, err := time.ParseDuration(c.BoardStaleReady)
	if err != nil || d <= 0 {
		return defaultBoardStale
	}
	if d < minBoardStale {
		return minBoardStale
	}
	return d
}
```

`internal/board/board.go`:

```go
// internal/board/board.go
package board

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 열 이름 — Hermes 칸반의 6열. 저장하는 값이 아니라 status에서 파생한 표시 이름이다.
const (
	ColTodo    = "todo"
	ColReady   = "ready"
	ColRunning = "running"
	ColBlocked = "blocked"
	ColReview  = "review"
	ColDone    = "done"
)

// Columns는 보드 열 순서.
var Columns = []string{ColTodo, ColReady, ColRunning, ColBlocked, ColReview, ColDone}

// Card는 보드에 놓이는 업무 하나.
type Card struct {
	ID      string
	Title   string
	Status  string    // task.md 원문 status
	Column  string    // 파생 열
	Session string    // 등록 세션[:창], 없으면 ""
	State   string    // 등록 세션의 상태 단어(idle·WORK·WAIT·DONE·ERR·dead), 없으면 ""
	Parents []string
	Updated time.Time // task.md mtime
	Ready   time.Time // ready 열 진입 시각(마지막 부모 done의 mtime, 부모 없으면 자기 mtime)
	LastLog string
	Unknown bool // status 값을 해석하지 못함(todo 열에 ? 배지)
}

// Link는 업무 등록의 board용 축약 — task 패키지가 board를 import하므로 역방향 의존을 피한다.
type Link struct{ TaskID, Session, Window, AgentID string }

// columnOf는 status → 열. ready는 부모가 전부 done일 때만.
func columnOf(status string, parents []string, done map[string]bool) (string, bool) {
	switch {
	case status == "pending":
		for _, p := range parents {
			if !done[p] {
				return ColTodo, false
			}
		}
		return ColReady, false
	case status == "in_progress":
		return ColRunning, false
	case strings.HasPrefix(status, "waiting_"):
		return ColBlocked, false
	case status == "reviewing":
		return ColReview, false
	case status == "done":
		return ColDone, false
	}
	return ColTodo, true
}

// Load는 root/tasks/*/task.md를 전부 읽어 카드로 만든다(ID 오름차순). tasks/가 없으면 빈 목록.
// states는 AgentID → 상태 단어.
func Load(root string, links []Link, states map[string]string, now time.Time) ([]Card, error) {
	entries, err := os.ReadDir(filepath.Join(root, "tasks"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := map[string]TaskFile{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		tf, err := ReadTaskFile(root, e.Name())
		if err != nil {
			continue // task.md 없는 폴더는 업무가 아니다
		}
		files[e.Name()] = tf
	}
	done := map[string]bool{}
	for id, tf := range files {
		done[id] = tf.Status == "done"
	}
	byTask := map[string]Link{}
	for _, l := range links {
		byTask[l.TaskID] = l
	}
	var cards []Card
	for id, tf := range files {
		col, unknown := columnOf(tf.Status, tf.Parents, done)
		c := Card{ID: id, Title: tf.Title, Status: tf.Status, Column: col, Parents: tf.Parents,
			Updated: tf.Updated, Ready: tf.Updated, LastLog: ReadLastLog(root, id), Unknown: unknown}
		if col == ColReady {
			for _, p := range tf.Parents {
				if pt := files[p]; pt.Updated.After(c.Ready) {
					c.Ready = pt.Updated
				}
			}
		}
		if l, ok := byTask[id]; ok {
			c.Session = l.Session
			if l.Window != "" {
				c.Session += ":" + l.Window
			}
			c.State = states[l.AgentID]
		}
		cards = append(cards, c)
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].ID < cards[j].ID })
	return cards, nil
}

// Children은 parents에 parentID를 가진 카드들(입력 순서 유지).
func Children(cards []Card, parentID string) []Card {
	var out []Card
	for _, c := range cards {
		for _, p := range c.Parents {
			if p == parentID {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// StaleReady — ready 열은 Ready 시각, blocked 열은 Updated 시각 기준으로 limit을 넘겼는가.
func StaleReady(c Card, now time.Time, limit time.Duration) bool {
	switch c.Column {
	case ColReady:
		return now.Sub(c.Ready) > limit
	case ColBlocked:
		return now.Sub(c.Updated) > limit
	}
	return false
}

// Counts는 열별 카드 수.
func Counts(cards []Card) map[string]int {
	m := map[string]int{}
	for _, c := range cards {
		m[c.Column]++
	}
	return m
}

// InferRoot는 "<root>/runtime/inbox" 규약에서 root를 뽑는다. 규약 밖이면 "".
func InferRoot(inbox string) string {
	inbox = filepath.Clean(inbox)
	if filepath.Base(inbox) != "inbox" || filepath.Base(filepath.Dir(inbox)) != "runtime" {
		return ""
	}
	return filepath.Dir(filepath.Dir(inbox))
}

// Root는 설정값 우선, 없으면 inbox 목록에서 첫 유추 성공 값. 설정 0개로 동작하기 위한 단일 지점.
func Root(configured string, inboxes []string) string {
	if configured != "" {
		return configured
	}
	for _, in := range inboxes {
		if r := InferRoot(in); r != "" {
			return r
		}
	}
	return ""
}

// CompanyName은 <root>/직원명부.json의 name. 없으면 폴더명.
func CompanyName(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "직원명부.json"))
	if err == nil {
		var v struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(b, &v) == nil && v.Name != "" {
			return v.Name
		}
	}
	return filepath.Base(root)
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/board/ ./internal/config/ -count=1`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
git add internal/board/board.go internal/board/board_test.go internal/config/config.go internal/config/config_test.go
git commit -m "feat(board): 카드 로드·열 파생·루트 유추, 설정 company_root·board_stale_ready"
```

---

### Task 3: `task assign` — 등록에 TaskDir 연결, status in_progress, [ASSIGN] 기록

**Files:**
- Modify: `internal/task/assign.go` (Assignment 구조체)
- Modify: `internal/cli/taskcmd.go` (`taskAssign`, `taskUsage`)
- Test: `internal/cli/taskcmd_test.go`(기존에 추가), `internal/task/assign_test.go`(기존에 추가)

**Interfaces:**
- Consumes: `board.InferRoot`, `board.TaskDir`, `board.ReadTaskFile`, `board.SetStatus`, `board.AppendLog`.
- Produces: `Assignment.TaskDir string` (json `task_dir,omitempty`), `task assign … [--root <회사루트>]`.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/task/assign_test.go` 끝에:

```go
func TestAssignmentTaskDirRoundTrip(t *testing.T) {
	dir := t.TempDir()
	as := Assignment{TaskID: "A", AgentID: "claude-%1", Session: "s", Pane: "%1", Inbox: "/x/runtime/inbox",
		TaskDir: "/x/tasks/A", AssignedAt: time.Now()}
	if err := Assign(dir, as, false); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(dir, "claude-%1")
	if err != nil || !ok || got.TaskDir != "/x/tasks/A" {
		t.Errorf("got %+v ok=%v err=%v", got, ok, err)
	}
}
```

`internal/cli/taskcmd_test.go` 끝에(기존 테스트가 쓰는 store 픽스처 헬퍼 이름을 확인해 재사용. 아래는 `newStoreWith(t, agents...)`가 없다고 가정하고 직접 만든다):

```go
func companyRoot(t *testing.T, id string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "tasks", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# " + id + "\n```yaml\nstatus: pending\nupdated: 2026-01-01\nparents: []\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "runtime", "inbox"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestTaskAssignLinksTaskDirAndMarksInProgress(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Put(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}})
	root := companyRoot(t, "LAB-1")
	var out bytes.Buffer
	err := RunTask(context.Background(), &out, st, stateDir,
		[]string{"assign", "LAB-1", "collab-bot", "--inbox", filepath.Join(root, "runtime", "inbox")}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	as, ok, _ := task.Load(stateDir, "claude-%1")
	if !ok || as.TaskDir != filepath.Join(root, "tasks", "LAB-1") {
		t.Errorf("TaskDir = %q", as.TaskDir)
	}
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "LAB-1", "task.md"))
	if !strings.Contains(string(b), "status: in_progress") {
		t.Errorf("status 미전이:\n%s", b)
	}
	lg, _ := os.ReadFile(filepath.Join(root, "tasks", "LAB-1", "log.md"))
	if !strings.Contains(string(lg), "[ASSIGN] collab-bot") {
		t.Errorf("log 미기록: %s", lg)
	}
}

func TestTaskAssignWithoutTaskFileWarnsButRegisters(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Put(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}})
	var out bytes.Buffer
	err := RunTask(context.Background(), &out, st, stateDir,
		[]string{"assign", "NOFILE", "collab-bot", "--inbox", t.TempDir()}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	as, ok, _ := task.Load(stateDir, "claude-%1")
	if !ok || as.TaskDir != "" {
		t.Errorf("TaskDir는 비어야 함: %q", as.TaskDir)
	}
	if !strings.Contains(out.String(), "보드에 표시되지 않음") {
		t.Errorf("경고 문구 없음: %s", out.String())
	}
}

func TestTaskAssignExplicitRoot(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Put(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}})
	root := companyRoot(t, "LAB-1")
	var out bytes.Buffer
	err := RunTask(context.Background(), &out, st, stateDir,
		[]string{"assign", "LAB-1", "collab-bot", "--inbox", t.TempDir(), "--root", root}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	as, _, _ := task.Load(stateDir, "claude-%1")
	if as.TaskDir != filepath.Join(root, "tasks", "LAB-1") {
		t.Errorf("--root 무시됨: %q", as.TaskDir)
	}
}
```

(필요한 import: `bytes`, `context`, `os`, `path/filepath`, `strings`, `time`, `internal/state`, `internal/task`. `st.Put`이 실제 Store 메서드 이름인지 `internal/state/store.go`에서 확인하고 다르면 맞춘다.)

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/task/ ./internal/cli/ -run 'TaskDir|TaskAssign' -count=1`
Expected: 컴파일 실패(TaskDir 필드 없음) 또는 FAIL.

- [ ] **Step 3: 구현**

`internal/task/assign.go` — Assignment에 필드 추가(Inbox 뒤):

```go
	// TaskDir은 회사 루트의 tasks/<업무ID>/ — 있으면 훅이 task.md status·log.md를 자동 갱신한다.
	TaskDir    string    `json:"task_dir,omitempty"`
```

`internal/cli/taskcmd.go`:

- `taskUsage` 첫 줄을 `agentlayer task assign <업무ID> <세션[:창]> --inbox <폴더> [--root <회사루트>] [--replace]`로.
- `taskAssign`의 플래그 루프에 `--root` 케이스 추가(`root` 변수), `as` 구성 뒤 등록 전에:

```go
	if root == "" {
		root = board.InferRoot(abs)
	}
	warn := ""
	if root != "" {
		if _, err := board.ReadTaskFile(root, pos[0]); err == nil {
			as.TaskDir = board.TaskDir(root, pos[0])
		}
	}
	if as.TaskDir == "" {
		warn = "  ⚠ tasks/" + pos[0] + "/task.md가 없어 보드에 표시되지 않음"
	}
	if err := task.Assign(stateDir, as, replace); err != nil {
		return err
	}
	if as.TaskDir != "" {
		if err := board.SetStatus(root, pos[0], "in_progress", now); err != nil {
			fmt.Fprintln(w, "  ⚠ task.md status 갱신 실패:", err)
		}
		if err := board.AppendLog(root, pos[0], "ASSIGN", SessionLabel(a), now); err != nil {
			fmt.Fprintln(w, "  ⚠ log.md 기록 실패:", err)
		}
	}
	fmt.Fprintf(w, "업무 %s → %s %s [%s] 등록. 보고는 %s/pending/ 에 떨어집니다.%s\n",
		as.TaskID, SessionLabel(a), a.Tmux.PaneID, a.State, ShortenHome(abs), warn)
	return nil
```

(`--root`도 `filepath.Abs`로 정규화. import에 `github.com/netwaif/agentlayer/internal/board` 추가.)

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/task/ ./internal/cli/ -count=1`
Expected: PASS(기존 테스트 포함).

- [ ] **Step 5: 커밋**

```bash
git add internal/task/assign.go internal/task/assign_test.go internal/cli/taskcmd.go internal/cli/taskcmd_test.go
git commit -m "feat(task): assign이 회사 task.md를 연결해 in_progress 전이·[ASSIGN] 기록"
```

---

### Task 4: 훅 전이 → task.md·log.md 자동 갱신, 보고 JSON에 task_dir

**Files:**
- Create: `internal/task/board.go`
- Modify: `internal/task/report.go` (`Report.TaskDir`, `ReportFor`에서 채움)
- Modify: `main.go:466-480` (runHook 콜백)
- Test: `internal/task/board_test.go`, `internal/task/report_test.go`(추가)

**Interfaces:**
- Consumes: `board.SetStatus`, `board.AppendLog`; `task.Load`; `state.Agent.Headline()`, `Ask`, `Tmux.Session`.
- Produces:
  ```go
  // ApplyTransition은 등록된 에이전트의 전이를 task.md·log.md에 반영한다. 미등록·TaskDir 없음·해당 없는 전이는 무동작(false).
  func ApplyTransition(stateDir string, a *state.Agent, prev, to state.AgentState, now time.Time) (applied bool, err error)
  ```
  `Report.TaskDir string` (json `task_dir,omitempty`).

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/task/board_test.go
package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

func linkedAgent(t *testing.T) (stateDir, root string, a *state.Agent) {
	t.Helper()
	stateDir, root = t.TempDir(), t.TempDir()
	dir := filepath.Join(root, "tasks", "LAB-1")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "task.md"), []byte("# LAB-1\n```yaml\nstatus: in_progress\n```\n"), 0o644)
	a = &state.Agent{ID: "claude-%1", Kind: "claude", Task: "OK 답하기", Ask: "폴더 밖 읽어도 될까요?",
		Tmux: state.TmuxRef{Session: "collab-bot", WindowName: "t123456", PaneID: "%1"}}
	if err := Assign(stateDir, Assignment{TaskID: "LAB-1", AgentID: a.ID, Session: "collab-bot", Window: "t123456",
		Pane: "%1", Inbox: filepath.Join(root, "runtime", "inbox"), TaskDir: dir, AssignedAt: time.Now()}, false); err != nil {
		t.Fatal(err)
	}
	return
}

func readTask(t *testing.T, root string) (string, string) {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "LAB-1", "task.md"))
	l, _ := os.ReadFile(filepath.Join(root, "tasks", "LAB-1", "log.md"))
	return string(b), string(l)
}

func TestApplyTransitionTable(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		prev, to    state.AgentState
		wantStatus  string // "" = 변경 없음(in_progress 유지)
		wantLog     string // "" = 기록 없음
		wantApplied bool
	}{
		{state.StateWorking, state.StateWaiting, "waiting_collab-bot", "[ASK] 폴더 밖 읽어도 될까요?", true},
		{state.StateWaiting, state.StateWorking, "in_progress", "", true},
		{state.StateIdle, state.StateWorking, "in_progress", "", true},
		{state.StateWorking, state.StateDoneUnread, "reviewing", "[REPORT] DONE: 폴더 밖 읽어도 될까요?", true},
		{state.StateWorking, state.StateError, "", "[ERROR] 폴더 밖 읽어도 될까요?", true},
		{state.StateWorking, state.StateWorking, "", "", false}, // heartbeat
		{state.StateDoneUnread, state.StateIdle, "", "", false},  // 읽음
	}
	for _, c := range cases {
		stateDir, root, a := linkedAgent(t)
		applied, err := ApplyTransition(stateDir, a, c.prev, c.to, now)
		if err != nil {
			t.Fatalf("%s→%s: %v", c.prev, c.to, err)
		}
		if applied != c.wantApplied {
			t.Errorf("%s→%s applied=%v want %v", c.prev, c.to, applied, c.wantApplied)
		}
		task, lg := readTask(t, root)
		wantStatus := c.wantStatus
		if wantStatus == "" {
			wantStatus = "in_progress"
		}
		if !strings.Contains(task, "status: "+wantStatus) {
			t.Errorf("%s→%s status:\n%s", c.prev, c.to, task)
		}
		if c.wantLog == "" && lg != "" {
			t.Errorf("%s→%s 기록이 있으면 안 됨: %s", c.prev, c.to, lg)
		}
		if c.wantLog != "" && !strings.Contains(lg, c.wantLog) {
			t.Errorf("%s→%s log:\n%s", c.prev, c.to, lg)
		}
	}
}

func TestApplyTransitionUnlinkedIsNoop(t *testing.T) {
	stateDir, root, a := linkedAgent(t)
	as, _, _ := Load(stateDir, a.ID)
	as.TaskDir = ""
	_ = Assign(stateDir, *as, true)
	applied, err := ApplyTransition(stateDir, a, state.StateWorking, state.StateDoneUnread, time.Now())
	if applied || err != nil {
		t.Errorf("TaskDir 없으면 무동작: applied=%v err=%v", applied, err)
	}
	if _, lg := readTask(t, root); lg != "" {
		t.Error("기록이 있으면 안 됨")
	}
	other := &state.Agent{ID: "codex-%9", Tmux: state.TmuxRef{Session: "x", PaneID: "%9"}}
	if applied, _ := ApplyTransition(stateDir, other, state.StateWorking, state.StateDoneUnread, time.Now()); applied {
		t.Error("미등록 에이전트는 무동작")
	}
}

func TestApplyTransitionStaleAssignmentIsNoop(t *testing.T) {
	stateDir, _, a := linkedAgent(t)
	a.Tmux.PaneID = "%77" // 재사용된 ID — 등록 당시 pane과 다름
	if applied, _ := ApplyTransition(stateDir, a, state.StateWorking, state.StateDoneUnread, time.Now()); applied {
		t.Error("세션·pane 불일치면 무동작")
	}
}
```

`internal/task/report_test.go` 끝에:

```go
func TestReportCarriesTaskDir(t *testing.T) {
	stateDir, root, a := linkedAgent(t)
	rep, ok := ReportFor(stateDir, a, state.StateWorking, state.StateDoneUnread, time.Now())
	if !ok || rep.TaskDir != filepath.Join(root, "tasks", "LAB-1") {
		t.Errorf("rep=%+v ok=%v", rep, ok)
	}
	b, _ := json.Marshal(rep)
	if !strings.Contains(string(b), `"task_dir"`) {
		t.Errorf("JSON에 task_dir 없음: %s", b)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/task/ -count=1`
Expected: 컴파일 실패(ApplyTransition·TaskDir).

- [ ] **Step 3: 구현**

`internal/task/report.go` — Report에 `TaskDir string \`json:"task_dir,omitempty"\`` 추가(CWD 뒤), `ReportFor`의 `r := &Report{…}`에 `TaskDir: as.TaskDir` 추가.

`internal/task/board.go`:

```go
// internal/task/board.go
package task

import (
	"path/filepath"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/state"
)

// ApplyTransition은 등록된 에이전트의 상태 전이를 회사 task.md(status)·log.md에 반영한다.
//
//	WAIT  → waiting_<세션> + [ASK]
//	WORK  → in_progress (승인됨·새 턴 — 기록 없음)
//	DONE  → reviewing + [REPORT] DONE: …
//	ERR   → status 유지 + [ERROR] …
//
// 미등록·TaskDir 없음·세션/pane 불일치·그 밖의 전이는 무동작(false, nil). 실패해도 에이전트는 막지 않는다(호출자 몫).
func ApplyTransition(stateDir string, a *state.Agent, prev, to state.AgentState, now time.Time) (bool, error) {
	if prev == to {
		return false, nil
	}
	as, ok, err := Load(stateDir, a.ID)
	if err != nil || !ok || as.TaskDir == "" {
		return false, err
	}
	if as.Session != a.Tmux.Session || as.Pane != a.Tmux.PaneID {
		return false, nil
	}
	root, id := filepath.Dir(filepath.Dir(as.TaskDir)), filepath.Base(as.TaskDir)
	status, tag := "", ""
	switch to {
	case state.StateWaiting:
		status, tag = "waiting_"+a.Tmux.Session, "ASK"
	case state.StateWorking:
		status = "in_progress"
	case state.StateDoneUnread:
		status, tag = "reviewing", "REPORT"
	case state.StateError:
		tag = "ERROR"
	default:
		return false, nil
	}
	if status != "" {
		if err := board.SetStatus(root, id, status, now); err != nil {
			return true, err
		}
	}
	if tag != "" {
		text := a.Headline()
		if tag == "REPORT" {
			text = "DONE: " + text
		}
		if err := board.AppendLog(root, id, tag, text, now); err != nil {
			return true, err
		}
	}
	return true, nil
}
```

`main.go` runHook 콜백 — `task.ReportFor` 블록 **앞**에 추가(보고보다 파일 갱신이 먼저여야 총괄이 READY·DONE을 받았을 때 task.md가 이미 새 상태다):

```go
		// 회사 보드: 등록된 업무면 task.md status·log.md를 먼저 갱신(2초 상한, 실패는 stderr만)
		bdone := make(chan error, 1)
		go func() { _, err := task.ApplyTransition(st.Dir, a, prev, to, time.Now()); bdone <- err }()
		select {
		case err := <-bdone:
			if err != nil {
				fmt.Fprintln(os.Stderr, "agentlayer board:", err)
			}
		case <-time.After(2 * time.Second):
			fmt.Fprintln(os.Stderr, "agentlayer board: 2초 안에 task.md를 못 썼습니다")
		}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/task/ -count=1 && go build ./... && go vet ./...`
Expected: PASS, 빌드 OK.

- [ ] **Step 5: 커밋**

```bash
git add internal/task/board.go internal/task/board_test.go internal/task/report.go internal/task/report_test.go main.go
git commit -m "feat(task): 훅 전이를 task.md status·log.md에 자동 반영, 보고에 task_dir"
```

---

### Task 5: `send` — 등록 세션에 보낸 메시지를 [SEND]로 기록

**Files:**
- Modify: `internal/cli/sendcmd.go` (`RunSend` 시그니처에 `stateDir string` 추가, 전송 성공 뒤 기록)
- Modify: `main.go` (`send` 라우팅에 `state.DefaultDir()` 전달)
- Test: `internal/cli/sendcmd_test.go`(기존 호출부 시그니처 갱신 + 추가)

**Interfaces:**
- Consumes: `task.List(stateDir)`, `board.AppendLog`.
- Produces: `func RunSend(w io.Writer, stdin io.Reader, st *state.Store, stateDir string, tm TextSender, args []string) error`, `func LogExcerpt(msg string) string`(1000 rune 절단·개행 ⏎).

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/cli/sendcmd_test.go` — 기존 `RunSend(` 호출 전부에 `t.TempDir()`를 st 다음 인자로 끼워 넣고, 끝에 추가:

```go
func TestLogExcerpt(t *testing.T) {
	if got := LogExcerpt("a\nb"); got != "a⏎b (3자)" {
		t.Errorf("got %q", got)
	}
	long := strings.Repeat("가", 1001)
	got := LogExcerpt(long)
	if !strings.HasPrefix(got, strings.Repeat("가", 1000)+"…") || !strings.HasSuffix(got, "(1001자)") {
		t.Errorf("절단 실패: len=%d tail=%q", len([]rune(got)), got[len(got)-12:])
	}
}

func TestRunSendLogsToLinkedTask(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	a := &state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}}
	_ = st.Put(a)
	root := t.TempDir()
	dir := filepath.Join(root, "tasks", "LAB-1")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "task.md"), []byte("# LAB-1\n```yaml\nstatus: in_progress\n```\n"), 0o644)
	_ = task.Assign(stateDir, task.Assignment{TaskID: "LAB-1", AgentID: a.ID, Session: "collab-bot", Pane: "%1",
		Inbox: filepath.Join(root, "runtime", "inbox"), TaskDir: dir, AssignedAt: time.Now()}, false)
	var out bytes.Buffer
	fake := &fakeSender{} // 기존 테스트 파일의 페이크 이름을 확인해 맞춘다
	if err := RunSend(&out, nil, st, stateDir, fake, []string{"collab-bot", "네, 읽어도 됩니다"}); err != nil {
		t.Fatal(err)
	}
	lg, _ := os.ReadFile(filepath.Join(dir, "log.md"))
	if !strings.Contains(string(lg), "[SEND] 네, 읽어도 됩니다 (10자)") {
		t.Errorf("log.md:\n%s", lg)
	}
}

func TestRunSendUnlinkedWritesNoLog(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Put(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}})
	var out bytes.Buffer
	if err := RunSend(&out, nil, st, stateDir, &fakeSender{}, []string{"collab-bot", "hi"}); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(stateDir); len(entries) > 1 { // agents/ 외에 아무것도 안 생김
		t.Errorf("stateDir에 잔재: %v", entries)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/cli/ -run 'Send|LogExcerpt' -count=1`
Expected: 컴파일 실패.

- [ ] **Step 3: 구현**

`internal/cli/sendcmd.go`:

```go
// maxLogRunes — log.md에 남기는 send 본문 상한. 넘으면 절단 + …
const maxLogRunes = 1000

// LogExcerpt는 log.md 한 줄용 발췌: 개행 ⏎, 1000자 절단, 끝에 "(n자)".
func LogExcerpt(msg string) string {
	n := len([]rune(msg))
	flat := strings.ReplaceAll(msg, "\n", "⏎")
	if r := []rune(flat); len(r) > maxLogRunes {
		flat = string(r[:maxLogRunes]) + "…"
	}
	return fmt.Sprintf("%s (%d자)", flat, n)
}
```

`RunSend`에 `stateDir string` 인자(st 다음) 추가. `tm.SendText` 성공 직후:

```go
	// 회사 업무가 등록된 세션이면 총괄의 지시·답변을 log.md에 남긴다([ASK] 뒤의 [SEND]가 Q&A 한 쌍).
	if as, ok, _ := task.Load(stateDir, a.ID); ok && as.TaskDir != "" && as.Session == a.Tmux.Session && as.Pane == a.Tmux.PaneID {
		root, id := filepath.Dir(filepath.Dir(as.TaskDir)), filepath.Base(as.TaskDir)
		if err := board.AppendLog(root, id, "SEND", LogExcerpt(message), time.Now()); err != nil {
			fmt.Fprintln(w, "  ⚠ log.md 기록 실패:", err)
		}
	}
```

(import: `path/filepath`, `time`, `internal/board`, `internal/task`.) `main.go`의 `send` 케이스: `cli.RunSend(os.Stdout, os.Stdin, st, state.DefaultDir(), tmuxx.Tmux{}, args[1:])`. `internal/cli/wtcmd.go` 등 다른 호출부가 있으면 `grep -rn "RunSend(" --include=*.go`로 찾아 같이 고친다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/cli/ -count=1 && go build ./...`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
git add internal/cli/sendcmd.go internal/cli/sendcmd_test.go main.go
git commit -m "feat(send): 등록 세션에 보낸 지시를 tasks/<ID>/log.md에 [SEND]로 기록"
```

---

### Task 6: `task done` — status done, [COMPLETE], 자식 READY 이벤트

**Files:**
- Modify: `internal/task/report.go` (`ReadyReport`)
- Create/Modify: `internal/task/board.go` (`MarkDone`, `ReadyChildren`)
- Modify: `internal/cli/taskcmd.go` (`done` 분기, `--root`)
- Test: `internal/task/board_test.go`(추가), `internal/cli/taskcmd_test.go`(추가)

**Interfaces:**
- Consumes: `board.Load`, `board.Children`, `board.SetStatus`, `board.AppendLog`, `WriteReport`, `NewID`.
- Produces:
  ```go
  // MarkDone: task.md status=done + [COMPLETE], 그 뒤 부모가 전부 done이 된 자식마다 READY 보고를 inbox에 쓴다. 쓴 자식 ID 목록 반환.
  func MarkDone(root, id, inbox string, now time.Time) ([]string, error)
  func ReadyReport(child board.Card, root string, now time.Time) *Report
  ```
  CLI: `agentlayer task done <업무ID> [--root <회사루트>]`.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/task/board_test.go` 끝에:

```go
func writeCompanyTask(t *testing.T, root, id, status, parents string) {
	t.Helper()
	dir := filepath.Join(root, "tasks", id)
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "task.md"),
		[]byte("# "+id+" 제목\n```yaml\nstatus: "+status+"\nparents: "+parents+"\n```\n"), 0o644)
}

func TestMarkDoneEmitsReadyOnlyWhenAllParentsDone(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "runtime", "inbox")
	writeCompanyTask(t, root, "A", "reviewing", "[]")
	writeCompanyTask(t, root, "B", "in_progress", "[]")
	writeCompanyTask(t, root, "C", "pending", "[A]")     // A만 부모 → READY
	writeCompanyTask(t, root, "D", "pending", "[A, B]")  // B 미완 → 안 씀
	writeCompanyTask(t, root, "E", "in_progress", "[A]") // 이미 진행 중 → 안 씀
	now := time.Now()
	ready, err := MarkDone(root, "A", inbox, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 || ready[0] != "C" {
		t.Errorf("ready = %v, want [C]", ready)
	}
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "A", "task.md"))
	if !strings.Contains(string(b), "status: done") {
		t.Errorf("A status:\n%s", b)
	}
	lg, _ := os.ReadFile(filepath.Join(root, "tasks", "A", "log.md"))
	if !strings.Contains(string(lg), "[COMPLETE]") {
		t.Errorf("A log:\n%s", lg)
	}
	rep, ok, err := Poll(inbox)
	if err != nil || !ok {
		t.Fatalf("READY 이벤트 없음: ok=%v err=%v", ok, err)
	}
	if rep.Kind != "board" || rep.To != "READY" || rep.From != "pending" || rep.TaskID != "C" ||
		rep.Task != "C 제목" || rep.TaskDir != filepath.Join(root, "tasks", "C") {
		t.Errorf("rep = %+v", rep)
	}
	if _, ok, _ := Poll(inbox); ok {
		t.Error("이벤트는 하나여야 함")
	}
	// B까지 끝나면 D가 READY
	ready, _ = MarkDone(root, "B", inbox, now)
	if len(ready) != 1 || ready[0] != "D" {
		t.Errorf("ready = %v, want [D]", ready)
	}
}

func TestMarkDoneWithoutTaskFileFails(t *testing.T) {
	if _, err := MarkDone(t.TempDir(), "NOPE", "", time.Now()); err == nil {
		t.Error("task.md 없으면 에러")
	}
}
```

`internal/cli/taskcmd_test.go` 끝에:

```go
func TestTaskDoneMarksCompanyTask(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Put(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}})
	root := companyRoot(t, "LAB-1")
	inbox := filepath.Join(root, "runtime", "inbox")
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"assign", "LAB-1", "collab-bot", "--inbox", inbox}, time.Now()); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"done", "LAB-1"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "LAB-1", "task.md"))
	if !strings.Contains(string(b), "status: done") {
		t.Errorf("status:\n%s", b)
	}
	if list, _ := task.List(stateDir); len(list) != 0 {
		t.Error("등록이 해제돼야 함")
	}
}

func TestTaskDoneGoneNeedsRoot(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	root := companyRoot(t, "LAB-1")
	var out bytes.Buffer
	err := RunTask(context.Background(), &out, st, stateDir, []string{"done", "LAB-1"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "--root") {
		t.Errorf("등록 없으면 --root 안내: %v", err)
	}
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"done", "LAB-1", "--root", root}, time.Now()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "LAB-1", "task.md"))
	if !strings.Contains(string(b), "status: done") {
		t.Errorf("--root 경로로 done 실패:\n%s", b)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/task/ ./internal/cli/ -run 'MarkDone|TaskDone' -count=1`
Expected: 컴파일 실패.

- [ ] **Step 3: 구현**

`internal/task/report.go` 끝에:

```go
// ReadyReport는 부모가 전부 끝나 배정 가능해진 자식 업무를 총괄에게 알리는 이벤트.
// 기존 watch가 그대로 흘려보낸다(version·id·task_id·to만 검사).
func ReadyReport(child board.Card, root string, now time.Time) *Report {
	return &Report{Version: 1, ID: NewID(), TaskID: child.ID, Kind: "board", From: "pending", To: "READY",
		Task: child.Title, TaskDir: board.TaskDir(root, child.ID), At: now}
}
```

(import `internal/board`.)

`internal/task/board.go` 끝에:

```go
// MarkDone은 업무를 done으로 닫고([COMPLETE]), 그 결과 부모가 전부 done이 된 pending 자식마다
// inbox에 READY 이벤트를 쓴다. inbox가 비면 이벤트는 쓰지 않는다. 돌려주는 값은 이벤트를 쓴 자식 ID들.
func MarkDone(root, id, inbox string, now time.Time) ([]string, error) {
	if err := board.SetStatus(root, id, "done", now); err != nil {
		return nil, err
	}
	if err := board.AppendLog(root, id, "COMPLETE", "", now); err != nil {
		return nil, err
	}
	cards, err := board.Load(root, nil, nil, now)
	if err != nil {
		return nil, err
	}
	var ready []string
	for _, c := range board.Children(cards, id) {
		if c.Column != board.ColReady {
			continue
		}
		ready = append(ready, c.ID)
		if inbox == "" {
			continue
		}
		r := ReadyReport(c, root, now)
		r.Inbox = inbox
		if _, err := WriteReport(r); err != nil {
			return ready, err
		}
	}
	return ready, nil
}
```

`internal/cli/taskcmd.go` `done` 분기를 함수로:

```go
	case "done":
		return taskDone(w, stateDir, args[1:], now)
```

```go
func taskDone(w io.Writer, stateDir string, args []string, now time.Time) error {
	var pos []string
	root := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--root" {
			if i+1 >= len(args) {
				return errors.New("--root 뒤에 회사 루트가 필요합니다")
			}
			root = args[i+1]
			i++
			continue
		}
		pos = append(pos, args[i])
	}
	if len(pos) != 1 {
		return errors.New(taskUsage)
	}
	id := pos[0]
	// 등록에서 루트·inbox를 얻는다(있으면). 없으면 --root가 있어야 보드를 닫을 수 있다.
	inbox := ""
	list, err := task.List(stateDir)
	if err != nil {
		return err
	}
	for _, as := range list {
		if as.TaskID == id {
			inbox = as.Inbox
			if root == "" && as.TaskDir != "" {
				root = filepath.Dir(filepath.Dir(as.TaskDir))
			}
		}
	}
	found, err := task.Done(stateDir, id)
	if err != nil {
		return err
	}
	if root == "" {
		if found {
			fmt.Fprintf(w, "업무 %s 등록 해제 (보드 연결 없음)\n", id)
			return nil
		}
		return fmt.Errorf("업무 %q이 등록돼 있지 않습니다 — 보드만 닫으려면 --root <회사루트>", id)
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if inbox == "" {
		inbox = filepath.Join(root, "runtime", "inbox")
	}
	ready, err := task.MarkDone(root, id, inbox, now)
	if err != nil {
		return err
	}
	if found {
		fmt.Fprintf(w, "업무 %s 등록 해제 · task.md done\n", id)
	} else {
		fmt.Fprintf(w, "업무 %s task.md done (등록은 없었음)\n", id)
	}
	for _, c := range ready {
		fmt.Fprintf(w, "  → %s 배정 가능(READY 이벤트 전송)\n", c)
	}
	return nil
}
```

`taskUsage`의 done 줄: `agentlayer task done <업무ID> [--root <회사루트>]`.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/task/ ./internal/cli/ -count=1`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
git add internal/task/board.go internal/task/board_test.go internal/task/report.go internal/cli/taskcmd.go internal/cli/taskcmd_test.go
git commit -m "feat(task): done이 task.md를 닫고 부모 완료된 자식에 READY 이벤트"
```

---

### Task 7: 디스코드 카드 — 업무 보드 컨테이너

**Files:**
- Modify: `internal/discord/card.go` (`CardData.Board`, `boardContainer`, `BuildCard`)
- Modify: `main.go` publishCard(`Board` 채우기)
- Test: `internal/discord/card_test.go`(추가)

**Interfaces:**
- Consumes: `board.Card`, `board.Counts`, `board.StaleReady`, `board.Columns`.
- Produces:
  ```go
  type BoardData struct { Name string; Cards []board.Card; StaleLimit time.Duration }
  CardData.Board *BoardData   // nil이면 컨테이너 생략
  func boardContainer(b *BoardData, now time.Time) map[string]any   // nil 반환 = 생략
  ```

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/discord/card_test.go` 끝에(기존 픽스처 `t0` 재사용):

```go
func boardFixture() *BoardData {
	return &BoardData{Name: "AI 치트키 회사", StaleLimit: 30 * time.Minute, Cards: []board.Card{
		{ID: "VIDEO-07-TOPICS", Column: board.ColBlocked, Session: "search-youtube-bot:t170966", State: "WAIT",
			Updated: t0.Add(-45 * time.Minute), LastLog: "[2026-08-25 11:15] [ASK] 참고자료 폴더 밖을 읽어도 될까요?"},
		{ID: "VIDEO-07-SCRIPT", Column: board.ColRunning, Session: "collab-bot", State: "WORK", Updated: t0.Add(-3 * time.Minute)},
		{ID: "VIDEO-07-THUMB", Column: board.ColReady, Parents: []string{"VIDEO-07-TOPICS"}, Ready: t0.Add(-41 * time.Minute)},
		{ID: "VIDEO-06", Column: board.ColDone, Updated: t0.Add(-time.Hour)},
		{ID: "VIDEO-08", Column: board.ColTodo, Parents: []string{"VIDEO-07-THUMB"}},
		{ID: "VIDEO-07-REVIEW", Column: board.ColReview, Session: "sendmanual-bot", State: "DONE", Updated: t0.Add(-time.Minute)},
	}}
}

func TestBoardContainerOrderCountsAndStale(t *testing.T) {
	c := boardContainer(boardFixture(), t0)
	if c == nil {
		t.Fatal("컨테이너 없음")
	}
	txt := containerText(c) // 기존 테스트 헬퍼가 있으면 재사용, 없으면 comps의 content를 이어 붙이는 헬퍼를 이 파일에 추가
	for _, want := range []string{
		"### 업무 보드 — AI 치트키 회사",
		"ready 1 · running 1 · blocked 1 · review 1 · todo 1 · done 1",
		"VIDEO-07-TOPICS", "search-youtube-bot:t170966", "WAIT", "⚠",
		"[ASK] 참고자료 폴더 밖을 읽어도 될까요?",
		"VIDEO-07-THUMB", "← VIDEO-07-TOPICS",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("보드 텍스트에 %q 없음:\n%s", want, txt)
		}
	}
	// 정렬: blocked → review → running → ready
	iB, iRv, iRn, iRd := strings.Index(txt, "VIDEO-07-TOPICS"), strings.Index(txt, "VIDEO-07-REVIEW"),
		strings.Index(txt, "VIDEO-07-SCRIPT"), strings.Index(txt, "VIDEO-07-THUMB")
	if !(iB < iRv && iRv < iRn && iRn < iRd) {
		t.Errorf("정렬 어긋남: %d %d %d %d", iB, iRv, iRn, iRd)
	}
	// todo·done은 행으로 안 나옴
	if strings.Contains(txt, "VIDEO-06 ") || strings.Contains(txt, "VIDEO-08") {
		t.Error("todo·done 카드는 집계에만")
	}
	// ⚠는 blocked 45분·ready 41분에만, running 3분에는 없음
	line := lineContaining(txt, "VIDEO-07-SCRIPT")
	if strings.Contains(line, "⚠") {
		t.Errorf("running에 ⚠: %s", line)
	}
}

func TestBoardContainerCapsAtEightRows(t *testing.T) {
	b := &BoardData{Name: "X", StaleLimit: time.Hour}
	for i := 0; i < 11; i++ {
		b.Cards = append(b.Cards, board.Card{ID: fmt.Sprintf("T-%02d", i), Column: board.ColRunning, Updated: t0})
	}
	txt := containerText(boardContainer(b, t0))
	if strings.Count(txt, "T-") != 8 || !strings.Contains(txt, "외 3") {
		t.Errorf("8행 상한 위반:\n%s", txt)
	}
}

func TestBuildCardOmitsBoardWhenNil(t *testing.T) {
	d := fixtureData()
	d.Board = nil
	if s := fmt.Sprint(BuildCard(d, t0)); strings.Contains(s, "업무 보드") {
		t.Error("Board nil이면 컨테이너 없음")
	}
	d.Board = &BoardData{Name: "빈 회사"}
	if s := fmt.Sprint(BuildCard(d, t0)); strings.Contains(s, "업무 보드") {
		t.Error("카드 0장이면 컨테이너 없음")
	}
	d.Board = boardFixture()
	if s := fmt.Sprint(BuildCard(d, t0)); !strings.Contains(s, "업무 보드") {
		t.Error("카드가 있으면 컨테이너 있음")
	}
}
```

`containerText`·`lineContaining` 헬퍼가 없으면 테스트 파일에 추가:

```go
func containerText(c map[string]any) string {
	var sb strings.Builder
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if s, ok := x["content"].(string); ok {
				sb.WriteString(s + "\n")
			}
			if comps, ok := x["components"].([]any); ok {
				for _, c := range comps {
					walk(c)
				}
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(c)
	return sb.String()
}

func lineContaining(txt, sub string) string {
	for _, l := range strings.Split(txt, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}
```

(agentsContainer가 만드는 map 구조 — `components` 배열 안의 `{"type": typeText, "content": …}` — 를 `card.go:260-300`에서 확인해 헬퍼를 맞춘다.)

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/discord/ -run Board -count=1`
Expected: 컴파일 실패.

- [ ] **Step 3: 구현**

`internal/discord/card.go`:

```go
// BoardData는 업무 보드 절 재료. nil이면 절 생략(회사가 없는 사용자에겐 아무것도 안 보임).
type BoardData struct {
	Name       string
	Cards      []board.Card
	StaleLimit time.Duration
}
```

`CardData`에 `Board *BoardData` 필드(Tasks 뒤). `BuildCard`에서 `agentsContainer` 뒤:

```go
	if bc := boardContainer(d.Board, now); bc != nil {
		comps = append(comps, bc)
	}
```

```go
// 열 → 행 순서와 이모지. blocked가 맨 위(사람이 풀어야 함), 그다음 review(총괄 검토), running, ready(배정 대기).
var boardRowOrder = []string{board.ColBlocked, board.ColReview, board.ColRunning, board.ColReady}
var boardEmoji = map[string]string{board.ColBlocked: "🟡", board.ColReview: "🔵", board.ColRunning: "🟢", board.ColReady: "⚪"}

const boardMaxRows = 8

// boardContainer는 업무 보드 절: 집계 한 줄 + 행(blocked→review→running→ready, 최대 8). 카드 0장이면 nil.
func boardContainer(b *BoardData, now time.Time) map[string]any {
	if b == nil || len(b.Cards) == 0 {
		return nil
	}
	cnt := board.Counts(b.Cards)
	head := "### 업무 보드 — " + b.Name + "\n-# " + fmt.Sprintf("ready %d · running %d · blocked %d · review %d · todo %d · done %d",
		cnt[board.ColReady], cnt[board.ColRunning], cnt[board.ColBlocked], cnt[board.ColReview], cnt[board.ColTodo], cnt[board.ColDone])
	var rows []board.Card
	for _, col := range boardRowOrder {
		for _, c := range b.Cards {
			if c.Column == col {
				rows = append(rows, c)
			}
		}
	}
	lines := []string{head}
	shown := 0
	for _, c := range rows {
		if shown == boardMaxRows {
			lines = append(lines, fmt.Sprintf("-# 외 %d", len(rows)-shown))
			break
		}
		shown++
		line := boardEmoji[c.Column] + " **" + c.ID + "**"
		if c.Session != "" {
			line += "  " + c.Session
		}
		switch c.Column {
		case board.ColReady:
			line += "  (ready " + since(c.Ready, now)
			if board.StaleReady(c, now, b.StaleLimit) {
				line += " ⚠"
			}
			line += ")"
		default:
			if c.State != "" {
				line += "  " + c.State + " " + since(c.Updated, now)
			}
			if board.StaleReady(c, now, b.StaleLimit) {
				line += "  ⚠"
			}
		}
		if len(c.Parents) > 0 {
			line += "  ← " + strings.Join(c.Parents, ", ")
		}
		lines = append(lines, line)
		if c.LastLog != "" {
			// "[시각] [TAG] 내용"에서 시각은 뺀다(카드 갱신 시각이 따로 있다)
			l := c.LastLog
			if i := strings.Index(l, "] ["); i > 0 {
				l = l[i+2:]
			}
			lines = append(lines, "-# "+truncateRunes(l, 60))
		}
	}
	return map[string]any{"type": typeContainer, "accent_color": accentInt("#5865F2"),
		"components": []any{map[string]any{"type": typeText, "content": strings.Join(lines, "\n")}}}
}
```

(`typeContainer`·`accentInt`·`since`·`truncateRunes`의 실제 이름은 `agentsContainer`의 반환부(`card.go:260-300`)를 보고 같은 것을 쓴다. accent 색은 AgentLoops 하우스 팔레트에서 이미 카드가 쓰는 색 중 하나를 고른다 — 새 색 도입 금지.)

`main.go` publishCard — `build` 클로저 앞에:

```go
	// 업무 보드: 설정 company_root 또는 등록된 업무의 inbox에서 루트를 유추
	var boardData *discord.BoardData
	if links, err := task.List(st.Dir); err == nil {
		var inboxes []string
		var bl []board.Link
		for _, as := range links {
			inboxes = append(inboxes, as.Inbox)
			bl = append(bl, board.Link{TaskID: as.TaskID, Session: as.Session, Window: as.Window, AgentID: as.AgentID})
		}
		if root := board.Root(cfg.CompanyRoot, inboxes); root != "" {
			states := map[string]string{}
			for _, a := range agents {
				states[a.ID] = cli.StateWord(a.State)
			}
			if cards, err := board.Load(root, bl, states, now); err == nil {
				boardData = &discord.BoardData{Name: board.CompanyName(root), Cards: cards, StaleLimit: cfg.BoardStaleLimit()}
			}
		}
	}
```

`cfg := config.Load()`를 이 블록보다 위로 올린다(이미 `wired` 계산에 `config.Load()`를 쓰고 있으니 변수 하나로 합친다). `CardData{…, Board: boardData}`. `cli.StateWord`는 `taskcmd.go`의 `stateWord`를 export한 것(`func StateWord(s state.AgentState) string`) — 이름을 바꾸고 기존 호출부(`taskList`)도 고친다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/discord/ ./internal/cli/ -count=1 && go build ./... && ./agentlayer card --out | head -80`
Expected: PASS. `card --out` JSON에 업무 보드 컨테이너가 있거나(회사 등록 있음), 없거나(등록 없음) — 오류 없이 출력.

- [ ] **Step 5: 커밋**

```bash
git add internal/discord/card.go internal/discord/card_test.go internal/cli/taskcmd.go main.go
git commit -m "feat(card): 업무 보드 컨테이너 — 집계·blocked/review/running/ready 행·⚠"
```

---

### Task 8: `agentlayer board` — HTML 생성·전용 브라우저 열기·--json

**Files:**
- Create: `internal/board/html.go`, `internal/cli/boardcmd.go`
- Modify: `internal/cli/helpcmd.go`, `internal/cli/helpcmd_test.go`, `main.go`(라우팅), `internal/cli/browsercmd.go`(`browserOpen`을 재사용할 수 있게 `OpenInBrowser(url string) error` export)
- Test: `internal/board/html_test.go`, `internal/cli/boardcmd_test.go`

**Interfaces:**
- Consumes: `board.Load`, `board.Root`, `board.CompanyName`, `board.StaleReady`, `browser.Connect`.
- Produces: `func HTML(name string, cards []Card, now time.Time, stale time.Duration) []byte`; `func RunBoard(w io.Writer, st *state.Store, stateDir string, cfg *config.Config, open func(url string) error, args []string, now time.Time) error`; CLI `agentlayer board [--out <경로>] [--json] [--no-open]`.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
// internal/board/html_test.go
package board

import (
	"strings"
	"testing"
	"time"
)

func TestHTMLHasSixColumnsCardsAndEscapes(t *testing.T) {
	cards := []Card{
		{ID: "A", Title: "<script>alert(1)</script>", Column: ColReady, Ready: now.Add(-time.Hour), Parents: []string{"Z"}},
		{ID: "B", Title: "실행 중", Column: ColRunning, Session: "collab-bot", State: "WORK", Updated: now, LastLog: "[..] [SEND] 지시 & 답"},
		{ID: "C", Title: "이상", Column: ColTodo, Unknown: true},
	}
	h := string(HTML("AI 치트키 회사", cards, now, 30*time.Minute))
	for _, want := range []string{"<!doctype html>", "AI 치트키 회사", "&lt;script&gt;", "&amp; 답",
		`class="col" data-col="todo"`, `data-col="ready"`, `data-col="running"`, `data-col="blocked"`, `data-col="review"`, `data-col="done"`,
		"collab-bot", "WORK", "← Z", "⚠", "?"} {
		if !strings.Contains(h, want) {
			t.Errorf("HTML에 %q 없음", want)
		}
	}
	if strings.Contains(h, "<script>alert") {
		t.Error("이스케이프 실패")
	}
	if strings.Contains(h, "<script") {
		t.Error("자바스크립트 없음(정적 페이지)")
	}
}

func TestHTMLEmptyBoardSaysSo(t *testing.T) {
	h := string(HTML("빈 회사", nil, now, time.Minute))
	if !strings.Contains(h, "업무 없음") {
		t.Error("빈 보드 안내 없음")
	}
}
```

```go
// internal/cli/boardcmd_test.go
package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

func TestRunBoardJSONAndOut(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Put(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateWorking,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}})
	root := companyRoot(t, "LAB-1")
	_ = task.Assign(stateDir, task.Assignment{TaskID: "LAB-1", AgentID: "claude-%1", Session: "collab-bot", Pane: "%1",
		Inbox: filepath.Join(root, "runtime", "inbox"), TaskDir: filepath.Join(root, "tasks", "LAB-1"), AssignedAt: time.Now()}, false)
	var out bytes.Buffer
	opened := ""
	open := func(u string) error { opened = u; return nil }
	if err := RunBoard(&out, st, stateDir, &config.Config{}, open, []string{"--json"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	var cards []map[string]any
	if err := json.Unmarshal(out.Bytes(), &cards); err != nil || len(cards) != 1 || cards[0]["ID"] != "LAB-1" || cards[0]["State"] != "WORK" {
		t.Errorf("json = %s err=%v", out.String(), err)
	}
	if opened != "" {
		t.Error("--json은 브라우저를 열지 않음")
	}
	outPath := filepath.Join(t.TempDir(), "b.html")
	out.Reset()
	if err := RunBoard(&out, st, stateDir, &config.Config{}, open, []string{"--out", outPath}, time.Now()); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(outPath)
	if err != nil || !strings.Contains(string(b), "LAB-1") {
		t.Errorf("--out 파일: %v", err)
	}
	if opened != "" {
		t.Error("--out은 브라우저를 열지 않음")
	}
	out.Reset()
	if err := RunBoard(&out, st, stateDir, &config.Config{}, open, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(opened, "file://") || !strings.HasSuffix(opened, "/board.html") {
		t.Errorf("기본은 <state>/board.html을 브라우저로: %q", opened)
	}
}

func TestRunBoardWithoutCompanyExplains(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	var out bytes.Buffer
	err := RunBoard(&out, st, stateDir, &config.Config{}, func(string) error { return nil }, nil, time.Now())
	if err == nil || !strings.Contains(err.Error(), "company_root") {
		t.Errorf("회사 루트를 못 찾으면 company_root 안내: %v", err)
	}
}
```

`helpcmd_test.go`의 `TestHelpTextListsAllCommands` 목록에 `"board"` 추가.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/board/ ./internal/cli/ -run 'HTML|Board|HelpText' -count=1`
Expected: 컴파일 실패 / help FAIL.

- [ ] **Step 3: 구현**

`internal/board/html.go`:

```go
// internal/board/html.go
package board

import (
	"fmt"
	"html"
	"strings"
	"time"
)

var colTitle = map[string]string{ColTodo: "Todo", ColReady: "Ready", ColRunning: "Running", ColBlocked: "Blocked", ColReview: "Review", ColDone: "Done"}

// HTML은 정적 보드 페이지. 자바스크립트 없음 — 새로고침은 `agentlayer board` 재실행.
// 색은 AgentLoops 하우스 팔레트(카드·TUI와 같은 계열): 바탕 #0B0D12, 패널 #151923, 글자 #E6E8EE, 흐림 #8B93A7, 강조 #5865F2, 경고 #F0B232.
func HTML(name string, cards []Card, now time.Time, stale time.Duration) []byte {
	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html lang=\"ko\"><head><meta charset=\"utf-8\"><title>")
	sb.WriteString(html.EscapeString(name))
	sb.WriteString(" — 업무 보드</title><style>\n")
	sb.WriteString(`body{margin:0;background:#0B0D12;color:#E6E8EE;font:14px/1.5 -apple-system,"Apple SD Gothic Neo",sans-serif;padding:24px}
h1{font-size:18px;margin:0 0 4px}.meta{color:#8B93A7;font-size:12px;margin-bottom:20px}
.board{display:grid;grid-template-columns:repeat(6,minmax(180px,1fr));gap:12px;overflow-x:auto}
.col{background:#151923;border-radius:10px;padding:10px;min-height:120px}.col h2{font-size:12px;color:#8B93A7;letter-spacing:.06em;text-transform:uppercase;margin:0 0 10px}
.card{background:#0B0D12;border:1px solid #232838;border-radius:8px;padding:8px 10px;margin-bottom:8px}.card .id{font-weight:600}.card .t{color:#C7CCD8}
.card .s{color:#8B93A7;font-size:12px}.card .log{color:#8B93A7;font-size:12px;margin-top:4px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.warn{color:#F0B232}.empty{color:#8B93A7;padding:40px 0}
</style></head><body>`)
	fmt.Fprintf(&sb, "<h1>%s — 업무 보드</h1><div class=\"meta\">%s 생성 · 새로고침은 <code>agentlayer board</code></div>",
		html.EscapeString(name), now.Format("2006-01-02 15:04"))
	if len(cards) == 0 {
		sb.WriteString("<div class=\"empty\">업무 없음 — tasks/&lt;ID&gt;/task.md가 없습니다.</div></body></html>\n")
		return []byte(sb.String())
	}
	sb.WriteString("<div class=\"board\">")
	for _, col := range Columns {
		fmt.Fprintf(&sb, "<section class=\"col\" data-col=\"%s\"><h2>%s</h2>", col, colTitle[col])
		for _, c := range cards {
			if c.Column != col {
				continue
			}
			sb.WriteString("<div class=\"card\"><div class=\"id\">" + html.EscapeString(c.ID))
			if c.Unknown {
				sb.WriteString(" <span class=\"warn\" title=\"status 값을 해석하지 못함\">?</span>")
			}
			if StaleReady(c, now, stale) {
				sb.WriteString(" <span class=\"warn\">⚠</span>")
			}
			sb.WriteString("</div>")
			if c.Title != "" && c.Title != c.ID {
				sb.WriteString("<div class=\"t\">" + html.EscapeString(c.Title) + "</div>")
			}
			var s []string
			if c.Session != "" {
				s = append(s, html.EscapeString(c.Session))
			}
			if c.State != "" {
				s = append(s, c.State+" "+ago(c.Updated, now))
			} else if col == ColReady {
				s = append(s, "ready "+ago(c.Ready, now))
			}
			if len(c.Parents) > 0 {
				s = append(s, "← "+html.EscapeString(strings.Join(c.Parents, ", ")))
			}
			if len(s) > 0 {
				sb.WriteString("<div class=\"s\">" + strings.Join(s, " · ") + "</div>")
			}
			if c.LastLog != "" {
				sb.WriteString("<div class=\"log\" title=\"" + html.EscapeString(c.LastLog) + "\">" + html.EscapeString(c.LastLog) + "</div>")
			}
			sb.WriteString("</div>")
		}
		sb.WriteString("</section>")
	}
	sb.WriteString("</div></body></html>\n")
	return []byte(sb.String())
}

func ago(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "방금"
	case d < time.Hour:
		return fmt.Sprintf("%d분", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d시간", int(d.Hours()))
	}
	return fmt.Sprintf("%d일", int(d.Hours()/24))
}
```

`internal/cli/boardcmd.go`:

```go
// internal/cli/boardcmd.go
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

const boardUsage = "사용법: agentlayer board [--out <경로>] [--json] [--no-open]"

// LoadBoard는 회사 루트를 찾아 카드를 읽는다. 루트를 못 찾으면 ("", nil, nil).
func LoadBoard(st *state.Store, stateDir string, cfg *config.Config, now time.Time) (root string, cards []board.Card, err error) {
	links, err := task.List(stateDir)
	if err != nil {
		return "", nil, err
	}
	var inboxes []string
	var bl []board.Link
	for _, as := range links {
		inboxes = append(inboxes, as.Inbox)
		bl = append(bl, board.Link{TaskID: as.TaskID, Session: as.Session, Window: as.Window, AgentID: as.AgentID})
	}
	root = board.Root(cfg.CompanyRoot, inboxes)
	if root == "" {
		return "", nil, nil
	}
	agents, err := st.List()
	if err != nil {
		return "", nil, err
	}
	states := map[string]string{}
	for _, a := range agents {
		states[a.ID] = StateWord(a.State)
	}
	cards, err = board.Load(root, bl, states, now)
	return root, cards, err
}

// RunBoard: agentlayer board — HTML을 <state>/board.html에 쓰고 전용 브라우저로 연다.
// --out은 파일만, --json은 카드 배열만(stdout), --no-open은 파일만 쓰고 경로 출력.
func RunBoard(w io.Writer, st *state.Store, stateDir string, cfg *config.Config, open func(url string) error, args []string, now time.Time) error {
	out, asJSON, noOpen := "", false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--no-open":
			noOpen = true
		case "--out":
			if i+1 >= len(args) {
				return errors.New(boardUsage)
			}
			out = args[i+1]
			i++
		default:
			return fmt.Errorf("알 수 없는 인자: %s\n%s", args[i], boardUsage)
		}
	}
	root, cards, err := LoadBoard(st, stateDir, cfg, now)
	if err != nil {
		return err
	}
	if root == "" {
		return errors.New("회사 루트를 찾지 못했습니다 — 업무를 하나 등록하거나(task assign --inbox <root>/runtime/inbox) 설정 company_root를 지정하세요: " + config.Path())
	}
	if asJSON {
		if cards == nil {
			cards = []board.Card{}
		}
		return json.NewEncoder(w).Encode(cards)
	}
	page := board.HTML(board.CompanyName(root), cards, now, cfg.BoardStaleLimit())
	path := out
	if path == "" {
		path = filepath.Join(stateDir, "board.html")
	}
	if err := os.WriteFile(path, page, 0o644); err != nil {
		return err
	}
	if out != "" || noOpen {
		fmt.Fprintln(w, "보드:", ShortenHome(path))
		return nil
	}
	url := "file://" + path
	if err := open(url); err != nil {
		return fmt.Errorf("브라우저 열기 실패(%v) — 파일은 %s", err, ShortenHome(path))
	}
	fmt.Fprintln(w, "열림:", ShortenHome(path))
	return nil
}
```

`internal/cli/browsercmd.go` — `browserOpen`의 본문을 `func OpenInBrowser(url string) error`로 빼고 `browserOpen`이 그것을 부르게 한다.

`main.go` run switch:

```go
	case "board":
		st, err := storeWithSync()
		if err != nil {
			return err
		}
		return cli.RunBoard(os.Stdout, st, state.DefaultDir(), config.Load(), cli.OpenInBrowser, args[1:], time.Now())
```

`helpcmd.go` task 줄 아래:

```
  board          회사 업무 보드 — HTML 생성 후 전용 브라우저로 열기  [--out 경로] [--json] [--no-open]
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./... -count=1 && go vet ./... && make install && agentlayer board --json | head -c 400; agentlayer help | grep board`
Expected: 전부 PASS. 이 맥에는 `~/ai-folder/company`에 업무가 없으니 `board --json`은 루트 안내 오류 또는 `[]`.

- [ ] **Step 5: 커밋**

```bash
git add internal/board/html.go internal/board/html_test.go internal/cli/boardcmd.go internal/cli/boardcmd_test.go internal/cli/browsercmd.go internal/cli/helpcmd.go internal/cli/helpcmd_test.go main.go
git commit -m "feat: agentlayer board — 6열 정적 HTML을 전용 브라우저로, --json·--out"
```

---

### Task 9: README·config 문서

**Files:**
- Modify: `README.md` ("세션 지시·업무 보고" 절 뒤에 "업무 보드" 절, 설정 표에 `company_root`·`board_stale_ready`)

- [ ] **Step 1: README에 절 추가**

"세션 지시·업무 보고" 절 끝에:

```markdown
### 업무 보드 (칸반 라이트)

회사 루트(`tasks/<업무ID>/task.md`·`log.md`)를 agentlayer가 자동으로 갱신하고 보여 준다. 상태값은 mat과 같은
`pending / in_progress / waiting_<세션> / reviewing / done`이고, 보드 6열은 여기서 파생한다
(`ready` = pending이면서 `parents:`가 전부 done).

- `task assign` → `in_progress` + `[ASSIGN]`, 훅 WAIT → `waiting_<세션>` + `[ASK]`, WORK 복귀 → `in_progress`, DONE → `reviewing` + `[REPORT]`, ERR → `[ERROR]`(status 유지).
- `agentlayer send`로 등록 세션에 보낸 지시는 `[SEND]`로 남는다 — `[ASK]` 뒤의 `[SEND]`가 Q&A 한 쌍.
- `task done <ID>` → `done` + `[COMPLETE]`, 그 결과 부모가 전부 끝난 자식마다 수신함에 `to: READY` 이벤트.
- ready·blocked가 30분(`board_stale_ready`) 넘게 방치되면 ⚠.
- 디스코드 카드에 "업무 보드" 절, `agentlayer board`는 6열 HTML을 전용 브라우저로 연다(`--json`·`--out`).
- 회사 루트는 `company_root` 설정이 없으면 등록된 업무의 `<root>/runtime/inbox`에서 유추한다.

task.md는 `status:`·`updated:` 줄만 agentlayer가 건드린다. 총괄은 status를 손으로 고치지 말 것(훅과 충돌).
```

설정 표에 두 행:

```
| `company_root` | AI 회사 루트. 비면 등록 업무의 inbox에서 유추 | (없음) |
| `board_stale_ready` | 보드 ready·blocked 방치 경고 기준(Go duration, 하한 1m) | `30m` |
```

- [ ] **Step 2: 커밋**

```bash
git add README.md
git commit -m "docs: 업무 보드 절, company_root·board_stale_ready"
```

---

### Task 10: ai-company v0.2 — 템플릿·총괄 절차·doctor·SKILL·매니페스트

**Files (레포 `~/VSCodeWorkspace/ai-company`):**
- Modify: `plugins/ai-company/skills/configure-company/assets/task.md`
- Modify: `plugins/ai-company/skills/configure-company/assets/company-block.md`
- Modify: `plugins/ai-company/skills/configure-company/SKILL.md`
- Modify: `plugins/ai-company/skills/configure-company/generator/companyctl.py` (doctor: agentlayer ≥ 1.6.0, parents 끊긴 참조 WARN)
- Modify: `plugins/ai-company/.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json` (0.2.0)
- Test: `tests/test_companyctl.py`

- [ ] **Step 1: 실패하는 테스트 작성** (`tests/test_companyctl.py` 끝에; 기존 픽스처·헬퍼 이름은 파일 상단을 읽고 맞춘다)

```python
def test_task_template_has_parents(tmp_path, run):
    run("init", "--root", str(tmp_path), "--name", "T")
    run("install", "--root", str(tmp_path))
    tpl = (tmp_path / "_templates" / "task.md").read_text()
    assert "parents: []" in tpl
    assert "status: pending" in tpl


def test_doctor_warns_on_broken_parent(tmp_path, run):
    run("init", "--root", str(tmp_path), "--name", "T")
    run("install", "--root", str(tmp_path))
    d = tmp_path / "tasks" / "B"
    d.mkdir(parents=True)
    (d / "task.md").write_text("# B\n```yaml\nstatus: pending\nparents: [A]\n```\n")
    out = run("doctor", "--root", str(tmp_path), check=False).stdout
    assert "WARN" in out and "B" in out and "A" in out


def test_doctor_requires_agentlayer_1_6(monkeypatch):
    from generator import companyctl  # 실제 import 경로는 conftest.py를 따른다
    assert companyctl.version_ok("agentlayer 1.6.0 (abc)") is True
    assert companyctl.version_ok("agentlayer 1.5.0 (abc)") is False
    assert companyctl.version_ok("agentlayer 2.0.0") is True
```

- [ ] **Step 2: 실패 확인**

Run: `cd ~/VSCodeWorkspace/ai-company && python3 -m pytest tests -q`
Expected: 새 테스트 3개 FAIL.

- [ ] **Step 3: 구현**

`assets/task.md` yaml 블록:

```yaml
status: pending
# pending | in_progress | waiting_<세션> | reviewing | done  (agentlayer가 자동 갱신 — 손으로 고치지 말 것)
# 보드 열: todo=pending(부모 미완) ready=pending(부모 전부 done) running=in_progress blocked=waiting_* review=reviewing done=done
parents: []
# 선행 업무ID 목록. 예: parents: [VIDEO-07-TOPICS]. 전부 done이면 총괄 수신함에 READY 이벤트가 온다.
created: <YYYY-MM-DD>
updated: <YYYY-MM-DD>
priority: medium
```

`assets/company-block.md` 총괄 절차:
- 1단계에 "`agentlayer board`로 보드를 본다(디스코드 카드에도 '업무 보드' 절이 있다)" 추가.
- 2단계: "`tasks/<업무ID>/task.md`(status: in_progress)" → "`tasks/<업무ID>/task.md`(status는 pending 그대로, 선행 업무가 있으면 `parents: [ID, …]`)".
- 3단계 assign 두 곳에 ` --root {ROOT}` 추가.
- 4단계 앞에: "`to`가 `READY`면 `task_id`의 부모가 전부 끝난 것 — 그 업무를 배정한다." `WAITING` 줄 뒤에: "대표의 답(또는 추가 지시)은 `agentlayer send <session>[:창] "<답>"`으로 보낸다 — `tasks/<업무ID>/log.md`에 `[SEND]`로 남아 `[ASK]`와 짝이 된다."
- 5단계: "`agentlayer task done <업무ID>`(task.md를 done으로 닫고 자식 업무의 READY 이벤트를 보낸다), `log.md`에 한 줄, 결과물을 …" — "task.md의 status를 done으로" 문구 삭제.
- 6단계 하지 말 것에 "`tasks/*/task.md`의 `status:` 줄 직접 수정(훅과 충돌)" 추가.

`companyctl.py`:
- 버전 검사 상수 `1.5.0` → `1.6.0`(`version_ok` 같은 함수가 없으면 `def version_ok(text: str) -> bool`로 뽑아내고 doctor가 쓰게 한다).
- doctor에 tasks 검사 추가: 각 `tasks/*/task.md`의 yaml `parents:`(한 줄 `[A, B]`·여러 줄 `- A` 둘 다)를 읽어 존재하지 않는 ID면 `WARN tasks/B: parents에 없는 업무 A`.
- `install`이 `_templates/task.md`를 새 템플릿으로 갱신하되(비파괴 원칙은 SESSION.md·지침 블록에만 적용됨 — 템플릿은 엔진 소유), 기존 `tasks/*/task.md`는 건드리지 않는다.

`SKILL.md`:
- 1단계 전제 "agentlayer ≥ 1.5.0" → "≥ 1.6.0".
- 7단계 검증: "LAB-1 뒤에 `tasks/LAB-2/task.md`를 `parents: [LAB-1]`로 만들고 `agentlayer task done LAB-1` → Monitor에 `to: READY, task_id: LAB-2`가 오면 선후 관계까지 성공. `agentlayer board`로 보드를 연다."
- 점검 절에 "parents 끊긴 참조 WARN" 한 줄.

매니페스트 두 파일 `"version": "0.2.0"`.

- [ ] **Step 4: 통과 확인**

Run: `cd ~/VSCodeWorkspace/ai-company && python3 -m pytest tests -q`
Expected: 전부 PASS.

- [ ] **Step 5: 커밋·태그**

```bash
cd ~/VSCodeWorkspace/ai-company && git add -A && git commit -m "feat: v0.2.0 — parents·보드 절차·doctor parents 검사, agentlayer ≥ 1.6.0" && git tag v0.2.0
```

(push는 Task 12에서 agentlayer 릴리즈와 함께.)

---

### Task 11: 실측 한 바퀴 (사용자 회사)

**Files:** 없음(코드 변경 없음). 실측 로그는 scratchpad에.

- [ ] **Step 1: 준비** — `make install`(v1.6.0 dev 빌드), `~/ai-folder/company`에서 `companyctl install --root ~/ai-folder/company`(v0.2 블록·템플릿 갱신), `tasks/LAB-1/task.md`·`tasks/LAB-2/task.md`(`parents: [LAB-1]`)를 템플릿에서 생성. `agentlayer board --json`에 두 장이 `ready`·`todo`로 보이는지 확인.
- [ ] **Step 2: 배정** — 임시 tmux 세션에 `claude`를 띄우거나 직원 봇 하나(collab-bot)를 골라 `agentlayer task assign LAB-1 <세션> --inbox ~/ai-folder/company/runtime/inbox` → task.md `in_progress`, log `[ASSIGN]`. `agentlayer send <세션> "도구 없이 OK라고만 답해"` → log `[SEND]`.
- [ ] **Step 3: 전이** — 직원 턴 종료 → task.md `reviewing`, log `[REPORT] DONE:`, inbox pending에 보고(task_dir 포함). `agentlayer card --out`에 업무 보드 절(review 1).
- [ ] **Step 4: 완료·READY** — `agentlayer task done LAB-1` → LAB-1 `done`, inbox에 `to: READY, task_id: LAB-2`. `agentlayer board`가 전용 브라우저에 열리고 LAB-2가 Ready 열.
- [ ] **Step 5: 정리** — LAB-1·LAB-2 폴더 삭제 여부는 사용자에게 보고만(삭제하지 않음), 임시 tmux 세션은 닫는다. 실측 결과를 SESSION.md 결정 기록에 한 줄.

---

### Task 12: 릴리즈 v1.6.0 + ai-company v0.2.0 공개

- [ ] **Step 1:** `go test ./... -count=1 && go vet ./...` 전부 통과 확인.
- [ ] **Step 2:** 릴리즈 노트 `scratchpad/notes-1.6.0.md`(한국어, 업무 보드 4항목·설정 2키·`board` 명령·`task assign --root`·`task done --root`·보고 `task_dir`).
- [ ] **Step 3:** `git tag v1.6.0 && git push origin main v1.6.0` → goreleaser(GitHub Actions)가 자산 5개 생성 → `gh release view v1.6.0`로 확인 → tap `Casks/agentlayer.rb` 1.6.0 갱신(이전 릴리즈 절차와 동일: sha256 5개).
- [ ] **Step 4:** ai-company `git push origin main v0.2.0`.
- [ ] **Step 5:** `brew upgrade agentlayer`(또는 `make install`)로 이 맥 `~/.local/bin/agentlayer` = v1.6.0, `agentlayer version` 확인. SESSION.md 갱신(현재 상태·다음 단계·결정 기록·파일 흔적)은 세션 마감 신호 때.
