// internal/cli/controlgate.go
package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/netwaif/agentlayer/internal/browser"
	"github.com/netwaif/agentlayer/internal/state"
)

// controlGate — mcp-serve 프록시의 소유권 게이트. tools/call 한 줄마다 파일 정본을 읽고
// 탭의 버튼 요청을 회수해 전이한 뒤, 사용자 소유면 잡고 기다린다.
// 설계: docs/superpowers/specs/2026-09-22-agent-browser-control-design.md 2절.
type controlGate struct {
	dir   string
	now   func() time.Time
	sleep func(time.Duration)
	wait  time.Duration                                // 사용자 소유 대기 상한
	poll  time.Duration                                // 대기 중 재확인 주기
	sync  func(c browser.Control) []browser.TabRequest // 미러 쓰기 + 요청 회수(Task 3 SyncTabs를 감싼 것)
	agent func() (id, label string)
	// lastKey — 직전에 실제로 써 보낸 미러의 실질 내용(last_call만 빼고). 통과 확정 뒤의
	// 미러 왕복은 내용이 그대로면 건너뛴다 — 호출당 CDP 왕복을 하나 줄인다.
	lastKey string
}

// mirrorKey — 미러 payload에서 시각(since·last_call)을 뺀 실질 내용. 이 값이 같으면
// 탭이 보는 화면도 같으므로 다시 쓸 이유가 없다(최종 리뷰 IMPORTANT 3-b).
func mirrorKey(c browser.Control) string {
	return fmt.Sprintf("%s|%s|%s|%t|%d", c.Owner, c.Agent, c.Label, c.Stopped, c.Waiting)
}

func newControlGate(dir string, wait time.Duration, sync func(browser.Control) []browser.TabRequest, agent func() (string, string)) *controlGate {
	return &controlGate{dir: dir, now: time.Now, sleep: time.Sleep, wait: wait, poll: 500 * time.Millisecond, sync: sync, agent: agent}
}

const (
	gateUserBusyText = "사용자가 브라우저를 직접 조작 중입니다 — 돌려줄 때까지 기다린 뒤 다시 시도하세요. 재시도하지 말고 사용자에게 알린 뒤 지시를 기다리세요."
	gateStoppedText  = "사용자가 브라우저 조작을 중단시켰습니다 — 재시도하지 말고 사용자 지시를 기다리세요."
)

// step — 파일을 읽고 만료를 반영하고, 탭 요청을 회수해 전이한 뒤 저장한다. 결과 상태를 돌려준다.
// 회수한 요청 중 ack(c.LastRequestMs)보다 새 것만 반영하고, 반영한 요청의 ms를 ack로
// 올려 저장한다 — 탭은 ack된 요청만 지우므로 느린 탭의 클릭도 버려지지 않는다.
func (g *controlGate) step(waiting int) browser.Control {
	now := g.now()
	c := browser.LoadControl(g.dir).Effective(now)
	c.Waiting = waiting
	reqs := g.syncOnce(c)
	if ev, ms, ok := browser.LatestRequestAfter(reqs, c.LastRequestMs); ok {
		c = browser.Apply(c, ev, "", "", now)
		c.LastRequestMs = ms
	}
	_ = browser.SaveControl(g.dir, c)
	return c
}

// syncOnce — 미러를 쓰고 요청을 회수한다. 무엇을 썼는지 lastKey에 남긴다.
func (g *controlGate) syncOnce(c browser.Control) []browser.TabRequest {
	g.lastKey = mirrorKey(c)
	return g.sync(c)
}

// Pass — 이 줄을 서버로 넘길지. reply가 있으면 클라이언트에 그대로 쓴다(서버로 안 보냄).
func (g *controlGate) Pass(line []byte) (bool, []byte) {
	if !IsMCPToolCall(line) {
		return true, nil
	}
	c := g.step(0)
	if c.Owner == browser.OwnerUser {
		if c.Stopped {
			return false, gateErrorReply(rpcID(line), gateStoppedText)
		}
		deadline := g.now().Add(g.wait)
		for c.Owner == browser.OwnerUser && !c.Stopped {
			if !g.now().Before(deadline) {
				// 마지막 확인 — 바로 이 순간 사용자가 돌려줬으면 거절하지 않고 통과시킨다.
				c = g.step(0)
				break
			}
			g.sleep(g.poll)
			c = g.step(1)
		}
		if c.Stopped {
			g.step(0)
			return false, gateErrorReply(rpcID(line), gateStoppedText)
		}
		if c.Owner == browser.OwnerUser {
			return false, gateErrorReply(rpcID(line), gateUserBusyText)
		}
	}
	id, label := g.agent()
	c = browser.Apply(c, browser.EvCall, id, label, g.now())
	c.Waiting = 0
	_ = browser.SaveControl(g.dir, c)
	// 전이·파일 갱신 뒤 미러도 지금 반영한다 — 다음 step까지 기다리면 이번 호출 동안
	// 탭의 owner 표시가 한 박자 뒤처진다(직전 상태를 계속 보여줌). 이 sync가 회수하는
	// 버튼 요청은 버리지 않는다 — syncJS는 Eval 한 번으로 DOM 속성을 읽고 지우므로,
	// 바로 이 순간 사용자가 눌렀다면 여기서 잡지 않으면 다음 step()은 그 클릭을 영영
	// 못 본다(이번 호출 자체는 이미 통과가 확정됐으니 그대로 전달한다).
	// 내용이 그대로면(같은 에이전트가 이어서 부르는 흔한 경우) 이 왕복을 건너뛴다 —
	// 탭이 보는 화면은 이미 같고, 사용자의 버튼 요청은 이제 ack 전까지 DOM에 남으므로
	// 여기서 회수하지 않아도 다음 호출의 step()이 반드시 집어 간다(최종 리뷰 IMPORTANT 2·3).
	if mirrorKey(c) == g.lastKey {
		return true, nil
	}
	if ev, ms, ok := browser.LatestRequestAfter(g.syncOnce(c), c.LastRequestMs); ok {
		c = browser.Apply(c, ev, "", "", g.now())
		c.LastRequestMs = ms
		_ = browser.SaveControl(g.dir, c)
	}
	return true, nil
}

func rpcID(line []byte) json.RawMessage {
	var msg struct {
		ID json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal(line, &msg)
	return msg.ID
}

// gateErrorReply — MCP tools/call 결과(isError)를 stdio 한 줄로 합성.
func gateErrorReply(id json.RawMessage, text string) []byte {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]any{
			"isError": true,
			"content": []map[string]string{{"type": "text", "text": text}},
		},
	})
	return append(b, '\n')
}

// resolveAgent — 프록시가 물려받은 TMUX_PANE으로 관제탑 레코드를 찾아 (ID, 최근 작업).
func resolveAgent(st *state.Store, pane string) (string, string) {
	const defID, defLabel = "agent", "브라우저 작업"
	if st == nil || pane == "" {
		return defID, defLabel
	}
	agents, err := st.List()
	if err != nil {
		return defID, defLabel
	}
	for _, a := range agents {
		if a.Tmux.PaneID == pane {
			label := a.Task
			if label == "" {
				label = defLabel
			}
			return a.ID, label
		}
	}
	return defID, defLabel
}
