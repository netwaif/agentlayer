package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("AGENTLAYER_CONFIG", filepath.Join(t.TempDir(), "없음.json"))
	c := Load()
	if !c.MacOSEnabled() {
		t.Error("macOS 알림 기본 켜짐")
	}
	if c.NotifyDiscord || c.DiscordWebhookURL != "" {
		t.Error("Discord 기본 꺼짐")
	}
}

func TestLoadFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"discord_webhook_url":"https://discord.com/api/webhooks/1/x","notify_macos":false,"notify_discord":true}`), 0o600)
	t.Setenv("AGENTLAYER_CONFIG", p)
	c := Load()
	if c.MacOSEnabled() {
		t.Error("notify_macos:false 반영")
	}
	if !c.NotifyDiscord || c.DiscordWebhookURL == "" {
		t.Error("파일 값 반영")
	}
}

func TestLoadCorruptFallsBack(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{깨짐`), 0o600)
	t.Setenv("AGENTLAYER_CONFIG", p)
	c := Load()
	if !c.MacOSEnabled() {
		t.Error("파손 시 기본값")
	}
}
