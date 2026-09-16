// internal/board/html.go
package board

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"
)

var colTitle = map[string]string{ColTodo: "Todo", ColReady: "Ready", ColRunning: "Running", ColBlocked: "Blocked", ColReview: "Review", ColDone: "Done"}

// colDesc는 열 머리 아래 한 줄 설명 — Hermes 칸반처럼 열의 뜻을 보드 안에서 읽게 한다.
var colDesc = map[string]string{
	ColTodo:    "부모 업무가 끝나길 기다리는 중",
	ColReady:   "부모 전부 완료 — 배정 가능",
	ColRunning: "직원이 작업 중",
	ColBlocked: "직원이 대표·총괄의 입력을 기다림",
	ColReview:  "완료 보고 도착 — 총괄 검토 대기",
	ColDone:    "완료",
}

// maxDoneShown — Done 열은 오래된 회사일수록 끝없이 길어지므로 최근 N장만 펼치고 나머지는 접는다.
const maxDoneShown = 12

// HTML은 정적 보드 페이지. 자바스크립트 없음 — 새로고침은 `agentlayer board` 재실행. 카드를 누르면
// CSS :target으로 오른쪽 상세 패널이 열린다(task.md 본문·log.md 전체·부모/자식).
// 색은 AgentLoops 하우스 웜다크(관제탑 카드·슬라이드와 같은 계열): 바탕 #1f1e1d, 패널 #262624,
// 카드 #2d2c2a, 글자 #faf9f5, 흐림 #a8a69d, 강조 테라코타 #d97757.
func HTML(name string, cards []Card, now time.Time, stale time.Duration) []byte {
	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html lang=\"ko\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>")
	sb.WriteString(html.EscapeString(name))
	sb.WriteString(" — 업무 보드</title><style>\n")
	sb.WriteString(boardCSS)
	sb.WriteString("</style></head><body>\n")

	cnt := Counts(cards)
	var staleIDs []string
	for _, c := range cards {
		if StaleReady(c, now, stale) {
			staleIDs = append(staleIDs, c.ID)
		}
	}

	// 머리: 회사명 · 업무 수 · 생성 시각 · 새로고침 안내
	sb.WriteString("<header class=\"top\"><div><div class=\"eyebrow\">업무 보드</div><h1>" + html.EscapeString(name) + "</h1></div>")
	fmt.Fprintf(&sb, "<div class=\"top-meta\"><span class=\"mono\">%d 업무</span><span class=\"sep\">·</span><span class=\"mono\">%s</span><span class=\"sep\">·</span><span class=\"hint\">새로고침 <code>agentlayer board</code> 또는 관제탑 <kbd>t</kbd></span></div></header>\n",
		len(cards), now.Format("2006-01-02 15:04"))

	if len(cards) == 0 {
		sb.WriteString("<div class=\"empty-board\"><div class=\"mono dim\">— 업무 없음 —</div><p>tasks/&lt;업무ID&gt;/task.md가 없습니다. 총괄에게 업무를 지시하면 여기에 카드가 생깁니다.</p></div></body></html>\n")
		return []byte(sb.String())
	}

	// 집계 칩(누르면 그 열만 접기/펴기) + 도구 줄(검색·담당·방치만·Done) + 방치 경고
	sb.WriteString("<div class=\"summary\">")
	for _, col := range Columns {
		fmt.Fprintf(&sb, "<button type=\"button\" class=\"chip\" data-col=\"%s\" title=\"열 접기/펴기\"><i class=\"dot %s\"></i>%s <b class=\"mono\">%d</b></button>", col, col, colTitle[col], cnt[col])
	}
	sb.WriteString("</div>\n")
	sb.WriteString("<div class=\"tools\"><label class=\"field\"><span>검색</span><input id=\"q\" type=\"search\" placeholder=\"ID · 제목 · 담당 · 기록\" autocomplete=\"off\"></label>")
	sb.WriteString("<label class=\"field\"><span>담당</span><select id=\"who\"><option value=\"\">전체</option><option value=\"-\">미배정</option>")
	for _, who := range sessions(cards) {
		e := html.EscapeString(who)
		sb.WriteString("<option value=\"" + e + "\">" + e + "</option>")
	}
	sb.WriteString("</select></label>")
	sb.WriteString("<label class=\"check\"><input id=\"stale\" type=\"checkbox\"> 방치만</label>")
	sb.WriteString("<label class=\"check\"><input id=\"done\" type=\"checkbox\" checked> Done 표시</label>")
	sb.WriteString("<button type=\"button\" id=\"clear\" class=\"btn\">초기화</button><span id=\"shown\" class=\"dim mono\"></span></div>\n")
	if len(staleIDs) > 0 {
		fmt.Fprintf(&sb, "<div class=\"attention\"><span class=\"bang\">!!!</span><span>%d건이 %s 넘게 방치 —</span>", len(staleIDs), durKo(stale))
		for _, id := range staleIDs {
			sb.WriteString(" <a class=\"mono\" href=\"#" + html.EscapeString(id) + "\">" + html.EscapeString(id) + "</a>")
		}
		sb.WriteString("</div>\n")
	}

	// 6열
	sb.WriteString("<div class=\"board\">")
	for _, col := range Columns {
		fmt.Fprintf(&sb, "<section class=\"col\" id=\"col-%s\" data-col=\"%s\"><div class=\"col-head\"><i class=\"dot %s\"></i><h2>%s</h2><span class=\"count mono\">%d</span></div><div class=\"col-desc\">%s</div><div class=\"col-body\">",
			col, col, col, colTitle[col], cnt[col], colDesc[col])
		n := 0
		for _, c := range columnCards(cards, col) {
			n++
			writeCard(&sb, c, col, now, stale, col == ColDone && n > maxDoneShown)
		}
		sb.WriteString("<div class=\"none empty\">— 업무 없음 —</div><div class=\"none nomatch\">— 조건에 맞는 카드 없음 —</div>")
		if col == ColDone && n > maxDoneShown {
			fmt.Fprintf(&sb, "<button type=\"button\" class=\"none more\" data-more>외 %d건 더 보기</button>", n-maxDoneShown)
		}
		sb.WriteString("</div></section>")
	}
	sb.WriteString("</div>\n")

	// 상세 패널(카드마다 하나, :target으로 표시)
	for _, c := range cards {
		writeDetail(&sb, c, cards, now, stale)
	}
	sb.WriteString("<script>" + boardJS + "</script></body></html>\n")
	return []byte(sb.String())
}

