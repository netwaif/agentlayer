// internal/browser/pagemap_test.go
package browser

import "testing"

const pagesSample = "Note: the previously selected page was closed. Page 1 is now selected.\n## Pages\n1: 계약서 미리보기 (file:///tmp/a.html)\n2: JustWatch - 신작 (https://www.justwatch.com/kr/new) [selected]\n"

func TestPageMapUpdateAndLookup(t *testing.T) {
	var m PageMap
	if m.Update("Successfully clicked on the element") {
		t.Fatal("목록 없는 응답은 false")
	}
	if !m.Update(pagesSample) {
		t.Fatal("목록 있는 응답은 true")
	}
	if u, ok := m.URLFor(2); !ok || u != "https://www.justwatch.com/kr/new" {
		t.Fatalf("URLFor(2) = %q %v", u, ok)
	}
	if u, ok := m.SelectedURL(); !ok || u != "https://www.justwatch.com/kr/new" {
		t.Fatalf("SelectedURL = %q %v", u, ok)
	}
	if m.TitleFor(1) != "계약서 미리보기" {
		t.Fatalf("TitleFor(1) = %q", m.TitleFor(1))
	}
	// Selected는 url과 제목을 같이 준다 — pageId 없는 호출(list_pages 등)의 띠에 제목이 뜨게
	if u, tt, ok := m.Selected(); !ok || u != "https://www.justwatch.com/kr/new" || tt != "JustWatch - 신작" {
		t.Fatalf("Selected = %q %q %v", u, tt, ok)
	}
	if _, ok := m.URLFor(9); ok {
		t.Fatal("없는 id")
	}
	// 재번호: 다음 목록이 오면 이전 표는 버린다
	m.Update("## Pages\n1: only (https://example.com) [selected]\n")
	if _, ok := m.URLFor(2); ok {
		t.Fatal("재번호 뒤 옛 id는 사라져야 함")
	}
}

