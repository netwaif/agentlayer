package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/netwaif/agentlayer/internal/wiring"
)

// restore 체크리스트의 "꺼져 있는 봇" 묶음 — 자동 기동이 꺼져 있고 지금 안 떠 있는 봇(엔진 불문)을 골라 띄운다.
// 탐지는 wiring.OffBots, 띄우기는 RestoreEnv의 주입점(사이드카 → tmux new-session, 유닛 → launchctl/systemctl).
// 자동 기동이 꺼진 봇이 하나도 없으면 체크리스트·출력은 예전과 같다.

// restoreBotsFile — 지난번 체크리스트에서 고른 봇(기본 체크). <상태폴더>/restore-bots.json
const restoreBotsFile = "restore-bots.json"

type restoreBotsMemory struct {
	Sessions []string `json:"sessions"`
}

// loadRestoreBots — 기억된 선택(세션 이름). 없거나 깨졌으면 빈 집합(전부 해제).
func loadRestoreBots(stateDir string) map[string]bool {
	out := map[string]bool{}
	b, err := os.ReadFile(filepath.Join(stateDir, restoreBotsFile))
	if err != nil {
		return out
	}
	var m restoreBotsMemory
	if json.Unmarshal(b, &m) != nil {
		return out
	}
	for _, s := range m.Sessions {
		out[s] = true
	}
	return out
}

// saveRestoreBots — 이번 선택을 기억한다(실패는 무시 — 기억은 편의일 뿐).
func saveRestoreBots(stateDir string, bots []wiring.OffBot) {
	m := restoreBotsMemory{Sessions: []string{}}
	for _, b := range bots {
		m.Sessions = append(m.Sessions, b.Session)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(stateDir, 0o755)
	_ = os.WriteFile(filepath.Join(stateDir, restoreBotsFile), b, 0o644)
}

// printOffBots — 봇 목록을 방법과 함께 찍는다(dry-run·--bots --dry-run).
func printOffBots(w io.Writer, bots []wiring.OffBot, title string) {
	if len(bots) == 0 {
		return
	}
	fmt.Fprintln(w, title)
	for _, b := range bots {
		engine := b.Engine
		if engine == "" {
			engine = "?"
		}
		fmt.Fprintf(w, "  [봇] %-20s %-7s %s\n", b.Session, engine, b.Method())
	}
}

// launchOffBots — 고른 봇을 하나씩 띄운다. 세션이 이미 있으면 건너뛰고, 실패는 봇마다 한 줄로 보고하고 계속한다.
// 사이드카형: tmux new-session -d -s <세션> <명령>. 유닛형: 브리지면 데몬 유닛 먼저(이미 돌면 생략), 그다음 TUI/봇 유닛.
func launchOffBots(w io.Writer, env RestoreEnv, bots []wiring.OffBot) {
	exists := env.SessionExists
	if exists == nil {
		exists = func(string) bool { return false }
	}
	running := env.UnitRunning
	if running == nil {
		running = func(string) bool { return false }
	}
	n := 0
	for _, b := range bots {
		if exists(b.Session) {
			fmt.Fprintf(w, "  봇 %s: 세션이 이미 있어 건너뜀\n", b.Session)
			continue
		}
		var err error
		switch {
		case b.Sidecar != "":
			if env.StartSession == nil {
				err = errors.New("tmux 기동 주입점 없음")
			} else {
				err = env.StartSession(b.Session, b.Sidecar)
			}
		case b.Unit != "":
			if env.StartUnit == nil {
				err = errors.New("유닛 기동 주입점 없음")
				break
			}
			if b.Daemon != "" && !running(b.Daemon) {
				if err = env.StartUnit(b.Daemon); err != nil {
					err = fmt.Errorf("데몬 유닛 %s: %w", b.Daemon, err)
					break
				}
			}
			if err = env.StartUnit(b.Unit); err != nil {
				err = fmt.Errorf("유닛 %s: %w", b.Unit, err)
			}
		default:
			err = errors.New("띄울 방법이 없음")
		}
		if err != nil {
			fmt.Fprintf(w, "  ✖ 봇 %s 기동 실패: %v\n", b.Session, err)
			continue
		}
		fmt.Fprintf(w, "  ✔ 봇 %s 기동 (%s)\n", b.Session, b.Method())
		n++
	}
	if n > 0 {
		fmt.Fprintf(w, "봇 %d개 기동 — 세션이 뜨면 status에 나타난다.\n", n)
	}
}

// runRestoreBotsOnly — `restore --bots <세션,세션>`: 꺼져 있는 봇 중 그 이름들만 띄운다. 체크리스트·죽은 세션 복원은 하지 않는다.
// 목록에 없는 이름은 오류(아무것도 띄우지 않는다). 선택은 기억한다.
func runRestoreBotsOnly(w io.Writer, stateDir string, env RestoreEnv, spec string, dryRun bool) error {
	var all []wiring.OffBot
	if env.OffBots != nil {
		all = env.OffBots()
	}
	byName := map[string]wiring.OffBot{}
	for _, b := range all {
		byName[b.Session] = b
	}
	var picked []wiring.OffBot
	var missing []string
	for _, name := range strings.Split(spec, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if b, ok := byName[name]; ok {
			picked = append(picked, b)
		} else {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		names := make([]string, 0, len(all))
		for _, b := range all {
			names = append(names, b.Session)
		}
		list := strings.Join(names, ", ")
		if list == "" {
			list = "(없음)"
		}
		return fmt.Errorf("꺼져 있는 봇 목록에 없는 이름: %s — 지금 꺼져 있는 봇: %s", strings.Join(missing, ", "), list)
	}
	if len(picked) == 0 {
		return errors.New("--bots에 세션 이름이 없습니다 (예: --bots collab-bot,codex-qa)")
	}
	if dryRun {
		printOffBots(w, picked, "(dry-run — 실행 안 함) 띄울 봇:")
		return nil
	}
	launchOffBots(w, env, picked)
	saveRestoreBots(stateDir, picked)
	return nil
}

// startUnit — 유닛형 봇 기동. macOS: launchctl kickstart gui/<uid>/<라벨>. 리눅스: systemctl --user start <유닛>.
func startUnit(label string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("launchctl", "kickstart", fmt.Sprintf("gui/%d/%s", os.Getuid(), label))
	} else {
		cmd = exec.Command("systemctl", "--user", "start", label)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

// unitRunning — 브리지 데몬 유닛이 이미 돌고 있는가. macOS: launchctl print의 "state = running". 리눅스: systemctl --user is-active.
// 조회 실패는 "모름" = false — kickstart/start는 이미 도는 유닛에 무해하다.
func unitRunning(label string) bool {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("launchctl", "print", fmt.Sprintf("gui/%d/%s", os.Getuid(), label)).Output()
		return err == nil && strings.Contains(string(out), "state = running")
	}
	return exec.Command("systemctl", "--user", "is-active", "--quiet", label).Run() == nil
}
