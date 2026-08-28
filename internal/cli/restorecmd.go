// restorecmd: 재부팅 등으로 tmux 서버가 사라진 뒤, 상태 저장소의 죽은
// 레코드로 세션·window 배치를 재구성한다. 기본은 새 CLI 기동(재정박은
// wake-all/SESSION.md 방식이 우월), --resume일 때만 대화까지 부활시킨다.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
	"github.com/netwaif/agentlayer/internal/usage"
)

// RestoreItem은 복원 계획 한 건: 어느 레코드를 어떤 명령으로 살릴지.
type RestoreItem struct {
	Agent      *state.Agent
	NewSession bool   // true면 세션 생성, false면 기존 세션에 window 추가
	Cmd        string // pane에 입력할 기동 명령
}

// RestorePlan은 복원 계획 전체와 건너뛴 사유들.
type RestorePlan struct {
	Items   []RestoreItem
	Skipped []string
}

// PlanRestore는 죽은 레코드를 세션·window 순으로 훑어 복원 계획을 세운다.
// sessionExists는 tmux 현실 조회 주입점(테스트용).
func PlanRestore(agents []*state.Agent, sessionExists func(string) bool, resume bool) RestorePlan {
	sorted := append([]*state.Agent{}, agents...)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Tmux.Session != b.Tmux.Session {
			return a.Tmux.Session < b.Tmux.Session
		}
		if a.Tmux.Window != b.Tmux.Window {
			return a.Tmux.Window < b.Tmux.Window
		}
		return a.ID < b.ID
	})
	var plan RestorePlan
	seenWindow := map[string]bool{}   // "세션:window" — 분할 pane 중복 제거
	sessionPlanned := map[string]bool{}
	for _, a := range sorted {
		if a.State != state.StateDead {
			continue // 살아 있는 세션은 복원 대상이 아니다
		}
		if a.Tmux.Session == "" {
			plan.Skipped = append(plan.Skipped, a.ID+": 세션 이름 기록 없음")
			continue
		}
		if a.CWD == "" {
			plan.Skipped = append(plan.Skipped, a.ID+": 폴더 기록 없음")
			continue
		}
		if st, err := os.Stat(a.CWD); err != nil || !st.IsDir() {
			plan.Skipped = append(plan.Skipped, a.ID+": 폴더 없음 "+a.CWD)
			continue
		}
		wk := fmt.Sprintf("%s:%d", a.Tmux.Session, a.Tmux.Window)
		if seenWindow[wk] {
			continue // 같은 window의 분할 pane — 대표 하나만 복원
		}
		seenWindow[wk] = true
		cmd := freshCommand(a)
		if resume {
			if rc, err := ResumeCommand(a); err == nil {
				cmd = rc
			} else {
				plan.Skipped = append(plan.Skipped,
					a.ID+": 대화 부활 불가("+err.Error()+") — 새로 기동")
			}
		}
		if cmd == "" {
			plan.Skipped = append(plan.Skipped, a.ID+": 알 수 없는 종류 "+a.Kind)
			continue
		}
		item := RestoreItem{Agent: a, Cmd: cmd,
			NewSession: !sessionExists(a.Tmux.Session) && !sessionPlanned[a.Tmux.Session]}
		sessionPlanned[a.Tmux.Session] = true
		plan.Items = append(plan.Items, item)
	}
	return plan
}

// freshCommand는 종류별 새 기동 명령. gemini는 agy 흔적이 있으면 agy
// (usage.GeminiCommand — wt new와 같은 규칙).
func freshCommand(a *state.Agent) string {
	switch a.Kind {
	case "claude":
		return "claude"
	case "codex":
		return "codex"
	case "gemini":
		return usage.GeminiCommand()
	}
	return ""
}

