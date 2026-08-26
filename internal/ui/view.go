package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/charmbracelet/lipgloss"
	"github.com/netwaif/agentlayer/internal/cli"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/usage"
)

// 사용자 tmux 테마와 같은 팔레트: 짙은 회색 + 주황(#ffaf5f) 포인트.
var (
	styleHeader   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8a8a8a"))
	styleTitle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaf5f")).Bold(true)
	selBg         = lipgloss.Color("#3a3a3a") // 은은한 선택 배경 (tmux 테마 톤)
	styleSelected = lipgloss.NewStyle().Background(selBg).Foreground(lipgloss.Color("#e4e4e4")).Bold(true)
	styleHelp     = lipgloss.NewStyle().Foreground(lipgloss.Color("#6a6a6a"))
	styleModel    = lipgloss.NewStyle().Foreground(lipgloss.Color("#d0d0d0"))
	styleDiscord  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C8AFF")).Bold(true)

	stateStyles = map[state.AgentState]lipgloss.Style{
		state.StateWorking:    lipgloss.NewStyle().Foreground(lipgloss.Color("#43B581")),
		state.StateWaiting:    lipgloss.NewStyle().Foreground(lipgloss.Color("#FAA61A")).Bold(true),
		state.StateDoneUnread: lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaf5f")).Bold(true),
		state.StateError:      lipgloss.NewStyle().Foreground(lipgloss.Color("#F04747")).Bold(true),
		state.StateIdle:       lipgloss.NewStyle().Foreground(lipgloss.Color("#8a8a8a")),
		state.StateDead:       lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")),
	}
)

// stateText는 색 없는 상태 라벨 (선택 바 렌더용).
func stateText(a *state.Agent, now time.Time) string {
	label := map[state.AgentState]string{
		state.StateWorking:    "● WORK",
		state.StateWaiting:    "◆ WAIT",
		state.StateDoneUnread: "✔ DONE",
		state.StateError:      "✖ ERR ",
		state.StateIdle:       "· idle",
		state.StateDead:       "  dead",
	}[a.State]
	if a.State == state.StateWorking && a.Stale(now) {
		label = "● WORK?"
	}
	return label
}

func stateBadge(a *state.Agent, now time.Time) string {
	label := map[state.AgentState]string{
		state.StateWorking:    "● WORK",
		state.StateWaiting:    "◆ WAIT",
		state.StateDoneUnread: "✔ DONE",
		state.StateError:      "✖ ERR ",
		state.StateIdle:       "· idle",
		state.StateDead:       "  dead",
	}[a.State]
	if a.State == state.StateWorking && a.Stale(now) {
		label = "● WORK?"
	}
	return stateStyles[a.State].Render(label)
}

// summary는 헤더의 상태 집계 한 줄.
func summary(agents []*state.Agent) string {
	counts := map[state.AgentState]int{}
	for _, a := range agents {
		counts[a.State]++
	}
	var parts []string
	for _, s := range []state.AgentState{state.StateWaiting, state.StateDoneUnread,
		state.StateError, state.StateWorking, state.StateIdle} {
		if counts[s] > 0 {
			parts = append(parts, stateStyles[s].Render(fmt.Sprintf("%s %d", shortName(s), counts[s])))
		}
	}
	if len(parts) == 0 {
		return "에이전트 없음"
	}
	return strings.Join(parts, "  ")
}

func shortName(s state.AgentState) string {
	switch s {
	case state.StateWaiting:
		return "응답 필요"
	case state.StateDoneUnread:
		return "새 완료"
	case state.StateError:
		return "에러"
	case state.StateWorking:
		return "작업중"
	default:
		return "대기"
	}
}

// levelStyle은 coach level 색.
var levelStyle = map[string]lipgloss.Style{
	"red":    lipgloss.NewStyle().Foreground(lipgloss.Color("#F04747")).Bold(true),
	"yellow": lipgloss.NewStyle().Foreground(lipgloss.Color("#FAA61A")),
	"wait":   lipgloss.NewStyle().Foreground(lipgloss.Color("#7C8AFF")),
	"white":  lipgloss.NewStyle().Foreground(lipgloss.Color("#9AA4B2")),
	"green":  lipgloss.NewStyle().Foreground(lipgloss.Color("#43B581")),
}

