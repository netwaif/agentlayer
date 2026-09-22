package browser

import (
	"bytes"
	"encoding/json"
	"strings"
)

// 호출 효율(스펙 5절): wait_for·navigate_page 응답에 딸려오는 페이지 전체 스냅샷을 잘라낸다.
// 한 번에 1만 5천 토큰이 들어온 실측(2026-09-22). 필요하면 에이전트가 take_snapshot을 따로 부른다.

var trimTools = map[string]bool{"wait_for": true, "navigate_page": true}

const (
	snapshotMarker = "## Latest page snapshot"
	trimNotice     = "(스냅샷 생략 — 필요하면 take_snapshot)"
)

// TrimSnapshot은 서버→클라이언트 한 줄(JSON-RPC)에서, tool이 잘라낼 대상이고
// 텍스트에 스냅샷 마커가 있으면 그 이후를 안내 문구로 대체한다. 그 외(대상 도구가
// 아니거나 마커가 없음)는 입력을 그대로 돌려준다(같은 바이트).
func TrimSnapshot(line []byte, tool string) []byte {
	if !trimTools[tool] {
		return line
	}
	// UseNumber: id 등 숫자 필드를 float64로 굴리지 않고 원본 자릿수 그대로 json.Number로
	// 들고 있다가 재마샬링한다. 2^53을 넘는 JSON-RPC id가 있으면 float64 왕복에서
	// 자릿수가 뭉개져(예 9007265... 계열) 클라이언트의 요청/응답 짝짓기가 깨질 수 있다.
	var msg map[string]any
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	if dec.Decode(&msg) != nil {
		return line
	}
	result, _ := msg["result"].(map[string]any)
	content, _ := result["content"].([]any)
	changed := false
	for _, it := range content {
		item, _ := it.(map[string]any)
		text, _ := item["text"].(string)
		if i := strings.Index(text, snapshotMarker); item["type"] == "text" && i >= 0 {
			item["text"] = text[:i] + trimNotice + "\n"
			changed = true
		}
	}
	if !changed {
		return line
	}
	out, err := json.Marshal(msg)
	if err != nil {
		return line
	}
	return append(out, '\n')
}
