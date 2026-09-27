package cli

import "testing"

func TestSubUsageCoversHelpCommands(t *testing.T) {
	for _, c := range []string{"task", "remote", "wt", "board", "browser", "channel"} {
		u, ok := SubUsage(c)
		if !ok || u == "" {
			t.Errorf("%s: 사용법 있어야 함", c)
		}
	}
	if _, ok := SubUsage("send"); ok {
		t.Error("send는 --help 가로채기 대상이 아님(본문이 -h일 수 있다)")
	}
}

func TestHasHelpFlag(t *testing.T) {
	if !HasHelpFlag([]string{"watch", "--help"}) || !HasHelpFlag([]string{"-h"}) {
		t.Error("--help/-h 감지")
	}
	if HasHelpFlag([]string{"watch", "/tmp/inbox", "--once"}) || HasHelpFlag([]string{"--helpme"}) {
		t.Error("정확 일치만")
	}
}
