// internal/task/board.go
package task

import (
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
// 미등록·TaskDir 없음·세션/pane 불일치·그 밖의 전이는 무동작(false, nil). applied는 "실제로 반영됐다"는
// 뜻이라 board 쓰기가 실패하면(false, err)를 돌려준다. 실패해도 에이전트는 막지 않는다(호출자 몫).
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
	root, id := as.BoardRootID()
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
			return false, err
		}
	}
	if tag != "" {
		var text string
		switch tag {
		case "ASK":
			text = a.Ask
			if text == "" {
				text = "입력 대기(승인창 아님)"
			}
		case "REPORT":
			text = "DONE: " + a.Headline()
		default:
			text = a.Headline()
		}
		if err := board.AppendLog(root, id, tag, text, now); err != nil {
			return false, err
		}
	}
	return true, nil
}

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
