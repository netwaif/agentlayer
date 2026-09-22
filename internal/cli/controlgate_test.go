// internal/cli/controlgate_test.go
package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/browser"
	"github.com/netwaif/agentlayer/internal/state"
)

func gateLine(id int, tool string) []byte {
	return []byte(`{"jsonrpc":"2.0","id":` + itoa(id) + `,"method":"tools/call","params":{"name":"` + tool + `","arguments":{"pageId":1}}}` + "\n")
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func newTestGate(t *testing.T, dir string, sync func(browser.Control) []browser.TabRequest) (*controlGate, *time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 22, 14, 0, 0, 0, time.Local)
	g := newControlGate(dir, 3*time.Second, sync, func() (string, string) { return "claude-%1", "검색" })
	g.now = func() time.Time { return now }
	g.sleep = func(d time.Duration) { now = now.Add(d) }
	g.poll = time.Second
	return g, &now
}

func TestGatePassIdleBecomesAgent(t *testing.T) {
	dir := t.TempDir()
	g, _ := newTestGate(t, dir, func(browser.Control) []browser.TabRequest { return nil })
	fwd, reply := g.Pass(gateLine(1, "click"))
	if !fwd || reply != nil {
		t.Fatalf("idle이면 통과: %v %s", fwd, reply)
	}
	c := browser.LoadControl(dir)
	if c.Owner != browser.OwnerAgent || c.Agent != "claude-%1" || c.Label != "검색" {
		t.Fatalf("파일 갱신: %+v", c)
	}
}

