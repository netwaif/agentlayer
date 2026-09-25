package remote

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

const showDone = `{"task":{"id":"t_40b3eb2f","title":"PING","status":"done","result":"PONG-OK","workspace_path":"/opt/data/kanban/workspaces/t_40b3eb2f","created_by":"agentlayer"},
"latest_summary":"PONG-OK","comments":[],
"events":[{"kind":"created","payload":{},"created_at":1790320928},{"kind":"claimed","payload":{},"created_at":1790320929},{"kind":"completed","payload":{"summary":"PONG-OK"},"created_at":1790320946}],
"runs":[{"id":7,"profile":"tech-qa","status":"done","outcome":"completed","summary":"PONG-OK","error":null}]}`

const showBlocked = `{"task":{"id":"t_1","title":"Q","status":"blocked","result":null,"workspace_path":"/w"},
"latest_summary":null,"comments":[{"author":"tech-qa","body":"BLOCKED: 파일 이름을 알려주세요","created_at":1790320270}],
"events":[{"kind":"created","payload":{},"created_at":1790320100},{"kind":"blocked","payload":{"reason":"파일 이름을 알려주세요","kind":"needs_input"},"created_at":1790320270}],"runs":[]}`

const showCrashed = `{"task":{"id":"t_2","title":"X","status":"crashed","result":null,"workspace_path":"/w"},"latest_summary":null,"comments":[],
"events":[{"kind":"crashed","payload":{},"created_at":1790320400}],"runs":[{"id":1,"outcome":"crashed","error":"worker exited 1"}]}`

func TestStatusOfCard(t *testing.T) {
	cases := []struct {
		raw  string
		want state.AgentState
		ask  string
		sum  string
		seen int64
	}{
		{showDone, state.StateDoneUnread, "", "PONG-OK", 1790320946},
		{showBlocked, state.StateWaiting, "파일 이름을 알려주세요", "", 1790320270},
		{showCrashed, state.StateError, "", "", 1790320400},
		{`{"task":{"status":"ready"},"events":[]}`, state.StateIdle, "", "", 0},
		{`{"task":{"status":"running"},"events":[]}`, state.StateWorking, "", "", 0},
	}
	for _, c := range cases {
		var card Card
		if err := json.Unmarshal([]byte(c.raw), &card); err != nil {
			t.Fatal(err)
		}
		s := StatusOfCard(card)
		if s.State != c.want || s.Ask != c.ask || s.Summary != c.sum || s.Seen != c.seen {
			t.Errorf("%s: got %+v", c.raw[:30], s)
		}
	}
	var crashed Card
	_ = json.Unmarshal([]byte(showCrashed), &crashed)
	if StatusOfCard(crashed).Error != "worker exited 1" {
		t.Error("crashed는 runs[].error를 Error에")
	}
}

// replyTable은 argv 접두어 → 응답. FakeRunner.Reply에서 쓴다.
func replyTable(t *testing.T, table map[string]string) func([]string) ([]byte, error) {
	return func(args []string) ([]byte, error) {
		joined := strings.Join(args, " ")
		for prefix, out := range table {
			if strings.HasPrefix(joined, prefix) {
				return []byte(out), nil
			}
		}
		t.Logf("응답 없는 호출: %s", joined)
		return nil, nil
	}
}

func newHermes(f *FakeRunner) *Hermes {
	return &Hermes{R: f, Profile: "tech-qa", WorkspaceRoot: "/opt/data/ai-company/결과물", MailboxAssignee: "imac-manager", MaxRuntime: "2h",
		Now: func() time.Time { return time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC) }}
}

