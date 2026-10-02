package wiring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const plistOn = `<plist><dict><key>Label</key><string>com.folder-bot.on</string>
<key>ProgramArguments</key><array><string>/usr/local/bin/tmux</string><string>new-session</string><string>-d</string><string>-s</string><string>on-bot</string></array>
<key>RunAtLoad</key><true/></dict></plist>`

const plistOff = `<plist><dict><key>Label</key><string>com.folder-bot.off</string>
<key>ProgramArguments</key><array><string>/usr/local/bin/tmux</string><string>new-session</string><string>-d</string><string>-s</string><string>off-bot</string></array>
<key>RunAtLoad</key><false/></dict></plist>`

// 브리지 유닛은 실물 모양 그대로 쓴다(botctl write_codex_plists·codex-discord install.sh): TUI 유닛은 tui-up.sh와 env 파일
// 이름만 들고 세션 이름은 본문에 없다 — 세션은 <브리지 루트>/<env>의 TUI_PANE에 있다.
func bridgeTUIPlist(label, root, env string, autostart bool) string {
	arg := ""
	if env != "" {
		arg = "<string>" + env + "</string>"
	}
	run := "<false/>"
	if autostart {
		run = "<true/>"
	}
	return `<plist version="1.0"><dict><key>Label</key><string>` + label + `</string>
<key>ProgramArguments</key><array><string>/bin/bash</string><string>` + root + `/scripts/tui-up.sh</string>` + arg + `</array>
<key>RunAtLoad</key>` + run + `
<key>StandardOutPath</key><string>` + root + `/logs/tui-up.log</string></dict></plist>`
}

