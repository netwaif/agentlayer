package hookcmd

import (
	"testing"

	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
)

func TestRunCodexTurnComplete(t *testing.T) {
	st := newStore(t)
	payload := `{"type":"agent-turn-complete","turn-id":"x","cwd":"/Users/x/proj"}`
	if err := RunCodex(st, []string{payload}, env("%9"), t0); err != nil {
		t.Fatal(err)
	}
	a, err := st.Load(scan.IDForPane("codex", "%9"))
	if err != nil {
		t.Fatal(err)
	}
	if a.State != state.StateDoneUnread || a.Kind != "codex" || a.CWD != "/Users/x/proj" {
		t.Errorf("turn-complete → DONE_UNREAD: %+v", a)
	}
}

func TestRunCodexUnknownTypeIgnored(t *testing.T) {
	st := newStore(t)
	if err := RunCodex(st, []string{`{"type":"mystery"}`}, env("%9"), t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.List(); len(got) != 0 {
		t.Error("모르는 이벤트는 무시")
	}
}

func TestRunCodexOutsideTmux(t *testing.T) {
	st := newStore(t)
	if err := RunCodex(st, []string{`{"type":"agent-turn-complete"}`}, env(""), t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.List(); len(got) != 0 {
		t.Error("tmux 밖 no-op")
	}
}

func TestRunCodexEmptyArgs(t *testing.T) {
	st := newStore(t)
	if err := RunCodex(st, nil, env("%9"), t0); err != nil {
		t.Fatal(err)
	}
}
