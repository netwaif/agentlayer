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

// TestTrimSnapshotPreservesLargeAndStringIDs — 게이트 리뷰 1차: map[string]any 왕복이
// float64를 거치면 2^53을 넘는 id의 자릿수가 뭉개진다(9007199254740993 → ...992).
// UseNumber로 원본 자릿수를 그대로 들고 있는지, 문자열 id도 그대로 살아남는지 확인.
func TestTrimSnapshotPreservesLargeAndStringIDs(t *testing.T) {
	body := "ok\n## Latest page snapshot\nuid=1_0 RootWebArea\n"

	bigLine := []byte(`{"jsonrpc":"2.0","id":9007199254740993,"result":{"content":[{"type":"text","text":` + jsonStr(body) + `}]}}` + "\n")
	out := TrimSnapshot(bigLine, "wait_for")
	if !strings.Contains(string(out), `"id":9007199254740993`) {
		t.Fatalf("2^53 초과 id 자릿수 보존 실패: %s", out)
	}

	strLine := []byte(`{"jsonrpc":"2.0","id":"abc","result":{"content":[{"type":"text","text":` + jsonStr(body) + `}]}}` + "\n")
	out = TrimSnapshot(strLine, "wait_for")
	if !strings.Contains(string(out), `"id":"abc"`) {
		t.Fatalf("문자열 id 보존 실패: %s", out)
	}
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
