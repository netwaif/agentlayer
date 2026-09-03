package browser

import (
	"net/http"
	"net/http/httptest"
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

// 객체 인자는 CDP가 value를 채우지 않는다 — Description 폴백으로
// console.error(new Error(...))가 무정보(<nil>)로 덤프되면 안 된다.
func TestCollectErrorsObjectArgFallsBackToDescription(t *testing.T) {
	p := headlessPage(t, `<body></body>`)
	until := make(chan struct{})
	got := make(chan []string, 1)
	go func() { got <- CollectErrors(p, until) }()
	time.Sleep(300 * time.Millisecond)
	p.MustEval(`() => { console.error(new Error('객체에러')) }`)
	time.Sleep(500 * time.Millisecond)
	close(until)
	joined := strings.Join(<-got, "\n")
	if !strings.Contains(joined, "객체에러") {
		t.Errorf("객체 인자의 Description이 수집돼야: %s", joined)
	}
	if strings.Contains(joined, "<nil>") {
		t.Errorf("<nil> 무정보 덤프 금지: %s", joined)
	}
}

// 네트워크 404 같은 브라우저 생성 로그는 Log.entryAdded로만 온다 —
// 구독이 빠지면 콘솔 API 수집만으로는 잡히지 않는다.
func TestCollectErrorsCapturesLogEntry(t *testing.T) {
	// 페이지 자체를 같은 서버에서 서빙 — data: 오리진에서 loopback 접근 시
	// CORS/PNA 차단으로 문구가 흔들리는 환경 의존을 피하고 순수 404를 만든다.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<body></body>`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	p := headlessPage(t, `<body></body>`)
	p.MustNavigate(srv.URL).MustWaitLoad()
	until := make(chan struct{})
	got := make(chan []string, 1)
	go func() { got <- CollectErrors(p, until) }()
	time.Sleep(300 * time.Millisecond) // 구독 안착
	// 같은 오리진의 404 리소스 로드 실패 → Log.entryAdded(level=error) 발화
	p.MustEval(`() => { const i = document.createElement('img'); i.src = '/missing.png'; document.body.appendChild(i) }`)
	time.Sleep(500 * time.Millisecond)
	close(until)
	joined := strings.Join(<-got, "\n")
	if !strings.Contains(joined, "[log.error]") || !strings.Contains(joined, "404") {
		t.Errorf("404 리소스 실패가 [log.error]로 수집돼야: %s", joined)
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

// --reload 경로: 구독 뒤 리로드해 로드 시점 예외·404를 사람 개입 없이 모은다.
func TestCollectErrorsReloadCapturesLoadTimeErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<body><script>fetch('./stat.json'); document.querySelector('.nope').x = 1</script></body>`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	p := headlessPage(t, `<body></body>`)
	p.MustNavigate(srv.URL).MustWaitLoad()
	lines := CollectErrorsReload(p, 1500*time.Millisecond)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"TypeError", "404", "/stat.json"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%q 수집돼야: %s", want, joined)
		}
	}
}
