// internal/browser/pagemap.go
package browser

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
)

// PageMap — chrome-devtools MCP의 pageId(서버 내부 번호)를 url·제목으로 잇는 표.
// 서버는 list_pages·new_page·select_page·close_page·navigate_page 응답에 "## Pages" 목록을
// 실어 보내므로 그때마다 표를 통째로 바꾼다(닫히면 재번호되기 때문).
type PageMap struct {
	mu       sync.Mutex
	urls     map[int]string
	titles   map[int]string
	selected int
	unparsed int
}

func (m *PageMap) Update(text string) bool {
	i := strings.Index(text, "## Pages")
	if i < 0 {
		return false
	}
	urls, titles := map[int]string{}, map[int]string{}
	selected := 0
	unparsed := 0
	for _, ln := range strings.Split(text[i:], "\n")[1:] {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		// 줄의 형식: "N: title (url) [selected]" 또는 "N: (url) [selected]"
		// N을 추출
		colonIdx := strings.Index(ln, ": ")
		if colonIdx < 0 {
			unparsed++
			continue
		}
		idStr := ln[:colonIdx]
		id, err := strconv.Atoi(idStr)
		if err != nil {
			unparsed++
			continue
		}
		rest := ln[colonIdx+2:]

		// 뒤에서 [selected]를 제거
		if strings.HasSuffix(rest, " [selected]") {
			rest = rest[:len(rest)-len(" [selected]")]
			selected = id
		}
		rest = strings.TrimRight(rest, " ")

		// rest는 이제 "title (url)" 형식
		// 반드시 )로 끝나야 함
		if !strings.HasSuffix(rest, ")") {
			unparsed++
			continue
		}

		// ( 의 마지막 위치를 찾음
		lastParen := strings.LastIndex(rest, " (")
		if lastParen < 0 {
			// rest가 "("로 시작하는 경우: title 비어있음
			if strings.HasPrefix(rest, "(") {
				url := rest[1 : len(rest)-1] // "("와 ")" 사이
				urls[id] = url
				titles[id] = ""
			} else {
				unparsed++
				continue
			}
		} else {
			// "title (url)" 형식
			title := rest[:lastParen]
			url := rest[lastParen+2 : len(rest)-1] // " ("의 2글자 뒤에서 ")" 앞까지
			urls[id] = url
			titles[id] = title
		}
	}
	if len(urls) == 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.urls, m.titles, m.selected = urls, titles, selected
	m.unparsed = unparsed
	return true
}

func (m *PageMap) Unparsed() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unparsed
}

func (m *PageMap) URLFor(id int) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.urls[id]
	return u, ok
}

func (m *PageMap) SelectedURL() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.urls[m.selected]
	return u, ok
}

// Selected — [selected] 탭의 url과 제목. pageId 없는 호출(list_pages·new_page 등)의
// 작업 탭 추정에 쓴다. 제목까지 같이 줘야 다른 탭의 띠가 "AI가 다른 탭에서 작업 중 · "로
// 제목 없이 뜨지 않는다(실측 지적).
func (m *PageMap) Selected() (url, title string, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.urls[m.selected]
	return u, m.titles[m.selected], ok
}

// SetURL — 한 페이지의 url만 바꾼다(제목은 그대로). 클릭·입력으로 페이지가 이동하면
// 서버 응답에 "Page navigated to <url>."이 실리는데 "## Pages" 목록은 안 오므로, 그
// 문구로 표를 바로 고쳐야 다음 호출의 작업 탭 판정(url 일치)이 옛 주소에 묶이지 않는다.
// 교차 사이트 이동에서 Chrome이 탭의 타깃 id를 바꾸는 경우(프리렌더 활성화 등)엔
// fx.go의 기억한 id도 못 쓰므로 이 url 갱신이 유일한 실마리다(2026-09-24 드릴 실측).
func (m *PageMap) SetURL(id int, url string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.urls == nil {
		m.urls = map[int]string{}
	}
	m.urls[id] = url
}

// NavigatedURL — 도구 응답 본문의 "Page navigated to <url>." 문구에서 url을 뽑는다.
func NavigatedURL(text string) (string, bool) {
	const marker = "Page navigated to "
	i := strings.Index(text, marker)
	if i < 0 {
		return "", false
	}
	rest := text[i+len(marker):]
	if j := strings.IndexAny(rest, " \n\r\t"); j >= 0 {
		rest = rest[:j]
	}
	rest = strings.TrimSuffix(rest, ".")
	if !IsWebURL(rest) {
		return "", false
	}
	return rest, true
}

func (m *PageMap) TitleFor(id int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.titles[id]
}

// RPCID — JSON-RPC 줄의 id를 원문 그대로(숫자든 문자열이든) 돌려준다. 없으면 "".
// 요청과 응답을 짝지어 "이 응답이 어느 pageId를 향한 호출의 것인지" 알 때 쓴다.
func RPCID(line []byte) string {
	var msg struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(line, &msg) != nil {
		return ""
	}
	return string(msg.ID)
}

// PageIDFromCall — tools/call의 params.arguments.pageId. 없으면 false.
func PageIDFromCall(line []byte) (int, bool) {
	var msg struct {
		Params struct {
			Arguments struct {
				PageID *int `json:"pageId"`
			} `json:"arguments"`
		} `json:"params"`
	}
	if json.Unmarshal(line, &msg) != nil || msg.Params.Arguments.PageID == nil {
		return 0, false
	}
	return *msg.Params.Arguments.PageID, true
}

// ResultText — JSON-RPC 응답의 result.content[].text를 이어 붙인다. 없으면 "".
func ResultText(line []byte) string {
	var msg struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if json.Unmarshal(line, &msg) != nil {
		return ""
	}
	var sb strings.Builder
	for _, c := range msg.Result.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	return sb.String()
}