func bridgeDaemonPlist(label, root, env string) string {
	return `<plist version="1.0"><dict><key>Label</key><string>` + label + `</string>
<key>ProgramArguments</key><array><string>/usr/bin/node</string><string>--env-file=` + env + `</string><string>src/index.mjs</string></array>
<key>WorkingDirectory</key><string>` + root + `</string>
<key>RunAtLoad</key><false/><key>KeepAlive</key><false/></dict></plist>`
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func byOffSession(bots []OffBot) map[string]OffBot {
	m := map[string]OffBot{}
	for _, b := range bots {
		m[b.Session] = b
	}
	return m
}

func offFixture(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	la := filepath.Join(root, "LaunchAgents")
	bridge := filepath.Join(root, "codex-discord")
	writeFiles(t, bridge, map[string]string{".env.qa": "DISCORD_TOKEN=x\nTUI_PANE=codex-qa:0.0\n"})
	writeFiles(t, la, map[string]string{
		"com.folder-bot.on.plist": plistOn, "com.folder-bot.off.plist": plistOff,
		"com.codex-discord.qa-tui.plist": bridgeTUIPlist("com.codex-discord.qa-tui", bridge, ".env.qa", false),
		"com.codex-discord.qa.plist":     bridgeDaemonPlist("com.codex-discord.qa", bridge, ".env.qa"),
		"com.netwaif.card.plist":         `<plist>agentlayer card</plist>`,
	})
	side := filepath.Join(root, "folder-bot")
	writeFiles(t, side, map[string]string{
		"bots.json": `{"off":{"engine":"claude","folder":"/x/off","session":"off-bot"},
	  "side":{"engine":"agy","folder":"/x/side","session":"side-bot"}}`,
		"side-bot.tmux-cmd": "/bin/zsh -lc 'cd /x/side; exec bot-up.sh -n side'\n",
		"off-bot.tmux-cmd":  "cd /x/off; exec bot-up.sh -n off\n", // 유닛이 있으면 유닛형이 정본
		"empty.tmux-cmd":    "\n",
	})
	return Paths{BotsJSON: filepath.Join(side, "bots.json"), SidecarDir: side, LaunchAgentsDir: la}
}

// 꺼져 있는 봇 = 사이드카(유닛 없음) + 자동 기동 꺼진 유닛(세션 없음). 자동 기동 켜진 봇·떠 있는 세션은 제외.
func TestOffBotsDetectsSidecarUnitAndBridge(t *testing.T) {
	p := offFixture(t)
	got := OffBots(p, nil)
	if len(got) != 3 {
		t.Fatalf("3개 기대: %+v", got)
	}
	bridge, off, side := got[0], got[1], got[2]
	if bridge.Session != "codex-qa" || bridge.Unit != "com.codex-discord.qa-tui" || bridge.Daemon != "com.codex-discord.qa" || bridge.Engine != "codex" || bridge.Sidecar != "" {
		t.Errorf("브리지 쌍: %+v", bridge)
	}
	if off.Session != "off-bot" || off.Unit != "com.folder-bot.off" || off.Daemon != "" || off.Engine != "claude" || off.Sidecar != "" {
		t.Errorf("유닛형: %+v", off)
	}
	// bots.json의 agy는 체크리스트 표기(codex|gemini|claude)에 맞춰 gemini로
	if side.Session != "side-bot" || side.Sidecar != "/bin/zsh -lc 'cd /x/side; exec bot-up.sh -n side'" || side.Unit != "" || side.Engine != "gemini" {
		t.Errorf("사이드카형: %+v", side)
	}
	if bridge.Method() != "유닛(com.codex-discord.qa → com.codex-discord.qa-tui)" || off.Method() != "유닛(com.folder-bot.off)" || side.Method() != "사이드카" {
		t.Errorf("방법 표시: %q %q %q", bridge.Method(), off.Method(), side.Method())
	}
	// 떠 있는 세션은 뺀다
	got = OffBots(p, func(s string) bool { return s == "off-bot" || s == "side-bot" })
	if len(got) != 1 || got[0].Session != "codex-qa" {
		t.Errorf("떠 있는 세션 제외: %+v", got)
	}
}

// 실물 맥 구성(손으로 깐 codex-discord): 기본 브리지는 tui-up.sh에 인자가 없고(.env) 라벨이 .tui/.daemon,
// agy 브리지는 .env.gemini + ENGINE=agy. 세션은 env의 TUI_PANE, 데몬 짝은 env 파일 이름이 정확히 같은 유닛.
func TestOffBotsBridgeSessionFromEnvFile(t *testing.T) {
	root := t.TempDir()
	la := filepath.Join(root, "LaunchAgents")
	bridge := filepath.Join(root, "dev", "codex-discord")
	writeFiles(t, bridge, map[string]string{
		".env":            "DISCORD_TOKEN=x\nexport TUI_PANE=\"codex-live:0.0\"\n",
		".env.gemini":     "ENGINE=agy\nTUI_PANE=gemini-live:0.0\n",
		".env.textreview": "CODEX_WORKDIR=/x\n", // TUI_PANE 없음 → tui-up.sh 기본값 codex-live
	})
	writeFiles(t, la, map[string]string{
		// 파일 이름순으로 .aaa(다른 env를 쓰는 데몬)가 .daemon보다 먼저 읽힌다 — 부분 일치로 짝지으면 이쪽이 잡힌다
		"com.codex-discord.aaa.plist":        bridgeDaemonPlist("com.codex-discord.aaa", bridge, ".env.gemini2"),
		"com.codex-discord.daemon.plist":     bridgeDaemonPlist("com.codex-discord.daemon", bridge, ".env"),
		"com.codex-discord.tui.plist":        bridgeTUIPlist("com.codex-discord.tui", bridge, "", false),
		"com.codex-discord.gemini.plist":     bridgeDaemonPlist("com.codex-discord.gemini", bridge, ".env.gemini"),
		"com.codex-discord.gemini-tui.plist": bridgeTUIPlist("com.codex-discord.gemini-tui", bridge, ".env.gemini", false),
	})
	got := byOffSession(OffBots(Paths{LaunchAgentsDir: la, BotsJSON: "/없음"}, nil))
	if len(got) != 2 {
		t.Fatalf("브리지 봇 2개 기대: %+v", got)
	}
	if b := got["codex-live"]; b.Unit != "com.codex-discord.tui" || b.Daemon != "com.codex-discord.daemon" || b.Engine != "codex" {
		t.Errorf("기본 브리지(.env): %+v", b)
	}
	if b := got["gemini-live"]; b.Unit != "com.codex-discord.gemini-tui" || b.Daemon != "com.codex-discord.gemini" || b.Engine != "gemini" {
		t.Errorf("agy 브리지(.env.gemini): %+v", b)
	}
	// TUI_PANE이 없는 env는 tui-up.sh 기본값(codex-live)
	os.Remove(filepath.Join(la, "com.codex-discord.tui.plist"))
	writeFiles(t, la, map[string]string{"com.codex-discord.textreview-tui.plist": bridgeTUIPlist("com.codex-discord.textreview-tui", bridge, ".env.textreview", false)})
	got = byOffSession(OffBots(Paths{LaunchAgentsDir: la, BotsJSON: "/없음"}, nil))
	if b, ok := got["codex-live"]; !ok || b.Unit != "com.codex-discord.textreview-tui" {
		t.Errorf("TUI_PANE 기본값: %+v", got)
	}
	// 자동 기동이 켜진 TUI 유닛은 제외
	writeFiles(t, la, map[string]string{"com.codex-discord.gemini-tui.plist": bridgeTUIPlist("com.codex-discord.gemini-tui", bridge, ".env.gemini", true)})
	if _, ok := byOffSession(OffBots(Paths{LaunchAgentsDir: la, BotsJSON: "/없음"}, nil))["gemini-live"]; ok {
		t.Error("RunAtLoad true인 브리지 TUI는 꺼져 있는 봇이 아니다")
	}
}

// 스크립트 경로로 루트를 못 찾으면(상대 경로) WorkingDirectory, 그것도 없으면 Paths.BridgeRoots에서 env를 찾는다.
func TestOffBotsBridgeRootFallbacks(t *testing.T) {
	root := t.TempDir()
	la := filepath.Join(root, "LaunchAgents")
	bridge := filepath.Join(root, "codex-discord")
	writeFiles(t, bridge, map[string]string{".env.qa": "TUI_PANE=codex-qa:0.0\n"})
	writeFiles(t, la, map[string]string{"com.codex-discord.qa-tui.plist": `<plist><dict>
<key>ProgramArguments</key><array><string>scripts/tui-up.sh</string><string>.env.qa</string></array>
<key>WorkingDirectory</key><string>` + bridge + `</string><key>RunAtLoad</key><false/></dict></plist>`})
	if got := OffBots(Paths{LaunchAgentsDir: la}, nil); len(got) != 1 || got[0].Session != "codex-qa" {
		t.Errorf("WorkingDirectory로 env 찾기: %+v", got)
	}
	writeFiles(t, la, map[string]string{"com.codex-discord.qa-tui.plist": `<plist><dict>
<key>ProgramArguments</key><array><string>scripts/tui-up.sh</string><string>.env.qa</string></array>
<key>RunAtLoad</key><false/></dict></plist>`})
	if got := OffBots(Paths{LaunchAgentsDir: la, BridgeRoots: []string{"/없음", bridge}}, nil); len(got) != 1 || got[0].Session != "codex-qa" {
		t.Errorf("BridgeRoots로 env 찾기: %+v", got)
	}
	// env를 어디서도 못 읽고 본문에 kill-session도 없으면 세션을 모른다 — 짐작하지 않는다
	if got := OffBots(Paths{LaunchAgentsDir: la}, nil); len(got) != 0 {
		t.Errorf("세션을 모르는 유닛은 봇으로 잡지 않는다: %+v", got)
	}
}

// 데몬 짝은 브리지 TUI 유닛에만 붙는다. 명령에 .env가 나오는 폴더 봇에 브리지 데몬이 붙으면 그 봇을 띄울 때 무관한 데몬까지 깨운다.
func TestOffBotsDaemonPairOnlyForBridge(t *testing.T) {
	root := t.TempDir()
	la := filepath.Join(root, "LaunchAgents")
	bridge := filepath.Join(root, "codex-discord")
	writeFiles(t, la, map[string]string{
		"com.codex-discord.daemon.plist": bridgeDaemonPlist("com.codex-discord.daemon", bridge, ".env"),
		"com.folder-bot.x.plist": `<plist><dict><key>ProgramArguments</key><array><string>/usr/local/bin/tmux</string><string>new-session</string><string>-d</string><string>-s</string><string>x-bot</string><string>/bin/zsh -lc 'cd /x; source .env; exec bot-up.sh'</string></array>
<key>RunAtLoad</key><false/></dict></plist>`,
	})
	got := OffBots(Paths{LaunchAgentsDir: la}, nil)
	if len(got) != 1 || got[0].Session != "x-bot" || got[0].Daemon != "" {
		t.Errorf("폴더 봇에는 데몬 짝이 없다: %+v", got)
	}
}

// 같은 세션을 띄우는 유닛이 둘이고 하나라도 자동 기동이 켜져 있으면 restore가 건드리지 않는다 — 파일 이름 순서와 무관하게.
func TestOffBotsAutostartWinsRegardlessOfOrder(t *testing.T) {
	unit := func(run string) string {
		return `<plist><dict><key>ProgramArguments</key><array><string>tmux</string><string>new-session</string><string>-d</string><string>-s</string><string>same-bot</string></array><key>RunAtLoad</key>` + run + `</dict></plist>`
	}
	for _, names := range [][2]string{{"a.off.plist", "b.on.plist"}, {"b.off.plist", "a.on.plist"}} {
		la := t.TempDir()
		writeFiles(t, la, map[string]string{names[0]: unit("<false/>"), names[1]: unit("<true/>")})
		if got := OffBots(Paths{LaunchAgentsDir: la}, nil); len(got) != 0 {
			t.Errorf("%v: 자동 기동 유닛이 있는 세션은 제외: %+v", names, got)
		}
	}
}

func TestOffBotsNoneWhenAllAutostart(t *testing.T) {
	root := t.TempDir()
	la := filepath.Join(root, "LaunchAgents")
	os.MkdirAll(la, 0o755)
	os.WriteFile(filepath.Join(la, "com.folder-bot.on.plist"), []byte(plistOn), 0o644)
	if got := OffBots(Paths{LaunchAgentsDir: la, BotsJSON: "/없음", SidecarDir: filepath.Join(root, "none")}, nil); len(got) != 0 {
		t.Errorf("자동 기동 봇만 있으면 빈 목록: %+v", got)
	}
	if got := OffBots(Paths{}, nil); len(got) != 0 {
		t.Errorf("소스 없음: %+v", got)
	}
}

// 리눅스: enable 링크(default.target.wants)가 없는 유닛이 꺼진 것. 브리지 유닛은 botctl write_codex_units 모양 그대로 —
// 본문에 new-session이 없고 ExecStart는 tui-up.sh .env.<이름>, ExecStop은 `-<tmux> kill-session -t <세션>`.
func TestOffBotsSystemdDisabled(t *testing.T) {
	p, _ := systemdFixture(t) // com.folder-bot.collab(collab-bot) — 링크 없음 → disabled
	bridge := filepath.Join(t.TempDir(), "codex-discord")
	writeFiles(t, bridge, map[string]string{".env.qa": "ENGINE=agy\nTUI_PANE=agy-qa:0.0\n"})
	daemon := func(env string) string {
		return "[Unit]\nDescription=브리지 데몬\n[Service]\nType=simple\nWorkingDirectory=" + bridge + "\nExecStart=/usr/bin/node --env-file=" + env + " src/index.mjs\nRestart=always\n"
	}
	writeFiles(t, p.SystemdUserDir, map[string]string{
		"com.codex-discord.qa.service":  daemon(".env.qa"),
		"com.codex-discord.qa2.service": daemon(".env.qa2"),
		"com.codex-discord.qa-tui.service": "# folder-bot codex tui: name=qa session=agy-qa folder=/x\n[Unit]\nDescription=com.codex-discord.qa-tui (agy TUI tmux 세션 agy-qa)\n[Service]\nType=oneshot\nRemainAfterExit=yes\nWorkingDirectory=" + bridge +
			"\nExecStart=/bin/bash " + bridge + "/scripts/tui-up.sh .env.qa\nExecStop=-/usr/bin/tmux kill-session -t agy-qa\n",
	})
	got := OffBots(p, nil)
	if len(got) != 2 || got[0].Session != "agy-qa" || got[0].Daemon != "com.codex-discord.qa" || got[0].Engine != "gemini" || got[1].Session != "collab-bot" || got[1].Unit != "com.folder-bot.collab" {
		t.Fatalf("disabled 유닛: %+v", got)
	}
	// env 파일을 못 읽어도 브리지 TUI 유닛은 ExecStop의 kill-session -t에서 세션을 읽는다. 엔진은 라벨로 짐작.
	os.Remove(filepath.Join(bridge, ".env.qa"))
	if got := OffBots(p, nil); len(got) != 2 || got[0].Session != "agy-qa" || got[0].Engine != "codex" {
		t.Errorf("env 없음 → kill-session 대체: %+v", got)
	}
	// enable 링크를 만들면 자동 기동 봇 → 제외
	wants := filepath.Join(p.SystemdUserDir, "default.target.wants")
	os.MkdirAll(wants, 0o755)
	os.Symlink(filepath.Join(p.SystemdUserDir, "com.folder-bot.collab.service"), filepath.Join(wants, "com.folder-bot.collab.service"))
	got = OffBots(p, nil)
	if len(got) != 1 || got[0].Session != "agy-qa" {
		t.Errorf("enabled 유닛은 제외: %+v", got)
	}
}

func TestUnitSessionAndPlistBool(t *testing.T) {
	cases := map[string]string{
		plistOn: "on-bot",
		`exec "/usr/bin/tmux" new-session -d -s collab-bot "$(cat x)"`: "collab-bot",
		`<string>agentlayer card</string>`:                             "",
		// 따옴표·한글 세션 이름
		`tmux new-session -d -s "collab-bot" 'cmd'`: "collab-bot",
		`tmux new-session -d -s 'my bot' cmd`:       "my bot",
		`<string>tmux</string><string>new-session</string><string>-d</string><string>-s</string><string>봇</string><string>claude -t abc</string>`: "봇",
		`<string>/bin/sh</string><string>-c</string><string>tmux new-session -d -s &quot;q-bot&quot; x</string>`:                                  "q-bot",
		// 변수 세션 이름은 모른다 — 뒤따르는 다른 명령의 -t를 세션으로 잡지 않는다
		`tmux new-session -d -s "$SESSION" x; tmux send-keys -t other:0 y`: "",
		// new-session 없이 kill-session만 있는 일반 유닛은 대상이 아니다
		`ExecStop=/usr/bin/tmux kill-session -t sub-bot`: "",
	}
	for text, want := range cases {
		if got := unitSession(text); got != want {
			t.Errorf("unitSession(%.60q) = %q want %q", text, got, want)
		}
	}
	if plistAutostart(plistOff) || !plistAutostart(plistOn) || !plistAutostart(`<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>`) || plistAutostart(`<key>Label</key><string>x</string>`) {
		t.Error("plistAutostart")
	}
}

// 엔진 짐작: bots.json이 없으면 라벨로 — com.codex-discord.gemini-tui는 "codex"가 들어 있어도 gemini.
func TestEngineOfLabelGuess(t *testing.T) {
	for label, want := range map[string]string{
		"com.codex-discord.gemini-tui": "gemini", "com.codex-discord.agy-tui": "gemini",
		"com.codex-discord.tui": "codex", "com.folder-bot.slides": "claude", "com.netwaif.card": "",
	} {
		if got := engineOf(nil, "s", label); got != want {
			t.Errorf("engineOf(%s) = %q want %q", label, got, want)
		}
	}
	if got := engineOf(map[string]string{"s": "codex"}, "s", "com.codex-discord.gemini-tui"); got != "codex" {
		t.Errorf("bots.json이 정본: %q", got)
	}
	if strings.Contains(OffBot{Session: "s", Unit: "u"}.Method(), "→") {
		t.Error("데몬 없는 유닛형 표시에 화살표 없음")
	}
}