func TestHermesDispatch(t *testing.T) {
	f := &FakeRunner{}
	f.Reply = replyTable(t, map[string]string{
		"hermes kanban create":   `{"id":"t_new"}`,
		"hermes kanban dispatch": `{"spawned":[{"task_id":"t_new","assignee":"tech-qa"}],"skipped_nonspawnable":[]}`,
	})
	h, err := newHermes(f).Dispatch(context.Background(), DispatchRequest{TaskID: "PING-2", Title: "연결 시험", Body: "본문 it's", Attempt: "a1"})
	if err != nil || h != "t_new" {
		t.Fatalf("Dispatch: %q %v", h, err)
	}
	if f.Calls[0][0] != "mkdir" || f.Calls[0][2] != "/opt/data/ai-company/결과물/PING-2" {
		t.Errorf("첫 호출은 mkdir -p <ws>: %v", f.Calls[0])
	}
	create := strings.Join(f.Calls[1], " ")
	for _, want := range []string{"create PING-2 연결 시험", "--assignee tech-qa", "--idempotency-key agentlayer:PING-2:a1", "--created-by agentlayer",
		"--workspace dir:/opt/data/ai-company/결과물/PING-2", "--max-runtime 2h", "--body=본문 it's", "--json"} {
		if !strings.Contains(create, want) {
			t.Errorf("create에 %q 없음: %s", want, create)
		}
	}
	if strings.Contains(create, "--parent") {
		t.Error("parent 없으면 --parent 없음")
	}
	f.Calls = nil
	if _, err := newHermes(f).Dispatch(context.Background(), DispatchRequest{TaskID: "PING-2", Title: "연결 시험", Body: "후속", Parent: "t_prev", Attempt: "a1"}); err != nil {
		t.Fatal(err)
	}
	create = strings.Join(f.Calls[1], " ")
	if !strings.Contains(create, "--parent t_prev") || !strings.Contains(create, "--idempotency-key agentlayer:PING-2:a1:t_prev") {
		t.Errorf("후속 카드 옵션: %s", create)
	}
}

func TestHermesDispatchNotSpawned(t *testing.T) {
	f := &FakeRunner{Reply: replyTable(t, map[string]string{
		"hermes kanban create":   `{"id":"t_new"}`,
		"hermes kanban dispatch": `{"spawned":[],"skipped_per_profile_capped":["t_new"]}`,
	})}
	h, err := newHermes(f).Dispatch(context.Background(), DispatchRequest{TaskID: "PING-2", Body: "본문", Attempt: "a1"})
	if err == nil || !strings.Contains(err.Error(), "skipped_per_profile_capped") {
		t.Fatalf("기동 실패는 사유를 담은 에러: %v", err)
	}
	if h != "t_new" {
		t.Errorf("카드는 남긴다(재시도용 handle 반환): %q", h)
	}
}

func TestHermesPollReplyFinish(t *testing.T) {
	f := &FakeRunner{Reply: replyTable(t, map[string]string{
		"hermes kanban show t_1": showBlocked,
		"hermes kanban unblock":  "",
		"hermes kanban dispatch": `{"spawned":[{"task_id":"t_1"}]}`,
		"hermes kanban archive":  "",
	})}
	h := newHermes(f)
	s, err := h.Poll(context.Background(), "t_1")
	if err != nil || s.State != state.StateWaiting {
		t.Fatalf("Poll: %+v %v", s, err)
	}
	if err := h.Reply(context.Background(), "t_1", "이름은 a.txt"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.Calls[1], " "); got != "hermes kanban unblock t_1 --reason=이름은 a.txt" {
		t.Errorf("unblock argv=%q", got)
	}
	if err := h.Finish(context.Background(), "t_1"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.Calls[len(f.Calls)-1], " "); got != "hermes kanban archive t_1" {
		t.Errorf("archive argv=%q", got)
	}
}

func TestHermesBoardFlag(t *testing.T) {
	f := &FakeRunner{Reply: replyTable(t, map[string]string{"hermes kanban --board b1 show": showDone})}
	h := newHermes(f)
	h.Board = "b1"
	if _, err := h.Poll(context.Background(), "t_40b3eb2f"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.Calls[0], " "); !strings.HasPrefix(got, "hermes kanban --board b1 show t_40b3eb2f --json") {
		t.Errorf("argv=%q", got)
	}
}

func TestHermesMailbox(t *testing.T) {
	list := `[{"id":"t_m1","title":"[VIDEO-07] 정리본","body":"정리 내용","assignee":"imac-manager","status":"ready","created_by":"default","created_at":1790320000},
	{"id":"t_m2","title":"다른 직원 카드","body":"x","assignee":"tech-qa","status":"ready","created_by":"agentlayer","created_at":1790320001},
	{"id":"t_m3","title":"이미 받은 편지","body":"y","assignee":"imac-manager","status":"done","created_by":"default","created_at":1790320002}]`
	f := &FakeRunner{Reply: replyTable(t, map[string]string{"hermes kanban list --json": list, "hermes kanban claim": "/w", "hermes kanban complete": ""})}
	letters, err := newHermes(f).Mailbox(context.Background())
	if err != nil || len(letters) != 1 {
		t.Fatalf("letters=%+v err=%v", letters, err)
	}
	l := letters[0]
	if l.ID != "t_m1" || l.From != "default" || l.Text != "정리 내용" || l.TaskID != "VIDEO-07" || l.At.Unix() != 1790320000 {
		t.Errorf("letter=%+v", l)
	}
	if got := strings.Join(f.Calls[1], " "); got != "hermes kanban claim t_m1" {
		t.Errorf("claim argv=%q", got)
	}
	if got := strings.Join(f.Calls[2], " "); got != "hermes kanban complete t_m1 --result received by agentlayer 2026-09-25T10:00:00Z" {
		t.Errorf("complete argv=%q", got)
	}
}

