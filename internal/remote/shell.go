package remote

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Runner는 원격(또는 로컬)에서 argv 하나를 실행하고 표준출력을 돌려준다. 원격 텍스트는 전부 인자로만 간다.
type Runner interface {
	Run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error)
}

// ShellQuote는 POSIX sh용 단일따옴표 인용. 원격 셸 문자열을 만드는 유일한 함수다.
func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

const safeChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_./:=@%+-"

// ShellJoin — 안전한 토큰([A-Za-z0-9_./:=@%+-])은 그대로, 나머지는 ShellQuote.
func ShellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if a != "" && strings.Trim(a, safeChars) == "" {
			parts[i] = a
		} else {
			parts[i] = ShellQuote(a)
		}
	}
	return strings.Join(parts, " ")
}

// SSHRunner는 `ssh <host> <Exec…> <args…>`. ControlMaster로 연결을 60초 재사용해 폴링을 싸게 한다.
type SSHRunner struct {
	Host       string
	Exec       []string
	ControlDir string
}

// Argv는 실행할 로컬 argv(로컬 셸 미경유). 마지막 원소가 원격 셸 명령 문자열.
func (s SSHRunner) Argv(args ...string) []string {
	argv := []string{"ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ControlMaster=auto",
		"-o", "ControlPath=" + s.ControlDir + "/ssh-%C", "-o", "ControlPersist=60s", s.Host}
	remote := append(append([]string{}, s.Exec...), args...)
	return append(argv, ShellJoin(remote))
}

func (s SSHRunner) Run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	if s.ControlDir != "" {
		_ = os.MkdirAll(s.ControlDir, 0o700)
	}
	argv := s.Argv(args...)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = stdin
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return out.Bytes(), fmt.Errorf("ssh %s: %w: %s", s.Host, err, msg)
	}
	return out.Bytes(), nil
}

// FakeRunner — 테스트용. 호출 argv·stdin을 기록하고 Reply로 응답한다.
type FakeRunner struct {
	Calls  [][]string
	Stdins []string
	Reply  func(args []string) ([]byte, error)
}

func (f *FakeRunner) Run(_ context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	in := ""
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		in = string(b)
	}
	f.Calls = append(f.Calls, append([]string{}, args...))
	f.Stdins = append(f.Stdins, in)
	if f.Reply == nil {
		return nil, nil
	}
	return f.Reply(args)
}
