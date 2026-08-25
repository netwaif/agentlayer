package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

var t0 = time.Date(2026, 8, 25, 12, 0, 0, 0, time.FixedZone("KST", 9*3600))

func fixtureStore(t *testing.T) *state.Store {
	t.Helper()
	st, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	agents := []*state.Agent{
		{ID: "claude-3", Kind: "claude", Task: "핸드오프 문서 확인", State: state.StateWorking,
			Tmux:      state.TmuxRef{Session: "ai", Window: 1, PaneID: "%3"},
			CWD:       "/Users/soonho/ai-folder/dev/agentlayer",
			UpdatedAt: t0.Add(-5 * time.Minute), StateSince: t0.Add(-5 * time.Minute)},
		{ID: "claude-7", Kind: "claude", Task: "승인 대기", State: state.StateWaiting,
			Tmux:      state.TmuxRef{Session: "collab-bot", Window: 0, PaneID: "%7"},
			CWD:       "/Users/soonho/ai-folder/collab",
			UpdatedAt: t0.Add(-time.Minute), StateSince: t0.Add(-time.Minute)},
		{ID: "codex-1", Kind: "codex", State: state.StateWorking,
			Tmux:      state.TmuxRef{Session: "codex-live", Window: 0, PaneID: "%1"},
			CWD:       "/Users/soonho/ai-folder/codex-discord-workspace",
			UpdatedAt: t0.Add(-2 * time.Hour), StateSince: t0.Add(-2 * time.Hour)}, // STALE
	}
	for _, a := range agents {
		if err := st.Save(a); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func TestStatusText(t *testing.T) {
	var buf bytes.Buffer
	if err := Status(&buf, fixtureStore(t), false, t0); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 { // 헤더 + 3행
		t.Fatalf("4줄이어야 함(헤더+3):\n%s", out)
	}
	if !strings.Contains(lines[0], "STATE") || !strings.Contains(lines[0], "AGENT") {
		t.Errorf("헤더 누락: %q", lines[0])
	}
	// 정렬: WAITING이 맨 위
	if !strings.Contains(lines[1], "[WAIT]") || !strings.Contains(lines[1], "collab-bot") {
		t.Errorf("첫 행은 WAITING: %q", lines[1])
	}
	// STALE 표기
	if !strings.Contains(out, "[WORK?]") {
		t.Errorf("오래된 WORKING은 [WORK?]: \n%s", out)
	}
	// 홈 축약
	if strings.Contains(out, "/Users/soonho/ai-folder/collab") {
		t.Errorf("경로는 ~로 축약: \n%s", out)
	}
	if !strings.Contains(out, "~/ai-folder/collab") {
		t.Errorf("~ 축약 경로가 보여야 함: \n%s", out)
	}
}

func TestStatusJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Status(&buf, fixtureStore(t), true, t0); err != nil {
		t.Fatal(err)
	}
	var agents []state.Agent
	if err := json.Unmarshal(buf.Bytes(), &agents); err != nil {
		t.Fatalf("JSON 파싱: %v\n%s", err, buf.String())
	}
	if len(agents) != 3 {
		t.Errorf("3개: %d", len(agents))
	}
	if agents[0].State != state.StateWaiting {
		t.Errorf("JSON도 정렬 유지: %s", agents[0].State)
	}
}

func TestStatusKoreanAlignment(t *testing.T) {
	// 한글 TASK(2칸 폭)가 섞여도 DIR 열의 시작 위치가 모든 행에서 같아야 한다
	st, _ := state.NewStore(t.TempDir())
	st.Save(&state.Agent{ID: "a", Kind: "claude", Task: "핸드오프 문서 확인", State: state.StateWorking,
		Tmux: state.TmuxRef{Session: "ai"}, CWD: "/x/one", UpdatedAt: t0, StateSince: t0})
	st.Save(&state.Agent{ID: "b", Kind: "codex", Task: "build", State: state.StateWorking,
		Tmux: state.TmuxRef{Session: "codex-live"}, CWD: "/x/two", UpdatedAt: t0, StateSince: t0})
	var buf bytes.Buffer
	if err := Status(&buf, st, false, t0); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	var cols []int
	for _, ln := range lines[1:] {
		idx := strings.Index(ln, "/x/")
		if idx < 0 {
			t.Fatalf("DIR 없음: %q", ln)
		}
		// 표시 폭 기준 위치
		cols = append(cols, displayWidth(ln[:idx]))
	}
	if cols[0] != cols[1] {
		t.Errorf("DIR 열 시작 표시폭이 달라짐: %v\n%s", cols, buf.String())
	}
}

func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r >= 0x1100 && (r <= 0x115F || (r >= 0xAC00 && r <= 0xD7A3) || (r >= 0x4E00 && r <= 0x9FFF)) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

func TestStatusEmpty(t *testing.T) {
	st, _ := state.NewStore(t.TempDir())
	var buf bytes.Buffer
	if err := Status(&buf, st, false, t0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "에이전트 없음") {
		t.Errorf("빈 상태 안내: %q", buf.String())
	}
}
