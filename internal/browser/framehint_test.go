package browser

import (
	"strings"
	"testing"
)

// 프록시 힌트(2026-09-23): 화면이 굳으면 chrome-devtools-mcp의 screenshot·click이 타임아웃으로
// 돌아온다. 그 에러 줄에 원인과 복구 명령을 덧붙여 에이전트가 "낡은 연결"로 오진하지 않게 한다.
func TestFrozenScreenHintAppendsToScreenshotTimeout(t *testing.T) {
	line := []byte(`{"jsonrpc":"2.0","id":7,"result":{"isError":true,"content":[{"type":"text","text":"Page.captureScreenshot timed out. Increase the 'protocolTimeout' setting"}]}}` + "\n")
	out := FrozenScreenHint(line)
	txt := ResultText(out)
	if !strings.Contains(txt, "agentlayer browser restart") || !strings.Contains(txt, "프레임") {
		t.Fatalf("힌트가 붙어야 한다: %q", txt)
	}
	if out[len(out)-1] != '\n' {
		t.Fatal("줄 끝 개행 유지")
	}
}

func TestFrozenScreenHintAppendsToClickTimeout(t *testing.T) {
	line := []byte(`{"jsonrpc":"2.0","id":8,"result":{"isError":true,"content":[{"type":"text","text":"Element did not become interactive within the configured timeout"}]}}` + "\n")
	if !strings.Contains(ResultText(FrozenScreenHint(line)), "agentlayer browser restart") {
		t.Fatal("click 타임아웃에도 힌트")
	}
}

func TestFrozenScreenHintLeavesOtherLinesAlone(t *testing.T) {
	for _, l := range []string{
		`{"jsonrpc":"2.0","id":9,"result":{"content":[{"type":"text","text":"ok"}]}}` + "\n",
		`{"jsonrpc":"2.0","id":10,"result":{"isError":true,"content":[{"type":"text","text":"No page with id 3"}]}}` + "\n",
		"not json\n",
	} {
		if got := string(FrozenScreenHint([]byte(l))); got != l {
			t.Errorf("바뀌면 안 된다: %q → %q", l, got)
		}
	}
}
