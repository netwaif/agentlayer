package popup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExpectedInner(t *testing.T) {
	// tmux 실측: 100x30 클라이언트, -w 90% -h 80% → 팝업 내부 88x22 (테두리 2 제외)
	c, r := ExpectedInner(100, 30)
	if c != 88 || r != 22 {
		t.Errorf("got %dx%d, want 88x22", c, r)
	}
}

func TestMismatch(t *testing.T) {
	rec := Record{Cols: 88, Rows: 22}
	if Mismatch(rec, 100, 30) {
		t.Error("맞는 크기면 false")
	}
	if Mismatch(rec, 101, 30) {
		t.Error("1칸 오차는 허용")
	}
	if !Mismatch(rec, 200, 50) {
		t.Error("클라이언트가 커졌으면 true")
	}
}

func TestRecordRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, ok := Load(dir); ok {
		t.Error("없으면 ok=false")
	}
	Save(dir, Record{PID: 42, Cols: 88, Rows: 22, Cursor: "claude-7"})
	rec, ok := Load(dir)
	if !ok || rec.PID != 42 || rec.Cursor != "claude-7" || rec.Cols != 88 {
		t.Fatalf("라운드트립: %+v %v", rec, ok)
	}
	Remove(dir)
	if _, err := os.Stat(filepath.Join(dir, "popup.json")); err == nil {
		t.Error("Remove 후 파일 없어야")
	}
}

func TestDisplayArgs(t *testing.T) {
	args := strings.Join(DisplayArgs("/dev/ttys001", "/opt/agentlayer"), " ")
	for _, want := range []string{"display-popup", "-c /dev/ttys001", "-E", "-w 90%", "-h 80%", "-e AGENTLAYER_POPUP=1", "/opt/agentlayer"} {
		if !strings.Contains(args, want) {
			t.Errorf("%q 누락: %s", want, args)
		}
	}
}

// 클라이언트가 커졌고 팝업(살아 있는 pid)이 옛 크기면 닫고 다시 연다.
func TestRefreshReopensWhenClientGrew(t *testing.T) {
	dir := t.TempDir()
	Save(dir, Record{PID: os.Getpid(), Cols: 88, Rows: 22})
	var calls []string
	tm := func(args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		if args[0] == "display" {
			return "200 50\n", nil
		}
		return "", nil
	}
	open := func(args ...string) { // 비동기 재오픈 — 새 팝업이 기록을 갱신
		calls = append(calls, strings.Join(args, " "))
		Save(dir, Record{PID: os.Getpid(), Cols: 178, Rows: 38})
	}
	Refresh(dir, "/dev/ttys001", "/opt/agentlayer", tm, open, func(time.Duration) {})
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "display-popup -c /dev/ttys001 -C") {
		t.Errorf("닫기 호출 없음:\n%s", joined)
	}
	if !strings.Contains(joined, "-e AGENTLAYER_POPUP=1 /opt/agentlayer") {
		t.Errorf("재오픈 호출 없음:\n%s", joined)
	}
	if strings.Count(joined, "-C") != 1 {
		t.Errorf("크기가 맞은 뒤에는 멈춰야 함:\n%s", joined)
	}
}

func TestRefreshNoopWhenNoPopupOrFits(t *testing.T) {
	dir := t.TempDir()
	var calls []string
	tm := func(args ...string) (string, error) { calls = append(calls, args[0]); return "100 30\n", nil }
	open := func(args ...string) { calls = append(calls, args[0]) }
	Refresh(dir, "c", "/x", tm, open, func(time.Duration) {}) // 기록 없음
	Save(dir, Record{PID: 999999, Cols: 10, Rows: 5})         // 죽은 pid
	Refresh(dir, "c", "/x", tm, open, func(time.Duration) {})
	Save(dir, Record{PID: os.Getpid(), Cols: 88, Rows: 22}) // 크기 맞음
	Refresh(dir, "c", "/x", tm, open, func(time.Duration) {})
	for _, c := range calls {
		if c == "display-popup" {
			t.Fatalf("재오픈하면 안 되는 경우에 display-popup 호출: %v", calls)
		}
	}
}
