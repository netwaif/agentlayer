package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

// NotifyURL: 알림 채널 우선, 미분리 시 카드 채널 폴백 — 한도 핑이
// 대시보드 채널에 쌓이던 문제의 단일 규칙 지점.
func TestNotifyURL(t *testing.T) {
	c := &Config{DiscordWebhookURL: "card", NotifyWebhookURL: "notify"}
	if c.NotifyURL() != "notify" {
		t.Errorf("알림 채널 우선: %q", c.NotifyURL())
	}
	c.NotifyWebhookURL = ""
	if c.NotifyURL() != "card" {
		t.Errorf("미분리 시 카드 폴백: %q", c.NotifyURL())
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

// PreviewTick: preview_interval 파싱 — 미설정 1s 기본, 잘못된 값은 기본으로,
// 지나치게 짧은 값은 하한(200ms)으로 클램프. 설정 문제로 TUI가 폭주하면 안 된다.
func TestPreviewTick(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", time.Second},              // 미설정 → 기본
		{"500ms", 500 * time.Millisecond},
		{"2s", 2 * time.Second},
		{"바나나", time.Second},          // 파싱 불가 → 기본
		{"-1s", time.Second},           // 음수 → 기본
		{"50ms", 200 * time.Millisecond}, // 하한 클램프
	}
	for _, c := range cases {
		got := (&Config{PreviewInterval: c.in}).PreviewTick()
		if got != c.want {
			t.Errorf("PreviewTick(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
