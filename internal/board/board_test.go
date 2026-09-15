// internal/board/board_test.go
package board

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func task(status, parents string) string {
	return "# T\n```yaml\nstatus: " + status + "\nparents: " + parents + "\n```\n"
}

func TestLoadDerivesColumns(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "A", task("done", "[]"))
	writeTask(t, root, "B", task("pending", "[A]"))           // ready (부모 done)
	writeTask(t, root, "C", task("pending", "[A, B]"))        // todo (B 미완)
	writeTask(t, root, "D", task("in_progress", "[]"))        // running
	writeTask(t, root, "E", task("waiting_collab-bot", "[]")) // blocked
	writeTask(t, root, "F", task("reviewing", "[]"))          // review
	writeTask(t, root, "G", task("weird", "[]"))              // todo + Unknown
	writeTask(t, root, "H", task("pending", "[ZZZ]"))         // 부모 없음(끊긴 참조) → todo
	cards, err := Load(root, nil, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range cards {
		got[c.ID] = c.Column
	}
	want := map[string]string{"A": ColDone, "B": ColReady, "C": ColTodo, "D": ColRunning, "E": ColBlocked, "F": ColReview, "G": ColTodo, "H": ColTodo}
	for id, col := range want {
		if got[id] != col {
			t.Errorf("%s: %s, want %s", id, got[id], col)
		}
	}
	for _, c := range cards {
		if c.ID == "G" && !c.Unknown {
			t.Error("알 수 없는 status는 Unknown=true")
		}
	}
}

func TestLoadAttachesSessionStateAndLastLog(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "A", task("in_progress", "[]"))
	_ = AppendLog(root, "A", "ASK", "질문?", now)
	links := []Link{{TaskID: "A", Session: "collab-bot", Window: "t123456", AgentID: "claude-%3", Thread: true}}
	cards, err := Load(root, links, map[string]string{"claude-%3": "WAIT"}, now)
	if err != nil {
		t.Fatal(err)
	}
	c := cards[0]
	if c.Session != "collab-bot:t123456" || c.State != "WAIT" || c.LastLog != "[2026-09-15 14:30] [ASK] 질문?" {
		t.Errorf("got %+v", c)
	}
}

func TestLoadReadyTimeIsLatestParentDone(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "A", task("done", "[]"))
	writeTask(t, root, "B", task("pending", "[A]"))
	old := now.Add(-2 * time.Hour)
	_ = os.Chtimes(filepath.Join(root, "tasks", "A", "task.md"), old, old)
	_ = os.Chtimes(filepath.Join(root, "tasks", "B", "task.md"), now.Add(-3*time.Hour), now.Add(-3*time.Hour))
	cards, _ := Load(root, nil, nil, now)
	for _, c := range cards {
		if c.ID == "B" && !c.Ready.Equal(old) {
			t.Errorf("Ready = %v, want 부모 done 시각 %v", c.Ready, old)
		}
	}
}

func TestLoadSortsByIDAndSkipsNonDirs(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "B", task("pending", "[]"))
	writeTask(t, root, "A", task("pending", "[]"))
	_ = os.WriteFile(filepath.Join(root, "tasks", "README.md"), []byte("x"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "tasks", "empty"), 0o755) // task.md 없음 → 건너뜀
	cards, _ := Load(root, nil, nil, now)
	if len(cards) != 2 || cards[0].ID != "A" || cards[1].ID != "B" {
		t.Errorf("cards = %+v", cards)
	}
}

func TestLoadNoTasksDirIsEmptyNotError(t *testing.T) {
	cards, err := Load(t.TempDir(), nil, nil, now)
	if err != nil || len(cards) != 0 {
		t.Errorf("cards=%v err=%v", cards, err)
	}
}

func TestChildrenAndStaleReady(t *testing.T) {
	cards := []Card{
		{ID: "B", Parents: []string{"A"}, Column: ColReady, Ready: now.Add(-31 * time.Minute)},
		{ID: "C", Parents: []string{"A", "X"}, Column: ColBlocked, Updated: now.Add(-5 * time.Minute)},
		{ID: "D", Column: ColRunning, Updated: now.Add(-3 * time.Hour)},
	}
	if ch := Children(cards, "A"); len(ch) != 2 || ch[0].ID != "B" || ch[1].ID != "C" {
		t.Errorf("Children = %+v", ch)
	}
	if !StaleReady(cards[0], now, 30*time.Minute) {
		t.Error("ready 31분은 ⚠")
	}
	if StaleReady(cards[1], now, 30*time.Minute) {
		t.Error("blocked 5분은 아직")
	}
	if StaleReady(cards[2], now, 30*time.Minute) {
		t.Error("running은 대상 아님")
	}
}

