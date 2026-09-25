// internal/cli/taskcmd.go
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

const taskUsage = `사용법:
  agentlayer task assign <업무ID> <세션[:창]> --inbox <폴더> [--root <회사루트>] [--replace]
  agentlayer task list [--json]
  agentlayer task done <업무ID> [--root <회사루트>]
  agentlayer task watch <inbox> [--once] [--interval 200ms]
  agentlayer task message [--task <업무ID>] <본문|->   # 직원 pane에서: 총괄에게 편지(MESSAGE)`

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
		return taskDone(w, st, stateDir, args[1:], now)
	case "watch":
		return taskWatch(ctx, w, st, stateDir, args[1:])
	case "message":
		return taskMessage(w, os.Stdin, st, stateDir, config.Load().CompanyRoot, os.Getenv, args[1:], now)
	default:
		return fmt.Errorf("알 수 없는 task 명령: %s\n%s", args[0], taskUsage)
	}
}

func taskDone(w io.Writer, st *state.Store, stateDir string, args []string, now time.Time) error {
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
	if !task.ValidID(id) {
		return fmt.Errorf("업무ID 형식 오류: %q (영숫자·점·밑줄·하이픈 1~64자)", id)
	}
	// 등록에서 루트·inbox를 얻는다(있으면). 없으면 --root가 있어야 보드를 닫을 수 있다.
	inbox, found := "", false
	var remoteRef *task.RemoteRef
	list, err := task.List(stateDir)
	if err != nil {
		return err
	}
	for _, as := range list {
		if as.TaskID == id {
			found = true
			inbox = as.Inbox
			if as.Remote != nil && as.Remote.Handle != "" {
				remoteRef = as.Remote
			}
			if root == "" {
				if r, _ := as.BoardRootID(); r != "" {
					root = r
				}
			}
		}
	}
	if root == "" {
		root = inferDoneRoot(stateDir, id, list)
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
	// 이 루트를 기억해 둔다 — 이 done으로 등록이 마지막 하나였다면 board.Root가 유추할
	// inbox가 더는 없으니, 기억한 값이 그 자리를 대신한다.
	if err := board.RememberRoot(stateDir, root); err != nil {
		fmt.Fprintln(w, "  ⚠ 회사 루트 기억 실패:", err)
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
	// 원격 직원이면 실행기 쪽 카드도 닫는다(archive). 실패는 경고만 — 보드·등록 해제는 그대로 간다.
	if remoteRef != nil {
		if r, ok, _ := remote.Load(stateDir, remoteRef.Name); ok {
			if ad, err := OpenRemote(*r, stateDir); err == nil {
				if err := ad.Finish(context.Background(), remoteRef.Handle); err != nil {
					fmt.Fprintln(w, "  ⚠ 원격 카드 마감(archive) 실패:", err)
				}
			}
		}
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
	refreshBoard(w, st, stateDir, now)
	return nil
}

// inferDoneRoot는 --root 없이 온 task done의 회사 루트를 찾는다 — 등록이 없거나 등록에 루트가
// 없을 때(이미 done으로 닫혀 등록 해제된 부모에 자식을 늦게 붙이고 다시 닫는 경우, WSL2 실기
// 2026-09-22). 후보는 현재 폴더 → board.Root(설정 → 등록 inbox → 기억된 루트) 순이고, 그 루트에
// tasks/<id>/task.md가 실제로 있어야 채택한다(다른 회사의 루트로 엉뚱한 파일을 만들지 않게).
func inferDoneRoot(stateDir, id string, list []task.Assignment) string {
	var cands []string
	if cwd, err := os.Getwd(); err == nil {
		cands = append(cands, cwd)
	}
	var inboxes []string
	for _, as := range list {
		inboxes = append(inboxes, as.Inbox)
	}
	if r := board.Root(config.Load().CompanyRoot, inboxes, board.RememberedRoot(stateDir)); r != "" {
		cands = append(cands, r)
	}
	for _, r := range cands {
		if _, err := board.ReadTaskFile(r, id); err == nil {
			return r
		}
	}
	return ""
}

// refreshBoard는 열어 둔 보드 파일을 다시 쓴다(없으면 아무것도 안 함). 실패해도 명령은 성공 —
// 보드는 부산물이다.
func refreshBoard(w io.Writer, st *state.Store, stateDir string, now time.Time) {
	if st == nil {
		return
	}
	if _, err := RefreshBoardFile(st, stateDir, config.Load(), now); err != nil {
		fmt.Fprintln(w, "  ⚠ 보드 갱신 실패:", err)
	}
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
	if !task.ValidID(pos[0]) {
		return fmt.Errorf("업무ID 형식 오류: %q (영숫자·점·밑줄·하이픈 1~64자)", pos[0])
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
	if r, ok, err := remote.Load(stateDir, pos[1]); err == nil && ok {
		return taskAssignRemote(w, st, stateDir, r, pos[0], abs, root, replace, now)
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
		// 이 루트를 기억해 둔다 — 나중에 등록이 전부 사라져도(task done으로 마지막 업무를
		// 닫는 등) board.Root가 여전히 이 루트를 찾을 수 있게.
		if err := board.RememberRoot(stateDir, root); err != nil {
			fmt.Fprintln(w, "  ⚠ 회사 루트 기억 실패:", err)
		}
		if err := board.SetStatus(root, pos[0], "in_progress", now); err != nil {
			fmt.Fprintln(w, "  ⚠ task.md status 갱신 실패:", err)
		}
		if err := board.AppendLog(root, pos[0], "ASSIGN", targetLabel(a.Tmux.Session, a.Tmux.WindowName), now); err != nil {
			fmt.Fprintln(w, "  ⚠ log.md 기록 실패:", err)
		}
	}
	fmt.Fprintf(w, "업무 %s → %s %s [%s] 등록. 보고는 %s/pending/ 에 떨어집니다.%s\n",
		as.TaskID, SessionLabel(a), a.Tmux.PaneID, a.State, ShortenHome(abs), warn)
	refreshBoard(w, st, stateDir, now)
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
		if as.Remote != nil {
			s = string(state.StateIdle)
			if as.Remote.LastState != "" {
				s = string(as.Remote.LastState)
			}
			rows = append(rows, row{as, s})
			continue
		}
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
		fmt.Fprintln(w, "등록된 업무 없음. (완료된 업무는 목록에서 빠진다 — agentlayer board에서 Done 열로 확인)")
		return nil
	}
	fmt.Fprintln(w, PadRight("업무ID", 24)+PadRight("세션", 30)+PadRight("상태", 8)+"경과")
	for _, r := range rows {
		label := targetLabel(r.Session, r.Window)
		if r.Remote != nil {
			kind := "remote"
			if rr, ok, _ := remote.Load(stateDir, r.Remote.Name); ok {
				kind = rr.Kind
			}
			label = r.Session + " (" + kind + ")"
		}
		fmt.Fprintln(w, PadRight(r.TaskID, 24)+PadRight(label, 30)+PadRight(StateWord(state.AgentState(r.State)), 8)+Since(r.AssignedAt, now))
	}
	return nil
}

// targetLabel은 "세션[:창]" 표기 — 스레드 창(state.IsThreadWindow, t+6자리)일 때만 창 이름을
// 붙인다. claude 등은 창 이름을 자기 버전으로 바꿔버려(al-lab:2.1.272) 그 밖의 창 이름은 의미가
// 없다. task assign의 [ASSIGN] 로그, task list 표가 같은 문구를 쓴다.
func targetLabel(session, window string) string {
	if state.IsThreadWindow(window) {
		return session + ":" + window
	}
	return session
}

// StateWord는 status 표의 단어와 맞춘다(idle·WAIT·DONE·WORK·ERR·dead).
func StateWord(s state.AgentState) string {
	switch s {
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
	return string(s)
}

func taskWatch(ctx context.Context, w io.Writer, st *state.Store, stateDir string, args []string) error {
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
	// 원격 직원 폴링은 감시가 켜져 있는 동안만 돈다(데몬 없음). 전이는 inbox 파일로 떨어져 아래 Watch가 같은 길로 흘린다.
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
	enc := json.NewEncoder(w)
	err = task.Watch(ctx, abs, interval, once, func(r *task.Report) {
		_ = enc.Encode(r)
		refreshBoard(io.Discard, st, stateDir, time.Now())
	})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

// taskAssignRemote — 원격 직원 등록. 카드는 만들지 않는다(첫 send가 만든다). agents/에는 기록하지 않는다.
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
