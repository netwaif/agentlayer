package remote

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestShellQuoteRoundTrip(t *testing.T) {
	// 실제 sh로 되돌려 본다 — 이스케이프 규칙의 정본은 셸이다.
	inputs := []string{"plain", "한글 본문", "it's", "a$b `c` \"d\"", "line1\nline2\n", "  spaces  ", ""}
	for _, in := range inputs {
		out, err := exec.Command("sh", "-c", "printf '%s' "+ShellQuote(in)).Output()
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if string(out) != in {
			t.Errorf("ShellQuote(%q) → %q", in, out)
		}
	}
	if got := ShellJoin([]string{"hermes", "kanban", "create", "제목 A"}); got != "hermes kanban create '제목 A'" {
		t.Errorf("ShellJoin=%q", got)
	}
}

func TestSSHRunnerArgv(t *testing.T) {
	s := SSHRunner{Host: "hostinger", Exec: []string{"docker", "exec", "-i", "-u", "hermes", "c1"}, ControlDir: "/tmp/ctl"}
	argv := s.Argv("hermes", "kanban", "show", "t_1", "--json")
	joined := strings.Join(argv, " ")
	for _, want := range []string{"ssh", "-o BatchMode=yes", "-o ControlMaster=auto", "-o ControlPath=/tmp/ctl/ssh-%C", "hostinger"} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv에 %q 없음: %v", want, argv)
		}
	}
	if argv[len(argv)-1] != "docker exec -i -u hermes c1 hermes kanban show t_1 --json" {
		t.Errorf("원격 명령 문자열=%q", argv[len(argv)-1])
	}
}

func TestFakeRunnerRecords(t *testing.T) {
	f := &FakeRunner{Reply: func(args []string) ([]byte, error) { return []byte(`{"ok":true}`), nil }}
	out, err := f.Run(context.Background(), strings.NewReader("in"), "hermes", "--version")
	if err != nil || string(out) != `{"ok":true}` || len(f.Calls) != 1 || f.Calls[0][1] != "--version" || f.Stdins[0] != "in" {
		t.Errorf("FakeRunner: out=%s err=%v calls=%v stdins=%v", out, err, f.Calls, f.Stdins)
	}
}