// sessions는 카드에 등장하는 담당 세션(중복 제거, 이름순) — 담당 필터 목록.
func sessions(cards []Card) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range cards {
		if c.Session != "" && !seen[c.Session] {
			seen[c.Session] = true
			out = append(out, c.Session)
		}
	}
	sort.Strings(out)
	return out
}

// searchText는 검색 대상 문자열(소문자): ID·제목·상태·담당·부모·기록 전체.
func searchText(c Card) string {
	parts := []string{c.ID, c.Title, c.Status, c.Session, strings.Join(c.Parents, " ")}
	parts = append(parts, c.Log...)
	return strings.ToLower(strings.Join(parts, " "))
}

// columnCards는 열의 카드를 최근 갱신 순(같으면 ID 내림차순)으로 — 열 안에서 방금 움직인 카드가
// 위에 오고, Done 열은 최근 완료가 위에 남는다(입력 순서는 ID 오름차순이라 접두어가 다르면
// 시간 순이 아니다).
func columnCards(cards []Card, col string) []Card {
	var out []Card
	for _, c := range cards {
		if c.Column == col {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Updated.Equal(out[j].Updated) {
			return out[i].Updated.After(out[j].Updated)
		}
		return out[i].ID > out[j].ID
	})
	return out
}

func writeCard(sb *strings.Builder, c Card, col string, now time.Time, stale time.Duration, folded bool) {
	cls := "card"
	isStale := StaleReady(c, now, stale)
	if isStale {
		cls += " stale"
	}
	if folded {
		cls += " folded"
	}
	who := "-"
	if c.Session != "" {
		who = c.Session
	}
	staleAttr := ""
	if isStale {
		staleAttr = " data-stale=\"1\""
	}
	fmt.Fprintf(sb, "<a class=\"%s\" href=\"#%s\" data-who=\"%s\" data-text=\"%s\"%s>", cls, html.EscapeString(c.ID),
		html.EscapeString(who), html.EscapeString(searchText(c)), staleAttr)
	sb.WriteString("<div class=\"row\"><span class=\"id mono\">" + html.EscapeString(c.ID) + "</span>")
	if c.Unknown {
		sb.WriteString("<span class=\"badge warn\" title=\"status 값을 해석하지 못함: " + html.EscapeString(c.Status) + "\">?</span>")
	}
	if isStale {
		sb.WriteString("<span class=\"badge bang\" title=\"방치\">!!!</span>")
	}
	if c.State != "" {
		sb.WriteString("<span class=\"badge state st-" + strings.ToLower(html.EscapeString(c.State)) + "\">" + html.EscapeString(c.State) + "</span>")
	}
	sb.WriteString("</div>")
	title := c.Title
	if title == "" {
		title = c.ID
	}
	sb.WriteString("<div class=\"title\">" + html.EscapeString(title) + "</div>")
	sb.WriteString("<div class=\"row meta\">")
	if c.Session != "" {
		sb.WriteString("<span class=\"who mono\" title=\"담당 세션\">@" + html.EscapeString(c.Session) + "</span>")
	} else if len(c.Parents) > 0 {
		sb.WriteString("<span class=\"who mono dim\">← " + html.EscapeString(strings.Join(c.Parents, ", ")) + "</span>")
	} else {
		sb.WriteString("<span class=\"who dim\">미배정</span>")
	}
	when := c.Updated
	if col == ColReady {
		when = c.Ready
	}
	if a := Ago(when, now); a != "" {
		sb.WriteString("<span class=\"ago\">" + a + "</span>")
	}
	sb.WriteString("</div>")
	if c.LastLog != "" {
		sb.WriteString("<div class=\"log mono\" title=\"" + html.EscapeString(c.LastLog) + "\">" + html.EscapeString(logTail(c.LastLog)) + "</div>")
	}
	sb.WriteString("</a>")
}

