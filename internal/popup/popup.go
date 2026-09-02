// Package popup은 tmux display-popup 안에서 도는 관제탑의 "크기 따라가기"를 맡는다.
// tmux 팝업은 클라이언트가 작아지면 줄지만 커지면 원래대로 안 돌아온다(3.6a 실측).
// 그래서 TUI가 자기 크기·커서를 기록해 두고, client-resized 훅이 `agentlayer
// popup-refresh <client>`를 불러 기록과 기대 크기가 어긋나면 닫고 다시 연다.
package popup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// 팝업 크기 규약 — bind-key 안내문과 재오픈이 같은 값을 쓴다.
const (
	WidthPct  = 90
	HeightPct = 80
	EnvFlag   = "AGENTLAYER_POPUP=1"
	border    = 2 // tmux 팝업 테두리(양쪽 1칸)
)

// Record는 팝업 안 TUI가 남기는 상태.
type Record struct {
	PID    int    `json:"pid"`
	Cols   int    `json:"cols"`
	Rows   int    `json:"rows"`
	Cursor string `json:"cursor,omitempty"` // 선택 에이전트 ID — 재오픈 뒤 복원
}

func path(dir string) string { return filepath.Join(dir, "popup.json") }

func Save(dir string, r Record) {
	b, err := json.Marshal(r)
	if err != nil {
		return
	}
	_ = os.WriteFile(path(dir), b, 0o600)
}

func Load(dir string) (Record, bool) {
	var r Record
	b, err := os.ReadFile(path(dir))
	if err != nil || json.Unmarshal(b, &r) != nil {
		return r, false
	}
	return r, true
}

func Remove(dir string) { _ = os.Remove(path(dir)) }

// InPopup은 이 프로세스가 팝업 바인딩(-e AGENTLAYER_POPUP=1)으로 떴는지.
func InPopup() bool { return os.Getenv("AGENTLAYER_POPUP") == "1" }

// ExpectedInner는 클라이언트 크기에서 팝업 내부(TUI가 보는) 크기.
func ExpectedInner(clientW, clientH int) (cols, rows int) {
	return clientW*WidthPct/100 - border, clientH*HeightPct/100 - border
}

// Mismatch는 기록된 팝업 크기가 지금 클라이언트에 맞지 않는지(1칸 오차 허용).
func Mismatch(r Record, clientW, clientH int) bool {
	c, rw := ExpectedInner(clientW, clientH)
	return abs(r.Cols-c) > 1 || abs(r.Rows-rw) > 1
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// DisplayArgs는 팝업을 여는 tmux 인자 — bind-key 안내문과 동일 규약.
func DisplayArgs(client, bin string) []string {
	return []string{"display-popup", "-c", client, "-E",
		"-w", fmt.Sprintf("%d%%", WidthPct), "-h", fmt.Sprintf("%d%%", HeightPct),
		"-e", EnvFlag, bin}
}

// BindLine은 ~/.tmux.conf에 넣을 bind-key 한 줄.
func BindLine(bin string) string {
	return fmt.Sprintf("bind-key a display-popup -E -w %d%% -h %d%% -e %s \"%s\"", WidthPct, HeightPct, EnvFlag, bin)
}

// HookLine은 클라이언트 리사이즈 때 팝업을 다시 여는 훅 한 줄.
func HookLine(bin string) string {
	return fmt.Sprintf("set-hook -g client-resized 'run-shell -b \"%s popup-refresh #{client_name}\"'", bin)
}

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// Refresh는 팝업이 떠 있고(기록의 pid 생존) 크기가 클라이언트와 어긋나면 닫고
// 다시 연다. 리사이즈는 드래그 중 연발하므로 잠금으로 1개만 돌고, 새 팝업이
// 기록을 갱신할 때까지 잠깐 기다렸다 다시 확인한다(최대 5회).
// tm은 결과를 기다리는 tmux 호출, open은 기다리지 않는 호출 — `display-popup -E`는
// 팝업이 닫힐 때까지 블록되므로 기다리면 잠금을 쥔 채 멈춰 다음 리사이즈를 놓친다(실측).
func Refresh(dir, client, bin string, tm func(args ...string) (string, error), open func(args ...string), sleep func(time.Duration)) {
	lock, err := os.OpenFile(filepath.Join(dir, "popup.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return // 다른 refresh가 처리 중
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	for i := 0; i < 5; i++ {
		rec, ok := Load(dir)
		if !ok || !alive(rec.PID) {
			return
		}
		out, err := tm("display", "-p", "-t", client, "#{client_width} #{client_height}")
		if err != nil {
			return
		}
		f := strings.Fields(out)
		if len(f) != 2 {
			return
		}
		w, _ := strconv.Atoi(f[0])
		h, _ := strconv.Atoi(f[1])
		if w == 0 || h == 0 || !Mismatch(rec, w, h) {
			return
		}
		_, _ = tm("display-popup", "-c", client, "-C")
		open(DisplayArgs(client, bin)...)
		sleep(300 * time.Millisecond)
	}
}
