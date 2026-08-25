// Package cli는 비대화형 서브커맨드(status, init)를 구현한다.
// SSH·스크립트에서 그대로 쓰이므로 색·커서 제어 없이 plain 텍스트만 낸다.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

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
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "STATE\tAGENT\tSESSION\tTASK\tDIR\tSINCE")
	for _, a := range agents {
		task := a.Task
		if len([]rune(task)) > 40 {
			task = string([]rune(task)[:39]) + "…"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			stateLabel(a, now), a.Kind, a.Tmux.Session, task,
			ShortenHome(a.CWD), Since(a.StateSince, now))
	}
	return tw.Flush()
}
