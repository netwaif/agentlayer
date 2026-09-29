package hookcmd

import (
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

// WAITING에 머문 채 다른 승인 문구가 오면 전이 콜백은 WAITING→WAITING과 새 질문을 받는다 — 보고 쪽(task.ReportFor)이
// 이 질문을 마지막으로 보고한 것과 비교해 새 편지를 보낸다. 유휴 에코는 콜백까지 오지 않는다.
func TestNotificationWhileWaitingCarriesNewAsk(t *testing.T) {
	st := newStore(t)
	type call struct {
		prev, to state.AgentState
		ask      string
	}
	var calls []call
	SetTransitionHook(func(a *state.Agent, prev, to state.AgentState) { calls = append(calls, call{prev, to, a.Ask}) })
	defer SetTransitionHook(nil)
	send := func(msg string, at time.Time) {
		t.Helper()
		in := `{"session_id":"s1","message":"` + msg + `"}`
		if err := RunClaude(st, "notification", strings.NewReader(in), env("%3"), at); err != nil {
			t.Fatal(err)
		}
	}
	send("Bash 실행 승인이 필요합니다", t0)
	send("파일 쓰기 승인이 필요합니다", t0.Add(time.Minute))
	send("Claude is waiting for your input", t0.Add(2*time.Minute))
	if len(calls) != 2 {
		t.Fatalf("유휴 에코는 콜백을 부르지 않는다: %+v", calls)
	}
	if calls[0].to != state.StateWaiting || calls[0].ask != "Bash 실행 승인이 필요합니다" {
		t.Errorf("첫 승인 요청: %+v", calls[0])
	}
	if calls[1].prev != state.StateWaiting || calls[1].to != state.StateWaiting || calls[1].ask != "파일 쓰기 승인이 필요합니다" {
		t.Errorf("질문만 바뀐 알림: %+v", calls[1])
	}
}
