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
	"runtime/debug"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/netwaif/agentlayer/internal/cli"
	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/discord"
	"github.com/netwaif/agentlayer/internal/hookcmd"
	"github.com/netwaif/agentlayer/internal/notify"
	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/starter"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
	"github.com/netwaif/agentlayer/internal/ui"
	"github.com/netwaif/agentlayer/internal/usage"
	"github.com/netwaif/agentlayer/internal/wiring"
	"github.com/netwaif/agentlayer/internal/wt"
)

// 빌드 시 주입되는 버전 정보. goreleaser 기본 ldflags가
// -X main.version / main.commit / main.date 로 채운다.
// 로컬 go build에서는 비어 있고 debug.ReadBuildInfo로 폴백한다.
var (
	version string
	commit  string
	date    string
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
	case "restore":
		return runRestore(args[1:])
	case "info":
		return runInfo(args[1:])
	case "wake-all", "close-all", "broadcast":
		return runAll(args[0], args[1:])
	case "version", "--version", "-v":
		fmt.Println(cli.FormatVersion(buildVersion()))
		return nil
	case "help", "--help", "-h":
		fmt.Print(cli.HelpText())
		return nil
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
		return fmt.Errorf("알 수 없는 명령: %s\n'agentlayer help'로 전체 명령을 볼 수 있다", args[0])
	}
}

// buildVersion은 ldflags 주입값을 우선 쓰고, 비어 있으면
// 모듈 빌드 정보(go install·로컬 go build의 vcs 스탬프)로 채운다.
func buildVersion() cli.VersionInfo {
	v := cli.VersionInfo{Version: version, Commit: commit, Date: date}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return v
	}
	// 커밋이 ldflags로 왔으면 build info의 dirty 표시를 덧붙이지 않는다.
	commitFromBuildInfo := v.Commit == ""
	dirty := false
	for _, set := range bi.Settings {
		switch set.Key {
		case "vcs.revision":
			if v.Commit == "" {
				v.Commit = set.Value
			}
		case "vcs.time":
			if v.Date == "" {
				v.Date = set.Value
			}
		case "vcs.modified":
			dirty = set.Value == "true"
		}
	}
	if v.Version == "" && cli.IsReleaseVersion(bi.Main.Version) {
		v.Version = bi.Main.Version // go install <tag> 경로
	}
	if dirty && commitFromBuildInfo && v.Commit != "" {
		v.Commit += "+dirty"
	}
	return v
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

// runCard: agentlayer card [--out] [--event]
// 사용량 + 에이전트 상태를 Discord 카드 하나로 업서트한다.
// LaunchAgent 등에서 주기 실행하는 용도. --out은 payload JSON만 출력.
// --event는 hook 전이가 발사하는 즉시 갱신 모드: 동시 발사를 single-flight로
// 합치고, usage는 캐시만 쓴다(콜드 coach 실행 금지 — 5분 LaunchAgent 담당).
func runCard(args []string) error {
	fs := flag.NewFlagSet("card", flag.ContinueOnError)
	outOnly := fs.Bool("out", false, "전송 없이 카드 JSON만 출력")
	fromEvent := fs.Bool("event", false, "hook 전이 트리거 모드 (코얼레싱·캐시 usage)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fromEvent {
		return discord.RunCoalesced(state.DefaultDir(), func() error {
			return publishCard(false, 24*time.Hour)
		})
	}
	return publishCard(*outOnly, 4*time.Minute)
}