var levelEmoji = map[string]string{
	"red": "🔴", "yellow": "🟡", "wait": "⏳", "white": "⚪", "green": "🟢",
}

// provider 브랜드 색·점 — Discord 카드와 동일 규칙:
// 평상시(green/white)는 브랜드 색, 위험(red/yellow/wait)이면 level 색이 덮는다.
var provStyle = map[string]lipgloss.Style{
	"claude":      lipgloss.NewStyle().Foreground(lipgloss.Color("#E5B567")).Bold(true),
	"codex":       lipgloss.NewStyle().Foreground(lipgloss.Color("#7ED5F5")).Bold(true),
	"antigravity": lipgloss.NewStyle().Foreground(lipgloss.Color("#C89BF0")).Bold(true),
}

var provEmoji = map[string]string{"claude": "🟠", "codex": "🔵", "antigravity": "🟣"}

func providerStyle(key, level string) lipgloss.Style {
	if level == "green" || level == "white" {
		if s, ok := provStyle[key]; ok {
			return s
		}
	}
	return levelStyle[level]
}

func providerEmoji(key, level string) string {
	if level == "green" || level == "white" {
		if e, ok := provEmoji[key]; ok {
			return e
		}
	}
	return levelEmoji[level]
}

// 게이지 라벨 — Discord 카드와 동일 축
var winLabel = map[string]string{"5h": "5h", "7d": "7d", "daily": "1d", "fable_7d": "Fable"}

// ctxStyle은 컨텍스트 사용률 색: 40%↑ 노랑(사용자의 마감 습관 기준), 80%↑ 빨강.
func ctxStyle(used float64) lipgloss.Style {
	switch {
	case used >= 80:
		return levelStyle["red"]
	case used >= 40:
		return levelStyle["yellow"]
	default:
		return levelStyle["green"]
	}
}

// defaultModelLine은 헤더의 CLI별 기본 모델 조각 — 관제탑은 3사 공통이므로
// claude·codex·gemini 모두 보여준다. 미설정은 "자동"(그 CLI가 알아서 고른다는 뜻).
// Claude 기본이 Fable이면 새로 띄우는 모든 claude가 최상위 티어로 돌아 토큰이 샌다 — 경고색.
func (m Model) defaultModelLine() string {
	if m.defModels == nil {
		return "" // 아직 미수집 (usageCmd 첫 응답 전)
	}
	entries := []struct{ key, label, styleKey string }{
		{"claude", "Claude", "claude"},
		{"codex", "Codex", "codex"},
		{"gemini", "Gemini", "antigravity"}, // 같은 Google 계열 보라
	}
	var parts []string
	for _, e := range entries {
		v := m.defModels[e.key]
		switch {
		case v == "":
			parts = append(parts, styleHelp.Render(e.label+" 자동"))
		case e.key == "claude" && usage.IsFable(v):
			parts = append(parts, levelStyle["red"].Render("⚠ "+e.label+" "+usage.PrettyModel(v)))
		case e.key == "claude":
			parts = append(parts, provStyle[e.styleKey].Render(e.label+" "+usage.PrettyModel(v)))
		default:
			parts = append(parts, provStyle[e.styleKey].Render(e.label+" "+v))
		}
	}
	return styleHelp.Render("기본모델 ") + strings.Join(parts, styleHelp.Render(" · "))
}