func TestHermesMailboxSkipsOnAckFailure(t *testing.T) {
	list := `[{"id":"t_m1","title":"편지","body":"b","assignee":"imac-manager","status":"ready","created_by":"default","created_at":1}]`
	f := &FakeRunner{Reply: func(args []string) ([]byte, error) {
		if args[2] == "list" {
			return []byte(list), nil
		}
		return nil, os.ErrPermission
	}}
	letters, _ := newHermes(f).Mailbox(context.Background())
	if len(letters) != 0 {
		t.Error("수신 확인 실패한 편지는 돌려주지 않는다(다음 주기 재시도)")
	}
}

func TestHermesPullWritesResult(t *testing.T) {
	tarBytes := tarOf(t, map[string]string{"out.txt": "hello"})
	f := &FakeRunner{Reply: func(args []string) ([]byte, error) {
		if args[0] == "sh" {
			return tarBytes, nil
		}
		return []byte(showDone), nil
	}}
	dest := filepath.Join(t.TempDir(), "remote")
	if err := newHermes(f).Pull(context.Background(), "t_40b3eb2f", dest); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "out.txt")); string(b) != "hello" {
		t.Error("tar 내용이 풀려야 함")
	}
	res, _ := os.ReadFile(filepath.Join(dest, "RESULT.md"))
	if !strings.Contains(string(res), "PONG-OK") || !strings.Contains(string(res), "t_40b3eb2f") {
		t.Errorf("RESULT.md=%s", res)
	}
	// 원격에서 잘라 받는다(메모리·대역폭 상한): sh -c 'tar … | head -c <상한+1>'
	if got := strings.Join(f.Calls[1], " "); !strings.HasPrefix(got, "sh -c tar -C '/opt/data/kanban/workspaces/t_40b3eb2f' -cf - . | head -c ") {
		t.Errorf("tar argv=%q", got)
	}
}

func TestHermesCheck(t *testing.T) {
	f := &FakeRunner{Reply: replyTable(t, map[string]string{
		"hermes --version":        "Hermes Agent v0.20.0 (2026.8.3)\nInstall directory: /opt/hermes\n",
		"hermes kanban assignees": "NAME      ON DISK   COUNTS\ntech-qa   yes       (idle)\n",
	})}
	info, err := newHermes(f).Check(context.Background())
	if err != nil || info.Version != "Hermes Agent v0.20.0 (2026.8.3)" || !info.ProfileOK {
		t.Errorf("info=%+v err=%v", info, err)
	}
}

func TestHermesDispatchTitleAlreadyHasID(t *testing.T) {
	f := &FakeRunner{Reply: replyTable(t, map[string]string{
		"hermes kanban create":   `{"id":"t_x"}`,
		"hermes kanban dispatch": `{"spawned":[{"task_id":"t_x"}]}`,
	})}
	if _, err := newHermes(f).Dispatch(context.Background(), DispatchRequest{TaskID: "PING-2", Title: "PING-2 원격 연결 시험", Body: "b", Attempt: "a1"}); err != nil {
		t.Fatal(err)
	}
	if got := f.Calls[1][3]; got != "PING-2 원격 연결 시험" {
		t.Errorf("제목이 업무ID로 시작하면 ID를 다시 붙이지 않는다: %q", got)
	}
}

func TestHermesResumeDispatchesOnly(t *testing.T) {
	f := &FakeRunner{Reply: replyTable(t, map[string]string{"hermes kanban dispatch": `{"spawned":[{"task_id":"t_1"}]}`})}
	if err := newHermes(f).Resume(context.Background(), "t_1"); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls) != 1 || f.Calls[0][2] != "dispatch" {
		t.Errorf("Resume는 create 없이 dispatch만: %v", f.Calls)
	}
}

func TestHermesPullRejectsOversize(t *testing.T) {
	big := make([]byte, MaxPullBytes+1)
	f := &FakeRunner{Reply: func(args []string) ([]byte, error) {
		if args[0] == "sh" {
			return big, nil
		}
		return []byte(showDone), nil
	}}
	err := newHermes(f).Pull(context.Background(), "t_40b3eb2f", filepath.Join(t.TempDir(), "r"))
	if err == nil || !strings.Contains(err.Error(), "초과") {
		t.Errorf("상한 초과 스트림은 거부: %v", err)
	}
}
