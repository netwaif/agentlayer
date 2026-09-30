package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

// 업무 등록이 없는 원격은 예전에 오류였다 — 그 경우에만 직송(Dispatch). 등록이 있으면 기존 경로 그대로.

func stubRemoteAdapter(t *testing.T, ad remote.Adapter) {
	t.Helper()
	old := OpenRemote
	OpenRemote = func(remote.Remote, string) (remote.Adapter, error) { return ad, nil }
	t.Cleanup(func() { OpenRemote = old })
}

func TestSendRemoteDirectWithoutTask(t *testing.T) {
	ad := &scriptedAdapter{handle: "card-9"}
	stubRemoteAdapter(t, ad)
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	if err := remote.Save(stateDir, remote.Remote{Name: "hermes-qa", Kind: "hermes", SSH: "h", Profile: "p", WorkspaceRoot: "/w"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunSend(context.Background(), &out, nil, st, stateDir, nil, []string{"--json", "hermes-qa", "제목 줄\n본문 둘째 줄"}); err != nil {
		t.Fatal(err)
	}
	if len(ad.dispatched) != 1 || !strings.HasPrefix(ad.dispatched[0], "MSG-") || !strings.Contains(ad.dispatched[0], "|제목 줄|제목 줄\n본문 둘째 줄|") {
		t.Errorf("Dispatch: %v", ad.dispatched)
	}
	var res map[string]any
	_ = json.Unmarshal(out.Bytes(), &res)
	if res["via"] != "remote" || res["action"] != "직송" || res["handle"] != "card-9" || res["sent"] != true || !strings.HasPrefix(res["task_id"].(string), "MSG-") {
		t.Errorf("JSON: %s", out.String())
	}
	out.Reset()
	if err := RunSend(context.Background(), &out, nil, st, stateDir, nil, []string{"hermes-qa", "둘"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "전송 완료 → hermes-qa (hermes card-9) [업무 등록 없음] 직송") {
		t.Errorf("출력: %s", out.String())
	}
	// 어댑터 실패는 오류
	ad.dispatchErr = errors.New("ssh 끊김")
	if err := RunSend(context.Background(), &out, nil, st, stateDir, nil, []string{"hermes-qa", "셋"}); err == nil || !strings.Contains(err.Error(), "ssh 끊김") {
		t.Errorf("실패: %v", err)
	}
	// 첨부 직송은 hermes 어댑터(*remote.Hermes)만 — scriptedAdapter는 거부
	if err := RunSend(context.Background(), &out, nil, st, stateDir, nil, []string{"--file", "x.png", "hermes-qa", "넷"}); err == nil || !strings.Contains(err.Error(), "hermes 원격만") {
		t.Errorf("첨부: %v", err)
	}
}

// 업무가 등록된 원격은 기존 경로(sendRemote) 그대로이고, --file은 오류다.
func TestSendRemoteWithTaskKeepsOldPath(t *testing.T) {
	ad := &scriptedAdapter{handle: "card-1", status: remote.Status{State: state.StateIdle}}
	st, stateDir, _ := remoteSendFixture(t, ad)
	var out bytes.Buffer
	if err := RunSend(context.Background(), &out, nil, st, stateDir, nil, []string{"--file", "a.md", "hermes-qa", "x"}); !errors.Is(err, errNoFileHere) {
		t.Errorf("--file은 기존 경로에서 오류: %v", err)
	}
	if err := RunSend(context.Background(), &out, nil, st, stateDir, nil, []string{"hermes-qa", "지시"}); err != nil {
		t.Fatal(err)
	}
	if len(ad.dispatched) != 1 || !strings.HasPrefix(ad.dispatched[0], "PING-2|") {
		t.Errorf("기존 경로(등록된 업무ID로 Dispatch): %v", ad.dispatched)
	}
}

type recordingRunner struct {
	args  [][]string
	stdin []string
	err   error
}

func (r *recordingRunner) Run(_ context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	r.args = append(r.args, args)
	b, _ := io.ReadAll(stdin)
	r.stdin = append(r.stdin, string(b))
	return nil, r.err
}

// 첨부는 Hermes.upload과 같은 셸 한 줄로 카드 작업 폴더 아래 from-company/에 올린다.
func TestUploadToHermes(t *testing.T) {
	rr := &recordingRunner{}
	h := &remote.Hermes{R: rr, WorkspaceRoot: "/w"}
	local := filepath.Join(t.TempDir(), "그림 1.png")
	if err := os.WriteFile(local, []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := uploadToHermes(context.Background(), h, "/w/MSG-1/from-company", local)
	if err != nil || p != "/w/MSG-1/from-company/그림 1.png" {
		t.Fatalf("%q %v", p, err)
	}
	if len(rr.args) != 1 || rr.args[0][0] != "sh" || !strings.Contains(rr.args[0][2], "mkdir -p '/w/MSG-1/from-company' && cat > '/w/MSG-1/from-company/그림 1.png'") || rr.stdin[0] != "PNG" {
		t.Errorf("셸 한 줄·stdin: %v %v", rr.args, rr.stdin)
	}
	if _, err := uploadToHermes(context.Background(), h, "/w/x", filepath.Join(t.TempDir(), "없음")); err == nil {
		t.Error("없는 파일은 오류")
	}
	if _, err := uploadToHermes(context.Background(), h, "/w/x", t.TempDir()); err == nil || !strings.Contains(err.Error(), "폴더") {
		t.Errorf("폴더는 오류: %v", err)
	}
	rr.err = errors.New("no space")
	if _, err := uploadToHermes(context.Background(), h, "/w/x", local); err == nil || !strings.Contains(err.Error(), "업로드 실패") {
		t.Errorf("러너 실패: %v", err)
	}
}

func TestFirstLineTitle(t *testing.T) {
	if got := firstLineTitle("  제목  \n본문"); got != "제목" {
		t.Errorf("%q", got)
	}
	long := strings.Repeat("가", 70)
	if got := firstLineTitle(long); got != strings.Repeat("가", 60)+"…" {
		t.Errorf("%q", got)
	}
}
