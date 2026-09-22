// internal/browser/pagemap.go
package browser

import (
	"encoding/json"
	"regexp"
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
}

// pageLineRe: "2: JustWatch - 신작 (https://www.justwatch.com/kr/new) [selected]"
var pageLineRe = regexp.MustCompile(`^(\d+): (.*?) \(([^()\s]+)\)( \[selected\])?\s*$`)

func (m *PageMap) Update(text string) bool {
	i := strings.Index(text, "## Pages")
	if i < 0 {
		return false
	}
	urls, titles := map[int]string{}, map[int]string{}
	selected := 0
	for _, ln := range strings.Split(text[i:], "\n")[1:] {
		mm := pageLineRe.FindStringSubmatch(ln)
		if mm == nil {
			continue
		}
		id, _ := strconv.Atoi(mm[1])
		urls[id], titles[id] = mm[3], mm[2]
		if mm[4] != "" {
			selected = id
		}
	}
	if len(urls) == 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.urls, m.titles, m.selected = urls, titles, selected
	return true
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