// ctxBadge는 행 끝의 "[모델 · ctx% · age]" 조각.
func (m Model) ctxBadge(a *state.Agent) string {
	info, ok := m.ctx[a.ID]
	if !ok {
		return ""
	}
	var parts []string
	if info.Model != "" {
		st := styleModel
		if usage.IsFable(info.Model) {
			st = levelStyle["yellow"].Bold(true) // Fable로 도는 세션은 한눈에
		}
		parts = append(parts, st.Render(info.Model))
	}
	if info.UsedPct != nil {
		parts = append(parts, ctxStyle(*info.UsedPct).Bold(true).Render("ctx "+ctxPctText(info)))
	}
	if !info.TS.IsZero() {
		parts = append(parts, styleHeader.Render(cli.Since(info.TS, m.now)))
	}
	if len(parts) == 0 {
		return ""
	}
	return styleHeader.Render("[") + strings.Join(parts, styleHeader.Render(" · ")) + styleHeader.Render("]")
}

// ctxPctText는 % 숫자 조각 — 근사값(gemini류)은 ~ 접두로 정직하게 표시.
func ctxPctText(info usage.CtxInfo) string {
	s := fmt.Sprintf("%d%%", int(*info.UsedPct))
	if info.Approx {
		return "~" + s
	}
	return s
}

// ctxBadgePlain은 색 없는 컨텍스트 뱃지 (선택 바 렌더용).
func (m Model) ctxBadgePlain(a *state.Agent) string {
	info, ok := m.ctx[a.ID]
	if !ok {
		return ""
	}
	var parts []string
	if info.Model != "" {
		parts = append(parts, info.Model)
	}
	if info.UsedPct != nil {
		parts = append(parts, "ctx "+ctxPctText(info))
	}
	if !info.TS.IsZero() {
		parts = append(parts, cli.Since(info.TS, m.now))
	}
	if len(parts) == 0 {
		return ""
	}
	return "[" + strings.Join(parts, " · ") + "]"
}

// usageAge는 coach 데이터의 나이 표시 — 늦게 갱신될 수 있음을 숨기지 않는다.
func (m Model) usageAge() string {
	if m.usagePay == nil || m.usagePay.TS == "" {
		return ""
	}
	ts, err := time.Parse(time.RFC3339, m.usagePay.TS)
	if err != nil {
		return ""
	}
	age := m.now.Sub(ts)
	label := cli.Since(ts, m.now) + " 전 데이터"
	if age > 15*time.Minute {
		return levelStyle["yellow"].Render("⚠ " + label)
	}
	return styleHelp.Render(label)
}

// usageSummaryLine은 메인 뷰 헤더의 사용량 한 줄 요약.
func (m Model) usageSummaryLine() string {
	if m.usagePay == nil {
		return ""
	}
	var parts []string
	for _, key := range []string{"claude", "codex", "antigravity"} {
		p, ok := m.usagePay.Providers[key]
		if !ok || !p.OK {
			continue
		}
		var wins []string
		for _, wk := range []string{"5h", "7d", "fable_7d"} {
			if w, ok := p.Windows[wk]; ok && w.LeftPct != nil {
				wins = append(wins, fmt.Sprintf("%s %d%%", winLabel[wk], int(*w.LeftPct)))
			}
		}
		if len(wins) == 0 { // antigravity: 계정 창 중 데이터 있는 첫 것
			for wk, w := range p.Windows {
				if w.LeftPct != nil {
					wins = append(wins, fmt.Sprintf("%s %d%%", wk, int(*w.LeftPct)))
					break
				}
			}
		}
		if len(wins) > 0 {
			parts = append(parts, providerStyle(key, p.Level).Render(
				providerEmoji(key, p.Level)+" "+strings.Title(key)+" "+strings.Join(wins, " · ")))
		}
	}
	line := strings.Join(parts, styleHelp.Render("  |  "))
	if age := m.usageAge(); age != "" {
		line += styleHelp.Render("  ·  ") + age
	}
	return line
}

// starterLine은 MultiAgent 활성 작업 요약 한 줄 (활성 있을 때만).
func (m Model) starterLine() string {
	if len(m.starterTasks) == 0 {
		return ""
	}
	var parts []string
	for i, t := range m.starterTasks {
		if i >= 3 {
			parts = append(parts, fmt.Sprintf("외 %d", len(m.starterTasks)-3))
			break
		}
		parts = append(parts, fmt.Sprintf("%s(%s)", t.Name, t.Status))
	}
	return styleHelp.Render("MultiAgent: ") + strings.Join(parts, styleHelp.Render(" · "))
}