// publishCard는 카드 한 장을 조립해 업서트한다. usageMaxAge는 coach 캐시
// 허용 나이 — 이보다 오래됐을 때만 coach를 실제 실행한다.
func publishCard(outOnly bool, usageMaxAge time.Duration) error {
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
	ctx := usage.AgentCtx(agents, usage.LoadSnapshots(usage.SnapshotsDir()),
		usage.CodexSessionsRoot(), usage.GeminiDir())
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
	// worktree 브랜치 표시 (TUI의 ⎇와 동일 소스)
	branches := map[string]string{}
	if metas, err := wt.ListMetas(state.DefaultDir()); err == nil {
		for _, m := range metas {
			branches[m.Path] = m.Branch
		}
	}
	build := func(pay *usage.Payload) []any {
		return discord.BuildCard(discord.CardData{
			Pay: pay, Agents: agents, Ctx: ctx, Wired: wired, Branches: branches,
			DefModels: usage.DefaultModels(home),
			Tasks:     starter.ActiveTasks(starter.DefaultRoot()),
			Home:      home,
		}, now)
	}

	if outOnly {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(build(usage.FetchCached(st.Dir, usageMaxAge, usage.CoachRunner, now)))
	}

	cfg := config.Load()
	if cfg.DiscordWebhookURL == "" {
		return fmt.Errorf("discord_webhook_url이 설정에 없습니다: %s", config.Path())
	}
	statePath := discord.CardStatePath(state.DefaultDir())
	cs := discord.LoadCardState(statePath)
	client := discord.NewClient(cfg.DiscordWebhookURL)
	// 1차: 낡은 usage라도 캐시로 즉시 게시 — 에이전트 상태가 콜드 coach를
	// 기다리지 않는다 (TUI stale-while-revalidate와 같은 원칙)
	stale := usage.ReadCached(st.Dir)
	published := false
	if stale != nil {
		mid, err := client.Upsert(build(stale), cs.MessageID)
		if err != nil {
			return err
		}
		cs.MessageID = mid
		published = true
	}
	// 2차: usage 갱신이 실제로 있었을 때만 재게시. --event(24h 허용)는
	// 캐시 그대로라 1차로 끝난다 — 오늘까지의 단일 게시와 동일
	pay := usage.FetchCached(st.Dir, usageMaxAge, usage.CoachRunner, now)
	if !published || usagePayloadChanged(stale, pay) {
		mid, err := client.Upsert(build(pay), cs.MessageID)
		if err != nil {
			return err
		}
		cs.MessageID = mid
	}
	pings, lv := discord.WorsenedPings(pay, cs.LastLevels)
	cs.LastLevels = lv
	// 한도 핑은 알림 채널로 — 대시보드 채널은 카드 한 장 전용
	pingClient := discord.NewClient(cfg.NotifyURL())
	for _, p := range pings {
		_ = pingClient.Ping(p)
	}
	return discord.SaveCardState(statePath, cs)
}

// usagePayloadChanged는 2차 게시가 필요한지 판정한다 — coach 갱신이
// 실제 새 데이터(TS 변화)를 가져왔을 때만 true.
func usagePayloadChanged(old, fresh *usage.Payload) bool {
	if fresh == nil {
		return false
	}
	if old == nil {
		return true
	}
	return old.TS != fresh.TS
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
	// /orchestration 스킬 — 바이너리 동봉본을 ~/.claude/skills에 설치
	skillsDir := filepath.Join(home, ".claude", "skills")
	fmt.Println("orchestration 스킬 설치:", skillsDir)
	if err := cli.InstallOrchestrationSkill(os.Stdout, skillsDir, *dryRun); err != nil {
		return err
	}
	fmt.Println()
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
	transitioned := false
	hookcmd.SetTransitionHook(func(a *state.Agent, prev, to state.AgentState) {
		if prev != to {
			transitioned = true
		}
		notify.Notify(cfg, sender, a, prev, to)
	})
	// 전이가 실제로 있었으면 카드 즉시 갱신을 백그라운드로 발사한다.
	// hook은 에이전트를 막으면 안 되므로 기다리지 않는다(detached).
	defer func() {
		if !transitioned || cfg.DiscordWebhookURL == "" {
			return
		}
		self, err := os.Executable()
		if err != nil {
			return
		}
		cmd := exec.Command(self, "card", "--event")
		cmd.Stdout, cmd.Stderr, cmd.Stdin = nil, nil, nil
		if cmd.Start() == nil {
			_ = cmd.Process.Release()
		}
	}()
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
	infoCtx := usage.AgentCtx([]*state.Agent{a}, usage.LoadSnapshots(usage.SnapshotsDir()),
		usage.CodexSessionsRoot(), usage.GeminiDir())
	d := cli.InfoData{
		Agent:  a,
		Wiring: wiring.Collect(wiring.DefaultPaths(), a.CWD, a.Tmux.Session, cfg.ChannelLabels),
		Ctx:    infoCtx[a.ID],
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
			if _, err := cli.ResumeCommand(a); err != nil {
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
	cmd, err := cli.ResumeCommand(a)
	if err != nil {
		return err
	}
	tm := tmuxx.Tmux{}
	name := "resume-" + id
	if err := tm.NewWindow(name, a.CWD, cmd); err != nil {
		return err
	}
	// 이중 행 방지 — 부활 성공한 원본 dead 레코드는 즉시 삭제 (restore·TUI와 동일)
	_ = st.Delete(id)
	fmt.Printf("새 window %q에서 대화를 이어갑니다 (%s)\n", name, cli.ShortenHome(a.CWD))
	return nil
}

// runRestore: agentlayer restore [--resume] [--dry-run]
// 재부팅으로 사라진 tmux 배치를 죽은 레코드로 재구성한다.
func runRestore(args []string) error {
	st, err := state.NewStore(state.DefaultDir())
	if err != nil {
		return err
	}
	// tmux 현실과 먼저 동기화 — 서버가 아예 없으면(재부팅 직후) 저장된
	// 레코드가 전부 DEAD로 떨어져 그대로 복원 대상이 된다.
	if panes, err := (tmuxx.Tmux{}).ListPanes(); err == nil {
		_ = scan.Sync(st, panes, time.Now())
	}
	return cli.RunRestore(os.Stdout, st, tmuxx.Tmux{}, args)
}
