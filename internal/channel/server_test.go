package channel

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func readLine(t *testing.T, r *bufio.Reader) map[string]any {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("json %q: %v", line, err)
	}
	return m
}

func TestInitializeDeclaresChannelCapability(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}` + "\n")
	pr, pw := io.Pipe()
	s := New(in, pw, "1.10.0", "편지 안내")
	go func() { _ = s.Run(context.Background()); pw.Close() }()
	m := readLine(t, bufio.NewReader(pr))
	res := m["result"].(map[string]any)
	if res["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion 에코: %v", res["protocolVersion"])
	}
	caps := res["capabilities"].(map[string]any)
	if _, ok := caps["experimental"].(map[string]any)["claude/channel"]; !ok {
		t.Error("experimental[claude/channel] 선언돼야 함")
	}
	if res["serverInfo"].(map[string]any)["name"] != Name || res["instructions"] != "편지 안내" {
		t.Errorf("serverInfo/instructions: %v", res)
	}
}

func TestNotifyBeforeInitializeIsQueued(t *testing.T) {
	inr, inw := io.Pipe()
	pr, pw := io.Pipe()
	s := New(inr, pw, "", "")
	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()
	s.Notify(Notification{Content: "first", Meta: map[string]string{"event": "MESSAGE"}})
	s.Notify(Notification{Content: "second", Meta: map[string]string{}})
	go inw.Write([]byte(`{"jsonrpc":"2.0","id":7,"method":"initialize","params":{}}` + "\n"))
	br := bufio.NewReader(pr)
	if m := readLine(t, br); m["id"].(float64) != 7 {
		t.Fatalf("첫 줄은 initialize 응답: %v", m)
	}
	n1 := readLine(t, br)
	n2 := readLine(t, br)
	if n1["method"] != "notifications/claude/channel" || n1["params"].(map[string]any)["content"] != "first" {
		t.Errorf("큐 순서 1: %v", n1)
	}
	if n2["params"].(map[string]any)["content"] != "second" {
		t.Errorf("큐 순서 2: %v", n2)
	}
	inw.Close()
	if err := <-done; err != nil {
		t.Errorf("EOF는 nil 종료: %v", err)
	}
}

func TestNotifyDropsBadMetaKeys(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	pr, pw := io.Pipe()
	var logged []string
	s := New(in, pw, "", "")
	s.Log = func(m string) { logged = append(logged, m) }
	go func() { _ = s.Run(context.Background()) }()
	br := bufio.NewReader(pr)
	readLine(t, br)
	// Notify는 out에 동기로 쓴다 — io.Pipe의 독자가 이 고루틴이므로 별도 고루틴에서 부른다.
	go s.Notify(Notification{Content: "x", Meta: map[string]string{"task": "T-1", "bad-key": "v", "9x": "v"}})
	n := readLine(t, br)
	meta := n["params"].(map[string]any)["meta"].(map[string]any)
	if _, ok := meta["bad-key"]; ok || meta["task"] != "T-1" || len(meta) != 1 {
		t.Errorf("meta 키 검증: %v", meta)
	}
	if len(logged) == 0 {
		t.Error("버린 키는 로그에 남아야 함")
	}
}

func TestPingToolsListAndUnknownMethod(t *testing.T) {
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n" +
			`{"jsonrpc":"2.0","id":3,"method":"tools/list"}` + "\n" +
			`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"x"}}` + "\n" +
			`not json` + "\n")
	pr, pw := io.Pipe()
	s := New(in, pw, "", "")
	go func() { _ = s.Run(context.Background()); pw.Close() }()
	br := bufio.NewReader(pr)
	readLine(t, br) // initialize
	if m := readLine(t, br); m["id"].(float64) != 2 || len(m["result"].(map[string]any)) != 0 {
		t.Errorf("ping: %v", m)
	}
	if m := readLine(t, br); len(m["result"].(map[string]any)["tools"].([]any)) != 0 {
		t.Errorf("tools/list 빈 목록: %v", m)
	}
	m := readLine(t, br)
	if e, ok := m["error"].(map[string]any); !ok || e["code"].(float64) != -32601 {
		t.Errorf("미지 메서드 -32601: %v", m)
	}
	if _, err := br.ReadString('\n'); err != io.EOF {
		t.Errorf("알림·깨진 줄엔 응답 없음, EOF 기대: %v", err)
	}
}

func TestRunStopsOnEOF(t *testing.T) {
	inr, inw := io.Pipe()
	s := New(inr, io.Discard, "", "")
	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()
	inw.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("EOF → nil: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("EOF 뒤 2초 안에 끝나야 함")
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	inr, _ := io.Pipe() // 아무것도 안 쓰는 stdin
	s := New(inr, io.Discard, "", "")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("ctx 취소 → context.Canceled: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("취소 뒤 2초 안에 끝나야 함")
	}
}
