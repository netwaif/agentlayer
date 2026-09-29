package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

// 원격 send도 로컬 send처럼 열어 둔 보드 파일을 바로 다시 쓴다(없으면 만들지 않는다).
func TestSendRemoteRefreshesBoardFile(t *testing.T) {
	ad := &scriptedAdapter{handle: "t_new", status: remote.Status{State: state.StateIdle}}
	st, stateDir, _ := remoteSendFixture(t, ad)
	if err := RunSend(context.Background(), &bytes.Buffer{}, nil, st, stateDir, nil, []string{"hermes-qa", "첫 지시"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(BoardPath(stateDir)); !os.IsNotExist(err) {
		t.Fatal("보드를 연 적 없으면 파일을 만들지 않는다")
	}
	writeFile(t, BoardPath(stateDir), "STALE")
	ad.status = remote.Status{State: state.StateWaiting}
	var out bytes.Buffer
	if err := RunSend(context.Background(), &out, nil, st, stateDir, nil, []string{"--json", "hermes-qa", "답"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(BoardPath(stateDir))
	if string(b) == "STALE" || len(b) == 0 {
		t.Error("원격 send 뒤 보드 파일이 갱신되지 않음")
	}
	var v map[string]any
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Errorf("--json 출력에 다른 글이 섞이면 안 됨: %v: %s", err, out.String())
	}
}
