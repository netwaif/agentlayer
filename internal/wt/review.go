package wt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
)

// 코멘트 마커. diff의 어느 줄 아래든 이 접두사로 코멘트를 적으면
// wt send가 모아서 에이전트에게 수정 지시로 보낸다.
const commentMarker = "#> "

const reviewHeader = `# agentlayer 리뷰 파일 — 태스크 %s (%s 대비)
#
# 고치고 싶은 줄 아래에 "#> 코멘트"를 적고 저장하세요.
# 그다음 터미널에서:  agentlayer wt send %s
# "#"로 시작하는 다른 줄은 무시됩니다.

`

// ReviewPath는 태스크의 리뷰 파일 위치 (상태 디렉터리 안 — repo를 더럽히지 않는다).
func ReviewPath(stateDir, task string) string {
	return filepath.Join(stateDir, "worktrees", task+".review.diff")
}

// WriteReviewFile은 base 대비 diff에 안내 헤더를 붙여 리뷰 파일을 만든다.
func WriteReviewFile(stateDir, task string) (string, error) {
	m, err := LoadMeta(stateDir, task)
	if err != nil {
		return "", err
	}
	diff, err := Diff(m.Path, m.Base)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(diff) == "" {
		return "", fmt.Errorf("변경이 없습니다 — 리뷰할 diff가 없어요")
	}
	path := ReviewPath(stateDir, task)
	content := fmt.Sprintf(reviewHeader, task, m.Base, task) + diff + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Comment는 코멘트 하나: 사용자가 적은 텍스트와 직전 diff 문맥.
type Comment struct {
	Context string // 코멘트 바로 위의 diff 줄 (빈 줄·주석 제외)
	Text    string
}

// ExtractComments는 리뷰 파일에서 "#> " 코멘트를 문맥과 함께 모은다.
func ExtractComments(path string) ([]Comment, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("리뷰 파일이 없습니다 — 먼저 wt review를 실행하세요")
	}
	var out []Comment
	var lastCode string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, commentMarker) {
			text := strings.TrimSpace(strings.TrimPrefix(line, commentMarker))
			if text != "" {
				out = append(out, Comment{Context: lastCode, Text: text})
			}
			continue
		}
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		lastCode = line
	}
	return out, nil
}

// BuildInstruction은 코멘트 묶음을 에이전트에게 보낼 지시 문단으로 조립한다.
func BuildInstruction(task string, comments []Comment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "diff 리뷰 코멘트 %d건입니다. 각 항목을 반영해 주세요.", len(comments))
	for i, c := range comments {
		fmt.Fprintf(&b, " (%d)", i+1)
		if c.Context != "" {
			fmt.Fprintf(&b, " [%s 근처]", strings.TrimSpace(c.Context))
		}
		fmt.Fprintf(&b, " %s.", strings.TrimRight(c.Text, "."))
	}
	return b.String()
}

// SendComments는 태스크 에이전트의 pane에 지시를 입력한다.
// pane은 상태 저장소에서 worktree 경로(cwd)로 찾는다.
func SendComments(stateDir, task string, st *state.Store, tm tmuxx.Tmux) (int, error) {
	m, err := LoadMeta(stateDir, task)
	if err != nil {
		return 0, err
	}
	comments, err := ExtractComments(ReviewPath(stateDir, task))
	if err != nil {
		return 0, err
	}
	if len(comments) == 0 {
		return 0, fmt.Errorf("코멘트가 없습니다 — 리뷰 파일에 %q 줄을 추가하세요", strings.TrimSpace(commentMarker))
	}
	agents, err := st.List()
	if err != nil {
		return 0, err
	}
	var target *state.Agent
	for _, a := range agents {
		if a.CWD == m.Path && a.State != state.StateDead {
			target = a
			break
		}
	}
	if target == nil {
		return 0, fmt.Errorf("worktree %s에서 실행 중인 에이전트를 못 찾았습니다", m.Path)
	}
	if err := tm.SendText(target.Tmux.PaneID, BuildInstruction(task, comments)); err != nil {
		return 0, err
	}
	return len(comments), nil
}
