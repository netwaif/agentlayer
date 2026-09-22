// internal/cli/controlgate.go
package cli

import (
	"encoding/json"
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
}

func newControlGate(dir string, wait time.Duration, sync func(browser.Control) []browser.TabRequest, agent func() (string, string)) *controlGate {
	return &controlGate{dir: dir, now: time.Now, sleep: time.Sleep, wait: wait, poll: 500 * time.Millisecond, sync: sync, agent: agent}
}

const (
	gateUserBusyText = "사용자가 브라우저를 직접 조작 중입니다 — 돌려줄 때까지 기다린 뒤 다시 시도하세요. 재시도하지 말고 사용자에게 알린 뒤 지시를 기다리세요."
	gateStoppedText  = "사용자가 브라우저 조작을 중단시켰습니다 — 재시도하지 말고 사용자 지시를 기다리세요."
)

// step — 파일을 읽고 만료를 반영하고, 탭 요청을 회수해 전이한 뒤 저장한다. 결과 상태를 돌려준다.
func (g *controlGate) step(waiting int) browser.Control {
	now := g.now()
	c := browser.LoadControl(g.dir).Effective(now)
	c.Waiting = waiting
	if ev, ok := browser.LatestRequest(g.sync(c)); ok {
		c = browser.Apply(c, ev, "", "", now)
	}
	_ = browser.SaveControl(g.dir, c)
	return c
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
	// 탭의 owner 표시가 한 박자 뒤처진다(직전 상태를 계속 보여줌). 이 호출이 회수하는
	// 버튼 요청은 버린다 — 다음 tools/call의 step()이 정상적으로 집어간다.
	_ = g.sync(c)
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
