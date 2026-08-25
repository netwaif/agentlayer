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
	if ShortID("1533823223442182294") != "153382…" {
		t.Errorf("축약: %s", ShortID("1533823223442182294"))
	}
	if ShortID("abc") != "abc" {
		t.Error("짧은 건 그대로")
	}
}
