package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netwaif/agentlayer/internal/state"
)

// 코덱스 데스크톱 앱 세션에는 훅·tmux가 없어 레코드가 없다 — 세션 ID(전체 또는 앞자리 접두)만으로 큐에 보낸다.

func TestLooksLikeSessionID(t *testing.T) {
	yes := []string{"0199a1b2", "0199a1b2-2222", "0199A1B2-2222-4C1E", "0199a1b2-2222-4c1e-9f1a-000000000002"}
	no := []string{"collab-bot", "0199a1b", "0199a1b2x", "0199a1b22222", "0199a1b2-2222-4c1e-9f1a-000000000002-1", "search-youtube-bot:t170966"}
	for _, s := range yes {
		if !LooksLikeSessionID(s) {
			t.Errorf("%q는 세션 ID 형식", s)
		}
	}
	for _, s := range no {
		if LooksLikeSessionID(s) {
			t.Errorf("%q는 세션 ID 형식이 아님", s)
		}
	}
}

func TestParseSendFlagsCwd(t *testing.T) {
	o, rest, err := ParseSendFlags([]string{"--json", "--cwd", "/w/a", "0199a1b2", "본문"})
	if err != nil || !o.JSON || o.CWD != "/w/a" || len(rest) != 2 || rest[0] != "0199a1b2" {
		t.Fatalf("%+v %v %v", o, rest, err)
	}
	o, rest, err = ParseSendFlags([]string{"--cwd=/w/b", "x", "y"})
	if err != nil || o.CWD != "/w/b" || len(rest) != 2 {
		t.Fatalf("%+v %v %v", o, rest, err)
	}
	if _, _, err := ParseSendFlags([]string{"--cwd"}); err == nil {
		t.Error("--cwd 뒤에 폴더가 없으면 오류")
	}
}

// 훅이 세션 ID를 남긴 tmux 코덱스는 세션 ID 접두로도 찾는다(큐·관문은 예전 경로).
func TestResolveTargetBySessionIDPrefix(t *testing.T) {
	a := mkAgent("codex", "codex-live", "%2", state.StateIdle)
	a.SessionID = "0199a1b2-1111-4c1e-9f1a-000000000001"
	b := mkAgent("codex", "codex-2", "%3", state.StateWorking)
	b.SessionID = "0199a1b2-2222-4c1e-9f1a-000000000002"
	dead := mkAgent("codex", "codex-old", "%4", state.StateDead)
	dead.SessionID = "0199a1b2-2222-4c1e-9f1a-000000000002" // 같은 세션의 옛 레코드
	agents := []*state.Agent{a, b, dead}
	if got, err := ResolveTarget(agents, "0199a1b2-1111"); err != nil || got != a {
		t.Fatalf("접두: %v %v", got, err)
	}
	if got, err := ResolveTarget(agents, "0199a1b2-2222"); err != nil || got != b {
		t.Fatalf("산 레코드 우선: %v %v", got, err)
	}
	if _, err := ResolveTarget(agents, "0199a1b2"); err == nil || !strings.Contains(err.Error(), "둘 이상") || !strings.Contains(err.Error(), "codex-live(%2") {
		t.Fatalf("모호하면 후보와 함께 거부: %v", err)
	}
	if _, err := ResolveTarget(agents, "0199a1b2-1111:t123456"); err == nil {
		t.Error("창 지정은 tmux 세션 이름에만")
	}
	if _, err := ResolveTarget(agents, "0199a1"); err == nil || strings.Contains(err.Error(), "둘 이상") {
		t.Errorf("8자 미만 접두는 세션 ID로 보지 않는다: %v", err)
	}
}

