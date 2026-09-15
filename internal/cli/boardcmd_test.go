// internal/cli/boardcmd_test.go
package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/state"
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
