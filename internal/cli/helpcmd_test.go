package cli

import (
	"strings"
	"testing"
)

// help는 라우팅되는 모든 명령을 빠짐없이 보여줘야 한다.
// main.go run()의 switch에 명령을 추가하면 이 목록도 갱신할 것.
func TestHelpTextListsAllCommands(t *testing.T) {
	out := HelpText()
	for _, cmd := range []string{
		"status", "info", "card", "init", "resume", "restore",
		"wake-all", "close-all", "broadcast", "wt", "version", "help",
	} {
		if !strings.Contains(out, cmd) {
			t.Errorf("help에 %q 명령이 없다", cmd)
		}
	}
}

// 인자 없이 실행하면 TUI라는 안내가 있어야 한다.
func TestHelpTextMentionsTUI(t *testing.T) {
	if !strings.Contains(HelpText(), "TUI") {
		t.Error("help에 TUI 안내가 없다")
	}
}

// 자주 쓰는 플래그는 help만 보고 알 수 있어야 한다.
func TestHelpTextMentionsKeyFlags(t *testing.T) {
	out := HelpText()
	for _, flag := range []string{"--json", "--resume", "--dry-run", "--yes"} {
		if !strings.Contains(out, flag) {
			t.Errorf("help에 %q 플래그가 없다", flag)
		}
	}
}
