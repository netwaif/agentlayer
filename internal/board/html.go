// internal/board/html.go
package board

import (
	"fmt"
	"html"
	"strings"
	"time"
)

var colTitle = map[string]string{ColTodo: "Todo", ColReady: "Ready", ColRunning: "Running", ColBlocked: "Blocked", ColReview: "Review", ColDone: "Done"}

// HTML은 정적 보드 페이지. 자바스크립트 없음 — 새로고침은 `agentlayer board` 재실행.
// 색은 AgentLoops 하우스 팔레트(카드·TUI와 같은 계열): 바탕 #0B0D12, 패널 #151923, 글자 #E6E8EE, 흐림 #8B93A7, 강조 #5865F2, 경고 #F0B232.
func HTML(name string, cards []Card, now time.Time, stale time.Duration) []byte {
	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html lang=\"ko\"><head><meta charset=\"utf-8\"><title>")
	sb.WriteString(html.EscapeString(name))
	sb.WriteString(" — 업무 보드</title><style>\n")
	sb.WriteString(`body{margin:0;background:#0B0D12;color:#E6E8EE;font:14px/1.5 -apple-system,"Apple SD Gothic Neo",sans-serif;padding:24px}
h1{font-size:18px;margin:0 0 4px}.meta{color:#8B93A7;font-size:12px;margin-bottom:20px}
.board{display:grid;grid-template-columns:repeat(6,minmax(180px,1fr));gap:12px;overflow-x:auto}
.col{background:#151923;border-radius:10px;padding:10px;min-height:120px}.col h2{font-size:12px;color:#8B93A7;letter-spacing:.06em;text-transform:uppercase;margin:0 0 10px}
.card{background:#0B0D12;border:1px solid #232838;border-radius:8px;padding:8px 10px;margin-bottom:8px}.card .id{font-weight:600}.card .t{color:#C7CCD8}
.card .s{color:#8B93A7;font-size:12px}.card .log{color:#8B93A7;font-size:12px;margin-top:4px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.warn{color:#F0B232}.empty{color:#8B93A7;padding:40px 0}
</style></head><body>`)
	fmt.Fprintf(&sb, "<h1>%s — 업무 보드</h1><div class=\"meta\">%s 생성 · 새로고침은 <code>agentlayer board</code></div>",
		html.EscapeString(name), now.Format("2006-01-02 15:04"))
	if len(cards) == 0 {
		sb.WriteString("<div class=\"empty\">업무 없음 — tasks/&lt;ID&gt;/task.md가 없습니다.</div></body></html>\n")
		return []byte(sb.String())
	}
	sb.WriteString("<div class=\"board\">")
	for _, col := range Columns {
		fmt.Fprintf(&sb, "<section class=\"col\" data-col=\"%s\"><h2>%s</h2>", col, colTitle[col])
		for _, c := range cards {
			if c.Column != col {
				continue
			}
			sb.WriteString("<div class=\"card\"><div class=\"id\">" + html.EscapeString(c.ID))
			if c.Unknown {
				sb.WriteString(" <span class=\"warn\" title=\"status 값을 해석하지 못함\">?</span>")
			}
			if StaleReady(c, now, stale) {
				sb.WriteString(" <span class=\"warn\">⚠</span>")
			}
			sb.WriteString("</div>")
			if c.Title != "" && c.Title != c.ID {
				sb.WriteString("<div class=\"t\">" + html.EscapeString(c.Title) + "</div>")
			}
			var s []string
			if c.Session != "" {
				s = append(s, html.EscapeString(c.Session))
			}
			if c.State != "" {
				s = append(s, c.State+" "+ago(c.Updated, now))
			} else if col == ColReady {
				s = append(s, "ready "+ago(c.Ready, now))
			}
			if len(c.Parents) > 0 {
				s = append(s, "← "+html.EscapeString(strings.Join(c.Parents, ", ")))
			}
			if len(s) > 0 {
				sb.WriteString("<div class=\"s\">" + strings.Join(s, " · ") + "</div>")
			}
			if c.LastLog != "" {
				sb.WriteString("<div class=\"log\" title=\"" + html.EscapeString(c.LastLog) + "\">" + html.EscapeString(c.LastLog) + "</div>")
			}
			sb.WriteString("</div>")
		}
		sb.WriteString("</section>")
	}
	sb.WriteString("</div></body></html>\n")
	return []byte(sb.String())
}

func ago(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "방금"
	case d < time.Hour:
		return fmt.Sprintf("%d분", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d시간", int(d.Hours()))
	}
	return fmt.Sprintf("%d일", int(d.Hours()/24))
}
