// internal/cli/taskcmd.go
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

const taskUsage = `사용법:
  agentlayer task assign <업무ID> <세션[:창]> --inbox <폴더> [--root <회사루트>] [--replace]
  agentlayer task list [--json]
  agentlayer task done <업무ID> [--root <회사루트>]
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
		return taskDone(w, stateDir, args[1:], now)
	case "watch":
		return taskWatch(ctx, w, args[1:])
	default:
		return fmt.Errorf("알 수 없는 task 명령: %s\n%s", args[0], taskUsage)
	}
}

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
	inbox, found := "", false
	list, err := task.List(stateDir)
	if err != nil {
		return err
	}
	for _, as := range list {
		if as.TaskID == id {
			found = true
			inbox = as.Inbox
			if root == "" {
				if r, _ := as.BoardRootID(); r != "" {
					root = r
				}
			}
		}
	}
	if root == "" {
		if !found {
			return fmt.Errorf("업무 %q이 등록돼 있지 않습니다 — 보드만 닫으려면 --root <회사루트>", id)
		}
		if _, err := task.Done(stateDir, id); err != nil {
			return err
		}
		fmt.Fprintf(w, "업무 %s 등록 해제 (보드 연결 없음)\n", id)
		return nil
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if inbox == "" {
		inbox = filepath.Join(root, "runtime", "inbox")
	}
	// 보드를 먼저 닫는다 — 실패하면 등록을 그대로 둬 재시도할 수 있게 한다(등록 해제가 먼저면
	// SetStatus·WriteReport 실패 시 root·inbox를 잃어버려 복구가 어려워진다).
	ready, err := task.MarkDone(root, id, inbox, now)
	if err != nil {
		return err
	}
	if found {
		if _, err := task.Done(stateDir, id); err != nil {
			return err
		}
		fmt.Fprintf(w, "업무 %s 등록 해제 · task.md done\n", id)
	} else {
		fmt.Fprintf(w, "업무 %s task.md done (등록은 없었음)\n", id)
	}
	for _, c := range ready {
		fmt.Fprintf(w, "  → %s 배정 가능(READY 이벤트 전송)\n", c)
	}
	return nil
}

func taskAssign(w io.Writer, st *state.Store, stateDir string, args []string, now time.Time) error {
	var pos []string
	var inbox, root string
	replace := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--inbox":
			if i+1 >= len(args) {
				return errors.New("--inbox 뒤에 폴더가 필요합니다")
			}
			inbox = args[i+1]
			i++
		case "--root":
			if i+1 >= len(args) {
				return errors.New("--root 뒤에 회사 루트가 필요합니다")
			}
			root = args[i+1]
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
	if root != "" {
		root, err = filepath.Abs(root)
		if err != nil {
			return err
		}
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	a, err := ResolveTarget(agents, pos[1])
	if err != nil {
		return err
	}
	if a.State == state.StateDead {
		return errors.New("세션이 죽었습니다 — 'agentlayer restore' 뒤 다시 등록하세요")
	}
	as := task.Assignment{TaskID: pos[0], AgentID: a.ID, Session: a.Tmux.Session, Window: a.Tmux.WindowName,
		Pane: a.Tmux.PaneID, Inbox: abs, AssignedAt: now}
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
		if err := board.AppendLog(root, pos[0], "ASSIGN", targetLabel(a.Tmux.Session, a.Tmux.WindowName), now); err != nil {
			fmt.Fprintln(w, "  ⚠ log.md 기록 실패:", err)
		}
	}
	fmt.Fprintf(w, "업무 %s → %s %s [%s] 등록. 보고는 %s/pending/ 에 떨어집니다.%s\n",
		as.TaskID, SessionLabel(a), a.Tmux.PaneID, a.State, ShortenHome(abs), warn)
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
			if a.Tmux.Session != as.Session || a.Tmux.PaneID != as.Pane {
				s = "stale" // 에이전트 ID는 살아 있지만 세션·pane이 등록 당시와 다름(재사용)
			} else {
				s = string(a.State)
			}
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
		label := targetLabel(r.Session, r.Window)
		fmt.Fprintln(w, PadRight(r.TaskID, 24)+PadRight(label, 30)+PadRight(stateWord(r.State), 8)+Since(r.AssignedAt, now))
	}
	return nil
}

// targetLabel은 "세션[:창]" 표기 — 스레드 창(t+6자리)이면 창 이름까지 붙인다.
// task assign의 [ASSIGN] 로그, task list 표가 같은 문구를 쓴다.
func targetLabel(session, window string) string {
	if window != "" {
		return session + ":" + window
	}
	return session
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
