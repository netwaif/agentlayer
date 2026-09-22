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

func (m *PageMap) TitleFor(id int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.titles[id]
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
