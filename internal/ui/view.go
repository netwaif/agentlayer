package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/netwaif/agentlayer/internal/cli"
	"github.com/netwaif/agentlayer/internal/state"
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

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("AgentLayer") + "  " + summary(m.agents) + "\n")
	b.WriteString(styleHeader.Render(fmt.Sprintf("%-8s %-7s %-20s %-30s %s", "STATE", "AGENT", "SESSION", "TASK", "DIR·SINCE")) + "\n")

	for i, a := range m.agents {
		task := a.Task
		if r := []rune(task); len(r) > 28 {
			task = string(r[:27]) + "…"
		}
		line := fmt.Sprintf("%s %-7s %-20s %-30s %s · %s",
			stateBadge(a, m.now), a.Kind, a.Tmux.Session, task,
			cli.ShortenHome(a.CWD), cli.Since(a.StateSince, m.now))
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
	b.WriteString("\n" + styleHelp.Render("j/k 이동 · enter 점프+읽음 · o 읽음 · r 새로고침 · q 종료"))
	if m.err != nil {
		b.WriteString("\n" + stateStyles[state.StateError].Render("점프 실패: "+m.err.Error()))
	}
	return b.String()
}
