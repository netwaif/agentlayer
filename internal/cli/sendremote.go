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

// sendRemote — send의 원격 분기. 카드 없음/기동 실패(IDLE)면 Dispatch, WAIT면 Reply, DONE 뒤 지시는 후속 카드.
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
