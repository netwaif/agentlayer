package browser

import (
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func cleanupProfile(dir string) {
	for i := 0; i < 30; i++ {
		if err := os.RemoveAll(dir); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// 기록(browser.json)이 사라져도 고정 포트 프로브로 같은 브라우저에 attach해야 한다.
func TestConnectProbesFixedPortIntegration(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok {
		t.Skip("Chrome 없음")
	}
	SetHeadlessForTest(true)
	defer SetHeadlessForTest(false)
	dir := t.TempDir()
	port := freePort(t)
	b1, err := Connect(dir, port)
	if err != nil {
		t.Fatal(err)
	}
	in1, _ := LoadInstance(dir)
	if !strings.Contains(in1.WSURL, fmt.Sprintf(":%d/", port)) {
		t.Fatalf("고정 포트로 안 떴다: %s", in1.WSURL)
	}
	RemoveInstance(dir) // 재부팅 뒤 낡은 기록이 지워진 상황
	b2, err := Connect(dir, port)
	if err != nil {
		t.Fatal(err)
	}
	in2, _ := LoadInstance(dir)
	if in1.WSURL != in2.WSURL {
		t.Errorf("프로브 attach가 아니라 재기동됨: %s vs %s", in1.WSURL, in2.WSURL)
	}
	b2.MustClose()
	_ = b1
	cleanupProfile(dir)
}

// 포트를 CDP가 아닌 프로세스가 물고 있으면 명시적으로 실패해야 한다.
func TestConnectPortOccupied(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	_, err = Connect(t.TempDir(), port)
	if err == nil || !strings.Contains(err.Error(), "사용 중") {
		t.Fatalf("점유 포트 에러 기대, got %v", err)
	}
}

// rod의 기본 기기 에뮬레이션(1280×800)이 걸리면 창 크기를 바꿔도 뷰포트가
// 따라오지 않는다 — 사람이 보는 창 아래가 회색으로 비는 원인. Connect가
// 돌려주는 브라우저의 페이지는 에뮬레이션 없이 창 크기를 그대로 따라야 한다.
func TestConnectPagesFollowWindowSizeIntegration(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok {
		t.Skip("Chrome 없음")
	}
	SetHeadlessForTest(true)
	defer SetHeadlessForTest(false)
	dir := t.TempDir()
	b, err := Connect(dir, freePort(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { b.MustClose(); cleanupProfile(dir) }()
	page, err := b.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		t.Fatal(err)
	}
	w, err := proto.BrowserGetWindowForTarget{TargetID: page.TargetID}.Call(b)
	if err != nil {
		t.Fatal(err)
	}
	width, height := 900, 650
	if err := (proto.BrowserSetWindowBounds{WindowID: w.WindowID,
		Bounds: &proto.BrowserBounds{Width: &width, Height: &height}}).Call(b); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	v, err := page.Eval(`() => [innerWidth, innerHeight]`)
	if err != nil {
		t.Fatal(err)
	}
	got := v.Value.Arr()
	if got[0].Int() != width {
		t.Errorf("innerWidth = %v, want %d (기기 에뮬레이션에 묶임)", got[0], width)
	}
}

func TestCheckPortFreeIsNil(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	if err := CheckPort(port); err != nil {
		t.Fatalf("빈 포트는 nil이어야 함: %v", err)
	}
}
