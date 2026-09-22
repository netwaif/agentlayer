package browser

import (
	"bytes"
	"encoding/json"
	"strings"
)

// 화면 굳음 힌트(2026-09-23). 프레임이 안 나오는 브라우저에서 chrome-devtools-mcp는
// screenshot을 "Page.captureScreenshot timed out", click을 "did not become interactive within
// the configured timeout"으로 돌려준다. 에이전트가 이걸 요소·연결 문제로 오진해 재시도를
// 반복하지 않게, 그 isError 결과 줄에 원인과 복구 명령을 덧붙인다. 다른 줄은 바이트 그대로.
const frozenScreenHint = "\n\n[agentlayer] 에이전트 브라우저가 화면 프레임을 못 내는 상태(창 전체가 그림으로 굳음)일 수 있습니다. 요소를 바꿔 재시도하지 말고 `agentlayer browser restart`로 재시작한 뒤 new_page부터 다시 하세요(행 감시가 곧 자동 재시작하기도 합니다)."

var frozenScreenMarkers = []string{
	"Page.captureScreenshot timed out",
	"did not become interactive within the configured timeout",
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
