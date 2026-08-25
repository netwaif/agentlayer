// Package discord는 상태 카드(Components V2)를 조립해 웹훅 메시지 하나로
// 업서트한다. discord_dash.py의 카드 형식을 승계하되, 봇 섹션에
// 에이전트 의미 상태(WORKING/WAITING/DONE_UNREAD)를 추가한다.
// 기존 discord_dash의 메시지·상태 파일은 건드리지 않는다.
package discord

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/usage"
)

const (
	typeContainer = 17
	typeText      = 10
	barWidth      = 14
)

// level → (accent 색, 이모지, 한 줄 평가) — discord_dash와 동일 축
var levels = map[string][3]string{
	"red":    {"#F04747", "🔴", "주간 한도부터 챙기세요"},
	"yellow": {"#FAA61A", "🟡", "큰 작업은 미루세요"},
	"wait":   {"#7C8AFF", "⏳", "잠깐 기다리면 풀로 가능"},
	"white":  {"#9AA4B2", "⚪", "평소대로"},
	"green":  {"#43B581", "🟢", "큰 작업 OK"},
}

var severity = map[string]int{"red": 0, "yellow": 1, "wait": 2, "white": 3, "green": 4}

var provEmoji = map[string]string{"claude": "🟠", "codex": "🔵", "antigravity": "🟣"}
var provHex = map[string]string{"claude": "#E5B567", "codex": "#7ED5F5", "antigravity": "#C89BF0"}

// 게이지 라벨 — 코드 스팬 정렬을 위해 ASCII만
var winCode = map[string]string{"5h": "5h", "7d": "7d", "daily": "1d", "fable_7d": "Fable", "gemini": "Gemini"}

var stateEmoji = map[state.AgentState]string{
	state.StateWorking:    "🟢",
	state.StateWaiting:    "🟡",
	state.StateDoneUnread: "🟠",
	state.StateError:      "🔴",
	state.StateIdle:       "⚪",
	state.StateDead:       "⚫",
}

var stateWord = map[state.AgentState]string{
	state.StateWorking:    "작업중",
	state.StateWaiting:    "응답 필요",
	state.StateDoneUnread: "새 완료(안 봄)",
	state.StateError:      "에러",
	state.StateIdle:       "대기",
	state.StateDead:       "종료",
}

func accent(hex string) int {
	var v int
	fmt.Sscanf(strings.TrimPrefix(hex, "#"), "%x", &v)
	return v
}

func winEmoji(pct *float64) string {
	if pct == nil {
		return "⚪"
	}
	switch {
	case *pct < 20:
		return "🔴"
	case *pct < 50:
		return "🟡"
	default:
		return "🟢"
	}
}

// accentFor는 컨테이너 강조색: 위험 level이면 level 색, 평상시 브랜드색.
func accentFor(key, level string) string {
	if level == "red" || level == "yellow" || level == "wait" {
		return levels[level][0]
	}
	if h, ok := provHex[key]; ok {
		return h
	}
	return "#9AA4B2"
}

func title(key string) string {
	if key == "" {
		return key
	}
	return strings.ToUpper(key[:1]) + key[1:]
}

func providerContainer(key string, p usage.Provider) map[string]any {
	if !p.OK {
		return map[string]any{"type": typeContainer, "accent_color": accent("#565B66"),
			"components": []any{map[string]any{"type": typeText,
				"content": fmt.Sprintf("### %s\n⚠️ 조회 실패", title(key))}}}
	}
	emoji := levels[p.Level][1]
	if p.Level == "green" || p.Level == "white" {
		if e, ok := provEmoji[key]; ok {
			emoji = e
		}
	}
	head := fmt.Sprintf("### %s %s — %s", emoji, title(key), p.Action)
	if p.Email != "" {
		head += "\n-# " + p.Email
	}

	keys := windowOrder(p.Windows)
	lw := 0
	for _, wk := range keys {
		if l := len(label(wk)); l > lw {
			lw = l
		}
	}
	var lines []string
	for _, wk := range keys {
		w := p.Windows[wk]
		pct := "—"
		if w.LeftPct != nil {
			pct = fmt.Sprintf("%d", int(*w.LeftPct))
		}
		line := fmt.Sprintf("%s `%-*s  %s` **%s%%**",
			winEmoji(w.LeftPct), lw, label(wk), usage.Gauge(w.LeftPct, barWidth), pct)
		if r := usage.ResetLabel(w.ResetMin); r != "" {
			line += " · 리셋 " + r
		}
		lines = append(lines, line)
	}

	children := []any{map[string]any{"type": typeText, "content": head}}
	if len(lines) > 0 {
		children = append(children, map[string]any{"type": typeText, "content": strings.Join(lines, "\n")})
	}
	if p.Reason != "" {
		children = append(children, map[string]any{"type": typeText, "content": "**" + p.Reason + "**"})
	}
	return map[string]any{"type": typeContainer,
		"accent_color": accent(accentFor(key, p.Level)), "components": children}
}

