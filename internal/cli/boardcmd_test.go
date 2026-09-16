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

// 총괄이 마지막 업무를 task done으로 닫으면 등록이 하나도 안 남는다 — task assign이 기억해 둔
// 루트로 LoadBoard가 여전히 회사를 찾아야 한다(company.json).
func TestLoadBoardFindsRootAfterLastRegistrationDone(t *testing.T) {
	stateDir := t.TempDir()
	st, err := state.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}}); err != nil {
		t.Fatal(err)
	}
	root := companyRoot(t, "LAB-1")
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir,
		[]string{"assign", "LAB-1", "collab-bot", "--inbox", filepath.Join(root, "runtime", "inbox")}, time.Now()); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"done", "LAB-1"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if list, err := task.List(stateDir); err != nil || len(list) != 0 {
		t.Fatalf("등록이 남아있으면 이 테스트의 전제가 깨짐: list=%v err=%v", list, err)
	}
	gotRoot, _, err := LoadBoard(st, stateDir, &config.Config{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != root {
		t.Errorf("root = %q, want 기억된 %q", gotRoot, root)
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

// --json과 --out은 같이 쓸 수 없다 — 둘 다 "출력 목적지"라 하나만 의미가 있다.
func TestRunBoardJSONAndOutTogetherIsError(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	var out bytes.Buffer
	err := RunBoard(&out, st, stateDir, &config.Config{}, func(string) error { return nil },
		[]string{"--json", "--out", filepath.Join(t.TempDir(), "b.html")}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "--json과 --out은 같이 쓸 수 없습니다") {
		t.Errorf("--json+--out은 오류여야 함: %v", err)
	}
}

// company_root가 지정됐지만 그 아래 tasks/ 폴더가 없으면(오타·잘못된 경로) 조용히 빈 보드를
// 만들지 말고 경로를 짚어 알려야 한다.
func TestRunBoardMissingTasksDirExplains(t *testing.T) {
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	root := t.TempDir() // tasks/ 없음
	var out bytes.Buffer
	err := RunBoard(&out, st, stateDir, &config.Config{CompanyRoot: root}, func(string) error { return nil }, nil, time.Now())
	if err == nil || !strings.Contains(err.Error(), "tasks/ 폴더가 없습니다") || !strings.Contains(err.Error(), root) {
		t.Errorf("tasks/ 없으면 경로를 짚어 알려야 함: %v", err)
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

// RefreshBoardFile은 보드 파일이 이미 있을 때만 다시 쓴다 — 보드를 연 적 없는 사용자에게 파일을
// 만들어 주지 않고, 있으면 현재 카드로 덮어쓴다. --refresh는 같은 동작을 조용히 한다.
func TestRefreshBoardFileOnlyWhenFileExists(t *testing.T) {
	stateDir := t.TempDir()
	st, err := state.NewStore(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&state.Agent{ID: "claude-%1", Kind: "claude", State: state.StateIdle,
		Tmux: state.TmuxRef{Session: "collab-bot", PaneID: "%1"}}); err != nil {
		t.Fatal(err)
	}
	root := companyRoot(t, "LAB-1")
	var out bytes.Buffer
	if err := RunTask(context.Background(), &out, st, stateDir,
		[]string{"assign", "LAB-1", "collab-bot", "--inbox", filepath.Join(root, "runtime", "inbox")}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if p, err := RefreshBoardFile(st, stateDir, &config.Config{}, time.Now()); err != nil || p != "" {
		t.Fatalf("파일 없을 때 = (%q, %v), want (\"\", nil)", p, err)
	}
	if _, err := os.Stat(BoardPath(stateDir)); !os.IsNotExist(err) {
		t.Fatal("파일이 없어야 하는데 생김")
	}
	if err := RunBoard(&out, st, stateDir, &config.Config{}, nil, []string{"--no-open"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(BoardPath(stateDir))
	// 카드가 바뀐 뒤(done) 갱신 → 내용이 달라져야 한다
	if err := RunTask(context.Background(), &out, st, stateDir, []string{"done", "LAB-1"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(BoardPath(stateDir))
	if string(before) == string(after) || !strings.Contains(string(after), `data-col="done"`) {
		t.Error("task done 뒤 보드 파일이 갱신되지 않음")
	}
	out.Reset()
	if err := RunBoard(&out, st, stateDir, &config.Config{}, nil, []string{"--refresh"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("--refresh는 조용해야 함: %q", out.String())
	}
	_ = json.Valid
}
