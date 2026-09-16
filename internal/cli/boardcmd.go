// internal/cli/boardcmd.go
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

const boardUsage = "사용법: agentlayer board [--out <경로>] [--json] [--no-open] [--refresh]"

// BoardPath는 기본 보드 파일 경로(<state>/board.html).
func BoardPath(stateDir string) string { return filepath.Join(stateDir, "board.html") }

// RefreshBoardFile은 보드 파일이 이미 있을 때만 현재 상태로 다시 쓴다(경로, nil). 파일이 없으면
// ("", nil) — 한 번도 보드를 연 적 없는 사용자에게 파일을 만들어 주지 않는다. 훅(상태 전이)·
// task assign/done·send가 부르며, 열어 둔 보드 페이지는 스스로 다시 읽어 최신을 보여 준다.
// 회사 루트를 못 찾으면 그대로 둔다(빈 보드로 덮어쓰지 않음).
func RefreshBoardFile(st *state.Store, stateDir string, cfg *config.Config, now time.Time) (string, error) {
	path := BoardPath(stateDir)
	if _, err := os.Stat(path); err != nil {
		return "", nil
	}
	root, cards, err := LoadBoard(st, stateDir, cfg, now)
	if err != nil || root == "" {
		return "", err
	}
	page := board.HTML(board.CompanyName(root), cards, now, cfg.BoardStaleLimit())
	return path, os.WriteFile(path, page, 0o644)
}

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
		links = append(links, board.Link{TaskID: as.TaskID, Session: as.Session, Window: as.Window, AgentID: as.AgentID,
			Thread: state.IsThreadWindow(as.Window)})
	}
	root = board.Root(cfg.CompanyRoot, inboxes, board.RememberedRoot(stateDir))
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

// RunBoard: agentlayer board — HTML을 <state>/board.html에 쓰고 전용 브라우저로 연다.
// --out은 파일만, --json은 카드 배열만(stdout), --no-open은 파일만 쓰고 경로 출력.
func RunBoard(w io.Writer, st *state.Store, stateDir string, cfg *config.Config, open func(url string) error, args []string, now time.Time) error {
	out, asJSON, noOpen, refresh := "", false, false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--refresh":
			refresh = true
		case "--json":
			asJSON = true
		case "--no-open":
			noOpen = true
		case "--out":
			if i+1 >= len(args) {
				return errors.New(boardUsage)
			}
			out = args[i+1]
			i++
		default:
			return fmt.Errorf("알 수 없는 인자: %s\n%s", args[i], boardUsage)
		}
	}
	if asJSON && out != "" {
		return errors.New("--json과 --out은 같이 쓸 수 없습니다")
	}
	if refresh {
		// 훅이 detached로 부르는 조용한 갱신 — 파일이 있을 때만, 출력·실패 없음(stderr만).
		if _, err := RefreshBoardFile(st, stateDir, cfg, now); err != nil {
			fmt.Fprintln(os.Stderr, "agentlayer board --refresh:", err)
		}
		return nil
	}
	root, cards, err := LoadBoard(st, stateDir, cfg, now)
	if err != nil {
		return err
	}
	if root == "" {
		return errors.New("회사 루트를 찾지 못했습니다 — 업무를 하나 등록하면(task assign --inbox <root>/runtime/inbox) " +
			"그 루트를 기억해 두므로 이후 등록이 모두 사라져도 보드가 유지됩니다. 설정 company_root를 지정해도 됩니다: " + config.Path())
	}
	if _, err := os.Stat(filepath.Join(root, "tasks")); os.IsNotExist(err) {
		return fmt.Errorf("회사 루트에 tasks/ 폴더가 없습니다: %s (company_root 확인)", root)
	}
	if asJSON {
		if cards == nil {
			cards = []board.Card{}
		}
		return json.NewEncoder(w).Encode(cards)
	}
	page := board.HTML(board.CompanyName(root), cards, now, cfg.BoardStaleLimit())
	path := out
	if path == "" {
		path = BoardPath(stateDir)
	}
	if err := os.WriteFile(path, page, 0o644); err != nil {
		return err
	}
	if out != "" || noOpen {
		fmt.Fprintln(w, "보드:", ShortenHome(path))
		return nil
	}
	url := "file://" + path
	if err := open(url); err != nil {
		return fmt.Errorf("브라우저 열기 실패(%v) — 파일은 %s", err, ShortenHome(path))
	}
	fmt.Fprintln(w, "열림:", ShortenHome(path))
	return nil
}
