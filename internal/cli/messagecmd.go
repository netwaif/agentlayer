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
// 보낸 세션은 훅과 같은 규칙(TMUX_PANE + 기본 tmux 서버)으로 판정한다. companyRoot는 설정값(테스트 격리용 인자).
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
