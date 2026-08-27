package hookcmd

import (
	"strings"
	"testing"
)

// envOn: TMUX 소켓과 pane을 지정하는 env 주입자.
func envOn(tmuxVar, pane string) func(string) string {
	return func(k string) string {
		switch k {
		case "TMUX":
			return tmuxVar
		case "TMUX_PANE":
			return pane
		}
		return ""
	}
}

// 비기본 서버(-L/-S 소켓)의 hook은 무시해야 한다 — pane ID 공간이 서버마다
// 독립이라, 공유 저장소에 쓰면 본 서버의 같은 번호 레코드를 오염시킨다
// (tmux-resurrect가 테스트 서버에 실세션 사본을 부활시킨 실사고).
func TestHooksIgnoreNonDefaultServer(t *testing.T) {
	cases := []struct {
		name string
		tmux string
		want int // 저장소에 남는 레코드 수
	}{
		{"기본 서버", "/private/tmp/tmux-501/default,123,0", 1},
		{"별도 -L 서버", "/private/tmp/tmux-501/agentlayer-tui-18089,19607,0", 0},
		{"TMUX 없음(잔류 TMUX_PANE)", "", 0},
	}
	for _, c := range cases {
		t.Run("claude/"+c.name, func(t *testing.T) {
			st := newStore(t)
			err := RunClaude(st, "stop", strings.NewReader(payload), envOn(c.tmux, "%3"), t0)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := st.List()
			if len(got) != c.want {
				t.Errorf("레코드 %d개, want %d", len(got), c.want)
			}
		})
		t.Run("codex/"+c.name, func(t *testing.T) {
			st := newStore(t)
			args := []string{`{"type":"agent-turn-complete","cwd":"/tmp"}`}
			if err := RunCodex(st, args, envOn(c.tmux, "%3"), t0); err != nil {
				t.Fatal(err)
			}
			got, _ := st.List()
			if len(got) != c.want {
				t.Errorf("레코드 %d개, want %d", len(got), c.want)
			}
		})
		t.Run("gemini/"+c.name, func(t *testing.T) {
			st := newStore(t)
			in := strings.NewReader(`{"hook_event_name":"stop","cwd":"/tmp"}`)
			if err := RunGemini(st, "stop", in, envOn(c.tmux, "%3"), t0); err != nil {
				t.Fatal(err)
			}
			got, _ := st.List()
			if len(got) != c.want {
				t.Errorf("레코드 %d개, want %d", len(got), c.want)
			}
		})
	}
}
