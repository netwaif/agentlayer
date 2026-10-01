package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/netwaif/agentlayer/internal/remote"
)

// scriptedAdapter에 편지함·답장을 더한다(기존 테스트는 이 메서드를 부르지 않는다).
func (s *scriptedAdapter) Mailbox(context.Context) ([]remote.Letter, error) { return s.letters, nil }
func (s *scriptedAdapter) Answer(_ context.Context, id, text string, files []string) error {
	s.answered = append(s.answered, id+"|"+text+"|"+strings.Join(files, ","))
	return s.answerErr
}

// appAdapter — Claude 앞 편지함(AppMailboxer)까지 있는 어댑터(Hermes 역할).
type appAdapter struct {
	*scriptedAdapter
	appLetters []remote.Letter
}

func (a *appAdapter) AppMailbox(context.Context) ([]remote.Letter, error) { return a.appLetters, nil }

func saveQA(t *testing.T, stateDir string) {
	t.Helper()
	if err := remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"}); err != nil {
		t.Fatal(err)
	}
}

// --app-mailbox: claude-app 앞 편지만 받고, 출력에 letter: <원격>:<카드ID> 줄이 붙는다. --mailbox 출력은 그대로.
func TestInboxWaitAppMailbox(t *testing.T) {
	stubSession(t, 6262, map[int]bool{6262: true})
	stateDir := t.TempDir()
	saveQA(t, stateDir)
	ad := &appAdapter{scriptedAdapter: &scriptedAdapter{
		letters: []remote.Letter{{ID: "t_m1", From: "default", Text: "회사 편지"}}},
		appLetters: []remote.Letter{{ID: "t_c1", From: "default", Text: "썸네일 문구 3개\n뽑아 줘"}}}
	stubRemoteAdapter(t, ad)
	var out, errb bytes.Buffer
	args := []string{"wait", "--name", "app", "--interval", "10ms", "--timeout", "3s", "--remote", "hermes-qa", "--remote-interval", "20ms"}
	if err := RunInboxWait(context.Background(), &out, &errb, stateDir, append(args, "--app-mailbox"), tnow); err != nil {
		t.Fatalf("wait: %v %s", err, errb.String())
	}
	if out.String() != "from: hermes-qa/default\nletter: hermes-qa:t_c1\n\n썸네일 문구 3개\n뽑아 줘\n" {
		t.Errorf("app 편지 출력: %q", out.String())
	}
	// 회사 편지함(--mailbox)은 예전 형식 그대로(letter: 줄 없음), 회사 카드만
	out.Reset()
	if err := RunInboxWait(context.Background(), &out, &errb, stateDir, append(args, "--mailbox"), tnow); err != nil {
		t.Fatalf("wait(mailbox): %v", err)
	}
	if out.String() != "from: hermes-qa/default\n\n회사 편지\n" {
		t.Errorf("--mailbox 출력 불변: %q", out.String())
	}
	// 카드도 편지함 지정도 없으면 오류(예전 그대로), AppMailboxer가 아닌 어댑터에 --app-mailbox는 오류
	if err := RunInboxWait(context.Background(), &out, &errb, stateDir, args, tnow); err == nil || !strings.Contains(err.Error(), "--app-mailbox") {
		t.Errorf("지정 없음: %v", err)
	}
	stubRemoteAdapter(t, &scriptedAdapter{})
	if err := RunInboxWait(context.Background(), &out, &errb, stateDir, append(args, "--app-mailbox"), tnow); err == nil || !strings.Contains(err.Error(), "편지함(--app-mailbox)이 없습니다") {
		t.Errorf("AppMailboxer 아님: %v", err)
	}
}

func TestInboxReply(t *testing.T) {
	stateDir := t.TempDir()
	saveQA(t, stateDir)
	ad := &scriptedAdapter{}
	stubRemoteAdapter(t, ad)
	var out bytes.Buffer
	if err := RunInboxReply(context.Background(), &out, nil, stateDir, []string{"hermes-qa:t_c1", "문구", "셋입니다"}); err != nil {
		t.Fatal(err)
	}
	if err := RunInboxReply(context.Background(), &out, strings.NewReader("첫 줄\n둘째 줄\n"), stateDir, []string{"--file", "a.png", "--file=b.md", "hermes-qa:t_c2", "-"}); err != nil {
		t.Fatal(err)
	}
	if len(ad.answered) != 2 || ad.answered[0] != "t_c1|문구 셋입니다|" || ad.answered[1] != "t_c2|첫 줄\n둘째 줄|a.png,b.md" {
		t.Errorf("Answer 호출: %q", ad.answered)
	}
	if !strings.Contains(out.String(), "답장 완료 → hermes-qa:t_c1 (카드 완료)") || !strings.Contains(out.String(), "hermes-qa:t_c2 (카드 완료 첨부 2개)") {
		t.Errorf("출력: %s", out.String())
	}
	// 오류: 형식·없는 원격·빈 답·어댑터 실패
	for _, bad := range [][]string{{"hermes-qa", "x"}, {":t_c1", "x"}, {"hermes-qa:", "x"}, {"nope:t_c1", "x"}, {"hermes-qa:t_c1"}, {"hermes-qa:t_c1", "  "}, {"--bogus", "hermes-qa:t_c1", "x"}} {
		if err := RunInboxReply(context.Background(), &out, nil, stateDir, bad); err == nil {
			t.Errorf("%v: 오류여야 함", bad)
		}
	}
	ad.answerErr = errors.New("kanban complete 실패")
	if err := RunInboxReply(context.Background(), &out, nil, stateDir, []string{"hermes-qa:t_c1", "x"}); err == nil || !strings.Contains(err.Error(), "답장 실패") {
		t.Errorf("어댑터 실패: %v", err)
	}
	if len(ad.answered) != 3 {
		t.Errorf("오류 입력은 Answer를 부르지 않는다: %q", ad.answered)
	}
}