// usageView는 u 키로 전환하는 사용량 전용 화면 — Discord 카드와 같은 정보.
func (m Model) usageView() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("AgentLayer — 사용량"))
	if age := m.usageAge(); age != "" {
		b.WriteString("  " + age)
	}
	b.WriteString("\n\n")
	if m.usagePay == nil {
		b.WriteString(styleHelp.Render("사용량 불러오는 중… 콜드 시작은 최대 2분 걸립니다.\n"))
		b.WriteString(styleHelp.Render("(계속 비어 있으면 usage-coach 미설치 — brew/GitHub에서 coach 설치 후 재시도)\n"))
	} else {
		for _, key := range []string{"claude", "codex", "antigravity"} {
			p, ok := m.usagePay.Providers[key]
			if !ok || !p.OK {
				continue
			}
			head := fmt.Sprintf("%s %s — %s", providerEmoji(key, p.Level), strings.Title(key), p.Action)
			b.WriteString(providerStyle(key, p.Level).Render(head) + "\n")
			b.WriteString(styleHelp.Render("  "+p.Email) + "\n")
			// 막대는 coach와 동일하게 provider 브랜드 색 (level 무관)
			barStyle, ok := provStyle[key]
			if !ok {
				barStyle = levelStyle["white"]
			}
			for _, wk := range windowOrder(p.Windows) {
				w := p.Windows[wk]
				label := winLabel[wk]
				if label == "" {
					label = wk // antigravity 계정명 등
				}
				line := fmt.Sprintf("  %-12s %s", label, barStyle.Render(usage.Gauge(w.LeftPct, 14)))
				if w.LeftPct != nil {
					line += fmt.Sprintf("  %3d%%", int(*w.LeftPct))
					if r := usage.ResetLabel(w.ResetMin); r != "" {
						line += styleHelp.Render(" · 리셋 " + r)
					}
				} else {
					line += styleHelp.Render("   —%")
				}
				b.WriteString(line + "\n")
			}
			b.WriteString("  " + p.Reason + "\n\n")
		}
	}
	b.WriteString(styleHelp.Render("u 관제 화면으로 · r 새로고침 · q 종료"))
	return b.String()
}

