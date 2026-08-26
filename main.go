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
	"strings"
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
	"github.com/netwaif/agentlayer/internal/wiring"
	"github.com/netwaif/agentlayer/internal/wt"
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
	case "resume":
		return runResume(args[1:])
	case "info":
		return runInfo(args[1:])
	case "wake-all", "close-all", "broadcast":
		return runAll(args[0], args[1:])
	case "wt":
		st, err := state.NewStore(state.DefaultDir())
		if err != nil {
			return err
		}
		if panes, err := (tmuxx.Tmux{}).ListPanes(); err == nil {
			_ = scan.Sync(st, panes, time.Now())
		}
		return cli.RunWT(os.Stdout, state.DefaultDir(), st, tmuxx.Tmux{}, args[1:])
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
	// Discord 연결 표시: 채널 라벨이 있으면 ⌁라벨, 없으면 ⌁
	cfgForCard := config.Load()
	wired := map[string]string{}
	wp := wiring.DefaultPaths()
	for _, a := range agents {
		if a.CWD == "" || wired[a.CWD] != "" {
			continue
		}
		wi := wiring.Collect(wp, a.CWD, a.Tmux.Session, cfgForCard.ChannelLabels)
		if !wi.DiscordConnected() {
			continue
		}
		mark := "⌁"
		if wi.Discord != nil && len(wi.Discord.Channels) > 0 && wi.Discord.Channels[0].Label != "" {
			mark += wi.Discord.Channels[0].Label
		}
		wired[a.CWD] = mark
	}
	comps := discord.BuildComponents(pay, agents, ctx, wired, home, now)

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
	binPath, _ := os.Executable()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	fmt.Println("Claude Code hook 등록:", settingsPath)
	if err := cli.InstallClaudeHooks(os.Stdout, settingsPath, binPath, *dryRun); err != nil {
		return err
	}
	fmt.Println()
	codexConfig := filepath.Join(home, ".codex", "config.toml")
	if _, err := os.Stat(filepath.Dir(codexConfig)); err == nil {
		fmt.Println("Codex notify 등록:", codexConfig)
		if err := cli.InstallCodexNotify(os.Stdout, codexConfig, binPath, *dryRun); err != nil {
			return err
		}
		fmt.Println()
	}
	// agy(Antigravity CLI)가 설치된 경우에만 — 전역 훅 파일에 등록
	geminiHooks := filepath.Join(home, ".gemini", "config", "hooks.json")
	if _, err := os.Stat(filepath.Dir(geminiHooks)); err == nil {
		fmt.Println("Gemini(agy) hook 등록:", geminiHooks)
		if err := cli.InstallGeminiHooks(os.Stdout, geminiHooks, binPath, *dryRun); err != nil {
			return err
		}
		fmt.Println()
	}
	// stock Gemini CLI — ~/.gemini/settings.json의 hooks에 등록
	geminiSettings := filepath.Join(home, ".gemini", "settings.json")
	if _, err := os.Stat(filepath.Dir(geminiSettings)); err == nil {
		fmt.Println("Gemini CLI hook 등록:", geminiSettings)
		if err := cli.InstallGeminiCLIHooks(os.Stdout, geminiSettings, binPath, *dryRun); err != nil {
			return err
		}
		fmt.Println()
	}
	// prefix 'a' 충돌 검사: list-keys가 성공하면 이미 바인딩된 것
	conflict := exec.Command(tmuxx.Bin(), "list-keys", "-T", "prefix", "a").Run() == nil
	cli.PrintTmuxBinding(os.Stdout, conflict, binPath)
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
	case "codex":
		if err := hookcmd.RunCodex(st, fs.Args(), os.Getenv, time.Now()); err != nil {
			fmt.Fprintln(os.Stderr, "agentlayer hook:", err)
		}
	case "gemini":
		if err := hookcmd.RunGemini(st, *event, os.Stdin, os.Getenv, time.Now()); err != nil {
			fmt.Fprintln(os.Stderr, "agentlayer hook:", err)
		}
		// agy 훅 출력 규약: stdout으로 JSON 응답. 빈 객체 = 아무 개입 없음
		// (Stop에서 decision을 안 내면 종료 허용, PostToolUse는 {} 기대).
		fmt.Println("{}")
	}
	return nil
}

// runInfo: agentlayer info <세션이름|id> — 에이전트 배선 상세 카드
func runInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("사용법: agentlayer info <세션이름|id>")
	}
	st, err := state.NewStore(state.DefaultDir())
	if err != nil {
		return err
	}
	now := time.Now()
	if panes, err := (tmuxx.Tmux{}).ListPanes(); err == nil {
		_ = scan.Sync(st, panes, now)
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	a := cli.FindAgent(agents, args[0])
	if a == nil {
		return fmt.Errorf("에이전트 %q 없음 — agentlayer status로 확인하세요", args[0])
	}
	cfg := config.Load()
	d := cli.InfoData{
		Agent:  a,
		Wiring: wiring.Collect(wiring.DefaultPaths(), a.CWD, a.Tmux.Session, cfg.ChannelLabels),
		Ctx:    usage.LoadSnapshots(usage.SnapshotsDir())[a.CWD],
		Labels: cfg.ChannelLabels,
	}
	if metas, err := wt.ListMetas(state.DefaultDir()); err == nil {
		for _, m := range metas {
			if m.Path == a.CWD {
				d.Branch = m.Branch
			}
		}
	}
	cli.RenderInfo(os.Stdout, d, now)
	return nil
}