func writeDetail(sb *strings.Builder, c Card, all []Card, now time.Time, stale time.Duration) {
	id := html.EscapeString(c.ID)
	fmt.Fprintf(sb, "<aside class=\"detail\" id=\"%s\"><a class=\"scrim\" href=\"#_\" aria-label=\"닫기\"></a><div class=\"panel\">", id)
	sb.WriteString("<div class=\"panel-head\"><span class=\"mono\">" + id + "</span><a class=\"close\" href=\"#_\" title=\"닫기\">×</a></div>")
	sb.WriteString("<div class=\"panel-body\">")
	title := c.Title
	if title == "" {
		title = c.ID
	}
	sb.WriteString("<h3><i class=\"dot " + c.Column + "\"></i>" + html.EscapeString(title) + "</h3>")

	// 메타 표
	sb.WriteString("<dl class=\"kv\">")
	kv := func(k, v string) {
		if v != "" {
			sb.WriteString("<dt>" + k + "</dt><dd>" + v + "</dd>")
		}
	}
	kv("상태", "<span class=\"mono\">"+html.EscapeString(c.Status)+"</span> <span class=\"dim\">→ "+colTitle[c.Column]+"</span>")
	if c.Session != "" {
		st := ""
		if c.State != "" {
			st = " <span class=\"badge state st-" + strings.ToLower(html.EscapeString(c.State)) + "\">" + html.EscapeString(c.State) + "</span>"
		}
		kv("담당 세션", "<span class=\"mono\">"+html.EscapeString(c.Session)+"</span>"+st)
	} else {
		kv("담당 세션", "<span class=\"dim\">미배정</span>")
	}
	if len(c.Parents) > 0 {
		kv("부모", "<span class=\"mono\">"+linkIDs(c.Parents)+"</span>")
	} else {
		kv("부모", "<span class=\"dim\">없음</span>")
	}
	var kids []string
	for _, k := range Children(all, c.ID) {
		kids = append(kids, k.ID)
	}
	if len(kids) > 0 {
		kv("자식", "<span class=\"mono\">"+linkIDs(kids)+"</span>")
	} else {
		kv("자식", "<span class=\"dim\">없음</span>")
	}
	if !c.Updated.IsZero() {
		kv("갱신", "<span class=\"mono\">"+c.Updated.Format("2006-01-02 15:04")+"</span> <span class=\"dim\">"+Ago(c.Updated, now)+" 전</span>")
	}
	if c.Column == ColReady && !c.Ready.IsZero() {
		kv("ready 진입", "<span class=\"mono\">"+c.Ready.Format("2006-01-02 15:04")+"</span> <span class=\"dim\">"+Ago(c.Ready, now)+" 전</span>")
	}
	sb.WriteString("</dl>")

	if StaleReady(c, now, stale) {
		what := "ready인데 아무도 배정하지 않음"
		if c.Column == ColBlocked {
			what = "직원이 답을 기다리는데 아무도 답하지 않음"
		}
		fmt.Fprintf(sb, "<div class=\"diag\"><div class=\"diag-t\"><span class=\"bang\">!!!</span> %s 넘게 %s</div><div class=\"dim\">총괄 채널에서 배정·답변하거나 <code>agentlayer send</code>로 직접 보내세요. 기준 <code>board_stale_ready</code>.</div></div>", durKo(stale), what)
	}

	// 본문(task.md)
	sb.WriteString("<h4>업무 내용 <span class=\"dim mono\">task.md</span></h4>")
	if strings.TrimSpace(c.Body) == "" {
		sb.WriteString("<div class=\"none\">— 본문 없음 —</div>")
	} else {
		sb.WriteString(renderBody(c.Body))
	}

	// 기록(log.md)
	fmt.Fprintf(sb, "<h4>기록 <span class=\"dim mono\">log.md · %d</span></h4>", len(c.Log))
	if len(c.Log) == 0 {
		sb.WriteString("<div class=\"none\">— 기록 없음 —</div>")
	} else {
		sb.WriteString("<ol class=\"events\">")
		for i := len(c.Log) - 1; i >= 0; i-- { // 최신이 위
			ts, tag, text := splitLog(c.Log[i])
			sb.WriteString("<li><span class=\"ts mono\">" + html.EscapeString(ts) + "</span><span class=\"tag mono tag-" + strings.ToLower(html.EscapeString(tag)) + "\">" + html.EscapeString(tag) + "</span><span class=\"txt\">" + html.EscapeString(text) + "</span></li>")
		}
		sb.WriteString("</ol>")
	}
	sb.WriteString("</div></div></aside>\n")
}

