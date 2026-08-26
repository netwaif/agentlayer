package wiring

import (
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T) (Paths, string) {
	t.Helper()
	root := t.TempDir()
	folder := filepath.Join(root, "collab")
	// bots.json
	cfgDir := filepath.Join(root, "config")
	os.MkdirAll(cfgDir, 0o755)
	botsJSON := filepath.Join(cfgDir, "bots.json")
	os.WriteFile(botsJSON, []byte(`{
	  "collab": {"engine": "claude", "folder": "`+folder+`", "session": "collab-bot"},
	  "other":  {"engine": "codex",  "folder": "/x/other",  "session": "other-bot"}
	}`), 0o644)
	// access.json
	os.MkdirAll(filepath.Join(folder, ".discord-state"), 0o755)
	os.WriteFile(filepath.Join(folder, ".discord-state", "access.json"), []byte(`{
	  "dmPolicy": "allowlist",
	  "groups": {"1533823223442182294": {"requireMention": false, "allowFrom": ["1062698028051472516"]}}
	}`), 0o644)
	// LaunchAgents
	laDir := filepath.Join(root, "LaunchAgents")
	os.MkdirAll(laDir, 0o755)
	os.WriteFile(filepath.Join(laDir, "com.folder-bot.collab.plist"),
		[]byte(`<plist>... -s collab-bot ... cd `+folder+` ...</plist>`), 0o644)
	os.WriteFile(filepath.Join(laDir, "com.unrelated.plist"),
		[]byte(`<plist>다른 봇</plist>`), 0o644)
	return Paths{BotsJSON: botsJSON, LaunchAgentsDir: laDir}, folder
}

func TestCollectFullWiring(t *testing.T) {
	p, folder := fixture(t)
	labels := map[string]string{"1533823223442182294": "collab방"}
	info := Collect(p, folder, "collab-bot", labels)

	if info.BotName != "collab" || info.Engine != "claude" {
		t.Errorf("folder-bot 매칭: %+v", info)
	}
	if info.Discord == nil {
		t.Fatal("discord 정보 있어야 함")
	}
	if info.Discord.DMPolicy != "allowlist" || len(info.Discord.Channels) != 1 {
		t.Errorf("discord: %+v", info.Discord)
	}
	ch := info.Discord.Channels[0]
	if ch.Label != "collab방" || ch.AllowCount != 1 || ch.RequireMention {
		t.Errorf("채널: %+v", ch)
	}
	if len(info.LaunchAgents) != 1 || info.LaunchAgents[0] != "com.folder-bot.collab" {
		t.Errorf("launchagent: %v", info.LaunchAgents)
	}
}

func TestCollectMissingSourcesGraceful(t *testing.T) {
	info := Collect(Paths{BotsJSON: "/없음", LaunchAgentsDir: "/없음"}, "/없는폴더", "x", nil)
	if info.BotName != "" || info.Discord != nil || len(info.LaunchAgents) != 0 {
		t.Errorf("소스 없으면 빈 값: %+v", info)
	}
}

func TestCollectMatchBySessionName(t *testing.T) {
	p, _ := fixture(t)
	// 폴더가 달라도(worktree 등) 세션 이름으로 folder-bot 매칭
	info := Collect(p, "/다른/경로", "collab-bot", nil)
	if info.BotName != "collab" {
		t.Errorf("세션 이름 매칭: %+v", info)
	}
}

