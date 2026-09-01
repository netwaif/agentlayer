package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 수집 구간 동안의 console.error와 JS 예외가 실제 문자열로 잡혀야 한다.
func TestCollectErrorsCapturesConsoleAndException(t *testing.T) {
	p := headlessPage(t, `<body></body>`)
	until := make(chan struct{})
	got := make(chan []string, 1)
	go func() { got <- CollectErrors(p, until) }()
	time.Sleep(300 * time.Millisecond) // 구독 안착
	p.MustEval(`() => { console.error('망함'); setTimeout(() => { throw new Error('예외다') }, 0) }`)
	time.Sleep(500 * time.Millisecond)
	close(until)
	lines := <-got
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"망함", "예외다"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%q 수집돼야: %s", want, joined)
		}
	}
}

// console.log 같은 비에러 호출은 걸러져야 한다.
func TestCollectErrorsIgnoresLog(t *testing.T) {
	p := headlessPage(t, `<body></body>`)
	until := make(chan struct{})
	got := make(chan []string, 1)
	go func() { got <- CollectErrors(p, until) }()
	time.Sleep(300 * time.Millisecond)
	p.MustEval(`() => { console.log('평범'); console.warn('경고다') }`)
	time.Sleep(500 * time.Millisecond)
	close(until)
	joined := strings.Join(<-got, "\n")
	if strings.Contains(joined, "평범") {
		t.Errorf("console.log는 수집 대상이 아님: %s", joined)
	}
	if !strings.Contains(joined, "경고다") {
		t.Errorf("console.warn은 수집돼야: %s", joined)
	}
}

// SaveErrors는 picks/<ts>-errors.txt에 라인들을 개행으로 이어 저장한다.
func TestSaveErrorsWritesFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 1, 10, 30, 0, 0, time.Local)
	path, err := SaveErrors(dir, []string{"[console.error] 망함", "[exception] 예외다"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "picks", "20260901-103000-errors.txt"); path != want {
		t.Errorf("경로 got %q, want %q", path, want)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), "[console.error] 망함\n[exception] 예외다\n"; got != want {
		t.Errorf("내용 got %q, want %q", got, want)
	}
}

// 수집분이 없어도 빈 파일 대신 안내 문구를 남긴다.
func TestSaveErrorsEmpty(t *testing.T) {
	path, err := SaveErrors(t.TempDir(), nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "수집된 에러 없음") {
		t.Errorf("빈 수집 안내가 없다: %q", string(b))
	}
}
