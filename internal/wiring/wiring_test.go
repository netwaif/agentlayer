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

func TestShortID(t *testing.T) {
	if ShortID("1533823223442182294") != "153382…" {
		t.Errorf("축약: %s", ShortID("1533823223442182294"))
	}
	if ShortID("abc") != "abc" {
		t.Error("짧은 건 그대로")
	}
}
