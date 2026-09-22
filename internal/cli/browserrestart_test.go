package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/netwaif/agentlayer/internal/browser"
)

// agentlayer browser restart(2026-09-23): 굳은 브라우저를 사람이 한 마디로 되살리는 길.
// 행 감시와 같은 수단(Kill→Relaunch)을 쓴다.
func TestBrowserRestartKillsThenRelaunches(t *testing.T) {
	var order []string
	ops := browser.HangOps{
		Kill:     func(pid int) error { order = append(order, "kill"); return nil },
		Relaunch: func() error { order = append(order, "relaunch"); return nil },
	}
	var out bytes.Buffer
	if err := browserRestartWith(&out, 4242, ops); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "kill,relaunch" {
		t.Fatalf("순서: %v", order)
	}
	if !strings.Contains(out.String(), "재시작") {
		t.Fatalf("출력: %q", out.String())
	}
}

func TestBrowserRestartWithoutBrowserJustLaunches(t *testing.T) {
	var order []string
	ops := browser.HangOps{
		Kill:     func(pid int) error { order = append(order, "kill"); return nil },
		Relaunch: func() error { order = append(order, "relaunch"); return nil },
	}
	var out bytes.Buffer
	if err := browserRestartWith(&out, 0, ops); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "relaunch" {
		t.Fatalf("브라우저가 없으면 기동만: %v", order)
	}
}

func TestBrowserRestartReportsRelaunchFailure(t *testing.T) {
	ops := browser.HangOps{
		Kill:     func(pid int) error { return nil },
		Relaunch: func() error { return errors.New("포트 사용 중") },
	}
	var out bytes.Buffer
	if err := browserRestartWith(&out, 1, ops); err == nil || !strings.Contains(err.Error(), "포트 사용 중") {
		t.Fatalf("재기동 실패는 에러로: %v", err)
	}
}
