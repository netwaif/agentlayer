package wt

import (
	"fmt"
	"io"
	"os/exec"
	"time"
)

// RunTest는 worktree에서 테스트 명령을 실행하고 결과를 메타에 기록한다.
// 출력은 w로 그대로 흘린다 (사용자가 실패 내용을 봐야 한다).
func RunTest(w io.Writer, stateDir, task, cmdOverride string) (bool, error) {
	m, err := LoadMeta(stateDir, task)
	if err != nil {
		return false, err
	}
	cmd := m.TestCmd
	if cmdOverride != "" {
		cmd = cmdOverride
		m.TestCmd = cmdOverride // 다음부터 기본값
	}
	if cmd == "" {
		return false, fmt.Errorf("테스트 명령이 없습니다 — wt test %s --cmd '...'로 지정하세요", task)
	}
	c := exec.Command("sh", "-c", cmd)
	c.Dir = m.Path
	c.Stdout = w
	c.Stderr = w
	runErr := c.Run()
	pass := runErr == nil
	now := time.Now()
	m.TestPass = &pass
	m.TestAt = &now
	if err := SaveMeta(stateDir, m); err != nil {
		return pass, err
	}
	return pass, nil
}
