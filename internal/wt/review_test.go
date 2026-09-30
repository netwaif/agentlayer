package wt

import (
	"bytes"
	"github.com/netwaif/agentlayer/internal/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTestRecordsResult(t *testing.T) {
	repo := fixtureRepo(t)
	stateDir := t.TempDir()
	m, _ := New(stateDir, NewOptions{Task: "t", Repo: repo, NoWindow: true, TestCmd: "true"})
	var buf bytes.Buffer
	pass, err := RunTest(&buf, stateDir, "t", "")
	if err != nil || !pass {
		t.Fatalf("true는 통과: %v %v", pass, err)
	}
	m, _ = LoadMeta(stateDir, "t")
	if m.TestPass == nil || !*m.TestPass || m.TestAt == nil {
		t.Errorf("결과 기록: %+v", m)
	}
	// 실패 명령 + override 저장
	pass, err = RunTest(&buf, stateDir, "t", "false")
	if err != nil || pass {
		t.Fatalf("false는 실패: %v %v", pass, err)
	}
	m, _ = LoadMeta(stateDir, "t")
	if *m.TestPass || m.TestCmd != "false" {
		t.Errorf("실패 기록 + cmd override: %+v", m)
	}
}

func TestRunTestNoCmd(t *testing.T) {
	repo := fixtureRepo(t)
	stateDir := t.TempDir()
	New(stateDir, NewOptions{Task: "t", Repo: repo, NoWindow: true})
	if _, err := RunTest(&bytes.Buffer{}, stateDir, "t", ""); err == nil {
		t.Error("명령 없으면 에러")
	}
}

func TestReviewRoundTrip(t *testing.T) {
	repo := fixtureRepo(t)
	stateDir := t.TempDir()
	m, _ := New(stateDir, NewOptions{Task: "t", Repo: repo, NoWindow: true})

	// 변경 없으면 리뷰 거부
	if _, err := WriteReviewFile(stateDir, "t"); err == nil {
		t.Error("변경 없으면 리뷰 파일 안 만듦")
	}

	os.WriteFile(filepath.Join(m.Path, "a.txt"), []byte("변경된 내용\n"), 0o644)
	path, err := WriteReviewFile(stateDir, "t")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "wt send t") || !strings.Contains(string(content), "+변경된 내용") {
		t.Errorf("리뷰 파일 내용:\n%s", content)
	}

	// 코멘트 삽입 후 추출
	edited := strings.Replace(string(content), "+변경된 내용",
		"+변경된 내용\n#> 이 줄은 상수로 빼주세요\n#> 그리고 테스트 추가", 1)
	os.WriteFile(path, []byte(edited), 0o644)
	comments, err := ExtractComments(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 2 {
		t.Fatalf("코멘트 2건: %+v", comments)
	}
	if comments[0].Text != "이 줄은 상수로 빼주세요" || !strings.Contains(comments[0].Context, "변경된 내용") {
		t.Errorf("코멘트+문맥: %+v", comments[0])
	}
	inst := BuildInstruction("t", comments)
	if !strings.Contains(inst, "2건") || !strings.Contains(inst, "상수로") || strings.Contains(inst, "\n") {
		t.Errorf("지시 문단은 한 줄: %q", inst)
	}
}

// wt send는 pane에 직접 치지 않고 주입된 전송기(cli의 채널·큐·tmux 폴백 공통 경로)로 보낸다 — 전송 규칙을 한 곳으로(2026-09-30).
func TestSendCommentsUsesInjectedSender(t *testing.T) {
	stateDir := t.TempDir()
	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := SaveMeta(stateDir, &Meta{Task: "t", Path: wtPath, Agent: "claude"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(ReviewPath(stateDir, "t")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ReviewPath(stateDir, "t"), []byte("+foo\n#> 이름 바꿔\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ := state.NewStore(stateDir)
	_ = st.Save(&state.Agent{ID: "claude-9", Kind: "claude", CWD: wtPath, State: state.StateIdle, Tmux: state.TmuxRef{Session: "w", PaneID: "%9"}})
	var gotAgent *state.Agent
	var gotText string
	n, err := SendComments(stateDir, "t", st, func(a *state.Agent, text string) error { gotAgent, gotText = a, text; return nil })
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if gotAgent == nil || gotAgent.Tmux.PaneID != "%9" || !strings.Contains(gotText, "이름 바꿔") {
		t.Errorf("전송기에 대상 에이전트와 지시가 가야 함: %+v %q", gotAgent, gotText)
	}
}
