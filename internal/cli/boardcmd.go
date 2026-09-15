// internal/cli/boardcmd.go
package cli

import (
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

// LoadBoard는 회사 루트를 찾아 카드를 읽는다. 루트를 못 찾으면 ("", nil, nil).
func LoadBoard(st *state.Store, stateDir string, cfg *config.Config, now time.Time) (root string, cards []board.Card, err error) {
	list, err := task.List(stateDir)
	if err != nil {
		return "", nil, err
	}
	var inboxes []string
	var links []board.Link
	for _, as := range list {
		inboxes = append(inboxes, as.Inbox)
		links = append(links, board.Link{TaskID: as.TaskID, Session: as.Session, Window: as.Window, AgentID: as.AgentID})
	}
	root = board.Root(cfg.CompanyRoot, inboxes)
	if root == "" {
		return "", nil, nil
	}
	agents, err := st.List()
	if err != nil {
		return "", nil, err
	}
	states := map[string]string{}
	for _, a := range agents {
		states[a.ID] = StateWord(a.State)
	}
	cards, err = board.Load(root, links, states, now)
	if err != nil {
		return "", nil, err
	}
	return root, cards, nil
}
