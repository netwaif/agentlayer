// Package tmuxx는 tmux CLI를 감싼다. 조회(list-panes)와 이동(점프)만 하며,
// 세션·window·pane을 죽이는 명령은 어떤 경로로도 실행하지 않는다.
package tmuxx

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Pane은 tmux pane 하나의 스냅샷.
type Pane struct {
	Session string
	Window  int
	PaneID  string // "%3"
	Command string // pane_current_command
	Path    string // pane_current_path
	Title   string
	PanePID int
}

// 필드 순서는 parsePanes와 일치해야 한다. title은 탭을 품을 수 없다고
// 가정하지 않고, 필드 개수를 고정(7)해 앞 5개 + 마지막(pid)을 떼어낸다.
const panesFormat = "#{session_name}\t#{window_index}\t#{pane_id}\t#{pane_current_command}\t#{pane_current_path}\t#{pane_title}\t#{pane_pid}"

const numFields = 7

func parsePanes(out string) ([]Pane, error) {
	var panes []Pane
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < numFields {
			continue // 형식이 안 맞는 줄은 관제를 멈추게 하지 않는다
		}
		// title에 탭이 들어간 극단 케이스: 가운데(5번째~len-1)를 다시 합친다.
		title := strings.Join(parts[5:len(parts)-1], "\t")
		win, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		pid, _ := strconv.Atoi(parts[len(parts)-1])
		panes = append(panes, Pane{
			Session: parts[0], Window: win, PaneID: parts[2],
			Command: parts[3], Path: parts[4], Title: title, PanePID: pid,
		})
	}
	return panes, nil
}

// Tmux는 tmux 명령 실행기. Args는 모든 호출 앞에 붙는 인자로,
// 테스트에서 "-L <임시소켓>"을 넣어 실제 서버와 격리한다.
type Tmux struct {
	Args []string
}

func (t Tmux) run(args ...string) (string, error) {
	cmd := exec.Command("tmux", append(append([]string{}, t.Args...), args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// ListPanes는 서버 전체 pane을 반환한다.
func (t Tmux) ListPanes() ([]Pane, error) {
	out, err := t.run("list-panes", "-a", "-F", panesFormat)
	if err != nil {
		return nil, err
	}
	return parsePanes(out)
}

// Ref는 점프 대상 좌표. state.TmuxRef와 동형이지만 패키지 순환을 피해 따로 둔다.
type Ref struct {
	Session string
	Window  int
	PaneID  string
}

// JumpTo는 현재 클라이언트를 해당 pane으로 이동시킨다.
func (t Tmux) JumpTo(r Ref) error {
	if _, err := t.run("switch-client", "-t", r.Session); err != nil {
		return err
	}
	if _, err := t.run("select-window", "-t", fmt.Sprintf("%s:%d", r.Session, r.Window)); err != nil {
		return err
	}
	_, err := t.run("select-pane", "-t", r.PaneID)
	return err
}

// NewWindow는 현재 세션에 이름·시작 디렉터리·실행 명령을 지정해 window를
// 만든다. 기존 window는 건드리지 않는다.
func (t Tmux) NewWindow(name, dir, command string) error {
	args := []string{"new-window", "-n", name, "-c", dir}
	if command != "" {
		args = append(args, command)
	}
	_, err := t.run(args...)
	return err
}

// SendText는 pane에 텍스트를 입력하고 Enter를 보낸다.
// 에이전트 입력창에 지시를 넣는 용도 — 임의 키 시퀀스는 보내지 않는다.
func (t Tmux) SendText(paneID, text string) error {
	if _, err := t.run("send-keys", "-t", paneID, "-l", text); err != nil {
		return err
	}
	_, err := t.run("send-keys", "-t", paneID, "Enter")
	return err
}

// InsideTmux는 tmux 안에서 실행 중인지.
func InsideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// CurrentPaneID는 hook이 실행된 pane("%N"). tmux 밖이면 빈 문자열.
func CurrentPaneID() string {
	return os.Getenv("TMUX_PANE")
}