func TestPageIDFromCallAndResultText(t *testing.T) {
	line := []byte(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"click","arguments":{"pageId":2,"uid":"1_1"}}}`)
	if id, ok := PageIDFromCall(line); !ok || id != 2 {
		t.Fatalf("PageIDFromCall = %d %v", id, ok)
	}
	if _, ok := PageIDFromCall([]byte(`{"method":"tools/call","params":{"name":"list_pages","arguments":{}}}`)); ok {
		t.Fatal("pageId 없으면 false")
	}
	resp := []byte(`{"jsonrpc":"2.0","id":7,"result":{"content":[{"type":"text","text":"a\n"},{"type":"text","text":"## Pages\n1: x (https://x) [selected]\n"}]}}`)
	if got := ResultText(resp); got != "a\n## Pages\n1: x (https://x) [selected]\n" {
		t.Fatalf("ResultText = %q", got)
	}
	if ResultText([]byte(`{"id":1,"result":{}}`)) != "" {
		t.Fatal("content 없으면 빈 문자열")
	}
}

func TestPageMapParenthesesInURL(t *testing.T) {
	// Wikipedia-style URL with parentheses
	m := &PageMap{}
	text := "## Pages\n1: Go Wiki (https://en.wikipedia.org/wiki/Go_(programming_language)) [selected]\n"
	if !m.Update(text) {
		t.Fatal("Wikipedia URL should parse")
	}
	u, ok := m.URLFor(1)
	if !ok || u != "https://en.wikipedia.org/wiki/Go_(programming_language)" {
		t.Fatalf("Wikipedia URL = %q %v", u, ok)
	}
	if m.TitleFor(1) != "Go Wiki" {
		t.Fatalf("Title = %q", m.TitleFor(1))
	}
}

func TestPageMapEmptyTitle(t *testing.T) {
	// Single-space empty title: "3: (https://example.com)"
	m := &PageMap{}
	text := "## Pages\n3: (https://example.com) [selected]\n"
	if !m.Update(text) {
		t.Fatal("Empty title should parse")
	}
	u, ok := m.URLFor(3)
	if !ok || u != "https://example.com" {
		t.Fatalf("URLFor(3) = %q %v", u, ok)
	}
	if m.TitleFor(3) != "" {
		t.Fatalf("TitleFor(3) should be empty, got %q", m.TitleFor(3))
	}
	sel, _ := m.SelectedURL()
	if sel != "https://example.com" {
		t.Fatalf("SelectedURL = %q", sel)
	}
}

func TestPageMapTitleWithParentheses(t *testing.T) {
	// Title containing parentheses
	m := &PageMap{}
	text := "## Pages\n2: My Title (extra) (https://example.com) [selected]\n"
	if !m.Update(text) {
		t.Fatal("Title with parens should parse")
	}
	u, ok := m.URLFor(2)
	if !ok || u != "https://example.com" {
		t.Fatalf("URLFor(2) = %q %v", u, ok)
	}
	if m.TitleFor(2) != "My Title (extra)" {
		t.Fatalf("TitleFor(2) = %q, want 'My Title (extra)'", m.TitleFor(2))
	}
}

func TestPageMapURLWithQueryString(t *testing.T) {
	// URL with query string and special characters
	m := &PageMap{}
	text := "## Pages\n4: Search (https://example.com/search?q=test&lang=en) [selected]\n"
	if !m.Update(text) {
		t.Fatal("URL with query string should parse")
	}
	u, ok := m.URLFor(4)
	if !ok || u != "https://example.com/search?q=test&lang=en" {
		t.Fatalf("URLFor(4) = %q %v", u, ok)
	}
}

func TestPageMapUnparsedLineTracking(t *testing.T) {
	// Garbage line inside the block, good lines should still parse
	m := &PageMap{}
	text := "## Pages\n1: Good Page (https://example.com)\ngarbage line here\n2: Another (https://another.com) [selected]\n"
	if !m.Update(text) {
		t.Fatal("Update should succeed with some good lines")
	}
	if u, ok := m.URLFor(1); !ok || u != "https://example.com" {
		t.Fatalf("URLFor(1) = %q %v", u, ok)
	}
	if u, ok := m.URLFor(2); !ok || u != "https://another.com" {
		t.Fatalf("URLFor(2) = %q %v", u, ok)
	}
	if m.Unparsed() != 1 {
		t.Fatalf("Unparsed() = %d, want 1", m.Unparsed())
	}
}

// "Page navigated to <url>." 응답으로 한 페이지의 url만 갱신한다(SetURL 주석).
func TestNavigatedURLAndSetURL(t *testing.T) {
	var m PageMap
	m.Update("## Pages\n1: 새 탭 (chrome://new-tab-page/)\n2: Wikipedia (https://www.wikipedia.org/) [selected]\n")
	u, ok := NavigatedURL("Successfully clicked on the element\nPage navigated to https://ko.wikipedia.org/wiki/%EC%9D%B8.\n")
	if !ok || u != "https://ko.wikipedia.org/wiki/%EC%9D%B8" {
		t.Fatalf("이동 url을 뽑아야 한다: %q %v", u, ok)
	}
	if _, ok := NavigatedURL("Successfully clicked on the element"); ok {
		t.Fatal("문구가 없으면 false")
	}
	if _, ok := NavigatedURL("Page navigated to chrome://newtab."); ok {
		t.Fatal("웹 url이 아니면 false")
	}
	m.SetURL(2, u)
	if got, _ := m.URLFor(2); got != u {
		t.Fatalf("URLFor(2)=%q", got)
	}
	if m.TitleFor(2) != "Wikipedia" {
		t.Fatalf("제목은 그대로여야 한다: %q", m.TitleFor(2))
	}
	if got, _ := m.URLFor(1); got != "chrome://new-tab-page/" {
		t.Fatalf("다른 페이지는 그대로: %q", got)
	}
}
