// agentlayer — iTerm2+tmux 멀티 에이전트 관제탑.
// 서브커맨드: (없음)=TUI, status, hook, init
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/netwaif/agentlayer/internal/cli"
	"github.com/netwaif/agentlayer/internal/hookcmd"
	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "agentlayer:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("TUI는 아직 구현 전")
	}
	switch args[0] {
	case "hook":
		return runHook(args[1:])
	case "status":
		return runStatus(args[1:])
	default:
		return fmt.Errorf("알 수 없는 명령: %s", args[0])
	}
}

// runStatus: agentlayer status [--json]
// 출력 전에 tmux 현실과 동기화한다. tmux가 없으면 저장된 상태만 보여준다.
func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "JSON으로 출력")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := state.NewStore(state.DefaultDir())
	if err != nil {
		return err
	}
	now := time.Now()
	if panes, err := (tmuxx.Tmux{}).ListPanes(); err == nil {
		if err := scan.Sync(st, panes, now); err != nil {
			return err
		}
	}
	return cli.Status(os.Stdout, st, *jsonOut, now)
}

// runHook: agentlayer hook <agent> --event <event>
// hook 경로의 실패는 에이전트를 방해하지 않도록 항상 exit 0으로 삼킨다.
func runHook(args []string) error {
	if len(args) < 1 {
		return nil
	}
	agent := args[0]
	fs := flag.NewFlagSet("hook", flag.ContinueOnError)
	event := fs.String("event", "", "hook 이벤트 이름")
	if err := fs.Parse(args[1:]); err != nil {
		return nil
	}
	st, err := state.NewStore(state.DefaultDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentlayer hook:", err)
		return nil
	}
	switch agent {
	case "claude":
		if err := hookcmd.RunClaude(st, *event, os.Stdin, os.Getenv, time.Now()); err != nil {
			fmt.Fprintln(os.Stderr, "agentlayer hook:", err)
		}
	}
	return nil
}
