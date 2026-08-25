package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/usage"
	"github.com/netwaif/agentlayer/internal/wiring"
)

// InfoData는 상세 카드 한 장에 들어가는 모든 것.
type InfoData struct {
	Agent  *state.Agent
	Wiring wiring.Info
	Ctx    usage.CtxInfo // 모델·ctx% (없으면 zero)
	Branch string        // worktree 브랜치 (없으면 빈 값)
}

// RenderInfo는 에이전트 상세 카드를 plain 텍스트로 그린다.
// TUI와 CLI가 같은 내용을 쓴다 (색은 TUI가 자체 처리).
func RenderInfo(w io.Writer, d InfoData, now time.Time) {
	a := d.Agent
	name := a.Tmux.Session
	if name == "" {
		name = a.ID
	}
	fmt.Fprintf(w, "%s  %s\n", name, stateLabel(a, now))

	fmt.Fprintf(w, "  폴더       %s\n", ShortenHome(a.CWD))
	engine := a.Kind
	if d.Ctx.Model != "" {
		engine += " · " + d.Ctx.Model
	}
	if d.Ctx.UsedPct != nil {
		engine += fmt.Sprintf(" · ctx %d%%", int(*d.Ctx.UsedPct))
	}
	fmt.Fprintf(w, "  엔진       %s\n", engine)
	if d.Branch != "" {
		fmt.Fprintf(w, "  브랜치     ⎇ %s (worktree)\n", d.Branch)
	}
	fmt.Fprintf(w, "  tmux       %s:%d %s (pid %d)\n", a.Tmux.Session, a.Tmux.Window, a.Tmux.PaneID, a.PID)

	if d.Wiring.BotName != "" {
		fmt.Fprintf(w, "  folder-bot %s (engine %s)\n", d.Wiring.BotName, d.Wiring.Engine)
	}
	if dc := d.Wiring.Discord; dc != nil {
		for _, ch := range dc.Channels {
			label := ch.Label
			if label == "" {
				label = wiring.ShortID(ch.ID)
			} else {
				label += " (" + wiring.ShortID(ch.ID) + ")"
			}
			mention := "mention 불필요"
			if ch.RequireMention {
				mention = "mention 필요"
			}
			fmt.Fprintf(w, "  Discord    채널 %s · 허용 %d명 · %s\n", label, ch.AllowCount, mention)
		}
		if len(dc.Channels) == 0 {
			fmt.Fprintf(w, "  Discord    연결됨 (채널 미등록 · dmPolicy %s)\n", dc.DMPolicy)
		}
	} else if br := d.Wiring.Bridge; br != nil {
		alive := "데몬 살아있음"
		if !br.Alive {
			alive = "⚠ 데몬 죽음"
		}
		fmt.Fprintf(w, "  Discord    codex 브리지 (%s) · %s\n", ShortenHome(br.Dir), alive)
	} else if d.Wiring.DiscordConnected() {
		fmt.Fprintln(w, "  Discord    연결됨 (LaunchAgent 경유)")
	} else {
		fmt.Fprintln(w, "  Discord    연결 없음")
	}

	if len(d.Wiring.LaunchAgents) > 0 {
		fmt.Fprintf(w, "  구동       LaunchAgent %s\n", strings.Join(d.Wiring.LaunchAgents, ", "))
	} else {
		fmt.Fprintln(w, "  구동       수동 실행 (LaunchAgent 없음)")
	}

	if a.SessionID != "" {
		fmt.Fprintf(w, "  세션 ID    %s → 죽으면 agentlayer resume %s\n", shortSession(a.SessionID), a.ID)
	} else {
		fmt.Fprintln(w, "  세션 ID    미기록 (hook 이벤트가 오면 채워짐)")
	}
	if a.Task != "" {
		fmt.Fprintf(w, "  최근 작업  %s\n", a.Task)
	}
	fmt.Fprintf(w, "  상태 경과  %s 동안 %s (갱신 %s 전)\n",
		Since(a.StateSince, now), string(a.State), Since(a.UpdatedAt, now))
}

func shortSession(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:8] + "…"
}

// FindAgent는 세션 이름 또는 ID로 에이전트를 찾는다.
func FindAgent(agents []*state.Agent, key string) *state.Agent {
	for _, a := range agents {
		if a.ID == key || a.Tmux.Session == key {
			return a
		}
	}
	return nil
}
