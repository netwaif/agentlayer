// internal/cli/boardcmd_test.go
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

func TestLoadBoardFindsRootAndOneCard(t *testing.T) {
	stateDir := t.TempDir()
	st, err := state.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateWorking,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}}); err != nil {
		t.Fatal(err)
	}
	root := companyRoot(t, "LAB-1")
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir,
		[]string{"assign", "LAB-1", "collab-bot", "--inbox", filepath.Join(root, "runtime", "inbox")}, time.Now()); err != nil {
		t.Fatal(err)
	}
	gotRoot, cards, err := LoadBoard(st, stateDir, &config.Config{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != root {
		t.Errorf("root = %q, want %q", gotRoot, root)
	}
	if len(cards) != 1 || cards[0].State != "WORK" {
		t.Errorf("cards = %+v", cards)
	}
}

func TestLoadBoardNoRegistrationsReturnsEmptyRoot(t *testing.T) {
	stateDir := t.TempDir()
	st, err := state.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	root, cards, err := LoadBoard(st, stateDir, &config.Config{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if root != "" || cards != nil {
		t.Errorf("root=%q cards=%v, want (\"\", nil)", root, cards)
	}
}

func TestRunBoardJSONAndOut(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateWorking,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}})
	root := companyRoot(t, "LAB-1")
	_ = task.Assign(stateDir, task.Assignment{TaskID: "LAB-1", AgentID: "claude-%1", Session: "collab-bot", Pane: "%1",
		Inbox: filepath.Join(root, "runtime", "inbox"), TaskDir: filepath.Join(root, "tasks", "LAB-1"), AssignedAt: time.Now()}, false)
	var out bytes.Buffer
	opened := ""
	open := func(u string) error { opened = u; return nil }
	if err := RunBoard(&out, st, stateDir, &config.Config{}, open, []string{"--json"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	var cards []map[string]any
	if err := json.Unmarshal(out.Bytes(), &cards); err != nil || len(cards) != 1 || cards[0]["ID"] != "LAB-1" || cards[0]["State"] != "WORK" {
		t.Errorf("json = %s err=%v", out.String(), err)
	}
	if opened != "" {
		t.Error("--json은 브라우저를 열지 않음")
	}
	outPath := filepath.Join(t.TempDir(), "b.html")
	out.Reset()
	if err := RunBoard(&out, st, stateDir, &config.Config{}, open, []string{"--out", outPath}, time.Now()); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(outPath)
	if err != nil || !strings.Contains(string(b), "LAB-1") {
		t.Errorf("--out 파일: %v", err)
	}
	if opened != "" {
		t.Error("--out은 브라우저를 열지 않음")
	}
	out.Reset()
	if err := RunBoard(&out, st, stateDir, &config.Config{}, open, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(opened, "file://") || !strings.HasSuffix(opened, "/board.html") {
		t.Errorf("기본은 <state>/board.html을 브라우저로: %q", opened)
	}
}

func TestRunBoardWithoutCompanyExplains(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	var out bytes.Buffer
	err := RunBoard(&out, st, stateDir, &config.Config{}, func(string) error { return nil }, nil, time.Now())
	if err == nil || !strings.Contains(err.Error(), "company_root") {
		t.Errorf("회사 루트를 못 찾으면 company_root 안내: %v", err)
	}
}
