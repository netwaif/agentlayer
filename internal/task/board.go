// internal/task/board.go
package task

import (
	"encoding/json"
	"os"
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
			text = "DONE: " + headlineOrPlaceholder(a)
		default:
			text = headlineOrPlaceholder(a)
		}
		if err := board.AppendLog(root, id, tag, text, now); err != nil {
			return false, err
		}
	}
	return true, nil
}

// headlineOrPlaceholder는 a.Headline()이 비어 있으면(Ask도 Task도 없는 드문 경우) "(요약 없음)"을
// 대신 돌려준다 — [REPORT] DONE: 이나 [ERROR] 뒤에 끝공백만 남은 줄이 log.md에 쌓이면 안 된다.
func headlineOrPlaceholder(a *state.Agent) string {
	if h := a.Headline(); h != "" {
		return h
	}
	return "(요약 없음)"
}

// MarkDone은 업무를 done으로 닫고([COMPLETE]), 그 결과 부모가 전부 done이 된 pending 자식마다
// inbox에 READY 이벤트를 쓴다. inbox가 비면 이벤트는 쓰지 않는다. 이미 done인 업무는 상태·로그를
// 건드리지 않되(중복 [COMPLETE] 없음) 자식은 다시 평가한다 — 부모를 먼저 닫고 자식을 나중에 붙인
// 뒤 총괄이 부모를 재차 done 처리하는 순서가 흔해서다. 같은 자식의 READY가 이미 수신함
// (pending/·received/)에 있으면 다시 쓰지 않으므로 재실행은 안전하다. 돌려주는 값은 이번에
// 이벤트를 쓴 자식 ID들.
func MarkDone(root, id, inbox string, now time.Time) ([]string, error) {
	tf, err := board.ReadTaskFile(root, id)
	if err != nil {
		return nil, err
	}
	if tf.Status != "done" {
		if err := board.SetStatus(root, id, "done", now); err != nil {
			return nil, err
		}
		title := tf.Title
		if title == "" {
			title = "완료"
		}
		if err := board.AppendLog(root, id, "COMPLETE", title, now); err != nil {
			return nil, err
		}
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
		if inbox == "" {
			ready = append(ready, c.ID)
			continue
		}
		if readyAlreadySent(inbox, c.ID) {
			continue
		}
		ready = append(ready, c.ID)
		r := ReadyReport(c, root, now)
		r.Inbox = inbox
		if _, err := WriteReport(r); err != nil {
			return ready, err
		}
	}
	return ready, nil
}

// readyAlreadySent는 수신함 pending/·received/에 taskID의 READY 이벤트가 이미 있는지 본다.
// quarantine/은 보지 않는다(불량 건은 전달된 적이 없다). 읽기 실패는 "없음"으로 친다 — 중복 한 건이
// 누락 한 건보다 싸다.
func readyAlreadySent(inbox, taskID string) bool {
	for _, sub := range []string{"pending", "received"} {
		files, _ := filepath.Glob(filepath.Join(inbox, sub, "*.json"))
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			var r struct {
				TaskID string `json:"task_id"`
				To     string `json:"to"`
			}
			if json.Unmarshal(b, &r) == nil && r.TaskID == taskID && r.To == "READY" {
				return true
			}
		}
	}
	return false
}
