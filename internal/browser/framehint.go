package browser

import (
	"bytes"
	"encoding/json"
	"strings"
)

// 화면 굳음 힌트(2026-09-23). 프레임이 안 나오는 브라우저에서 chrome-devtools-mcp는
// screenshot을 "Page.captureScreenshot timed out"으로 돌려준다. 에이전트가 이걸 요소·연결
// 문제로 오진해 재시도를 반복하지 않게, 그 isError 결과 줄에 상황 설명을 덧붙인다.
//
// 2026-09-24 사고: 처음엔 click의 "did not become interactive within the configured timeout"도
// 굳음 표식으로 보고 힌트에 `agentlayer browser restart`를 적어 뒀다. 그런데 그 문구는 무거운
// SPA(CGV 예매)가 평범하게 느릴 때도 나오고, 에이전트(Codex)는 힌트대로 촬영 중인 브라우저를
// 8번 재시작했다. 그래서 ① click 타임아웃은 표식에서 뺐고 ② 힌트는 재시작을 시키지 않는다 —
// 굳음의 실제 판정과 재시작은 프레임 프로브를 3회 연속 확인하는 행 감시(hangwatch.go)만 한다.
const frozenScreenHint = "\n\n[agentlayer] 이 타임아웃은 페이지가 느린 것일 수도, 브라우저가 화면 프레임을 못 내는 상태일 수도 있습니다. 브라우저를 직접 재시작하지 마세요(행 감시가 굳음을 확인하면 자동으로 재시작합니다). 5초쯤 뒤 같은 호출을 한 번만 다시 시도하고, 그래도 같으면 사용자에게 알리고 멈추세요."

var frozenScreenMarkers = []string{
	"Page.captureScreenshot timed out",
}

func FrozenScreenHint(line []byte) []byte {
	var msg struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if json.Unmarshal(line, &msg) != nil || !msg.Result.IsError {
		return line
	}
	hit := false
	for _, c := range msg.Result.Content {
		for _, m := range frozenScreenMarkers {
			if c.Type == "text" && strings.Contains(c.Text, m) {
				hit = true
			}
		}
	}
	if !hit {
		return line
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(line, &raw) != nil {
		return line
	}
	var res map[string]json.RawMessage
	if json.Unmarshal(raw["result"], &res) != nil {
		return line
	}
	var content []map[string]any
	if json.Unmarshal(res["content"], &content) != nil {
		return line
	}
	for _, c := range content {
		if c["type"] == "text" {
			if t, ok := c["text"].(string); ok {
				c["text"] = t + frozenScreenHint
				break
			}
		}
	}
	cb, _ := json.Marshal(content)
	res["content"] = cb
	rb, _ := json.Marshal(res)
	raw["result"] = rb
	out, err := json.Marshal(raw)
	if err != nil {
		return line
	}
	if bytes.HasSuffix(line, []byte("\n")) {
		out = append(out, '\n')
	}
	return out
}