// runAll: wake-all("세션 이어서하자") / close-all("세션 마감하자"+감시) / broadcast <메시지>
func runAll(cmd string, args []string) error {
	defaultWatch := cmd == "close-all"
	o, rest, err := cli.ParseAllFlags(cmd, args, defaultWatch)
	if err != nil {
		return err
	}
	var message string
	switch cmd {
	case "wake-all":
		message = cli.WakeMessage
	case "close-all":
		message = cli.CloseMessage
	default:
		if len(rest) == 0 {
			return fmt.Errorf("broadcast에는 메시지가 필요합니다: agentlayer broadcast \"<메시지>\"")
		}
		message = strings.Join(rest, " ")
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
	return cli.RunAll(os.Stdout, st, tmuxx.Tmux{}, message, o, cmd != "broadcast", now)
}

// runResume: agentlayer resume [id]
// 마감 의식 없이 죽은 대화를 구조한다. 인자 없으면 후보 목록.
func runResume(args []string) error {
	st, err := state.NewStore(state.DefaultDir())
	if err != nil {
		return err
	}
	if panes, err := (tmuxx.Tmux{}).ListPanes(); err == nil {
		_ = scan.Sync(st, panes, time.Now())
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		var found bool
		fmt.Println("resume 가능한 세션 (죽었거나 에러난 것 우선):")
		for _, a := range agents {
			if _, err := resumeCommand(a); err != nil {
				continue
			}
			marker := " "
			if a.State == state.StateDead || a.State == state.StateError {
				marker = "!"
			}
			sid := a.SessionID
			if len(sid) > 8 {
				sid = sid[:8]
			}
			fmt.Printf("  %s %-14s %-8s %s  (%s)\n", marker, a.ID, a.State, cli.ShortenHome(a.CWD), sid)
			found = true
		}
		if !found {
			fmt.Println("  없음 — 재개 가능한 세션이 없습니다.")
		}
		fmt.Println("\n사용법: agentlayer resume <id>")
		return nil
	}
	id := args[0]
	a, err := st.Load(id)
	if err != nil {
		return err
	}
	cmd, err := resumeCommand(a)
	if err != nil {
		return err
	}
	tm := tmuxx.Tmux{}
	name := "resume-" + id
	if err := tm.NewWindow(name, a.CWD, cmd); err != nil {
		return err
	}
	fmt.Printf("새 window %q에서 대화를 이어갑니다 (%s)\n", name, cli.ShortenHome(a.CWD))
	return nil
}

// resumeCommand는 에이전트 종류별 대화 재개 명령을 만든다.
//   - claude: claude --resume <session_id>
//   - codex:  codex resume <session_id> (notify에 세션 ID가 없어 rollout에서 추출)
//   - gemini: agy --conversation <id> (agy 대화만 — stock Gemini CLI는 재개 CLI가 없다)
func resumeCommand(a *state.Agent) (string, error) {
	switch a.Kind {
	case "claude":
		if a.SessionID == "" {
			return "", fmt.Errorf("session_id가 기록되지 않은 claude 세션입니다")
		}
		return fmt.Sprintf("claude --resume %s", a.SessionID), nil
	case "codex":
		if a.CWD == "" {
			return "", fmt.Errorf("cwd가 없는 codex 세션입니다")
		}
		sid := usage.CodexSessionID(usage.CodexSessionsRoot(), a.CWD)
		if sid == "" {
			return "", fmt.Errorf("codex rollout에서 세션을 못 찾았습니다: %s", a.CWD)
		}
		return fmt.Sprintf("codex resume %s", sid), nil
	case "gemini":
		if a.SessionID == "" {
			return "", fmt.Errorf("대화 ID가 기록되지 않은 gemini 세션입니다")
		}
		// agy 대화인지 확인 — brain 폴더가 있으면 agy, 없으면 stock CLI(재개 불가)
		brain := filepath.Join(usage.GeminiDir(), "antigravity-cli", "brain", a.SessionID)
		if _, err := os.Stat(brain); err != nil {
			return "", fmt.Errorf("stock Gemini CLI 세션은 CLI 재개를 지원하지 않습니다 (agy 대화만 가능)")
		}
		return fmt.Sprintf("agy --conversation %s", a.SessionID), nil
	}
	return "", fmt.Errorf("%s 종류는 resume을 지원하지 않습니다", a.Kind)
}
