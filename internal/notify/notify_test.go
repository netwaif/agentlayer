package notify

import (
	"strings"
	"testing"

	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/state"
)

type capture struct {
	osa     []string
	posts   []string
	postURL []string
}

func sender(c *capture) Sender {
	return Sender{
		RunOSA: func(s string) error { c.osa = append(c.osa, s); return nil },
		PostJSON: func(u string, b []byte) error {
			c.postURL = append(c.postURL, u)
			c.posts = append(c.posts, string(b))
			return nil
		},
	}
}

func agent() *state.Agent {
	return &state.Agent{ID: "claude-7", Kind: "claude", Task: "빌드 승인 필요",
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%7"}}
}

func TestNotifyOnDoneAndWaiting(t *testing.T) {
	for _, to := range []state.AgentState{state.StateDoneUnread, state.StateWaiting, state.StateError} {
		c := &capture{}
		Notify(&config.Config{}, sender(c), agent(), state.StateWorking, to)
		if len(c.osa) != 1 {
			t.Errorf("%s: macOS 알림 1회, got %d", to, len(c.osa))
		}
		if !strings.Contains(c.osa[0], "collab-bot") {
			t.Errorf("세션명 포함: %q", c.osa[0])
		}
	}
}

func TestHeartbeatSilent(t *testing.T) {
	c := &capture{}
	Notify(&config.Config{}, sender(c), agent(), state.StateWorking, state.StateWorking)
	Notify(&config.Config{}, sender(c), agent(), state.StateIdle, state.StateWorking) // WORKING 진입도 무음
	if len(c.osa)+len(c.posts) != 0 {
		t.Error("heartbeat·WORKING 진입은 무음")
	}
}

func TestMacOSDisabled(t *testing.T) {
	c := &capture{}
	off := false
	Notify(&config.Config{NotifyMacOS: &off}, sender(c), agent(), state.StateWorking, state.StateDoneUnread)
	if len(c.osa) != 0 {
		t.Error("꺼짐 설정 반영")
	}
}

func TestDiscordOnlyWhenEnabled(t *testing.T) {
	c := &capture{}
	cfg := &config.Config{DiscordWebhookURL: "https://discord.com/api/webhooks/1/x"}
	Notify(cfg, sender(c), agent(), state.StateWorking, state.StateDoneUnread)
	if len(c.posts) != 0 {
		t.Error("notify_discord 꺼져 있으면 웹훅 미전송")
	}
	cfg.NotifyDiscord = true
	Notify(cfg, sender(c), agent(), state.StateWorking, state.StateDoneUnread)
	if len(c.posts) != 1 {
		t.Fatal("켜면 전송")
	}
	if !strings.Contains(c.posts[0], "완료") || !strings.Contains(c.posts[0], "빌드 승인 필요") {
		t.Errorf("내용: %s", c.posts[0])
	}
	if strings.Contains(c.posts[0], "webhooks/1/x") {
		t.Error("본문에 웹훅 URL 노출 금지")
	}
}
