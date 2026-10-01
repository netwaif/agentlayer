package wiring

import (
	"os"
	"path/filepath"
	"testing"
)

const plistOn = `<plist><dict><key>Label</key><string>com.folder-bot.on</string>
<key>ProgramArguments</key><array><string>/usr/local/bin/tmux</string><string>new-session</string><string>-d</string><string>-s</string><string>on-bot</string></array>
<key>RunAtLoad</key><true/></dict></plist>`

const plistOff = `<plist><dict><key>Label</key><string>com.folder-bot.off</string>
<key>ProgramArguments</key><array><string>/usr/local/bin/tmux</string><string>new-session</string><string>-d</string><string>-s</string><string>off-bot</string></array>
<key>RunAtLoad</key><false/></dict></plist>`

// 브리지 TUI 유닛: 사이드카 없이 ExecStop/주석에만 세션명, .env.qa로 데몬과 짝.
const plistBridgeTUI = `<plist><dict><key>Label</key><string>com.codex-discord.qa-tui</string>
<key>ProgramArguments</key><array><string>/bin/bash</string><string>/opt/codex-discord/tui-up.sh</string><string>--env</string><string>/opt/codex-discord/.env.qa</string></array>
<!-- tmux new-session -d -s codex-qa … / kill-session -t codex-qa -->
<key>RunAtLoad</key><false/></dict></plist>`

const plistBridgeDaemon = `<plist><dict><key>Label</key><string>com.codex-discord.qa</string>
<key>ProgramArguments</key><array><string>/opt/codex-discord/daemon.sh</string><string>/opt/codex-discord/.env.qa</string></array>
<key>RunAtLoad</key><false/></dict></plist>`

func offFixture(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	la := filepath.Join(root, "LaunchAgents")
	os.MkdirAll(la, 0o755)
	for name, body := range map[string]string{
		"com.folder-bot.on.plist": plistOn, "com.folder-bot.off.plist": plistOff,
		"com.codex-discord.qa-tui.plist": plistBridgeTUI, "com.codex-discord.qa.plist": plistBridgeDaemon,
		"com.netwaif.card.plist": `<plist>agentlayer card</plist>`,
	} {
		os.WriteFile(filepath.Join(la, name), []byte(body), 0o644)
	}
	side := filepath.Join(root, "folder-bot")
	os.MkdirAll(side, 0o755)
	os.WriteFile(filepath.Join(side, "bots.json"), []byte(`{"off":{"engine":"claude","folder":"/x/off","session":"off-bot"},
	  "side":{"engine":"gemini","folder":"/x/side","session":"side-bot"}}`), 0o644)
	os.WriteFile(filepath.Join(side, "side-bot.tmux-cmd"), []byte("/bin/zsh -lc 'cd /x/side; exec bot-up.sh -n side'\n"), 0o644)
	os.WriteFile(filepath.Join(side, "off-bot.tmux-cmd"), []byte("cd /x/off; exec bot-up.sh -n off\n"), 0o644) // 유닛이 있으면 유닛형이 정본
	os.WriteFile(filepath.Join(side, "empty.tmux-cmd"), []byte("\n"), 0o644)
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

// 리눅스: enable 링크(default.target.wants)가 없는 유닛이 꺼진 것. 브리지는 <이름>-tui ↔ <이름> 짝.
func TestOffBotsSystemdDisabled(t *testing.T) {
	p, _ := systemdFixture(t) // com.folder-bot.collab(collab-bot) — 링크 없음 → disabled
	os.WriteFile(filepath.Join(p.SystemdUserDir, "com.codex-discord.qa.service"), []byte("[Service]\nExecStart=/opt/codex-discord/daemon.sh /opt/codex-discord/.env.qa\n"), 0o644)
	os.WriteFile(filepath.Join(p.SystemdUserDir, "com.codex-discord.qa-tui.service"), []byte("[Unit]\nDescription=codex TUI (tmux new-session -d -s codex-qa)\n[Service]\nExecStart=/bin/bash /opt/codex-discord/tui-up.sh --env /opt/codex-discord/.env.qa\nExecStop=/usr/bin/tmux kill-session -t codex-qa\n"), 0o644)
	got := OffBots(p, nil)
	if len(got) != 2 || got[0].Session != "codex-qa" || got[0].Daemon != "com.codex-discord.qa" || got[1].Session != "collab-bot" || got[1].Unit != "com.folder-bot.collab" {
		t.Fatalf("disabled 유닛: %+v", got)
	}
	// enable 링크를 만들면 자동 기동 봇 → 제외
	wants := filepath.Join(p.SystemdUserDir, "default.target.wants")
	os.MkdirAll(wants, 0o755)
	os.Symlink(filepath.Join(p.SystemdUserDir, "com.folder-bot.collab.service"), filepath.Join(wants, "com.folder-bot.collab.service"))
	got = OffBots(p, nil)
	if len(got) != 1 || got[0].Session != "codex-qa" {
		t.Errorf("enabled 유닛은 제외: %+v", got)
	}
}

func TestUnitSessionAndPlistBool(t *testing.T) {
	cases := map[string]string{
		plistOn:        "on-bot",
		plistBridgeTUI: "codex-qa",
		`exec "/usr/bin/tmux" new-session -d -s collab-bot "$(cat x)"`: "collab-bot",
		`<string>agentlayer card</string>`:                             "",
	}
	for text, want := range cases {
		if got := unitSession(text); got != want {
			t.Errorf("unitSession(%.40q) = %q want %q", text, got, want)
		}
	}
	if plistAutostart(plistOff) || !plistAutostart(plistOn) || !plistAutostart(`<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>`) || plistAutostart(`<key>Label</key><string>x</string>`) {
		t.Error("plistAutostart")
	}
}