func TestGateNonToolCallPasses(t *testing.T) {
	g, _ := newTestGate(t, t.TempDir(), func(browser.Control) []browser.TabRequest { return nil })
	fwd, reply := g.Pass([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"))
	if !fwd || reply != nil {
		t.Fatal("tools/call 아니면 그대로 통과")
	}
	if c := browser.LoadControl(g.dir); c.Owner != browser.OwnerIdle {
		t.Fatal("상태도 안 건드림")
	}
}

func TestGateUserHoldsThenReleases(t *testing.T) {
	dir := t.TempDir()
	_ = browser.SaveControl(dir, browser.Control{Owner: browser.OwnerUser, Agent: "claude-%1"})
	calls := 0
	g, _ := newTestGate(t, dir, func(browser.Control) []browser.TabRequest {
		calls++
		if calls == 2 { // 두 번째 폴링에서 「돌려주기」
			return []browser.TabRequest{{Event: browser.EvUserReturn, At: 5}}
		}
		return nil
	})
	fwd, reply := g.Pass(gateLine(2, "click"))
	if !fwd || reply != nil {
		t.Fatalf("돌려주면 통과해야 함: %v %s", fwd, reply)
	}
	if c := browser.LoadControl(dir); c.Owner != browser.OwnerAgent {
		t.Fatalf("복귀 뒤 agent: %+v", c)
	}
}

func TestGateUserTimeoutReplies(t *testing.T) {
	dir := t.TempDir()
	_ = browser.SaveControl(dir, browser.Control{Owner: browser.OwnerUser})
	g, _ := newTestGate(t, dir, func(browser.Control) []browser.TabRequest { return nil })
	fwd, reply := g.Pass(gateLine(3, "click"))
	if fwd || reply == nil {
		t.Fatal("타임아웃이면 전달 안 하고 응답 합성")
	}
	var msg struct {
		ID     int `json:"id"`
		Result struct {
			IsError bool                    `json:"isError"`
			Content []struct{ Text string } `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(reply, &msg); err != nil || msg.ID != 3 || !msg.Result.IsError {
		t.Fatalf("응답 형식: %s (%v)", reply, err)
	}
	if !strings.Contains(msg.Result.Content[0].Text, "직접 조작 중") || !strings.Contains(msg.Result.Content[0].Text, "재시도하지") {
		t.Fatalf("문구: %s", msg.Result.Content[0].Text)
	}
	if !strings.HasSuffix(string(reply), "\n") {
		t.Fatal("stdio 한 줄이므로 개행으로 끝나야 함")
	}
	if c := browser.LoadControl(dir); c.Waiting != 0 {
		t.Fatalf("대기 해제 뒤 waiting 0: %+v", c)
	}
}

func TestGateStoppedRepliesImmediately(t *testing.T) {
	dir := t.TempDir()
	_ = browser.SaveControl(dir, browser.Control{Owner: browser.OwnerUser, Stopped: true})
	slept := 0
	g, _ := newTestGate(t, dir, func(browser.Control) []browser.TabRequest { return nil })
	g.sleep = func(time.Duration) { slept++ }
	fwd, reply := g.Pass(gateLine(4, "click"))
	if fwd || reply == nil || slept != 0 {
		t.Fatalf("stopped는 즉시 거절: %v %s slept=%d", fwd, reply, slept)
	}
	if !strings.Contains(string(reply), "중단") {
		t.Fatalf("문구: %s", reply)
	}
}

func TestGateExpiredAgentIsIdle(t *testing.T) {
	dir := t.TempDir()
	g, now := newTestGate(t, dir, func(browser.Control) []browser.TabRequest { return nil })
	_ = browser.SaveControl(dir, browser.Control{Owner: browser.OwnerAgent, Agent: "codex-%9", LastCall: now.Add(-time.Minute)})
	g.Pass(gateLine(5, "take_snapshot"))
	c := browser.LoadControl(dir)
	if c.Agent != "claude-%1" || !c.Since.Equal(*now) {
		t.Fatalf("만료 뒤 새 소유: %+v", c)
	}
}

func TestGateTakeRequestDuringAgent(t *testing.T) {
	// 통과 시점의 sync가 「내가 조작하기」를 회수하면 그 호출은 잡힌다(다음 폴링에서 돌려주면 풀림)
	dir := t.TempDir()
	n := 0
	g, _ := newTestGate(t, dir, func(browser.Control) []browser.TabRequest {
		n++
		switch n {
		case 1:
			return []browser.TabRequest{{Event: browser.EvUserTake, At: 1}}
		case 2:
			return []browser.TabRequest{{Event: browser.EvUserReturn, At: 2}}
		}
		return nil
	})
	fwd, reply := g.Pass(gateLine(6, "click"))
	// n==3: step(0)에서 take 회수(1) → 폴링 step(1)에서 return 회수(2) →
	// 통과 확정 뒤 미러를 지금 반영하는 마지막 sync(3, 이 호출은 무시).
	if !fwd || reply != nil || n != 3 {
		t.Fatalf("take→return: %v %s n=%d", fwd, reply, n)
	}
}

// TestGateMirrorsTransitionOnPass — Fix round 1 #1: 통과가 확정되면 그 즉시(다음 step을
// 기다리지 않고) 미러에 새 소유권이 실려야 한다. sync 클로저의 마지막 호출이 관찰하는
// Control이 전이 후 값(Owner=agent, 새 id·label)이어야 한다.
func TestGateMirrorsTransitionOnPass(t *testing.T) {
	dir := t.TempDir()
	var last browser.Control
	g, _ := newTestGate(t, dir, func(c browser.Control) []browser.TabRequest {
		last = c
		return nil
	})
	fwd, reply := g.Pass(gateLine(7, "click"))
	if !fwd || reply != nil {
		t.Fatalf("idle이면 통과: %v %s", fwd, reply)
	}
	if last.Owner != browser.OwnerAgent || last.Agent != "claude-%1" || last.Label != "검색" {
		t.Fatalf("마지막 sync 호출이 전이 후 소유권을 봐야 함: %+v", last)
	}
}

// TestGateUserReturnsExactlyAtDeadline — Fix round 1 #5: 대기 상한 마지막 확인
// (타임아웃 직전의 g.step(0))에서 사용자가 「AI에게 돌려주기」를 눌렀으면 거절 대신
// 통과시킨다. wait=3s, poll=1s이므로 폴링 4번(초기 1 + 폴링 3) 뒤 다섯 번째 sync 호출이
// 데드라인 확인이다.
func TestGateUserReturnsExactlyAtDeadline(t *testing.T) {
	dir := t.TempDir()
	_ = browser.SaveControl(dir, browser.Control{Owner: browser.OwnerUser})
	calls := 0
	g, _ := newTestGate(t, dir, func(browser.Control) []browser.TabRequest {
		calls++
		if calls == 5 { // 다섯 번째 = 데드라인 확인용 마지막 step(0)
			return []browser.TabRequest{{Event: browser.EvUserReturn, At: 99}}
		}
		return nil
	})
	fwd, reply := g.Pass(gateLine(9, "click"))
	if !fwd || reply != nil {
		t.Fatalf("데드라인 확인 시점에 돌아왔으면 통과해야 함: %v %s", fwd, reply)
	}
	if c := browser.LoadControl(dir); c.Owner != browser.OwnerAgent {
		t.Fatalf("돌아온 뒤 agent: %+v", c)
	}
}

func TestResolveAgent(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.NewStore(dir)
	_ = st.Save(&state.Agent{ID: "claude-%7", Kind: "claude", Task: "JustWatch 신작", Tmux: state.TmuxRef{PaneID: "%7"}})
	if id, label := resolveAgent(st, "%7"); id != "claude-%7" || label != "JustWatch 신작" {
		t.Fatalf("resolveAgent = %q %q", id, label)
	}
	if id, label := resolveAgent(st, "%8"); id != "agent" || label != "브라우저 작업" {
		t.Fatalf("없으면 기본값: %q %q", id, label)
	}
	if id, _ := resolveAgent(nil, "%7"); id != "agent" {
		t.Fatal("store nil이면 기본값")
	}
}
