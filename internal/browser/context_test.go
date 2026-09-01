package browser

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestSavePickWritesFiles(t *testing.T) {
	dir := t.TempDir()
	c := PickContext{URL: "http://localhost:3000/", Selector: "div.card > button",
		HTML: "<button>저장</button>", Instruction: "파랗게", Shot: []byte{1, 2}}
	md, png, err := SavePick(dir, c, time.Date(2026, 9, 1, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(md)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"div.card > button", "<button>저장</button>", "http://localhost:3000/"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("맥락 파일에 %q 없음", want)
		}
	}
	if fi, err := os.Stat(png); err != nil || fi.Size() != 2 {
		t.Error("스크린샷 파일 저장돼야")
	}
}

func TestSavePickNoShot(t *testing.T) {
	_, png, err := SavePick(t.TempDir(), PickContext{Selector: "p"}, time.Now())
	if err != nil || png != "" {
		t.Fatalf("샷 없으면 png 경로 빈 값: %q %v", png, err)
	}
}

func TestPromptLineSingleLine(t *testing.T) {
	got := PromptLine("이 버튼\n파랗게", "/tmp/a.md", "/tmp/a.png")
	if strings.Contains(got, "\n") {
		t.Error("한 줄이어야 — 여러 줄 send-keys는 조기 제출됨")
	}
	for _, want := range []string{"이 버튼 파랗게", "/tmp/a.md", "/tmp/a.png"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q 포함해야: %q", want, got)
		}
	}
}
