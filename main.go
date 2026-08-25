// agentlayer — iTerm2+tmux 멀티 에이전트 관제탑.
// 서브커맨드: (없음)=TUI, status, hook, init
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/netwaif/agentlayer/internal/cli"
	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/discord"
	"github.com/netwaif/agentlayer/internal/hookcmd"
	"github.com/netwaif/agentlayer/internal/notify"
	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
	"github.com/netwaif/agentlayer/internal/ui"
	"github.com/netwaif/agentlayer/internal/usage"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "agentlayer:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runTUI()
	}
	switch args[0] {
	case "hook":
		return runHook(args[1:])
	case "status":
		return runStatus(args[1:])
	case "init":
		return runInit(args[1:])
	case "card":
		return runCard(args[1:])
	default:
		return fmt.Errorf("알 수 없는 명령: %s", args[0])
	}
}

// runTUI는 기본 명령: 관제 TUI를 연다.
func runTUI() error {
	st, err := state.NewStore(state.DefaultDir())
	if err != nil {
		return err
	}
	p := tea.NewProgram(ui.New(st, tmuxx.Tmux{}), tea.WithAltScreen())
	_, err = p.Run()
	return err
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

// runCard: agentlayer card [--out]
// 사용량 + 에이전트 상태를 Discord 카드 하나로 업서트한다.
// LaunchAgent 등에서 주기 실행하는 용도. --out은 payload JSON만 출력.
func runCard(args []string) error {
	fs := flag.NewFlagSet("card", flag.ContinueOnError)
	outOnly := fs.Bool("out", false, "전송 없이 카드 JSON만 출력")
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
	agents, err := st.List()
	if err != nil {
		return err
	}
	pay := usage.FetchCached(st.Dir, 4*time.Minute, usage.CoachRunner, now)
	ctx := usage.LoadSnapshots(usage.SnapshotsDir())
	for _, a := range agents {
		if a.Kind == "codex" && a.CWD != "" {
			if _, ok := ctx[a.CWD]; !ok {
				if info := usage.CodexLatest(usage.CodexSessionsRoot(), a.CWD); info.Model != "" || info.UsedPct != nil {
					ctx[a.CWD] = info
				}
			}
		}
	}
	home, _ := os.UserHomeDir()
	comps := discord.BuildComponents(pay, agents, ctx, home, now)

	if *outOnly {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(comps)
	}

	cfg := config.Load()
	if cfg.DiscordWebhookURL == "" {
		return fmt.Errorf("discord_webhook_url이 설정에 없습니다: %s", config.Path())
	}
	statePath := discord.CardStatePath(state.DefaultDir())
	cs := discord.LoadCardState(statePath)
	client := discord.NewClient(cfg.DiscordWebhookURL)
	mid, err := client.Upsert(comps, cs.MessageID)
	if err != nil {
		return err
	}
	cs.MessageID = mid
	pings, lv := discord.WorsenedPings(pay, cs.LastLevels)
	cs.LastLevels = lv
	for _, p := range pings {
		_ = client.Ping(p)
	}
	return discord.SaveCardState(statePath, cs)
}

// runInit: agentlayer init [--dry-run]
// Claude hook 등록 + tmux 바인딩 안내. .tmux.conf는 건드리지 않는다.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "변경 없이 할 일만 출력")
	if err := fs.Parse(args); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	fmt.Println("Claude Code hook 등록:", settingsPath)
	if err := cli.InstallClaudeHooks(os.Stdout, settingsPath, *dryRun); err != nil {
		return err
	}
	fmt.Println()
	// prefix 'a' 충돌 검사: list-keys가 성공하면 이미 바인딩된 것
	conflict := exec.Command("tmux", "list-keys", "-T", "prefix", "a").Run() == nil
	cli.PrintTmuxBinding(os.Stdout, conflict)
	return nil
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
	// 상태가 실제로 바뀐 순간에만 알림 (heartbeat 무음은 notify가 보장)
	cfg := config.Load()
	sender := notify.DefaultSender()
	hookcmd.SetTransitionHook(func(a *state.Agent, prev, to state.AgentState) {
		notify.Notify(cfg, sender, a, prev, to)
	})
	switch agent {
	case "claude":
		if err := hookcmd.RunClaude(st, *event, os.Stdin, os.Getenv, time.Now()); err != nil {
			fmt.Fprintln(os.Stderr, "agentlayer hook:", err)
		}
	}
	return nil
}
