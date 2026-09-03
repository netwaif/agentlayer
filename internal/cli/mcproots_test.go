package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("JSON 아님: %s", b)
	}
	return m
}

// codex처럼 roots 능력이 없는 클라이언트: initialize에 능력을 끼워 넣고 roots/list는 프록시가 답한다.
func TestMCPRootsClientWithoutCapability(t *testing.T) {
	m := newMCPRoots("/Users/x/my proj")
	init := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"codex"}}}` + "\n")
	out := m.FromClient(init)
	caps := decode(t, out)["params"].(map[string]any)["capabilities"].(map[string]any)
	if _, ok := caps["roots"]; !ok {
		t.Fatalf("roots 능력이 끼워져야 함: %s", out)
	}
	if !strings.HasSuffix(string(out), "\n") {
		t.Error("줄바꿈 유지")
	}
	reply, fwd := m.FromServer([]byte(`{"jsonrpc":"2.0","id":7,"method":"roots/list"}` + "\n"))
	if fwd != nil || reply == nil {
		t.Fatalf("프록시가 대신 답해야 함: reply=%s fwd=%s", reply, fwd)
	}
	r := decode(t, reply)
	if r["id"] != float64(7) {
		t.Errorf("id 보존: %v", r["id"])
	}
	roots := r["result"].(map[string]any)["roots"].([]any)
	root := roots[0].(map[string]any)
	if root["uri"] != "file:///Users/x/my%20proj" || root["name"] != "my proj" {
		t.Errorf("root: %v", root)
	}
}

// Claude처럼 roots 능력이 있는 클라이언트: 요청은 넘기고, 빈 목록 응답만 cwd로 채운다.
func TestMCPRootsClientWithCapabilityEmptyList(t *testing.T) {
	m := newMCPRoots("/w")
	init := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{"roots":{"listChanged":true}}}}` + "\n")
	if out := m.FromClient(init); string(out) != string(init) {
		t.Errorf("능력이 있으면 원문 그대로: %s", out)
	}
	req := []byte(`{"jsonrpc":"2.0","id":"r1","method":"roots/list"}` + "\n")
	reply, fwd := m.FromServer(req)
	if reply != nil || string(fwd) != string(req) {
		t.Fatalf("요청은 클라이언트로 넘겨야 함")
	}
	out := m.FromClient([]byte(`{"jsonrpc":"2.0","id":"r1","result":{"roots":[]}}` + "\n"))
	roots := decode(t, out)["result"].(map[string]any)["roots"].([]any)
	if len(roots) != 1 || roots[0].(map[string]any)["uri"] != "file:///w" {
		t.Errorf("빈 목록은 cwd로: %s", out)
	}
	// 같은 id 재응답은 더 이상 보정하지 않는다(pending 소진)
	again := []byte(`{"jsonrpc":"2.0","id":"r1","result":{"roots":[]}}` + "\n")
	if out := m.FromClient(again); string(out) != string(again) {
		t.Error("pending 소진 후 원문 유지")
	}
}

// 클라이언트가 roots를 제대로 주면 손대지 않고, error 응답이면 결과로 바꾼다.
func TestMCPRootsKeepsRealRootsAndReplacesError(t *testing.T) {
	m := newMCPRoots("/w")
	m.FromClient([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{"roots":{}}}}`))
	m.FromServer([]byte(`{"jsonrpc":"2.0","id":2,"method":"roots/list"}`))
	good := []byte(`{"jsonrpc":"2.0","id":2,"result":{"roots":[{"uri":"file:///theirs","name":"theirs"}]}}` + "\n")
	if out := m.FromClient(good); string(out) != string(good) {
		t.Errorf("진짜 roots는 보존: %s", out)
	}
	m.FromServer([]byte(`{"jsonrpc":"2.0","id":3,"method":"roots/list"}`))
	out := m.FromClient([]byte(`{"jsonrpc":"2.0","id":3,"error":{"code":-32601,"message":"Method not found"}}` + "\n"))
	msg := decode(t, out)
	if _, has := msg["error"]; has {
		t.Errorf("error는 제거돼야: %s", out)
	}
	if roots := msg["result"].(map[string]any)["roots"].([]any); len(roots) != 1 {
		t.Errorf("cwd root 하나: %s", out)
	}
}

// 무관한 줄(tools/call·JSON 아님·알림)은 양방향 모두 원문 그대로.
func TestMCPRootsPassThrough(t *testing.T) {
	m := newMCPRoots("/w")
	for _, l := range []string{`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{}}`, `not json`, `{"jsonrpc":"2.0","method":"notifications/initialized"}`} {
		if out := m.FromClient([]byte(l)); string(out) != l {
			t.Errorf("클라이언트 원문 유지: %s → %s", l, out)
		}
		reply, fwd := m.FromServer([]byte(l))
		if reply != nil || string(fwd) != l {
			t.Errorf("서버 원문 유지: %s", l)
		}
	}
}
