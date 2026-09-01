package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 전체 페이지 스크린샷이 picks/에 PNG로 저장돼야 한다.
func TestShotSavesPNG(t *testing.T) {
	p := headlessPage(t, `<h1>hello</h1>`)
	dir := t.TempDir()
	now := time.Date(2026, 9, 1, 12, 34, 56, 0, time.Local)
	path, err := Shot(p, dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "picks", "20260901-123456-shot.png"); path != want {
		t.Errorf("경로가 다름: got %q want %q", path, want)
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) < 100 {
		t.Fatalf("PNG 저장돼야: %d바이트 %v", len(b), err)
	}
	if !strings.HasPrefix(string(b), "\x89PNG") {
		t.Errorf("PNG 시그니처가 아님: %x", b[:4])
	}
}
