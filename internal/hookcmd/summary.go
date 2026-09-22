package hookcmd

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// taskSummaryMax는 Task(최근 작업)에 싣는 마지막 답변 요약의 최대 길이(룬).
const taskSummaryMax = 120

// summarizeMessage는 에이전트의 마지막 답변(Claude Stop·Codex Stop의 last_assistant_message,
// codex notify의 last-assistant-message)을 한 줄 요약으로 줄인다 — 첫 비어 있지 않은 줄에서
// 마크다운 장식(굵게·코드·머리표·제목 기호)을 벗기고 taskSummaryMax 룬에서 말줄임한다.
// 이 값이 Agent.Task가 되어 status·보드 카드·총괄 수신함 [REPORT] DONE: 요약에 실린다.
func summarizeMessage(msg string) string {
	if isTitleJSON(msg) {
		return ""
	}
	for _, line := range strings.Split(msg, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "#>-*• \t")
		line = strings.NewReplacer("**", "", "`", "", "__", "").Replace(line)
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) > taskSummaryMax {
			r := []rune(line)
			line = string(r[:taskSummaryMax-1]) + "…"
		}
		return line
	}
	return ""
}

// isTitleJSON은 codex가 첫 턴 뒤 스레드 제목을 만들며 notify에 싣는 `{"title":"…"}`(codex 0.155.x,
// WSL2 실기 2026-09-22 — status 최근 작업 열에 JSON 그대로 찍힘)인지 본다. 키가 title 하나뿐인
// JSON 객체만 답변이 아닌 것으로 무시한다 — 진짜 JSON 답변까지 버리지 않게 좁게 잡는다.
func isTitleJSON(msg string) bool {
	msg = strings.TrimSpace(msg)
	if !strings.HasPrefix(msg, "{") {
		return false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(msg), &m) != nil {
		return false
	}
	_, ok := m["title"]
	return ok && len(m) == 1
}
