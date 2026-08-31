package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
)

// 일괄 지시 메시지 — 사용자의 loadout 세션 이어가기 조각 문구와 동일.
const (
	WakeMessage  = "세션 이어서하자"
	CloseMessage = "세션 마감하자"
)

// HasSessionHandoff는 폴더가 세션 이어가기 규율을 갖췄는지 검사한다:
// SESSION.md가 있거나, CLAUDE.md/AGENTS.md에 loadout 조각 마커가 있으면 참.
// wake-all/close-all 지시는 이 규율이 있는 세션에만 의미가 있다.
func HasSessionHandoff(cwd string) bool {
	if cwd == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(cwd, "SESSION.md")); err == nil {
		return true
	}
	for _, f := range []string{"CLAUDE.md", "AGENTS.md"} {
		if b, err := os.ReadFile(filepath.Join(cwd, f)); err == nil {
			if strings.Contains(string(b), "store:session-handoff") ||
				strings.Contains(string(b), "세션 이어가기") {
				return true
			}
		}
	}
	return false
}

// Targets는 일괄 전송 대상을 고른다: claude·codex(loadout이 양쪽에서 돌므로),
// 죽지 않은 것, 그리고 자기 자신 pane 제외(자기 세션에 지시를 쏘지 않게).
func Targets(agents []*state.Agent, selfPane string, except []string) []*state.Agent {
	skip := map[string]bool{}
	for _, e := range except {
		if e = strings.TrimSpace(e); e != "" {
			skip[e] = true
		}
	}
	var out []*state.Agent
	for _, a := range agents {
		// 3사 공통 원칙(8-26): 관제 대상 kind면 일괄 지시도 받는다
		if a.Kind != "claude" && a.Kind != "codex" && a.Kind != "gemini" {
			continue
		}
		if a.State == state.StateDead {
			continue
		}
		if selfPane != "" && a.Tmux.PaneID == selfPane {
			continue
		}
		if skip[a.Tmux.Session] {
			continue
		}
		out = append(out, a)
	}
	return out
}

type AllOptions struct {
	yes     bool
	except  []string
	watch   bool
	timeout time.Duration
}

func ParseAllFlags(name string, args []string, defaultWatch bool) (*AllOptions, []string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	yes := fs.Bool("yes", false, "확인 없이 전송 (자동화용)")
	except := fs.String("except", "", "제외할 세션 이름 (쉼표 구분)")
	watch := fs.Bool("watch", defaultWatch, "전송 후 완료(DONE) 감시")
	timeout := fs.Duration("timeout", 10*time.Minute, "감시 타임아웃")
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}
	var ex []string
	if *except != "" {
		ex = strings.Split(*except, ",")
	}
	return &AllOptions{yes: *yes, except: ex, watch: *watch, timeout: *timeout}, fs.Args(), nil
}