// RunRestore는 restore 서브커맨드 본체. 죽은 레코드로 배치를 재구성하고
// 각 pane에 기동 명령을 SendText로 입력한다 (셸이 살아 있어야 명령 종료
// 후에도 window가 남고, 사용자 셸 환경도 그대로 탄다).
func RunRestore(w io.Writer, st *state.Store, tm tmuxx.Tmux, args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	resume := fs.Bool("resume", false, "대화까지 부활 (claude --resume 등) — 부푼 컨텍스트도 그대로 재적재됨")
	dryRun := fs.Bool("dry-run", false, "실행 없이 계획만 출력")
	if err := fs.Parse(args); err != nil {
		return err
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	plan := PlanRestore(agents, tm.HasSession, *resume)
	for _, s := range plan.Skipped {
		fmt.Fprintln(w, "  건너뜀:", s)
	}
	if len(plan.Items) == 0 {
		fmt.Fprintln(w, "복원할 죽은 세션이 없습니다.")
		return nil
	}
	for _, it := range plan.Items {
		verb := "window 추가"
		if it.NewSession {
			verb = "세션 생성"
		}
		fmt.Fprintf(w, "  %s %s (%s) ← %s\n", verb, it.Agent.Tmux.Session, ShortenHome(it.Agent.CWD), it.Cmd)
	}
	if *dryRun {
		fmt.Fprintln(w, "(dry-run — 실행 안 함)")
		return nil
	}
	for _, it := range plan.Items {
		var pane string
		var err error
		if it.NewSession {
			pane, err = tm.NewSession(it.Agent.Tmux.Session, it.Agent.CWD)
		} else {
			pane, err = tm.NewWindowIn(it.Agent.Tmux.Session, filepath.Base(it.Agent.CWD), it.Agent.CWD)
		}
		if err != nil {
			return fmt.Errorf("%s 복원 실패: %w", it.Agent.Tmux.Session, err)
		}
		if err := tm.SendText(pane, it.Cmd); err != nil {
			return fmt.Errorf("%s 기동 명령 입력 실패: %w", it.Agent.Tmux.Session, err)
		}
		// 원본 죽은 레코드 정리 — 새 pane의 hook이 새 레코드를 만들므로
		// 옛 dead 행이 status에 나란히 남지 않게 한다.
		_ = st.Delete(it.Agent.ID)
	}
	fmt.Fprintf(w, "%d개 복원 완료.", len(plan.Items))
	if !*resume {
		fmt.Fprint(w, " 각 CLI가 뜨면 `agentlayer wake-all`로 재정박하세요 (SESSION.md 있는 폴더만 대상).")
	}
	fmt.Fprintln(w)
	return nil
}

// ResumeCommand는 에이전트 종류별 대화 재개 명령을 만든다.
//   - claude: claude --resume <session_id>
//   - codex:  codex resume <session_id> (notify에 세션 ID가 없어 rollout에서 추출)
//   - gemini: agy --conversation <id> (agy 대화만 — stock Gemini CLI는 재개 CLI가 없다)
func ResumeCommand(a *state.Agent) (string, error) {
	switch a.Kind {
	case "claude":
		if a.SessionID == "" {
			return "", fmt.Errorf("session_id가 기록되지 않은 claude 세션입니다")
		}
		return fmt.Sprintf("claude --resume %s", a.SessionID), nil
	case "codex":
		if a.CWD == "" {
			return "", fmt.Errorf("cwd가 없는 codex 세션입니다")
		}
		sid := usage.CodexSessionID(usage.CodexSessionsRoot(), a.CWD)
		if sid == "" {
			return "", fmt.Errorf("codex rollout에서 세션을 못 찾았습니다: %s", a.CWD)
		}
		return fmt.Sprintf("codex resume %s", sid), nil
	case "gemini":
		if a.SessionID == "" {
			return "", fmt.Errorf("대화 ID가 기록되지 않은 gemini 세션입니다")
		}
		// agy 대화인지 확인 — brain 폴더가 있으면 agy, 없으면 stock CLI(재개 불가)
		brain := filepath.Join(usage.GeminiDir(), "antigravity-cli", "brain", a.SessionID)
		if _, err := os.Stat(brain); err != nil {
			return "", fmt.Errorf("stock Gemini CLI 세션은 CLI 재개를 지원하지 않습니다 (agy 대화만 가능)")
		}
		return fmt.Sprintf("agy --conversation %s", a.SessionID), nil
	}
	return "", fmt.Errorf("%s 종류는 resume을 지원하지 않습니다", a.Kind)
}