// linkIDs는 업무ID들을 상세 패널 앵커로.
func linkIDs(ids []string) string {
	var parts []string
	for _, id := range ids {
		e := html.EscapeString(id)
		parts = append(parts, "<a href=\"#"+e+"\">"+e+"</a>")
	}
	return strings.Join(parts, ", ")
}

// splitLog는 "[2026-09-16 09:29] [SEND] 본문" → (시각, 태그, 본문). 형식이 다르면 본문만.
func splitLog(line string) (ts, tag, text string) {
	text = line
	if strings.HasPrefix(text, "[") {
		if i := strings.Index(text, "] "); i > 0 {
			ts, text = text[1:i], text[i+2:]
		}
	}
	if strings.HasPrefix(text, "[") {
		if i := strings.Index(text, "] "); i > 0 {
			tag, text = text[1:i], text[i+2:]
		} else if strings.HasSuffix(text, "]") && !strings.Contains(text, " ") {
			tag, text = strings.Trim(text, "[]"), ""
		}
	}
	return ts, tag, strings.ReplaceAll(text, "⏎", " ⏎ ")
}

// logTail은 카드 한 줄용: 시각을 떼고 "[TAG] 본문"만.
func logTail(line string) string {
	ts, tag, text := splitLog(line)
	_ = ts
	if tag == "" {
		return text
	}
	return "[" + tag + "] " + text
}

