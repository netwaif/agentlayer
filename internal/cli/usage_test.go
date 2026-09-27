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

func TestHelpInterceptSkipsLetterBodies(t *testing.T) {
	if _, ok := HelpIntercept([]string{"task", "watch", "--help"}); !ok {
		t.Error("task watch --help는 가로챈다")
	}
	if _, ok := HelpIntercept([]string{"channel", "serve", "-h"}); !ok {
		t.Error("channel serve -h는 가로챈다")
	}
	for _, args := range [][]string{{"task", "message", "-h"}, {"task", "reply", "t_1", "--help"}, {"send", "bot", "-h"}, {"task"}} {
		if _, ok := HelpIntercept(args); ok {
			t.Errorf("%v: 본문이 -h일 수 있는 명령·인자 없는 호출은 가로채지 않는다", args)
		}
	}
}
