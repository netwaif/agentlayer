package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/netwaif/agentlayer/internal/state"
)

func TestRunSendCodexQueueExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	const supported = `#!/usr/bin/env node
if [ "$1 $2" = "queue --help" ]; then
	printf 'Options:\n      --thread <THREAD>\n      --message <MESSAGE>\n'
elif [ "$#" = 5 ] && [ "$1 $2 $3 $4" = "queue --thread sid-1 --message" ] && [ "$5" = "본문" ]; then
	printf 'Queued message\n'
else
	exit 1
fi
`
	const unsupported = `#!/bin/sh
printf '%s\n' "$*" >> "$QUEUE_TEST_LOG"
if [ "$1 $2" = "queue --help" ]; then
	printf 'Usage: codex [OPTIONS] [PROMPT]\n'
else
	printf "error: unexpected argument '--thread' found\n" >&2
	exit 2
fi
`
	for _, tc := range []struct {
		name, script, nodeDir string
		nvmCodex              bool
	}{
		{name: "node-next-to-codex", script: supported, nodeDir: ".local/bin"},
		{name: "node-in-nvm", script: supported, nodeDir: ".nvm/versions/node/v22.22.0/bin"},
		{name: "codex-in-nvm", script: supported, nodeDir: ".nvm/versions/node/v22.22.0/bin", nvmCodex: true},
		{name: "unsupported-thread", script: unsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("PATH", t.TempDir())
			t.Setenv("AGENTLAYER_CONFIG", filepath.Join(home, "config.json"))
			logPath := filepath.Join(home, "calls")
			t.Setenv("QUEUE_TEST_LOG", logPath)
			writeTool := func(dir, name, content string) {
				t.Helper()
				dir = filepath.Join(home, dir)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0755); err != nil {
					t.Fatal(err)
				}
			}
			codexDir := ".local/bin"
			if tc.nvmCodex {
				codexDir = tc.nodeDir
			}
			writeTool(codexDir, "codex", tc.script)
			if tc.nodeDir != "" {
				writeTool(tc.nodeDir, "node", "#!/bin/sh\nexec /bin/sh \"$@\"\n")
			}
			stateDir := t.TempDir()
			st, err := state.NewStore(stateDir)
			if err != nil {
				t.Fatal(err)
			}
			a := codexAgent("sample", "%7", state.StateIdle, "sid-1")
			a.CWD = home
			var out bytes.Buffer
			sender := &fakeSender{}
			send := func(args ...string) error {
				t.Helper()
				if err := st.Save(a); err != nil {
					t.Fatal(err)
				}
				out.Reset()
				return RunSend(t.Context(), &out, nil, st, stateDir, sender, args)
			}
			if tc.nodeDir != "" {
				if err := send("sample", "본문"); err != nil || sender.calls != 0 || !strings.Contains(out.String(), "(codex queue)") {
					t.Fatalf("큐 전송: err=%v tmux=%d out=%s", err, sender.calls, &out)
				}
				return
			}
			for _, s := range []state.AgentState{state.StateIdle, state.StateDoneUnread} {
				a.State = s
				if err := send("sample", "본문"); err != nil || !strings.Contains(out.String(), "--thread") || !strings.Contains(out.String(), "tmux 입력으로 보냄") {
					t.Fatalf("%s 폴백: err=%v out=%s", s, err, &out)
				}
			}
			a.State = state.StateWorking
			if err := send("sample", "본문"); err == nil || !strings.Contains(err.Error(), "--force") || sender.calls != 2 {
				t.Fatalf("작업 중 보호: err=%v tmux=%d", err, sender.calls)
			}
			if err := send("--force", "sample", "본문"); err != nil || sender.calls != 3 || sender.pane != "%7" || sender.text != "본문" || !strings.Contains(out.String(), "tmux에 즉시 입력") {
				t.Fatalf("강제 폴백: err=%v sender=%+v out=%s", err, sender, &out)
			}
			calls, err := os.ReadFile(logPath)
			if err != nil || string(calls) != strings.Repeat("queue --help\n", 4) {
				t.Fatalf("본문 없이 지원만 확인: calls=%q err=%v", calls, err)
			}
		})
	}
}
