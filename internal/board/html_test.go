// internal/board/html_test.go
package board

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestHTMLHasSixColumnsCardsAndEscapes(t *testing.T) {
	cards := []Card{
		{ID: "A", Title: "<script>alert(1)</script>", Column: ColReady, Ready: now.Add(-time.Hour), Parents: []string{"Z"}},
		{ID: "B", Title: "실행 중", Column: ColRunning, Session: "collab-bot", State: "WORK", Updated: now, LastLog: "[..] [SEND] 지시 & 답"},
		{ID: "C", Title: "이상", Status: "weird", Column: ColTodo, Unknown: true},
	}
	h := string(HTML("AI 치트키 회사", cards, now, 30*time.Minute))
	for _, want := range []string{"<!doctype html>", "AI 치트키 회사", "&lt;script&gt;", "&amp; 답",
		`id="col-todo" data-col="todo"`, `data-col="ready"`, `data-col="running"`, `data-col="blocked"`, `data-col="review"`, `data-col="done"`,
		"collab-bot", "WORK", "← Z", "!!!", ">?</span>", `title="status 값을 해석하지 못함: weird"`,
		// 카드 → 상세 패널 앵커, 패널에 부모 링크·기록
		`href="#A"`, `<aside class="detail" id="A">`, `href="#Z"`, "&amp; 답"} {
		if !strings.Contains(h, want) {
			t.Errorf("HTML에 %q 없음", want)
		}
	}
	if strings.Contains(h, "<script>alert") {
		t.Error("이스케이프 실패")
	}
	if strings.Contains(h, "<script") {
		t.Error("자바스크립트 없음(정적 페이지)")
	}
}

func TestHTMLEmptyBoardSaysSo(t *testing.T) {
	h := string(HTML("빈 회사", nil, now, time.Minute))
	if !strings.Contains(h, "업무 없음") {
		t.Error("빈 보드 안내 없음")
	}
}

func TestHTMLDetailPanelHasBodyLogAndChildren(t *testing.T) {
	cards := []Card{
		{ID: "P", Title: "부모", Column: ColDone, Body: "## 목표\n초안 쓰기\n\n## 완료 기준\n- [x] 수집\n- [ ] 선정 <b>", Log: []string{"[2026-09-16 09:14] [ASSIGN] collab-bot:t1", "[2026-09-16 09:31] [ASK] 유료도?"}},
		{ID: "C", Title: "자식", Column: ColReady, Parents: []string{"P"}, Ready: now.Add(-2 * time.Hour)},
	}
	h := string(HTML("회사", cards, now, 30*time.Minute))
	for _, want := range []string{
		`<aside class="detail" id="P">`, "<h5>목표</h5>", "<h5>완료 기준</h5>", `<li class="done"><span class="box">☑</span>수집</li>`, "☐</span>선정 &lt;b&gt;",
		`class="tag mono tag-assign">ASSIGN</span>`, `class="ts mono">2026-09-16 09:31</span>`, "유료도?",
		"<dt>자식</dt><dd><span class=\"mono\"><a href=\"#C\">C</a></span></dd>", // 부모 패널에 자식 링크
		"<dt>부모</dt><dd><span class=\"mono\"><a href=\"#P\">P</a></span></dd>", // 자식 패널에 부모 링크
		"1건이 30분 넘게 방치", `class="card stale" href="#C"`, "ready인데 아무도 배정하지 않음",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("HTML에 %q 없음", want)
		}
	}
	if strings.Contains(h, "<script") {
		t.Error("자바스크립트 없음(정적 페이지)")
	}
}

func TestHTMLDoneColumnCapped(t *testing.T) {
	var cards []Card
	for i := 0; i < maxDoneShown+5; i++ {
		cards = append(cards, Card{ID: fmt.Sprintf("D-%02d", i), Column: ColDone, Updated: now})
	}
	h := string(HTML("회사", cards, now, time.Hour))
	if !strings.Contains(h, "외 5건") {
		t.Error("Done 열 접힘 안내 없음")
	}
	// 최신(ID 큰 쪽)이 남고 오래된 것이 접힌다 — 카드 앵커 기준(상세 패널은 전부 있다)
	if !strings.Contains(h, `class="card" href="#D-16"`) || strings.Contains(h, `class="card" href="#D-00"`) {
		t.Error("Done 열은 최신 카드만 펼쳐야 함")
	}
	if !strings.Contains(h, `id="D-00"`) {
		t.Error("접힌 카드도 상세 패널은 있어야 함")
	}
}

func TestSplitLogAndRenderBodyEdges(t *testing.T) {
	ts, tag, text := splitLog("[2026-09-16 09:29] [REPORT] DONE: (요약 없음)")
	if ts != "2026-09-16 09:29" || tag != "REPORT" || text != "DONE: (요약 없음)" {
		t.Errorf("splitLog = %q %q %q", ts, tag, text)
	}
	if _, tag, text := splitLog("형식 없는 줄"); tag != "" || text != "형식 없는 줄" {
		t.Errorf("형식 없는 줄: %q %q", tag, text)
	}
	if got := logTail("[2026-09-16 09:29] [SEND] a⏎b"); got != "[SEND] a ⏎ b" {
		t.Errorf("logTail = %q", got)
	}
	if got := renderBody("```\n<x>\n```"); !strings.Contains(got, "<pre class=\"mono\">&lt;x&gt;\n</pre>") {
		t.Errorf("코드 블록 렌더: %q", got)
	}
}
