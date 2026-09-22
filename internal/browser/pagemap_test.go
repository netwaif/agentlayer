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
