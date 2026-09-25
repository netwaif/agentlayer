package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

func tmuxEnv(pane string) func(string) string {
	return func(k string) string {
		switch k {
		case "TMUX_PANE":
			return pane
		case "TMUX":
			return "/private/tmp/tmux-501/default,123,0"
		}
		return ""
	}
}

func readPending(t *testing.T, inbox string) []task.Report {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(inbox, "pending", "*.json"))
	var out []task.Report
	for _, f := range files {
		b, _ := os.ReadFile(f)
		var r task.Report
		json.Unmarshal(b, &r)
		out = append(out, r)
	}
	return out
}

func TestTaskMessageOutsideTmux(t *testing.T) {
	st, stateDir := newStore(t)
	err := taskMessage(&bytes.Buffer{}, nil, st, stateDir, "", func(string) string { return "" }, []string{"안녕"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "tmux") {
		t.Errorf("tmux 밖은 거부: %v", err)
	}
}

func TestTaskMessageWithAssignedTaskLogs(t *testing.T) {
	st, stateDir := newStore(t)
	root := remoteCompanyRoot(t, "VIDEO-07")
	inbox := filepath.Join(root, "runtime", "inbox")
	st.Save(&state.Agent{ID: "codex-3", Kind: "codex", State: state.StateWorking, Tmux: state.TmuxRef{Session: "codex-live", PaneID: "%3"}})
	task.Assign(stateDir, task.Assignment{TaskID: "VIDEO-07", AgentID: "codex-3", Session: "codex-live", Pane: "%3", Inbox: inbox,
		TaskDir: board.TaskDir(root, "VIDEO-07"), AssignedAt: time.Now()}, false)
	var out bytes.Buffer
	if err := taskMessage(&out, strings.NewReader("정리본입니다\n둘째 줄"), st, stateDir, "", tmuxEnv("%3"), []string{"--task", "VIDEO-07", "-"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	reps := readPending(t, inbox)
	if len(reps) != 1 || reps[0].To != "MESSAGE" || reps[0].From != "codex-live" || reps[0].TaskID != "VIDEO-07" || reps[0].Task != "정리본입니다\n둘째 줄" {
		t.Fatalf("%+v", reps)
	}
	if !strings.Contains(board.ReadLastLog(root, "VIDEO-07"), "[MESSAGE]") {
		t.Error("[MESSAGE] 로그")
	}
}

func TestTaskMessageUnassignedTaskNoLog(t *testing.T) {
	st, stateDir := newStore(t)
	root := remoteCompanyRoot(t, "VIDEO-07")
	inbox := filepath.Join(root, "runtime", "inbox")
	board.RememberRoot(stateDir, root)
	st.Save(&state.Agent{ID: "gemini-5", Kind: "gemini", State: state.StateIdle, Tmux: state.TmuxRef{Session: "community-agy", PaneID: "%5"}})
	if err := taskMessage(&bytes.Buffer{}, nil, st, stateDir, "", tmuxEnv("%5"), []string{"--task", "VIDEO-07", "의견 있음"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	reps := readPending(t, inbox)
	if len(reps) != 1 || reps[0].TaskID != "VIDEO-07" || reps[0].From != "community-agy" {
		t.Fatalf("기억된 루트의 inbox로: %+v", reps)
	}
	if strings.Contains(board.ReadLastLog(root, "VIDEO-07"), "[MESSAGE]") {
		t.Error("이 세션에 등록되지 않은 업무면 로그 없이 이벤트만")
	}
}

func TestTaskMessageNoRootIsError(t *testing.T) {
	st, stateDir := newStore(t)
	st.Save(&state.Agent{ID: "codex-3", Kind: "codex", Tmux: state.TmuxRef{Session: "codex-live", PaneID: "%3"}})
	err := taskMessage(&bytes.Buffer{}, nil, st, stateDir, "", tmuxEnv("%3"), []string{"안녕"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "회사 루트") {
		t.Errorf("루트를 못 찾으면 에러: %v", err)
	}
}