func stubCodexRollouts(t *testing.T, sessions map[string]string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "2026", "09", "30")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for sid, cwd := range sessions {
		line := `{"timestamp":"2026-09-30T01:00:00.000Z","type":"session_meta","payload":{"session_id":"` + sid + `","id":"` + sid + `","timestamp":"2026-09-30T01:00:00.000Z","cwd":"` + cwd + `"}}` + "\n"
		if err := os.WriteFile(filepath.Join(dir, "rollout-"+sid+".jsonl"), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := codexSessionsRootFn
	codexSessionsRootFn = func() string { return root }
	t.Cleanup(func() { codexSessionsRootFn = old })
}

const appSID = "0199a1b2-2222-4c1e-9f1a-000000000002"

// 기록 없는 코덱스 세션: 접두면 rollout에서 전체 ID·cwd를 찾아 큐로, JSON은 via=queue. tmux 키 입력은 0회.
func TestRunSendCodexDirectByPrefix(t *testing.T) {
	calls := stubQueue(t, nil) // 설정·rollout 루트를 먼저 끼우고, 아래에서 rollout을 덮는다
	stubCodexRollouts(t, map[string]string{appSID: "/w/app", "ffff0000-3333-4c1e-9f1a-000000000003": "/w/c"})
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	_ = st.Save(mkAgent("codex", "codex-live", "%2", state.StateIdle)) // 다른 세션의 기록은 방해하지 않는다
	f := &fakeSender{}
	var out bytes.Buffer
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"--json", "0199a1b2", "안녕"}); err != nil {
		t.Fatal(err)
	}
	if f.calls != 0 || len(*calls) != 1 || (*calls)[0] != (queueCall{appSID, "/w/app", "안녕"}) {
		t.Fatalf("큐로 전체 ID·cwd: tmux=%d calls=%v", f.calls, *calls)
	}
	var res map[string]any
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res["via"] != "queue" || res["session_id"] != appSID || res["recorded"] != false || res["cwd"] != "/w/app" || res["sent"] != true {
		t.Errorf("JSON: %s", out.String())
	}
	// 사람이 읽는 출력
	out.Reset()
	if err := RunSend(context.Background(), &out, strings.NewReader("첫 줄\n둘째 줄\n"), st, stateDir, f, []string{"0199a1b2-2222", "-"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "전송 완료 → codex "+appSID+" [기록 없음] (codex queue)") || (*calls)[1].msg != "첫 줄\n둘째 줄" {
		t.Errorf("출력: %s calls=%v", out.String(), *calls)
	}
}

// 전체 UUID면 rollout이 없어도 보낸다. --cwd가 rollout의 cwd보다 우선한다.
func TestRunSendCodexDirectFullIDAndCwd(t *testing.T) {
	calls := stubQueue(t, nil)
	stubCodexRollouts(t, map[string]string{appSID: "/w/app"})
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	f := &fakeSender{}
	var out bytes.Buffer
	unknown := "deadbeef-0000-4c1e-9f1a-000000000009"
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{unknown, "x"}); err != nil {
		t.Fatalf("전체 ID는 rollout 없이도: %v", err)
	}
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"--cwd", "/w/override", appSID, "y"}); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 || (*calls)[0] != (queueCall{unknown, "", "x"}) || (*calls)[1] != (queueCall{appSID, "/w/override", "y"}) {
		t.Errorf("calls=%v", *calls)
	}
}

// 접두가 rollout에 없거나 둘 이상 맞으면 보내지 않고 오류. 큐 실패도 폴백 없이 오류.
func TestRunSendCodexDirectErrors(t *testing.T) {
	calls := stubQueue(t, nil)
	stubCodexRollouts(t, map[string]string{
		"0199a1b2-1111-4c1e-9f1a-000000000001": "/w/a",
		appSID:                                 "/w/app",
	})
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	f := &fakeSender{}
	var out bytes.Buffer
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"0199a1b2", "x"}); err == nil || !strings.Contains(err.Error(), "둘 이상") {
		t.Errorf("모호: %v", err)
	}
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"deadbeef", "x"}); err == nil || !strings.Contains(err.Error(), "찾지 못했습니다") {
		t.Errorf("없음: %v", err)
	}
	if err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"not-a-session", "x"}); err == nil || strings.Contains(err.Error(), "rollout") {
		t.Errorf("세션 ID 형식이 아니면 예전 오류: %v", err)
	}
	if len(*calls) != 0 || f.calls != 0 {
		t.Errorf("보내면 안 됨: queue=%v tmux=%d", *calls, f.calls)
	}
	calls = stubQueue(t, errors.New("Error: no rollout found"))
	stubCodexRollouts(t, map[string]string{appSID: "/w/app"})
	err := RunSend(context.Background(), &out, nil, st, stateDir, f, []string{"0199a1b2-2222", "x"})
	if err == nil || !strings.Contains(err.Error(), "폴백 없음") || !strings.Contains(err.Error(), "no rollout found") || f.calls != 0 || len(*calls) != 1 {
		t.Errorf("큐 실패는 오류: err=%v tmux=%d", err, f.calls)
	}
}

// 설정으로 큐를 끈 경우 기록 없는 세션에는 보낼 길이 없다.
func TestRunSendCodexDirectQueueDisabled(t *testing.T) {
	calls := stubQueue(t, nil) // 설정 경로를 끼우므로 그 뒤에 덮는다
	cfg := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfg, []byte(`{"codex_queue": false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTLAYER_CONFIG", cfg)
	stateDir := t.TempDir()
	st, _ := state.NewStore(stateDir)
	var out bytes.Buffer
	err := RunSend(context.Background(), &out, nil, st, stateDir, &fakeSender{}, []string{appSID, "x"})
	if err == nil || !strings.Contains(err.Error(), "codex_queue") || len(*calls) != 0 {
		t.Errorf("err=%v calls=%v", err, *calls)
	}
}