// renderBody는 task.md 본문을 최소 마크다운으로 — "## " 절 제목, "- [ ]"/"- [x]" 체크, "- " 항목,
// ``` 코드 블록, 나머지는 문단. 정식 파서가 아니라 총괄 템플릿(목표·담당·완료 기준)이 읽히는 정도.
func renderBody(body string) string {
	var sb strings.Builder
	sb.WriteString("<div class=\"body\">")
	inCode, inList := false, false
	closeList := func() {
		if inList {
			sb.WriteString("</ul>")
			inList = false
		}
	}
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			closeList()
			if inCode {
				sb.WriteString("</pre>")
			} else {
				sb.WriteString("<pre class=\"mono\">")
			}
			inCode = !inCode
			continue
		}
		if inCode {
			sb.WriteString(html.EscapeString(line) + "\n")
			continue
		}
		switch {
		case trimmed == "":
			closeList()
		case strings.HasPrefix(trimmed, "## "):
			closeList()
			sb.WriteString("<h5>" + html.EscapeString(strings.TrimPrefix(trimmed, "## ")) + "</h5>")
		case strings.HasPrefix(trimmed, "### "):
			closeList()
			sb.WriteString("<h6>" + html.EscapeString(strings.TrimPrefix(trimmed, "### ")) + "</h6>")
		case strings.HasPrefix(trimmed, "- [x] "), strings.HasPrefix(trimmed, "- [X] "):
			if !inList {
				sb.WriteString("<ul>")
				inList = true
			}
			sb.WriteString("<li class=\"done\"><span class=\"box\">☑</span>" + html.EscapeString(trimmed[6:]) + "</li>")
		case strings.HasPrefix(trimmed, "- [ ] "):
			if !inList {
				sb.WriteString("<ul>")
				inList = true
			}
			sb.WriteString("<li><span class=\"box\">☐</span>" + html.EscapeString(trimmed[6:]) + "</li>")
		case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
			if !inList {
				sb.WriteString("<ul>")
				inList = true
			}
			sb.WriteString("<li><span class=\"box\">•</span>" + html.EscapeString(trimmed[2:]) + "</li>")
		default:
			closeList()
			sb.WriteString("<p>" + html.EscapeString(trimmed) + "</p>")
		}
	}
	closeList()
	if inCode {
		sb.WriteString("</pre>")
	}
	sb.WriteString("</div>")
	return sb.String()
}

// durKo는 방치 기준을 "30분"·"2시간"으로.
func durKo(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d분", int(d.Minutes()))
	case d%time.Hour == 0:
		return fmt.Sprintf("%d시간", int(d.Hours()))
	}
	return fmt.Sprintf("%.1f시간", d.Hours())
}