func label(wk string) string {
	if l, ok := winCode[wk]; ok {
		return l
	}
	return wk // antigravity 계정명 등
}

func windowOrder(ws map[string]usage.Window) []string {
	var std, rest []string
	for _, k := range []string{"5h", "daily", "7d", "fable_7d"} {
		if _, ok := ws[k]; ok {
			std = append(std, k)
		}
	}
	for k := range ws {
		if _, ok := winCode[k]; !ok {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(std, rest...)
}

// agentsContainer는 에이전트 섹션: 상태 + 폴더 + 모델 + ctx 게이지 + 경과.
// agentsContainer는 행이 하나도 없으면 nil (빈 섹션 금지).
func agentsContainer(agents []*state.Agent, ctx map[string]usage.CtxInfo, home string, now time.Time) map[string]any {
	shorten := func(p string) string {
		if home != "" && strings.HasPrefix(p, home) {
			return "~" + strings.TrimPrefix(p, home)
		}
		return p
	}
	type row struct {
		head string
		a    *state.Agent
		info usage.CtxInfo
	}
	var rows []row
	for _, a := range agents {
		if a.State == state.StateDead {
			continue
		}
		info := ctx[a.CWD]
		tag := info.Model
		if tag == "" {
			tag = a.Kind
		}
		rows = append(rows, row{head: fmt.Sprintf("%s  [%s]", shorten(a.CWD), tag), a: a, info: info})
	}
	width := 1
	for _, r := range rows {
		if len(r.head) > width {
			width = len(r.head)
		}
	}
	var lines []string
	var worst *float64
	for _, r := range rows {
		pct := "—"
		if r.info.UsedPct != nil {
			pct = fmt.Sprintf("%d", int(*r.info.UsedPct+0.5))
			if worst == nil || *r.info.UsedPct > *worst {
				worst = r.info.UsedPct
			}
		}
		line := fmt.Sprintf("%s `%-*s  %s` **%s%%** · %s",
			stateEmoji[r.a.State], width, r.head, usage.Gauge(r.info.UsedPct, barWidth),
			pct, stateWord[r.a.State])
		if r.a.State != state.StateIdle {
			line += " " + since(r.a.StateSince, now)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return nil // 빈 content는 Discord가 400으로 거부한다
	}
	color := "#565B66"
	switch {
	case worst != nil && *worst >= 80:
		color = "#F04747"
	case worst != nil && *worst >= 40:
		color = "#FAA61A"
	case worst != nil:
		color = "#43B581"
	}
	return map[string]any{"type": typeContainer, "accent_color": accent(color),
		"components": []any{
			map[string]any{"type": typeText, "content": "### 에이전트"},
			map[string]any{"type": typeText, "content": strings.Join(lines, "\n")},
		}}
}

func since(from, now time.Time) string {
	d := now.Sub(from)
	switch {
	case d < time.Minute:
		return "방금"
	case d < time.Hour:
		return fmt.Sprintf("%d분", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d시간", int(d.Hours()))
	default:
		return fmt.Sprintf("%d일", int(d.Hours()/24))
	}
}

// BuildComponents는 카드 전체를 조립한다. usage가 nil이면 에이전트 섹션만.
func BuildComponents(pay *usage.Payload, agents []*state.Agent, ctx map[string]usage.CtxInfo, home string, now time.Time) []any {
	var comps []any
	if pay != nil {
		for _, key := range []string{"claude", "codex", "antigravity"} {
			if p, ok := pay.Providers[key]; ok {
				comps = append(comps, providerContainer(key, p))
			}
		}
	}
	if ac := agentsContainer(agents, ctx, home, now); ac != nil {
		comps = append(comps, ac)
	}
	comps = append(comps, map[string]any{"type": typeText,
		"content": fmt.Sprintf("-# 갱신 <t:%d:R>", now.Unix())})
	return comps
}

// WorsenedPings는 provider level이 악화된 순간의 핑 문구 목록과 갱신된
// level 맵을 돌려준다 (yellow/red 진입 시에만).
func WorsenedPings(pay *usage.Payload, last map[string]string) ([]string, map[string]string) {
	now := map[string]string{}
	var pings []string
	if pay == nil {
		return pings, last
	}
	for key, p := range pay.Providers {
		lv := ""
		if p.OK {
			lv = p.Level
		}
		now[key] = lv
		if lv != "yellow" && lv != "red" {
			continue
		}
		prev, seen := last[key]
		if seen && severity[lv] < severityOf(prev) {
			who := title(key)
			if p.Email != "" {
				who += "(" + strings.SplitN(p.Email, "@", 2)[0] + ")"
			}
			pings = append(pings, fmt.Sprintf("%s **%s** %s — %s",
				levels[lv][1], who, levels[lv][2], p.Action))
		}
	}
	return pings, now
}

func severityOf(lv string) int {
	if s, ok := severity[lv]; ok {
		return s
	}
	return 9
}
