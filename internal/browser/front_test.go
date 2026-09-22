package browser

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestRestoreFrontRemembersAndActivates(t *testing.T) {
	frontSleep = func(time.Duration) {}
	t.Cleanup(func() { frontSleep = time.Sleep })
	var activated []string
	ops := FrontOps{
		Frontmost: func() (string, error) { return "iTerm2", nil },
		Activate:  func(n string) error { activated = append(activated, n); return nil },
	}
	launched := false
	if err := RestoreFront(ops, func() error { launched = true; return nil }); err != nil || !launched {
		t.Fatal(err)
	}
	if len(activated) != 1 || activated[0] != "iTerm2" {
		t.Fatalf("복원: %v", activated)
	}
}

func TestRestoreFrontSkipsWhenBrowserWasFront(t *testing.T) {
	frontSleep = func(time.Duration) {}
	t.Cleanup(func() { frontSleep = time.Sleep })
	var activated []string
	ops := FrontOps{
		Frontmost: func() (string, error) { return EngineAppName, nil },
		Activate:  func(n string) error { activated = append(activated, n); return nil },
	}
	_ = RestoreFront(ops, func() error { return nil })
	if len(activated) != 0 {
		t.Fatal("브라우저가 앞이었으면 복원 안 함")
	}
}

func TestRestoreFrontSwallowsOpsErrors(t *testing.T) {
	frontSleep = func(time.Duration) {}
	t.Cleanup(func() { frontSleep = time.Sleep })
	ops := FrontOps{
		Frontmost: func() (string, error) { return "", errors.New("no osascript") },
		Activate:  func(string) error { return errors.New("x") },
	}
	if err := RestoreFront(ops, func() error { return nil }); err != nil {
		t.Fatal("ops 실패는 삼킨다")
	}
	if err := RestoreFront(ops, func() error { return errors.New("launch") }); err == nil {
		t.Fatal("launch 실패는 그대로")
	}
}

func TestRewriteNewPage(t *testing.T) {
	in := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"new_page","arguments":{"url":"https://x"}}}` + "\n")
	out := RewriteNewPage(in, false)
	var msg struct {
		Params struct {
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
	}
	if err := json.Unmarshal(out, &msg); err != nil || msg.Params.Arguments["background"] != true {
		t.Fatalf("background 채움: %s (%v)", out, err)
	}
	if out[len(out)-1] != '\n' {
		t.Fatal("개행 유지")
	}
	if got := RewriteNewPage(in, true); string(got) != string(in) {
		t.Fatal("브라우저가 앞이면 그대로")
	}
	explicit := []byte(`{"method":"tools/call","params":{"name":"new_page","arguments":{"url":"https://x","background":false}}}` + "\n")
	if got := RewriteNewPage(explicit, false); string(got) != string(explicit) {
		t.Fatal("명시된 값은 존중")
	}
	other := []byte(`{"method":"tools/call","params":{"name":"click","arguments":{"pageId":1}}}` + "\n")
	if got := RewriteNewPage(other, false); string(got) != string(other) {
		t.Fatal("다른 도구는 그대로")
	}
}