func TestInferRootAndRoot(t *testing.T) {
	if got := InferRoot("/Users/x/company/runtime/inbox"); got != "/Users/x/company" {
		t.Errorf("InferRoot = %q", got)
	}
	if got := InferRoot("/Users/x/company/runtime/inbox/"); got != "/Users/x/company" {
		t.Errorf("InferRoot(trailing slash) = %q", got)
	}
	if InferRoot("/Users/x/somewhere") != "" {
		t.Error("규약 밖 경로는 빈 문자열")
	}
	if Root("/cfg", []string{"/a/runtime/inbox"}, "/remembered") != "/cfg" {
		t.Error("설정이 우선")
	}
	if Root("", []string{"/nope", "/a/runtime/inbox"}, "/remembered") != "/a" {
		t.Error("유추가 기억된 값보다 우선")
	}
	if Root("", nil, "/remembered") != "/remembered" {
		t.Error("설정·유추 둘 다 없으면 기억된 값")
	}
	if Root("", nil, "") != "" {
		t.Error("아무것도 없으면 빈 문자열")
	}
}

func TestRememberRootRoundTripAndPriority(t *testing.T) {
	stateDir := t.TempDir()
	if got := RememberedRoot(stateDir); got != "" {
		t.Errorf("아직 기록 전이면 빈 문자열: %q", got)
	}
	if err := RememberRoot(stateDir, "/company/root"); err != nil {
		t.Fatal(err)
	}
	if got := RememberedRoot(stateDir); got != "/company/root" {
		t.Errorf("RememberedRoot = %q", got)
	}
	p := filepath.Join(stateDir, "company.json")
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("company.json mode = %v, want 0600", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(stateDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".company.json") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("임시 파일 잔재: %s", e.Name())
		}
	}
	// 우선순위: 기억된 값은 설정값·유추보다 낮다.
	if Root("/cfg", nil, "/company/root") != "/cfg" {
		t.Error("설정값이 기억된 값보다 우선")
	}
	if Root("", []string{"/a/runtime/inbox"}, "/company/root") != "/a" {
		t.Error("유추가 기억된 값보다 우선")
	}
	if Root("", nil, RememberedRoot(stateDir)) != "/company/root" {
		t.Error("설정·유추 없으면 기억된 값")
	}
}

func TestRememberedRootCorruptFileIsEmpty(t *testing.T) {
	stateDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateDir, "company.json"), []byte("{깨진"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := RememberedRoot(stateDir); got != "" {
		t.Errorf("깨진 파일은 빈 문자열: %q", got)
	}
}

// board 카드의 Session 열은 Link.Thread가 false면 창 이름을 붙이지 않는다(claude가 창 이름을
// 자기 버전으로 바꾸는 경우 등 의미 없는 창).
func TestLoadOmitsWindowWhenNotThread(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "A", task("in_progress", "[]"))
	links := []Link{{TaskID: "A", Session: "al-lab", Window: "2.1.272", AgentID: "claude-%1", Thread: false}}
	cards, err := Load(root, links, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if cards[0].Session != "al-lab" {
		t.Errorf("Session = %q, want 창 이름 없이 al-lab", cards[0].Session)
	}
}

// company_root가 "~" 또는 "~/…"로 시작하면 os.UserHomeDir()로 직접 펼쳐야 한다 — 설정 파일을 읽는
// 경로라 셸이 대신 펼쳐주지 않는다.
func TestRootExpandsHomeTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("홈 디렉터리를 못 구함")
	}
	if got := Root("~/company", nil, ""); got != filepath.Join(home, "company") {
		t.Errorf("Root(~/company) = %q, want %q", got, filepath.Join(home, "company"))
	}
	if got := Root("~", nil, ""); got != home {
		t.Errorf("Root(~) = %q, want %q", got, home)
	}
	if got := Root("/abs/company", nil, ""); got != "/abs/company" {
		t.Errorf("절대경로는 그대로: %q", got)
	}
}

func TestCompanyName(t *testing.T) {
	root := t.TempDir()
	if CompanyName(root) != filepath.Base(root) {
		t.Error("명부 없으면 폴더명")
	}
	_ = os.WriteFile(filepath.Join(root, "직원명부.json"), []byte(`{"name":"AI 치트키 회사"}`), 0o644)
	if CompanyName(root) != "AI 치트키 회사" {
		t.Errorf("CompanyName = %q", CompanyName(root))
	}
}

func TestCounts(t *testing.T) {
	c := Counts([]Card{{Column: ColReady}, {Column: ColReady}, {Column: ColDone}})
	if c[ColReady] != 2 || c[ColDone] != 1 || c[ColTodo] != 0 {
		t.Errorf("Counts = %v", c)
	}
}
