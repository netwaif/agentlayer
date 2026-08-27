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
	"github.com/netwaif/agentlayer/internal/wt"
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
		out, err := exec.Command("tmux", append([]string{"-f", "/dev/null", "-L", sock}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v (%s)", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	t.Cleanup(func() { exec.Command("tmux", "-f", "/dev/null", "-L", sock, "kill-server").Run() })

	tmux("new-session", "-d", "-s", "e2e", "-x", "120", "-y", "30")
	// Claude Code처럼 보이게: 제목에 ✳ 신호를 남긴다 (화면 스크래핑 아님 — 메타데이터)
	tmux("select-pane", "-t", "e2e:0.0", "-T", "✳ 통합 테스트 작업")
	paneID := tmux("display-message", "-p", "-t", "e2e:0.0", "#{pane_id}")

	st, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tm := tmuxx.Tmux{Args: []string{"-f", "/dev/null", "-L", sock}}
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
		switch k {
		case "TMUX_PANE":
			return paneID
		case "TMUX":
			return "/private/tmp/tmux-501/default,123,0" // hook 가드 통과용 기본 서버
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
		out, err := exec.Command("tmux", append([]string{"-f", "/dev/null", "-L", sock}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v (%s)", args, err, out)
		}
		return string(out)
	}
	t.Cleanup(func() { exec.Command("tmux", "-f", "/dev/null", "-L", sock, "kill-server").Run() })

	tmux("new-session", "-d", "-s", "tui", "-x", "120", "-y", "30",
		"env", "AGENTLAYER_STATE_DIR="+stateDir, bin)
	var screen string
	for i := 0; i < 100; i++ { // 최대 10초 대기 (전체 스위트 병렬 부하 여유)
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

// wt 종단: 임시 repo + 임시 tmux 서버에서 new → window 생성 → 작업 → merge → clean.
func TestWorktreeEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux 없음")
	}
	sock := fmt.Sprintf("agentlayer-wt-%d", os.Getpid())
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-f", "/dev/null", "-L", sock}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v (%s)", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	t.Cleanup(func() { exec.Command("tmux", "-f", "/dev/null", "-L", sock, "kill-server").Run() })
	tmux("new-session", "-d", "-s", "wt-e2e", "-x", "100", "-y", "30")
	t.Setenv("TMUX", "/fake,1,0") // InsideTmux 통과용 — 실제 호출은 -L 소켓으로 감

	// 임시 repo
	repo := t.TempDir()
	gitE := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	gitE("init", "-b", "main")
	os.WriteFile(repo+"/a.txt", []byte("hi\n"), 0o644)
	gitE("add", ".")
	gitE("commit", "-m", "init")

	stateDir := t.TempDir()
	st, err := state.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	tm := tmuxx.Tmux{Args: []string{"-f", "/dev/null", "-L", sock}}

	// new — 에이전트 명령 대신 window가 유지되도록 sh를 쓰는 편법 없이,
	// claude가 없을 수 있으므로 window 생성만 확인하고 종료돼도 무방
	var buf bytes.Buffer
	if err := cli.RunWT(&buf, stateDir, st, tm, []string{"new", "feat-x", "--repo", repo, "--test", "true"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "agent/feat-x") {
		t.Errorf("생성 요약:\n%s", buf.String())
	}
	wins := tmux("list-windows", "-a", "-F", "#{window_name}")
	if !strings.Contains(wins, "feat-x") {
		t.Errorf("tmux window 생성돼야 함: %q", wins)
	}

	// 작업 → 커밋 → list/test/merge/clean
	m, err := wt.LoadMeta(stateDir, "feat-x")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(m.Path+"/a.txt", []byte("changed\n"), 0o644)
	gitW := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", m.Path}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	gitW("add", ".")
	gitW("commit", "-m", "work")

	buf.Reset()
	if err := cli.RunWT(&buf, stateDir, st, tm, []string{"test", "feat-x"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "통과") {
		t.Errorf("테스트 통과:\n%s", buf.String())
	}
	buf.Reset()
	if err := cli.RunWT(&buf, stateDir, st, tm, []string{"list"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "feat-x") || !strings.Contains(buf.String(), "✔") {
		t.Errorf("list에 태스크+테스트 결과:\n%s", buf.String())
	}
	buf.Reset()
	if err := cli.RunWT(&buf, stateDir, st, tm, []string{"merge", "feat-x", "--yes"}); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := cli.RunWT(&buf, stateDir, st, tm, []string{"clean", "feat-x"}); err != nil {
		t.Fatalf("병합 후 정리: %v", err)
	}
}
