// Package cli는 비대화형 서브커맨드(status, init)를 구현한다.
// SSH·스크립트에서 그대로 쓰이므로 색·커서 제어 없이 plain 텍스트만 낸다.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/netwaif/agentlayer/internal/state"
)

func stateLabel(a *state.Agent, now time.Time) string {
	switch a.State {
	case state.StateWorking:
		if a.Stale(now) {
			return "[WORK?]"
		}
		return "[WORK]"
	case state.StateWaiting:
		return "[WAIT]"
	case state.StateDoneUnread:
		return "[DONE]"
	case state.StateError:
		return "[ERR ]"
	case state.StateIdle:
		return "[idle]"
	default:
		return "[dead]"
	}
}

// ShortenHome은 절대경로의 홈 부분을 ~로 줄인다.
func ShortenHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// Since는 상태 경과 시간을 짧게("5m", "2h", "3d") 표기한다.
func Since(from, now time.Time) string {
	d := now.Sub(from)
	switch {
	case d < time.Minute:
		return "방금"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// Status는 저장소 내용을 표 또는 JSON으로 출력한다.
// 동기화(Sync)는 호출자 책임 — 이 함수는 렌더링만 한다.
func Status(w io.Writer, st *state.Store, jsonOut bool, now time.Time) error {
	agents, err := st.List()
	if err != nil {
		return err
	}
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if agents == nil {
			agents = []*state.Agent{}
		}
		return enc.Encode(agents)
	}
	if len(agents) == 0 {
		fmt.Fprintln(w, "에이전트 없음 — tmux에서 claude/codex/gemini가 돌고 있는지 확인하세요.")
		return nil
	}
	// tabwriter는 한글(동아시아 폭 2칸)을 1칸으로 세서 열이 어긋난다 —
	// runewidth 기반 수동 패딩으로 표시 폭을 맞춘다.
	rows := [][]string{{"STATE", "AGENT", "SESSION", "TASK", "DIR", "SINCE"}}
	for _, a := range agents {
		task := a.Task
		if runewidth.StringWidth(task) > 40 {
			task = runewidth.Truncate(task, 39, "…")
		}
		rows = append(rows, []string{
			stateLabel(a, now), a.Kind, a.Tmux.Session, task,
			ShortenHome(a.CWD), Since(a.StateSince, now)})
	}
	widths := make([]int, len(rows[0]))
	for _, r := range rows {
		for i, c := range r {
			if w := runewidth.StringWidth(c); w > widths[i] {
				widths[i] = w
			}
		}
	}
	for _, r := range rows {
		var line strings.Builder
		for i, c := range r {
			if i == len(r)-1 {
				line.WriteString(c)
				break
			}
			line.WriteString(PadRight(c, widths[i]+2))
		}
		fmt.Fprintln(w, strings.TrimRight(line.String(), " "))
	}
	return nil
}

// PadRight는 표시 폭 기준으로 오른쪽 공백을 채운다 (한글 2칸 반영).
func PadRight(s string, width int) string {
	gap := width - runewidth.StringWidth(s)
	if gap < 0 {
		gap = 0
	}
	return s + strings.Repeat(" ", gap)
}
