// internal/board/html_test.go
package board

import (
	"strings"
	"testing"
	"time"
)

func TestHTMLHasSixColumnsCardsAndEscapes(t *testing.T) {
	cards := []Card{
		{ID: "A", Title: "<script>alert(1)</script>", Column: ColReady, Ready: now.Add(-time.Hour), Parents: []string{"Z"}},
		{ID: "B", Title: "실행 중", Column: ColRunning, Session: "collab-bot", State: "WORK", Updated: now, LastLog: "[..] [SEND] 지시 & 답"},
		{ID: "C", Title: "이상", Column: ColTodo, Unknown: true},
	}
	h := string(HTML("AI 치트키 회사", cards, now, 30*time.Minute))
	for _, want := range []string{"<!doctype html>", "AI 치트키 회사", "&lt;script&gt;", "&amp; 답",
		`class="col" data-col="todo"`, `data-col="ready"`, `data-col="running"`, `data-col="blocked"`, `data-col="review"`, `data-col="done"`,
		"collab-bot", "WORK", "← Z", "⚠", "?"} {
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