// Ago는 t~now 경과를 "방금"·"N분"·"N시간"·"N일"로. t가 제로값이면 "".
// board(HTML)·discord(카드) 양쪽이 같은 표현을 쓰는 단일 지점 — internal/discord/card.go의 since는 이걸 감싼다.
func Ago(t, now time.Time) string {
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

const boardCSS = `
:root{--bg:#1f1e1d;--panel:#262624;--card:#2d2c2a;--card2:#343331;--line:rgba(250,249,245,.08);--line2:rgba(250,249,245,.16);
--fg:#faf9f5;--dim:#a8a69d;--dim2:#7d7b73;--acc:#d97757;
--c-todo:#8f8d85;--c-ready:#e0b347;--c-running:#4cc98a;--c-blocked:#e06a5c;--c-review:#d97757;--c-done:#8fb4d9;
--sans:-apple-system,BlinkMacSystemFont,"Apple SD Gothic Neo","Pretendard","Segoe UI",sans-serif;--mono:"SF Mono",Menlo,"JetBrains Mono",ui-monospace,monospace}
*{box-sizing:border-box}html{scroll-behavior:smooth}
body{margin:0;background:var(--bg);color:var(--fg);font:14px/1.5 var(--sans);padding:24px 24px 48px;-webkit-font-smoothing:antialiased}
a{color:inherit;text-decoration:none}code,kbd,.mono{font-family:var(--mono);font-size:.92em}
code,kbd{background:var(--card2);border:1px solid var(--line);border-radius:4px;padding:1px 5px;color:var(--fg)}
.dim{color:var(--dim)}.sep{color:var(--dim2);margin:0 8px}
.top{display:flex;align-items:flex-end;justify-content:space-between;gap:16px;flex-wrap:wrap;margin-bottom:18px}
.eyebrow{font-size:11px;letter-spacing:.18em;text-transform:uppercase;color:var(--acc);margin-bottom:4px}
h1{font-size:22px;font-weight:600;margin:0;letter-spacing:-.01em}
.top-meta{color:var(--dim);font-size:12.5px;display:flex;align-items:center}.top-meta .hint{color:var(--dim2)}
.summary{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:12px}
.chip{display:inline-flex;align-items:center;gap:7px;border:1px solid var(--line);background:var(--panel);padding:5px 11px;border-radius:999px;font-size:12.5px;color:var(--dim)}
.chip b{color:var(--fg);font-weight:600}.chip:hover{border-color:var(--line2)}
.dot{display:inline-block;width:8px;height:8px;border-radius:50%;background:var(--dim2);flex:none}
.dot.todo{background:var(--c-todo)}.dot.ready{background:var(--c-ready)}.dot.running{background:var(--c-running)}.dot.blocked{background:var(--c-blocked)}.dot.review{background:var(--c-review)}.dot.done{background:var(--c-done)}
.attention{display:flex;align-items:center;gap:8px;flex-wrap:wrap;border:1px solid rgba(224,106,92,.55);background:rgba(224,106,92,.08);border-radius:8px;padding:9px 14px;margin-bottom:16px;font-size:13px}
.attention a{color:var(--fg);border-bottom:1px solid var(--line2)}.attention a:hover{border-color:var(--fg)}
.bang{color:var(--c-blocked);font-family:var(--mono);font-weight:700;letter-spacing:-.05em}
.board{display:grid;grid-template-columns:repeat(6,minmax(188px,1fr));gap:12px;align-items:start;overflow-x:auto;padding-bottom:8px}
.col{background:var(--panel);border:1px solid var(--line);border-radius:10px;min-height:180px;display:flex;flex-direction:column}
.col-head{display:flex;align-items:center;gap:8px;padding:12px 14px 4px}
.col-head h2{font-size:14px;font-weight:600;margin:0;flex:1}
.count{color:var(--dim);font-size:12px;background:var(--card);border:1px solid var(--line);border-radius:6px;padding:1px 7px;min-width:24px;text-align:center}
.col-desc{color:var(--dim2);font-size:11.5px;line-height:1.4;padding:0 14px 10px;border-bottom:1px solid var(--line)}
.col-body{padding:10px;display:flex;flex-direction:column;gap:8px;flex:1}
.none{border:1px dashed var(--line2);border-radius:8px;color:var(--dim2);text-align:center;padding:22px 8px;font-size:12.5px}
.none.more{border-style:solid;border-color:var(--line);padding:9px}
.card{display:block;background:var(--card);border:1px solid var(--line);border-radius:8px;padding:10px 12px;transition:border-color .12s,transform .12s;position:relative}
.card:hover{border-color:var(--line2);background:var(--card2)}
.card.stale{border-color:rgba(224,106,92,.6);box-shadow:inset 3px 0 0 var(--c-blocked)}
.row{display:flex;align-items:center;gap:6px;min-width:0}
.id{color:var(--dim);font-size:11.5px;letter-spacing:.02em}
.badge{font-family:var(--mono);font-size:10.5px;padding:0 5px;border-radius:4px;border:1px solid var(--line2);color:var(--dim);line-height:16px}
.badge.warn{color:var(--c-ready);border-color:rgba(224,179,71,.5)}.badge.bang{color:var(--c-blocked);border-color:rgba(224,106,92,.5);font-weight:700}
.badge.state{margin-left:auto}.st-work{color:var(--c-running);border-color:rgba(76,201,138,.45)}.st-wait{color:var(--c-ready);border-color:rgba(224,179,71,.45)}
.st-done{color:var(--c-done);border-color:rgba(143,180,217,.45)}.st-err,.st-dead{color:var(--c-blocked);border-color:rgba(224,106,92,.45)}
.title{font-size:13.5px;font-weight:500;line-height:1.4;margin:5px 0 7px;color:var(--fg);word-break:keep-all;overflow-wrap:anywhere}
.meta{justify-content:space-between;font-size:11.5px;color:var(--dim)}
.who{color:var(--c-done);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.who.dim{color:var(--dim2)}
.ago{color:var(--dim2);flex:none}
.log{margin-top:7px;padding-top:7px;border-top:1px solid var(--line);color:var(--dim2);font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.tools{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin-bottom:14px}
.field{display:flex;align-items:center;gap:8px;font-size:12px;color:var(--dim)}
.field span{font-size:11px;letter-spacing:.12em;text-transform:uppercase;color:var(--dim2)}
.tools input[type=search],.tools select{background:var(--panel);color:var(--fg);border:1px solid var(--line2);border-radius:6px;padding:6px 10px;font:13px var(--mono);outline:none;min-width:200px}
.tools select{min-width:150px}.tools input:focus,.tools select:focus{border-color:var(--acc)}
.check{display:flex;align-items:center;gap:6px;font-size:12.5px;color:var(--dim);cursor:pointer}.check input{accent-color:var(--acc)}
.btn{background:var(--card);color:var(--dim);border:1px solid var(--line2);border-radius:6px;padding:6px 12px;font:12.5px var(--sans);cursor:pointer}.btn:hover{color:var(--fg);border-color:var(--fg)}
.chip{cursor:pointer;font-family:var(--sans)}.chip.off{opacity:.38;text-decoration:line-through}
.col.off,.card.hide,.card.folded,.none{display:none}.col.is-empty .none.empty,.col.is-nomatch .none.nomatch{display:block}
.col.unfolded .card.folded,.card.folded.hit{display:block}.col.unfolded [data-more]{display:none}
button.none{width:100%;background:none;color:var(--dim);cursor:pointer;font:12.5px var(--sans)}button.none:hover{color:var(--fg);border-color:var(--fg)}
.empty-board{border:1px dashed var(--line2);border-radius:10px;padding:48px 24px;text-align:center;color:var(--dim)}
.empty-board p{margin:8px 0 0;font-size:13px;color:var(--dim2)}
/* 상세 패널 — :target */
.detail{display:none;position:fixed;inset:0;z-index:10}
.detail:target{display:block}
.scrim{position:absolute;inset:0;background:rgba(15,14,13,.55);backdrop-filter:blur(2px)}
.panel{position:absolute;top:0;right:0;bottom:0;width:min(560px,92vw);background:var(--bg);border-left:1px solid var(--line2);box-shadow:-24px 0 48px rgba(0,0,0,.35);display:flex;flex-direction:column}
.panel-head{display:flex;align-items:center;justify-content:space-between;padding:12px 18px;border-bottom:1px solid var(--line);color:var(--dim);font-size:12.5px}
.close{font-size:22px;line-height:1;color:var(--dim);padding:2px 6px;border-radius:6px}.close:hover{color:var(--fg);background:var(--card)}
.panel-body{padding:18px 22px 40px;overflow:auto}
.panel-body h3{display:flex;align-items:center;gap:10px;font-size:17px;font-weight:600;margin:0 0 14px;line-height:1.35}
.kv{display:grid;grid-template-columns:96px 1fr;gap:7px 12px;margin:0 0 16px;padding:12px 14px;background:var(--panel);border:1px solid var(--line);border-radius:8px;font-size:13px}
.kv dt{color:var(--dim);margin:0}.kv dd{margin:0;min-width:0;overflow-wrap:anywhere}.kv a{color:var(--c-done);border-bottom:1px solid transparent}.kv a:hover{border-color:var(--c-done)}
.diag{border-left:3px solid var(--c-blocked);background:rgba(224,106,92,.07);padding:10px 14px;border-radius:0 8px 8px 0;margin:0 0 16px;font-size:13px}
.diag-t{font-weight:600;margin-bottom:4px}
h4{font-size:12px;letter-spacing:.12em;text-transform:uppercase;color:var(--dim);margin:22px 0 8px;display:flex;align-items:baseline;gap:8px}
h4 .mono{text-transform:none;letter-spacing:0;font-size:11px}
.body{background:var(--panel);border:1px solid var(--line);border-radius:8px;padding:6px 16px 12px;font-size:13.5px;line-height:1.6}
.body h5{font-size:12px;color:var(--acc);letter-spacing:.06em;text-transform:uppercase;margin:14px 0 4px}.body h6{font-size:12.5px;color:var(--dim);margin:10px 0 2px}
.body p{margin:4px 0}.body ul{list-style:none;margin:2px 0;padding:0}.body li{display:flex;gap:8px;padding:1px 0}.body li.done{color:var(--dim)}
.body .box{color:var(--dim);flex:none;font-family:var(--mono)}.body pre{background:var(--card);border:1px solid var(--line);border-radius:6px;padding:8px 10px;margin:6px 0;overflow:auto;font-size:12px;line-height:1.5}
.events{list-style:none;margin:0;padding:0;border:1px solid var(--line);border-radius:8px;background:var(--panel);font-size:12.5px}
.events li{display:grid;grid-template-columns:118px 92px 1fr;gap:10px;padding:8px 14px;border-bottom:1px solid var(--line);align-items:baseline}.events li:last-child{border-bottom:0}
.events .ts{color:var(--dim2);font-size:11.5px}.events .tag{font-size:11px;font-weight:600;letter-spacing:.04em}
.events .txt{color:var(--fg);overflow-wrap:anywhere;line-height:1.5}
.tag-assign{color:var(--c-running)}.tag-send{color:var(--c-done)}.tag-ask{color:var(--c-ready)}.tag-report{color:var(--c-review)}.tag-verification{color:var(--dim)}
.tag-error{color:var(--c-blocked)}.tag-complete{color:var(--c-running)}.tag-decision{color:var(--acc)}
@media (max-width:900px){body{padding:18px 14px 32px}.board{display:flex}.col{min-width:250px}.events li{grid-template-columns:1fr;gap:2px}.kv{grid-template-columns:84px 1fr}}
`

// boardJS — 검색·담당·방치만·Done 표시·열 접기·Done 더 보기. 인라인 한 덩어리(외부 로드 없음),
// 정적 파일(file://)에서 그대로 동작. 상태는 localStorage에 남겨 재실행(새 HTML)에도 유지된다.
// 단축키: "/" 검색창, Esc 상세 패널 닫기.
const boardJS = `(function(){
var $=function(s,r){return (r||document).querySelector(s)},$$=function(s,r){return Array.prototype.slice.call((r||document).querySelectorAll(s))};
var q=$('#q'),who=$('#who'),stale=$('#stale'),done=$('#done'),clear=$('#clear'),shown=$('#shown');
var KEY='agentlayer.board.filters';
function load(){try{var v=JSON.parse(localStorage.getItem(KEY)||'{}');q.value=v.q||'';who.value=v.who||'';stale.checked=!!v.stale;done.checked=v.done!==false;(v.off||[]).forEach(function(c){var ch=$('.chip[data-col="'+c+'"]');if(ch)ch.classList.add('off')})}catch(e){}}
function save(){try{localStorage.setItem(KEY,JSON.stringify({q:q.value,who:who.value,stale:stale.checked,done:done.checked,off:$$('.chip.off').map(function(c){return c.dataset.col})}))}catch(e){}}
function apply(){
  var text=q.value.trim().toLowerCase(),w=who.value,st=stale.checked,dn=done.checked,total=0,vis=0,active=!!(text||w||st);
  $$('.chip').forEach(function(ch){var col=$('.col[data-col="'+ch.dataset.col+'"]');if(!col)return;var off=ch.classList.contains('off')||(ch.dataset.col==='done'&&!dn);col.classList.toggle('off',off)});
  $$('.col').forEach(function(col){
    var cards=$$('.card',col),n=0;
    cards.forEach(function(c){
      total++;
      var hit=(!text||c.dataset.text.indexOf(text)>=0)&&(!w||c.dataset.who===w)&&(!st||c.dataset.stale==='1');
      c.classList.toggle('hide',!hit);c.classList.toggle('hit',hit&&active);
      if(hit)n++;
    });
    if(!col.classList.contains('off'))vis+=n;
    col.classList.toggle('is-empty',cards.length===0);
    col.classList.toggle('is-nomatch',cards.length>0&&n===0);
  });
  shown.textContent=active?vis+' / '+total+' 표시':'';
  save();
}
load();
[q,who,stale,done].forEach(function(el){el.addEventListener('input',apply);el.addEventListener('change',apply)});
$$('.chip').forEach(function(ch){ch.addEventListener('click',function(){ch.classList.toggle('off');apply()})});
$$('[data-more]').forEach(function(b){b.addEventListener('click',function(){b.closest('.col').classList.add('unfolded')})});
clear.addEventListener('click',function(){q.value='';who.value='';stale.checked=false;done.checked=true;$$('.chip').forEach(function(c){c.classList.remove('off')});apply();q.focus()});
document.addEventListener('keydown',function(e){if(e.key==='/'&&document.activeElement!==q&&document.activeElement.tagName!=='INPUT'&&document.activeElement.tagName!=='SELECT'){e.preventDefault();q.focus()}if(e.key==='Escape'&&location.hash&&location.hash!=='#_'){location.hash='_'}});
apply();
})();`
