// internal/task/watch_test.go
package task

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writePending(t *testing.T, inbox, name, body string) string {
	t.Helper()
	dir := filepath.Join(inbox, "pending")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func goodReport(id string) string {
	b, _ := json.Marshal(Report{Version: 1, ID: id, TaskID: "T-1", Session: "s", Kind: "claude",
		From: "WORKING", To: "DONE_UNREAD", At: time.Now()})
	return string(b)
}

func TestPollMovesGoodToReceived(t *testing.T) {
	inbox := t.TempDir()
	id := strings.Repeat("a", 32)
	writePending(t, inbox, id+".json", goodReport(id))
	r, ok, err := Poll(inbox)
	if err != nil || !ok || r.ID != id || r.To != "DONE_UNREAD" {
		t.Fatalf("Poll: %+v ok=%v err=%v", r, ok, err)
	}
	if _, err := os.Stat(filepath.Join(inbox, "received", id+".json")); err != nil {
		t.Error("received/로 이동돼야 함")
	}
	if _, ok, _ := Poll(inbox); ok {
		t.Error("두 번째 Poll은 비어 있음")
	}
}

func TestPollQuarantinesBadFiles(t *testing.T) {
	inbox := t.TempDir()
	id := strings.Repeat("b", 32)
	writePending(t, inbox, "broken.json", "{not json")
	writePending(t, inbox, strings.Repeat("c", 32)+".json", `{"version":2,"id":"`+strings.Repeat("c", 32)+`"}`)
	writePending(t, inbox, "mismatch.json", goodReport(id)) // 파일명≠id
	writePending(t, inbox, strings.Repeat("d", 32)+".json", strings.Repeat("x", MaxReportBytes+1))
	target := writePending(t, inbox, "real.txt", goodReport(id))
	os.Symlink(target, filepath.Join(inbox, "pending", id+".json"))
	for i := 0; i < 5; i++ {
		if _, ok, err := Poll(inbox); ok || err != nil {
			t.Fatalf("불량 파일은 ok=false·에러 없음 (%d): ok=%v err=%v", i, ok, err)
		}
	}
	q, _ := os.ReadDir(filepath.Join(inbox, "quarantine"))
	if len(q) != 5 {
		t.Errorf("quarantine 5건이어야 함: %d", len(q))
	}
	if left, _ := filepath.Glob(filepath.Join(inbox, "pending", "*.json")); len(left) != 0 {
		t.Errorf("pending에 json이 남으면 안 됨: %v", left)
	}
}

func TestWatchOnceEmitsAndReturns(t *testing.T) {
	inbox := t.TempDir()
	id := strings.Repeat("e", 32)
	var got []*Report
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		time.Sleep(100 * time.Millisecond)
		writePending(t, inbox, id+".json", goodReport(id))
	}()
	if err := Watch(ctx, inbox, 20*time.Millisecond, true, func(r *Report) { got = append(got, r) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != id {
		t.Fatalf("한 건 수신 후 종료: %+v", got)
	}
}

func TestWatchStopsOnContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	err := Watch(ctx, t.TempDir(), 10*time.Millisecond, false, func(*Report) {})
	if err != context.DeadlineExceeded {
		t.Fatalf("컨텍스트 만료로 종료: %v", err)
	}
}