// RunAll은 wake-all / close-all / broadcast의 공통 본체.
// handoffOnly는 기상·마감(조각 있는 세션만), broadcast는 false(전체).
func RunAll(w io.Writer, st *state.Store, tm tmuxx.Tmux, message string, o *AllOptions, handoffOnly bool, now time.Time) error {
	agents, err := st.List()
	if err != nil {
		return err
	}
	targets := Targets(agents, tmuxx.CurrentPaneID(), o.except)
	if handoffOnly {
		var kept []*state.Agent
		for _, a := range targets {
			if HasSessionHandoff(a.CWD) {
				kept = append(kept, a)
			}
		}
		if len(kept) < len(targets) {
			fmt.Fprintf(w, "(세션 이어가기 조각·SESSION.md 없는 %d개 폴더는 제외)\n", len(targets)-len(kept))
		}
		targets = kept
	}
	if len(targets) == 0 {
		fmt.Fprintln(w, "전송할 대상이 없습니다.")
		return nil
	}
	fmt.Fprintf(w, "전송 대상 %d개 — 메시지: %q\n", len(targets), message)
	for _, a := range targets {
		note := ""
		if a.State == state.StateWorking {
			note = "  ⚠ 작업 중 — 지시가 현재 턴 뒤에 처리됩니다"
		}
		fmt.Fprintf(w, "  %-7s %-20s %s%s\n", a.Kind, a.Tmux.Session, ShortenHome(a.CWD), note)
	}
	if !o.yes {
		fmt.Fprint(w, "진행할까요? [y/N] ")
		r := bufio.NewReader(os.Stdin)
		line, _ := r.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(line)) != "y" {
			fmt.Fprintln(w, "취소했습니다.")
			return nil
		}
	}
	var sent []*state.Agent
	for _, a := range targets {
		if err := tm.SendText(a.Tmux.PaneID, message); err != nil {
			fmt.Fprintf(w, "  ✖ %s 전송 실패: %v\n", a.Tmux.Session, err)
			continue
		}
		sent = append(sent, a)
	}
	fmt.Fprintf(w, "%d개 세션에 전송 완료.\n", len(sent))
	if !o.watch || len(sent) == 0 {
		return nil
	}
	return watchDone(w, st, sent, now, o.timeout)
}

// SendAll은 확인 절차 없이 대상 전원에게 메시지를 보낸다.
// 확인(y/N)은 호출자(CLI 프롬프트 또는 TUI 키 확인)의 책임이다.
// handoffOnly=true(기상·마감)면 세션 이어가기 규율이 있는 폴더만 대상.
func SendAll(st *state.Store, tm tmuxx.Tmux, message string, handoffOnly bool) (sent, total int, err error) {
	agents, err := st.List()
	if err != nil {
		return 0, 0, err
	}
	targets := Targets(agents, tmuxx.CurrentPaneID(), nil)
	if handoffOnly {
		var kept []*state.Agent
		for _, a := range targets {
			if HasSessionHandoff(a.CWD) {
				kept = append(kept, a)
			}
		}
		targets = kept
	}
	for _, a := range targets {
		if tm.SendText(a.Tmux.PaneID, message) == nil {
			sent++
		}
	}
	return sent, len(targets), nil
}

// watchDone은 전송 후 각 대상의 턴 종료(DONE 전이)를 상태 파일로 감시한다.
// codex는 notify가 비활성이면 DONE이 안 잡히므로 타임아웃 시 미확인으로 표기.
func watchDone(w io.Writer, st *state.Store, sent []*state.Agent, sentAt time.Time, timeout time.Duration) error {
	fmt.Fprintf(w, "완료 감시 중 (타임아웃 %s) — Ctrl+C로 감시만 중단할 수 있습니다.\n", timeout)
	pending := map[string]*state.Agent{}
	for _, a := range sent {
		pending[a.ID] = a
	}
	deadline := time.Now().Add(timeout)
	for len(pending) > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Second)
		for id, a := range pending {
			cur, err := st.Load(id)
			if err != nil {
				continue
			}
			// 전송 이후에 끝난 턴만 인정
			if cur.State == state.StateDoneUnread && cur.StateSince.After(sentAt) {
				fmt.Fprintf(w, "  ✔ %-20s 완료 (%s)\n", a.Tmux.Session, Since(sentAt, time.Now()))
				delete(pending, id)
			}
		}
	}
	if len(pending) == 0 {
		fmt.Fprintf(w, "전체 %d개 세션 완료.\n", len(sent))
		return nil
	}
	fmt.Fprintf(w, "%d개 미확인 (타임아웃):\n", len(pending))
	for _, a := range pending {
		reason := "아직 진행 중이거나 응답 없음"
		if a.Kind == "codex" {
			reason = "codex는 notify 활성화 전이면 완료가 안 잡힙니다 — 직접 확인 필요"
		}
		fmt.Fprintf(w, "  ? %-20s %s\n", a.Tmux.Session, reason)
	}
	return nil
}
