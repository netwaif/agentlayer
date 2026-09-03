package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
)

// MCP roots 보정 — chrome-devtools-mcp는 take_screenshot의 filePath 같은 파일 쓰기를
// 클라이언트가 roots/list로 알려 준 작업 폴더 안에서만 허용한다. Claude Code는 roots를
// 주지만 codex는 능력을 선언하지 않아 MCP가 OS 임시 폴더만 허용했고, 코덱스가 스크린샷을
// base64로 받아 셸에서 파일로 쓰는 우회를 하던 원인이었다(2026-09-03 실측).
// 프록시(mcp-serve)가 중간에서: 클라이언트에 roots 능력이 없으면 initialize에 능력을
// 끼워 넣고 서버의 roots/list를 대신 답하며, 능력이 있어도 빈 목록이면 cwd를 채운다.
// cwd = 에이전트 CLI가 MCP 서버를 띄운 폴더(codex·claude 모두 자기 작업 폴더).
type mcpRoots struct {
	mu          sync.Mutex
	cwd         string
	clientRoots bool            // 클라이언트가 initialize에서 roots 능력을 선언했는지
	pending     map[string]bool // 클라이언트로 넘긴 roots/list 요청 id — 응답을 보정할 대상
}

func newMCPRoots(cwd string) *mcpRoots {
	return &mcpRoots{cwd: cwd, pending: map[string]bool{}}
}

// root는 MCP Root 객체 — uri는 file:// URL(공백·한글 경로는 퍼센트 인코딩).
func (m *mcpRoots) root() map[string]any {
	u := url.URL{Scheme: "file", Path: m.cwd}
	return map[string]any{"uri": u.String(), "name": filepath.Base(m.cwd)}
}

func idKey(msg map[string]any) (string, bool) {
	id, ok := msg["id"]
	if !ok || id == nil {
		return "", false
	}
	return fmt.Sprint(id), true
}

func marshalLine(msg map[string]any) []byte {
	b, err := json.Marshal(msg)
	if err != nil {
		return nil
	}
	return append(b, '\n')
}

// FromClient는 클라이언트→서버 한 줄을 보정한다. 바꿀 게 없으면 원문 그대로.
func (m *mcpRoots) FromClient(line []byte) []byte {
	var msg map[string]any
	if json.Unmarshal(line, &msg) != nil {
		return line
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if msg["method"] == "initialize" {
		params, _ := msg["params"].(map[string]any)
		if params == nil {
			params = map[string]any{}
			msg["params"] = params
		}
		caps, _ := params["capabilities"].(map[string]any)
		if caps == nil {
			caps = map[string]any{}
			params["capabilities"] = caps
		}
		if _, ok := caps["roots"]; ok {
			m.clientRoots = true
			return line
		}
		m.clientRoots = false
		caps["roots"] = map[string]any{"listChanged": false}
		if out := marshalLine(msg); out != nil {
			return out
		}
		return line
	}
	key, ok := idKey(msg)
	if !ok || !m.pending[key] {
		return line
	}
	delete(m.pending, key)
	if res, _ := msg["result"].(map[string]any); res != nil {
		if roots, _ := res["roots"].([]any); len(roots) > 0 {
			return line // 클라이언트가 제대로 줬다
		}
	}
	delete(msg, "error")
	msg["result"] = map[string]any{"roots": []any{m.root()}}
	if out := marshalLine(msg); out != nil {
		return out
	}
	return line
}

// FromServer는 서버→클라이언트 한 줄을 본다. 프록시가 대신 답할 roots/list면
// (서버에 쓸 응답, nil)을, 아니면 (nil, 클라이언트로 넘길 줄)을 돌려준다.
func (m *mcpRoots) FromServer(line []byte) (reply []byte, forward []byte) {
	var msg map[string]any
	if json.Unmarshal(line, &msg) != nil || msg["method"] != "roots/list" {
		return nil, line
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key, ok := idKey(msg)
	if !ok {
		return nil, line // 알림 형태면 건드리지 않는다
	}
	if m.clientRoots {
		m.pending[key] = true
		return nil, line
	}
	out := marshalLine(map[string]any{"jsonrpc": "2.0", "id": msg["id"],
		"result": map[string]any{"roots": []any{m.root()}}})
	if out == nil {
		return nil, line
	}
	return out, nil
}