func TestCollectBridge(t *testing.T) {
	root := t.TempDir()
	bridge := filepath.Join(root, "codex-discord")
	os.MkdirAll(filepath.Join(bridge, "data"), 0o755)
	workdir := "/Users/x/codex-workspace"
	os.WriteFile(filepath.Join(bridge, ".env"),
		[]byte("TOKEN=x\nCODEX_WORKDIR="+workdir+"\n"), 0o644)
	os.WriteFile(filepath.Join(bridge, "data", "daemon.pid"), []byte("999999999"), 0o644)

	p := Paths{BotsJSON: "/없음", LaunchAgentsDir: "/없음", BridgeRoots: []string{bridge}}
	info := Collect(p, workdir, "codex-live", nil)
	if info.Bridge == nil {
		t.Fatal("브리지 감지돼야 함")
	}
	if info.Bridge.Alive {
		t.Error("없는 pid는 죽음으로")
	}
	if !info.DiscordConnected() {
		t.Error("브리지 연결도 Discord 연결로 침")
	}
	// 다른 폴더는 매칭 안 됨
	if Collect(p, "/다른/폴더", "", nil).Bridge != nil {
		t.Error("WORKDIR 불일치는 미연결")
	}
}

func TestCollectPathBoundaryNoAncestorMatch(t *testing.T) {
	// 상위 폴더 에이전트(~/W)가 하위 폴더 봇(~/W/discord)의 plist에
	// 오매칭되면 "Discord 연결됨" 오표시가 난다 — 실전에서 발견된 버그.
	laDir := filepath.Join(t.TempDir(), "LaunchAgents")
	if err := os.MkdirAll(laDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(laDir, "com.soonho.claude-discord.plist"),
		[]byte(`<plist>cd /Users/x/VSCodeWorkspace/discord && run</plist>`), 0o644)
	p := Paths{BotsJSON: "/없음", LaunchAgentsDir: laDir}

	info := Collect(p, "/Users/x/VSCodeWorkspace", "", nil)
	if len(info.LaunchAgents) != 0 {
		t.Errorf("조상 경로가 하위 폴더 plist에 오매칭: %v", info.LaunchAgents)
	}
	if info.DiscordConnected() {
		t.Error("연결 오표시")
	}
	// 정확히 그 폴더는 매칭
	info = Collect(p, "/Users/x/VSCodeWorkspace/discord", "", nil)
	if len(info.LaunchAgents) != 1 {
		t.Errorf("정확한 폴더는 매칭돼야 함: %v", info.LaunchAgents)
	}
}

func TestCollectShortSessionNameIgnored(t *testing.T) {
	// "ai" 같은 초단문 tmux 세션명은 plist 라벨(ai.openclaw.gateway 등)에
	// 단어 경계로도 오탐된다 — 4자 미만은 세션명 매칭에 쓰지 않는다.
	laDir := filepath.Join(t.TempDir(), "LaunchAgents")
	if err := os.MkdirAll(laDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(laDir, "ai.openclaw.gateway.plist"),
		[]byte(`<plist>Label ai.openclaw.gateway</plist>`), 0o644)
	info := Collect(Paths{BotsJSON: "/없음", LaunchAgentsDir: laDir}, "/어딘가", "ai", nil)
	if len(info.LaunchAgents) != 0 {
		t.Errorf("짧은 세션명 오탐: %v", info.LaunchAgents)
	}
}

func TestDiscordConnectedByLaunchAgent(t *testing.T) {
	i := Info{LaunchAgents: []string{"com.soonho.claude-discord"}}
	if !i.DiscordConnected() {
		t.Error("discord 이름의 LaunchAgent도 연결로 침")
	}
	if (Info{LaunchAgents: []string{"com.folder-bot.x"}}).DiscordConnected() {
		t.Error("무관한 plist는 아님")
	}
}

func TestShortID(t *testing.T) {
	if ShortID("1533823223442182294") != "1533…2294" {
		t.Errorf("축약: %s", ShortID("1533823223442182294"))
	}
	// 앞부분이 같은 두 ID가 다르게 보여야 함 (스노우플레이크)
	if ShortID("1529498215651741706") == ShortID("1529494852000022780") {
		t.Error("다른 채널이 같은 축약이면 안 됨")
	}
	if ShortID("abc") != "abc" {
		t.Error("짧은 건 그대로")
	}
}
