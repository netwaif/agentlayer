package browser

import (
	"os"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
)

func TestInstanceStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadInstance(dir); err == nil {
		t.Error("파일 없으면 에러")
	}
	if err := SaveInstance(dir, Instance{WSURL: "ws://127.0.0.1:9222/x"}); err != nil {
		t.Fatal(err)
	}
	in, err := LoadInstance(dir)
	if err != nil || in.WSURL != "ws://127.0.0.1:9222/x" {
		t.Fatalf("라운드트립: %+v %v", in, err)
	}
	RemoveInstance(dir)
	if _, err := LoadInstance(dir); err == nil {
		t.Error("Remove 후에는 에러")
	}
}

func TestConnectIdempotentIntegration(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok {
		t.Skip("Chrome 없음")
	}
	SetHeadlessForTest(true)
	defer SetHeadlessForTest(false)
	dir := t.TempDir()
	b1, err := Connect(dir)
	if err != nil {
		t.Fatal(err)
	}
	in1, _ := LoadInstance(dir)
	b2, err := Connect(dir) // 두 번째는 attach여야 한다
	if err != nil {
		t.Fatal(err)
	}
	in2, _ := LoadInstance(dir)
	if in1.WSURL != in2.WSURL {
		t.Error("재기동됨 — attach가 아니다")
	}
	b2.MustClose()
	_ = b1

	// Chrome 프로세스가 프로필 파일 핸들을 잠깐 붙들고 있을 수 있어
	// t.TempDir()의 자동 정리보다 먼저 재시도로 넉넉히 지운다.
	for i := 0; i < 30; i++ {
		if err := os.RemoveAll(dir); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}