// windowOrder는 표시 순서: 표준 창(5h,7d,Fable) 먼저, 나머지는 이름순.
func windowOrder(ws map[string]usage.Window) []string {
	var std, rest []string
	for _, k := range []string{"5h", "daily", "7d", "fable_7d"} {
		if _, ok := ws[k]; ok {
			std = append(std, k)
		}
	}
	for k := range ws {
		if winLabel[k] == "" {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(std, rest...)
}

func (m Model) View() string {
	if m.showInfo {
		return styleTitle.Render("AgentLayer — 상세") + "\n\n" + m.infoText +
			"\n" + styleHelp.Render("i/esc 닫기 · enter 점프 · q 종료")
	}
	if m.showUsage {
		return m.usageView()
	}
	var b strings.Builder
	b.WriteString(styleTitle.Render("AgentLayer") + "  " + summary(m.agents))
	if s := m.defaultModelLine(); s != "" {
		b.WriteString(styleHelp.Render("  ·  ") + s)
	}
	b.WriteString("\n")
	if s := m.usageSummaryLine(); s != "" {
		b.WriteString(s + "\n")
	} else if m.usagePay == nil {
		b.WriteString(styleHelp.Render("사용량 불러오는 중… (콜드 시작은 최대 2분 걸립니다)") + "\n")
	}
	if s := m.starterLine(); s != "" {
		b.WriteString(s + "\n")
	}
	b.WriteString(styleHeader.Render("STATE    "+cli.PadRight("AGENT", 8)+cli.PadRight("SESSION", 21)+cli.PadRight("TASK", 31)+"DIR·SINCE") + "\n")

	for i, a := range m.agents {
		task := runewidth.Truncate(a.Task, 28, "…")
		if i == m.cursor {
			// 선택 행: 화면 전체 폭 바. 조각마다 같은 배경을 입혀
			// 상태 색은 살리면서 바가 중간에 끊기지 않게 한다.
			st := stateText(a, m.now)
			rest := fmt.Sprintf(" %s %s %s %s · %s",
				cli.PadRight(a.Kind, 7), cli.PadRight(a.Tmux.Session, 20),
				cli.PadRight(task, 30),
				cli.ShortenHome(a.CWD), cli.Since(a.StateSince, m.now))
			if m.discordWired[a.CWD] {
				rest += " ⌁"
			}
			if br, ok := m.wtBranch[a.CWD]; ok {
				rest += " ⎇ " + br
			}
			if badge := m.ctxBadgePlain(a); badge != "" {
				rest += " " + badge
			}
			used := 2 + runewidth.StringWidth(st) + runewidth.StringWidth(rest)
			width := m.width
			if width < used+1 {
				width = used + 1
			}
			rest = cli.PadRight(rest, width-2-runewidth.StringWidth(st))
			line := styleSelected.Render("▸ ") +
				stateStyles[a.State].Background(selBg).Bold(true).Render(st) +
				styleSelected.Render(rest)
			b.WriteString(line + "\n")
			continue
		}
		line := fmt.Sprintf("  %s %s %s %s %s · %s",
			stateBadge(a, m.now), cli.PadRight(a.Kind, 7), cli.PadRight(a.Tmux.Session, 20),
			cli.PadRight(task, 30),
			cli.ShortenHome(a.CWD), cli.Since(a.StateSince, m.now))
		if m.discordWired[a.CWD] {
			line += " " + styleDiscord.Render("⌁")
		}
		if br, ok := m.wtBranch[a.CWD]; ok {
			line += " " + styleTitle.Render("⎇ "+br)
		}
		if badge := m.ctxBadge(a); badge != "" {
			line += " " + badge
		}
		b.WriteString(line + "\n")
	}
	if len(m.agents) == 0 {
		b.WriteString(styleHelp.Render("  tmux에서 실행 중인 claude/codex/gemini가 없습니다\n"))
	}
	// 선택 세션 화면 미리보기 (C-b s 스타일) — 남는 공간에
	if h := m.previewHeight(); h >= 3 && m.preview != "" {
		sel := m.selected()
		title := "미리보기"
		if sel != nil {
			title = sel.Tmux.Session + " 미리보기"
		}
		width := m.width
		if width < 20 {
			width = 80
		}
		b.WriteString("\n" + styleTitle.Render("── "+title+" ") +
			styleHeader.Render(strings.Repeat("─", max(0, width-runewidth.StringWidth(title)-5))) + "\n")
		lines := strings.Split(m.preview, "\n")
		if len(lines) > h {
			lines = lines[len(lines)-h:]
		}
		for _, ln := range lines {
			b.WriteString(styleModel.Render(runewidth.Truncate(ln, width, "")) + "\n")
		}
	}

	if m.pendingCmd != "" {
		what := "이어서하기(기상)"
		if m.pendingCmd == "close" {
			what = "마감"
		}
		b.WriteString("\n" + stateStyles[state.StateWaiting].Render(
			fmt.Sprintf("⚠ 모든 세션 %s — 전 세션에 지시를 보냅니다. y 확인 / 다른 키 취소", what)))
	} else if m.notice != "" {
		b.WriteString("\n" + styleTitle.Render(m.notice))
	}
	b.WriteString("\n" + styleHelp.Render("j/k 이동 · enter 점프+읽음 · o 읽음 · i 상세 · g git · u 사용량 · W 전체기상 · C 전체마감 · r 새로고침 · q 종료"))
	if m.err != nil {
		b.WriteString("\n" + stateStyles[state.StateError].Render("⚠ "+m.err.Error()))
	}
	return b.String()
}
