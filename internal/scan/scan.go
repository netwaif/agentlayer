// Package scan은 tmux pane 현실과 상태 저장소를 동기화한다.
// 스캐너는 발견·좌표 갱신·소실 처리만 하고, 의미 상태(WORKING 등)는
// hook의 영역이므로 절대 덮어쓰지 않는다.
package scan

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
)

// DEAD 레코드를 보존하는 기간. 그 뒤에는 저장소에서 정리한다.
const deadRetention = 24 * time.Hour

// versionRe: Claude Code는 프로세스 이름을 자기 버전("2.1.241")으로 바꾼다.
var versionRe = regexp.MustCompile(`^\d+(\.\d+)+$`)

// DetectKind는 pane이 어떤 에이전트인지 판정한다.
// 화면 내용 스크래핑은 하지 않는다 — command와 title 메타데이터만 본다.
// 판정은 로케일 무관해야 한다: LANG 없는 환경(LaunchAgent)에서 tmux가
// 제목의 비ASCII를 치환하므로, ✳ 제목 신호는 보조로만 쓴다.
func DetectKind(p tmuxx.Pane) string {
	cmd := strings.ToLower(p.Command)
	switch {
	case strings.HasPrefix(cmd, "claude"):
		return "claude"
	case strings.HasPrefix(cmd, "codex"):
		return "codex"
	case strings.HasPrefix(cmd, "gemini"), strings.HasPrefix(cmd, "agy"):
		// agy = Antigravity CLI (Gemini 계열) — 같은 gemini kind로 관제한다
		return "gemini"
	case versionRe.MatchString(cmd):
		// 버전 형식 command = Claude Code (프로세스명을 버전으로 바꿈)
		return "claude"
	case strings.Contains(p.Title, "✳"):
		return "claude"
	}
	return ""
}

// AgentID는 결정적 ID. hook은 TMUX_PANE만 알므로 pane ID 기반으로 만든다.
// tmux 서버가 재시작되면 pane ID가 재사용될 수 있으나, 소실 레코드는
// DEAD로 정리되므로 관제 목적에는 충분하다.
func AgentID(kind string, p tmuxx.Pane) string {
	return IDForPane(kind, p.PaneID)
}

// IDForPane은 hook 경로(TMUX_PANE 환경변수)와 스캐너가 공유하는 ID 규칙.
func IDForPane(kind, paneID string) string {
	return fmt.Sprintf("%s-%s", kind, strings.TrimPrefix(paneID, "%"))
}

// Sync는 pane 목록을 정본 저장소에 반영한다.
func Sync(st *state.Store, panes []tmuxx.Pane, now time.Time) error {
	existing, err := st.List()
	if err != nil {
		return err
	}
	byID := make(map[string]*state.Agent, len(existing))
	for _, a := range existing {
		byID[a.ID] = a
	}

	alive := make(map[string]bool)
	for _, p := range panes {
		kind := DetectKind(p)
		if kind == "" {
			continue
		}
		id := AgentID(kind, p)
		alive[id] = true
		a, ok := byID[id]
		if !ok {
			a = &state.Agent{ID: id, Kind: kind, State: state.StateIdle,
				UpdatedAt: now, StateSince: now}
		}
		// 좌표·환경은 항상 현실을 따른다. 의미 상태는 건드리지 않는다.
		a.Tmux = state.TmuxRef{Session: p.Session, Window: p.Window, PaneID: p.PaneID}
		a.CWD = p.Path
		a.PID = p.PanePID
		if a.State == state.StateDead {
			// 같은 pane ID가 되살아났다(재사용 포함) — 새 관찰로 취급
			a.Transition(state.StateIdle, now)
		}
		if err := st.Save(a); err != nil {
			return err
		}
	}

	for _, a := range existing {
		if alive[a.ID] {
			continue
		}
		switch {
		case a.State == state.StateDead && now.Sub(a.StateSince) > deadRetention:
			if err := st.Delete(a.ID); err != nil {
				return err
			}
		case a.State != state.StateDead:
			a.Transition(state.StateDead, now)
			if err := st.Save(a); err != nil {
				return err
			}
		}
	}
	return nil
}
