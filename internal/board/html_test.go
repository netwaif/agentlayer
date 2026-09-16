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
	if strings.Contains(h, "<script src") || strings.Contains(h, "<link ") {
		t.Error("외부 로드 없는 단일 파일이어야 함")
	}
	for _, want := range []string{`id="q"`, `id="who"`, `<option value="collab-bot">`, `data-generated="`, `id="reload"`, `id="auto"`, `id="gen"`, `data-who="-"`, `data-text="a &lt;script&gt;alert(1)&lt;/script&gt;`, `class="chip" data-col="ready"`} {
		if !strings.Contains(h, want) {
			t.Errorf("검색·필터 재료 %q 없음", want)
		}
	}
}

func TestHTMLEmptyBoardSaysSo(t *testing.T) {
	h := string(HTML("빈 회사", nil, now, time.Minute))
	if !strings.Contains(h, "업무 없음") {
		t.Error("빈 보드 안내 없음")
	}
	if !strings.Contains(h, `id="reload"`) || !strings.Contains(h, "location.reload()") {
		t.Error("빈 보드도 새로고침·자동 갱신 스크립트가 있어야 함")
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
}

func TestHTMLDoneColumnCapped(t *testing.T) {
	var cards []Card
	for i := 0; i < maxDoneShown+5; i++ {
		cards = append(cards, Card{ID: fmt.Sprintf("D-%02d", i), Column: ColDone, Updated: now})
	}
	h := string(HTML("회사", cards, now, time.Hour))
	if !strings.Contains(h, "외 5건 더 보기") {
		t.Error("Done 열 더 보기 버튼 없음")
	}
	// 최신(ID 큰 쪽)이 펼쳐지고 오래된 것은 folded(검색에 걸리면 다시 보인다)
	if !strings.Contains(h, `class="card" href="#D-16"`) || !strings.Contains(h, `class="card folded" href="#D-00"`) {
		t.Error("Done 열은 최신 12장만 펼치고 나머지는 folded여야 함")
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

// 필터(검색·담당·방치만)가 카드를 전부 숨기면 "N장 숨김" 경고와 초기화 강조가 나와야 한다 —
// 검색어가 localStorage에 남아 다음 열림에도 빈 보드로 보이던 문제의 회귀 방지.
func TestHTMLFilterNoMatchWarns(t *testing.T) {
	out := string(HTML("회사", []Card{{ID: "A-1", Title: "a", Status: "done", Column: ColDone}}, time.Now(), 30*time.Minute))
	for _, want := range []string{"조건에 맞는 카드 없음 — '+total+'장 숨김", "shown.classList.toggle('warn',none)", "#shown.warn{", ".tools.nomatch #clear{"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
}
