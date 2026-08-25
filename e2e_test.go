package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/cli"
	"github.com/netwaif/agentlayer/internal/hookcmd"
	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
)

// 종단 시나리오: 임시 tmux 서버(기존 서버와 완전 격리)에서
// 발견 → hook 상태 전이 → status 출력 → pane 소실 → DEAD.
func TestEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux 없음")
	}
	sock := fmt.Sprintf("agentlayer-e2e-%d", os.Getpid())
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", sock}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v (%s)", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	t.Cleanup(func() { exec.Command("tmux", "-L", sock, "kill-server").Run() })

	tmux("new-session", "-d", "-s", "e2e", "-x", "120", "-y", "30")
	// Claude Code처럼 보이게: 제목에 ✳ 신호를 남긴다 (화면 스크래핑 아님 — 메타데이터)
	tmux("select-pane", "-t", "e2e:0.0", "-T", "✳ 통합 테스트 작업")
	paneID := tmux("display-message", "-p", "-t", "e2e:0.0", "#{pane_id}")

	st, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tm := tmuxx.Tmux{Args: []string{"-L", sock}}
	now := time.Now()

	// 1) 스캐너가 발견
	panes, err := tm.ListPanes()
	if err != nil {
		t.Fatal(err)
	}
	if err := scan.Sync(st, panes, now); err != nil {
		t.Fatal(err)
	}
	id := scan.IDForPane("claude", paneID)
	a, err := st.Load(id)
	if err != nil {
		t.Fatalf("발견 실패: %v", err)
	}
	if a.Tmux.Session != "e2e" {
		t.Errorf("좌표: %+v", a.Tmux)
	}

	// 2) hook 이벤트: 작업 시작 → 대기 → 완료
	env := func(k string) string {
		if k == "TMUX_PANE" {
			return paneID
		}
		return ""
	}
	steps := []struct {
		event string
		want  state.AgentState
	}{
		{"post-tool-use", state.StateWorking},
		{"notification", state.StateWaiting},
		{"stop", state.StateDoneUnread},
	}
	for i, s := range steps {
		payload := `{"session_id":"e2e-session","cwd":"/tmp/e2e"}`
		if err := hookcmd.RunClaude(st, s.event, strings.NewReader(payload), env, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
		a, _ := st.Load(id)
		if a.State != s.want {
			t.Fatalf("%s 후 상태 = %s, want %s", s.event, a.State, s.want)
		}
	}

	// 3) status 출력에 반영
	var buf bytes.Buffer
	if err := cli.Status(&buf, st, false, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "[DONE]") {
		t.Errorf("status에 DONE 표시:\n%s", buf.String())
	}

	// 4) 읽음 처리
	if err := st.MarkRead(id, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	a, _ = st.Load(id)
	if a.State != state.StateIdle {
		t.Fatalf("읽음 후 IDLE: %s", a.State)
	}

	// 5) pane 소실 → DEAD (임시 서버 안이므로 kill 안전)
	tmux("kill-session", "-t", "e2e")
	panes, _ = tm.ListPanes()
	if err := scan.Sync(st, panes, now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	a, _ = st.Load(id)
	if a.State != state.StateDead {
		t.Errorf("소실 후 DEAD: %s", a.State)
	}
}

// TUI가 실제 pty에서 뜨는지: 임시 tmux 서버의 pane에서 실행해 화면을 캡처한다.
func TestTUILaunchesInTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux 없음")
	}
	bin := t.TempDir() + "/agentlayer"
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v (%s)", err, out)
	}
	sock := fmt.Sprintf("agentlayer-tui-%d", os.Getpid())
	stateDir := t.TempDir()
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", sock}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v (%s)", args, err, out)
		}
		return string(out)
	}
	t.Cleanup(func() { exec.Command("tmux", "-L", sock, "kill-server").Run() })

	tmux("new-session", "-d", "-s", "tui", "-x", "120", "-y", "30",
		"env", "AGENTLAYER_STATE_DIR="+stateDir, bin)
	var screen string
	for i := 0; i < 20; i++ { // 최대 2초 대기
		time.Sleep(100 * time.Millisecond)
		screen = tmux("capture-pane", "-p", "-t", "tui:0.0")
		if strings.Contains(screen, "AgentLayer") {
			break
		}
	}
	if !strings.Contains(screen, "AgentLayer") {
		t.Errorf("TUI 헤더가 떠야 함:\n%s", screen)
	}
	if !strings.Contains(screen, "j/k") {
		t.Errorf("도움말 라인이 보여야 함:\n%s", screen)
	}
}
