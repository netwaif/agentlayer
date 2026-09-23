package browser

import (
	"strings"
	"testing"
)

// 프록시 힌트(2026-09-23, 2026-09-24 개정): 화면이 굳으면 chrome-devtools-mcp의 screenshot이
// 타임아웃으로 돌아온다. 그 에러 줄에 상황 설명을 덧붙이되, 에이전트에게 재시작을 시키지는
// 않는다(Codex가 힌트대로 촬영 중 브라우저를 8번 재시작한 사고). 재시작은 행 감시만 한다.
func TestFrozenScreenHintAppendsToScreenshotTimeout(t *testing.T) {
	line := []byte(`{"jsonrpc":"2.0","id":7,"result":{"isError":true,"content":[{"type":"text","text":"Page.captureScreenshot timed out. Increase the 'protocolTimeout' setting"}]}}` + "\n")
	out := FrozenScreenHint(line)
	txt := ResultText(out)
	if !strings.Contains(txt, "[agentlayer]") || !strings.Contains(txt, "프레임") {
		t.Fatalf("힌트가 붙어야 한다: %q", txt)
	}
	if strings.Contains(txt, "browser restart") {
		t.Fatalf("힌트가 에이전트에게 재시작을 시키면 안 된다: %q", txt)
	}
	if !strings.Contains(txt, "재시작하지 마세요") {
		t.Fatalf("직접 재시작 금지를 명시해야 한다: %q", txt)
	}
	if out[len(out)-1] != '\n' {
		t.Fatal("줄 끝 개행 유지")
	}
}

// click의 "did not become interactive"는 무거운 SPA가 평범하게 느릴 때도 나오므로 굳음
// 표식이 아니다 — 줄을 그대로 둔다.
func TestFrozenScreenHintIgnoresClickTimeout(t *testing.T) {
	line := `{"jsonrpc":"2.0","id":8,"result":{"isError":true,"content":[{"type":"text","text":"Element did not become interactive within the configured timeout"}]}}` + "\n"
	if got := string(FrozenScreenHint([]byte(line))); got != line {
		t.Fatalf("click 타임아웃은 손대면 안 된다: %q", got)
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
