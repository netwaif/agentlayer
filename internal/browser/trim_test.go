package browser

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrimSnapshot(t *testing.T) {
	body := "Element found\n## Latest page snapshot\nuid=1_0 RootWebArea\n  uid=1_1 button\n"
	line := []byte(`{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":` + jsonStr(body) + `}]}}` + "\n")
	out := TrimSnapshot(line, "wait_for")
	got := ResultText(out)
	if strings.Contains(got, "uid=1_1") || !strings.Contains(got, "Element found") || !strings.Contains(got, "스냅샷 생략") {
		t.Fatalf("잘라내기: %q", got)
	}
	if out[len(out)-1] != '\n' {
		t.Fatal("개행 유지")
	}
	if got := TrimSnapshot(line, "take_snapshot"); string(got) != string(line) {
		t.Fatal("take_snapshot은 그대로")
	}
	noMarker := []byte(`{"id":4,"result":{"content":[{"type":"text","text":"ok"}]}}` + "\n")
	if got := TrimSnapshot(noMarker, "wait_for"); string(got) != string(noMarker) {
		t.Fatal("마커 없으면 그대로")
	}
	if got := TrimSnapshot(line, "navigate_page"); strings.Contains(ResultText(got), "uid=1_1") {
		t.Fatal("navigate_page도 잘라냄")
	}
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
