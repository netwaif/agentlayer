// Package config는 ~/.config/agentlayer/config.json을 읽는다.
// 파일이 없으면 안전한 기본값으로 동작한다.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	// Discord 웹훅 URL. 알림·상태 카드에 사용. 값은 로그에 노출하지 않는다.
	DiscordWebhookURL string `json:"discord_webhook_url,omitempty"`
	// macOS 알림 (osascript). 기본 켜짐.
	NotifyMacOS *bool `json:"notify_macos,omitempty"`
	// Discord 단문 알림. 기본 꺼짐 (웹훅이 있어도 명시적으로 켜야 함).
	NotifyDiscord bool `json:"notify_discord,omitempty"`
	// multi-agent-starter 루트. 비면 자동 탐지(starter.DefaultRoot).
	StarterRoot string `json:"starter_root,omitempty"`
}

// MacOSEnabled는 기본값(true)을 반영한 접근자.
func (c *Config) MacOSEnabled() bool {
	if c.NotifyMacOS == nil {
		return true
	}
	return *c.NotifyMacOS
}

// Path는 설정 파일 경로. AGENTLAYER_CONFIG로 오버라이드 가능.
func Path() string {
	if p := os.Getenv("AGENTLAYER_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "agentlayer", "config.json")
}

// Load는 설정을 읽는다. 파일 부재·파손 시 기본값을 돌려준다 —
// 설정 문제로 관제·hook이 멈추면 안 된다.
func Load() *Config {
	c := &Config{}
	p := Path()
	if p == "" {
		return c
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return c
	}
	_ = json.Unmarshal(b, c)
	return c
}
