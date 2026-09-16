package tmuxx

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// 여러 줄 본문은 send-keys -l이 아니라 붙여넣기(load-buffer + paste-buffer -p)로 들어가야
// 한다 — send-keys -l의 원시 LF는 Claude Code 입력창에서 사라져 절 제목이 본문에 붙어
// 도착했다(2026-09-16 LAB-1 실측). pane에 브래킷 붙여넣기를 켠 cat -v를 띄우면 붙여넣기
// 경로만 ^[[200~ … ^[[201~ 로 감싸여 찍힌다. 한 줄은 예전대로 send-keys -l(감싸지 않음).
func newBracketedPane(t *testing.T, name string) Tmux {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux 없음")
	}
	sock := fmt.Sprintf("agentlayer-%s-%d", name, os.Getpid())
	t.Cleanup(func() { exec.Command("tmux", "-f", "/dev/null", "-L", sock, "kill-server").Run() })
	args := []string{"-f", "/dev/null", "-L", sock, "new-session", "-d", "-s", name, "-x", "120", "-y", "24",
		"sh", "-c", "printf '\\033[?2004h'; exec cat -v"}
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("tmux %v: %v\n%s", args, err, out)
	}
	return Tmux{Args: []string{"-f", "/dev/null", "-L", sock}}
}

func waitPane(t *testing.T, tm Tmux, pane string, ok func(string) bool) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		out, _ := tm.CapturePane(pane, 24)
		if ok(out) {
			return out
		}
		if time.Now().After(deadline) {
			t.Fatalf("기대한 출력이 pane에 안 보임:\n%s", out)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestSendTextMultilineUsesBracketedPaste(t *testing.T) {
	tm := newBracketedPane(t, "sendml")
	if err := tm.SendText("sendml", "# 제목 줄\n\n## 목표\n- 첫 항목"); err != nil {
		t.Fatalf("여러 줄 전송 실패: %v", err)
	}
	out := waitPane(t, tm, "sendml", func(s string) bool { return strings.Contains(s, "^[[201~") })
	for _, want := range []string{"^[[200~# 제목 줄\n", "## 목표\n", "- 첫 항목^[[201~"} {
		if !strings.Contains(out, want) {
			t.Errorf("붙여넣기 출력에 %q 없음:\n%s", want, out)
		}
	}
}

func TestSendTextSingleLineStaysSendKeys(t *testing.T) {
	tm := newBracketedPane(t, "sendsl")
	if err := tm.SendText("sendsl", "- 한 줄 지시"); err != nil {
		t.Fatalf("한 줄 전송 실패: %v", err)
	}
	out := waitPane(t, tm, "sendsl", func(s string) bool { return strings.Contains(s, "- 한 줄 지시") })
	if strings.Contains(out, "^[[200~") {
		t.Errorf("한 줄은 send-keys -l이어야 하는데 붙여넣기로 감쌈:\n%s", out)
	}
}
