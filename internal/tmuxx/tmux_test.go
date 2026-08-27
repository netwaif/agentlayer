package tmuxx

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestParsePanes(t *testing.T) {
	// 실측 환경(ai 세션 등)에서 가져온 형태의 샘플. 필드는 탭 구분.
	out := "ai\t1\t%3\t2.1.241\t/Users/soonho/ai-folder/dev/agentlayer\t✳ 핸드오프 문서 확인\t70882\n" +
		"codex-live\t0\t%1\tcodex\t/Users/soonho/ai-folder/codex-discord-workspace\tcodex-workspace | weekly 85% left\t555\n" +
		"ai\t0\t%0\tpython3.11\t/Users/soonho\t\t123\n"
	panes, err := parsePanes(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 3 {
		t.Fatalf("pane 수 = %d, want 3", len(panes))
	}
	p := panes[0]
	if p.Session != "ai" || p.Window != 1 || p.PaneID != "%3" ||
		p.Command != "2.1.241" || p.Title != "✳ 핸드오프 문서 확인" || p.PanePID != 70882 {
		t.Errorf("파싱 불일치: %+v", p)
	}
	if panes[2].Title != "" {
		t.Errorf("빈 title 허용해야 함: %q", panes[2].Title)
	}
}

func TestParsePanesMalformedLineSkipped(t *testing.T) {
	out := "ai\t1\t%3\n" + // 필드 부족 → skip
		"ok\t0\t%1\tzsh\t/tmp\ttitle\t9\n"
	panes, err := parsePanes(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 1 || panes[0].Session != "ok" {
		t.Errorf("불량 줄은 건너뛰어야 함: %+v", panes)
	}
}

// 통합: 임시 소켓으로 실제 tmux 서버를 띄워 검증. 기존 서버와 완전 격리.
func TestListPanesIntegration(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux 없음")
	}
	sock := fmt.Sprintf("agentlayer-test-%d", os.Getpid())
	tm := Tmux{Args: []string{"-L", sock}}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("tmux", append([]string{"-L", sock}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("tmux %v: %v (%s)", args, err, out)
		}
	}
	run("new-session", "-d", "-s", "al-test", "-x", "80", "-y", "24")
	t.Cleanup(func() { exec.Command("tmux", "-L", sock, "kill-server").Run() })

	panes, err := tm.ListPanes()
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 1 || panes[0].Session != "al-test" {
		t.Errorf("임시 서버 pane이 보여야 함: %+v", panes)
	}
}

// 통합: restore가 쓰는 세션·window 생성 프리미티브.
func TestRestorePrimitivesIntegration(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux 없음")
	}
	sock := fmt.Sprintf("agentlayer-test-restore-%d", os.Getpid())
	tm := Tmux{Args: []string{"-L", sock}}
	t.Cleanup(func() { exec.Command("tmux", "-L", sock, "kill-server").Run() })

	if tm.HasSession("ai") {
		t.Fatal("서버도 없는데 세션이 있다고 함")
	}
	pane, err := tm.NewSession("ai", os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if pane == "" {
		t.Error("새 세션의 pane ID를 돌려줘야 함")
	}
	if !tm.HasSession("ai") {
		t.Error("만든 세션을 감지해야 함")
	}
	if tm.HasSession("a") {
		t.Error("접두 매칭 금지 — 이름 완전 일치만")
	}
	pane2, err := tm.NewWindowIn("ai", "w2", os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if pane2 == "" || pane2 == pane {
		t.Errorf("새 window의 고유 pane ID: %q vs %q", pane2, pane)
	}
	panes, err := tm.ListPanes()
	if err != nil || len(panes) != 2 {
		t.Errorf("pane 2개여야 함: %+v (%v)", panes, err)
	}
}

func TestPickLatestClient(t *testing.T) {
	out := "1787580000\t/dev/ttys003\n1787581234\t/dev/ttys007\n1787580500\t/dev/ttys001\n"
	if got := pickLatestClient(out); got != "/dev/ttys007" {
		t.Errorf("최근 활동 클라이언트: %q", got)
	}
	if got := pickLatestClient(""); got != "" {
		t.Errorf("빈 목록: %q", got)
	}
	if got := pickLatestClient("깨진 줄\n"); got != "" {
		t.Errorf("불량 줄 무시: %q", got)
	}
}
