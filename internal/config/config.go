// Package config는 ~/.config/agentlayer/config.json을 읽는다.
// 파일이 없으면 안전한 기본값으로 동작한다.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	// Discord 웹훅 URL. 상태 카드(대시보드 채널)에 사용. 값은 로그에 노출하지 않는다.
	DiscordWebhookURL string `json:"discord_webhook_url,omitempty"`
	// 단문 알림 전용 웹훅(알림 채널). 비면 카드 웹훅으로 폴백 —
	// 분리하면 대시보드 채널이 카드 한 장짜리로 유지된다.
	NotifyWebhookURL string `json:"notify_webhook_url,omitempty"`
	// macOS 알림 (osascript). 기본 켜짐.
	NotifyMacOS *bool `json:"notify_macos,omitempty"`
	// Discord 단문 알림. 기본 꺼짐 (웹훅이 있어도 명시적으로 켜야 함).
	NotifyDiscord bool `json:"notify_discord,omitempty"`
	// multi-agent-starter 루트. 비면 자동 탐지(starter.DefaultRoot).
	StarterRoot string `json:"starter_root,omitempty"`
	// Discord 채널 ID → 사람이 읽을 라벨 (상세 카드 표시용, 선택)
	ChannelLabels map[string]string `json:"channel_labels,omitempty"`
	// TUI 미리보기 갱신 주기 (Go duration 문자열, 예 "500ms"·"2s"). 비면 1s.
	PreviewInterval string `json:"preview_interval,omitempty"`
	// 새 dev 서버가 감지되면 전용 브라우저에 자동으로 연다. 기본 켜짐.
	// 같은 서버는 한 번만, 이미 탭이 있으면 안 연다.
	PreviewAuto *bool `json:"preview_auto,omitempty"`
	// wt new로 띄우는 worker를 승인 없이 돌린다(gemini/agy: --dangerously-skip-permissions,
	// stock gemini: --yolo). claude·codex는 각자 설정(auto 모드·trusted)을 따르므로 손대지 않는다.
	// 기본 켜짐 — worker가 도구마다 승인을 물으면 사람이 창마다 붙어 있어야 한다.
	WorkerAutoApprove *bool `json:"worker_auto_approve,omitempty"`
	// 전용 브라우저의 CDP 디버깅 포트. 고정이라 MCP(chrome-devtools-mcp 등)가
	// 늘 같은 주소로 붙는다. 비면 9222.
	BrowserPort int `json:"browser_port,omitempty"`
}

const (
	defaultBrowserPort = 9222
	defaultPreviewTick = time.Second
	// capture-pane 서브프로세스 폭주 방지 하한
	minPreviewTick = 200 * time.Millisecond
)

// PreviewTick은 preview_interval을 반영한 미리보기 주기.
// 파싱 불가·0 이하는 기본값, 하한 미만은 하한으로 — 설정 실수로 TUI가 멈추거나 폭주하지 않게.
func (c *Config) PreviewTick() time.Duration {
	d, err := time.ParseDuration(c.PreviewInterval)
	if err != nil || d <= 0 {
		return defaultPreviewTick
	}
	if d < minPreviewTick {
		return minPreviewTick
	}
	return d
}

// PreviewAutoEnabled는 기본값(true)을 반영한 접근자.
func (c *Config) PreviewAutoEnabled() bool {
	if c.PreviewAuto == nil {
		return true
	}
	return *c.PreviewAuto
}

// WorkerAutoApproveEnabled는 기본값(true)을 반영한 접근자.
func (c *Config) WorkerAutoApproveEnabled() bool {
	if c.WorkerAutoApprove == nil {
		return true
	}
	return *c.WorkerAutoApprove
}

// BrowserPortOrDefault는 browser_port를 반영한 디버깅 포트. 범위 밖(0 이하·65535 초과)은 기본값.
func (c *Config) BrowserPortOrDefault() int {
	if c.BrowserPort <= 0 || c.BrowserPort > 65535 {
		return defaultBrowserPort
	}
	return c.BrowserPort
}

// MacOSEnabled는 기본값(true)을 반영한 접근자.
func (c *Config) MacOSEnabled() bool {
	if c.NotifyMacOS == nil {
		return true
	}
	return *c.NotifyMacOS
}

// NotifyURL은 단문 알림(상태 전이·한도 핑)이 갈 웹훅 — 알림 채널 우선,
// 미분리 시 카드 채널 폴백. 대시보드 채널을 카드 한 장으로 유지하는
// 규칙의 단일 지점이다.
func (c *Config) NotifyURL() string {
	if c.NotifyWebhookURL != "" {
		return c.NotifyWebhookURL
	}
	return c.DiscordWebhookURL
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
