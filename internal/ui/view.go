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
	styleSelected = lipgloss.NewStyle().Background(lipgloss.Color("#3a3a3a")).Bold(true)
	styleHelp     = lipgloss.NewStyle().Foreground(lipgloss.Color("#6a6a6a"))

	stateStyles = map[state.AgentState]lipgloss.Style{
		state.StateWorking:    lipgloss.NewStyle().Foreground(lipgloss.Color("#43B581")),
		state.StateWaiting:    lipgloss.NewStyle().Foreground(lipgloss.Color("#FAA61A")).Bold(true),
		state.StateDoneUnread: lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaf5f")).Bold(true),
		state.StateError:      lipgloss.NewStyle().Foreground(lipgloss.Color("#F04747")).Bold(true),
		state.StateIdle:       lipgloss.NewStyle().Foreground(lipgloss.Color("#8a8a8a")),
		state.StateDead:       lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")),
	}
)

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
		return "대기"
	case state.StateDoneUnread:
		return "안읽음"
	case state.StateError:
		return "에러"
	case state.StateWorking:
		return "작업중"
	default:
		return "휴지"
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

// ctxBadge는 행 끝의 "[모델 · ctx% · age]" 조각.
func (m Model) ctxBadge(a *state.Agent) string {
	info, ok := m.ctx[a.CWD]
	if !ok {
		return ""
	}
	var parts []string
	if info.Model != "" {
		parts = append(parts, info.Model)
	}
	if info.UsedPct != nil {
		parts = append(parts, ctxStyle(*info.UsedPct).Render(fmt.Sprintf("ctx %d%%", int(*info.UsedPct))))
	}
	if !info.TS.IsZero() {
		parts = append(parts, cli.Since(info.TS, m.now))
	}
	if len(parts) == 0 {
		return ""
	}
	return styleHelp.Render("[") + strings.Join(parts, styleHelp.Render(" · ")) + styleHelp.Render("]")
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
			parts = append(parts, levelStyle[p.Level].Render(
				levelEmoji[p.Level]+" "+strings.Title(key)+" "+strings.Join(wins, " · ")))
		}
	}
	return strings.Join(parts, styleHelp.Render("  |  "))
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
	b.WriteString(styleTitle.Render("AgentLayer — 사용량") + "\n\n")
	if m.usagePay == nil {
		b.WriteString(styleHelp.Render("coach 데이터 없음 — usage-coach가 설치돼 있는지 확인하세요\n"))
	} else {
		for _, key := range []string{"claude", "codex", "antigravity"} {
			p, ok := m.usagePay.Providers[key]
			if !ok || !p.OK {
				continue
			}
			head := fmt.Sprintf("%s %s — %s", levelEmoji[p.Level], strings.Title(key), p.Action)
			b.WriteString(levelStyle[p.Level].Render(head) + "\n")
			b.WriteString(styleHelp.Render("  "+p.Email) + "\n")
			for _, wk := range windowOrder(p.Windows) {
				w := p.Windows[wk]
				label := winLabel[wk]
				if label == "" {
					label = wk // antigravity 계정명 등
				}
				line := fmt.Sprintf("  %-12s %s", label, usage.Gauge(w.LeftPct, 14))
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
	if m.showUsage {
		return m.usageView()
	}
	var b strings.Builder
	b.WriteString(styleTitle.Render("AgentLayer") + "  " + summary(m.agents) + "\n")
	if s := m.usageSummaryLine(); s != "" {
		b.WriteString(s + "\n")
	}
	if s := m.starterLine(); s != "" {
		b.WriteString(s + "\n")
	}
	b.WriteString(styleHeader.Render("STATE    "+cli.PadRight("AGENT", 8)+cli.PadRight("SESSION", 21)+cli.PadRight("TASK", 31)+"DIR·SINCE") + "\n")

	for i, a := range m.agents {
		task := runewidth.Truncate(a.Task, 28, "…")
		line := fmt.Sprintf("%s %s %s %s %s · %s",
			stateBadge(a, m.now), cli.PadRight(a.Kind, 7), cli.PadRight(a.Tmux.Session, 20),
			cli.PadRight(task, 30),
			cli.ShortenHome(a.CWD), cli.Since(a.StateSince, m.now))
		if br, ok := m.wtBranch[a.CWD]; ok {
			line += " " + styleTitle.Render("⎇ "+br)
		}
		if badge := m.ctxBadge(a); badge != "" {
			line += " " + badge
		}
		if i == m.cursor {
			line = styleSelected.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	if len(m.agents) == 0 {
		b.WriteString(styleHelp.Render("  tmux에서 실행 중인 claude/codex/gemini가 없습니다\n"))
	}
	b.WriteString("\n" + styleHelp.Render("j/k 이동 · enter 점프+읽음 · o 읽음 · u 사용량 · r 새로고침 · q 종료"))
	if m.err != nil {
		b.WriteString("\n" + stateStyles[state.StateError].Render("점프 실패: "+m.err.Error()))
	}
	return b.String()
}
