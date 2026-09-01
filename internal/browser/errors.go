package browser

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// CollectErrors는 until까지 페이지의 콘솔 error/warning·JS 예외·브라우저 생성
// 로그(Log.entryAdded — 네트워크 404·CORS·mixed content 등)를 모은다.
// 온디맨드 — 상주 감시가 아니다.
func CollectErrors(page *rod.Page, until <-chan struct{}) []string {
	// EachEvent는 호출 시점에 동기로 구독한다 — cancel로 이벤트 채널을 닫아
	// wait()를 풀고 구독 누수를 막는다 (pick.go의 abort 패턴과 동일).
	ctx, cancel := context.WithCancel(page.GetContext())
	defer cancel()
	p := page.Context(ctx)
	var mu sync.Mutex
	var lines []string
	add := func(s string) { mu.Lock(); lines = append(lines, s); mu.Unlock() }
	wait := p.EachEvent(
		func(e *proto.RuntimeExceptionThrown) {
			s := "[exception] " + e.ExceptionDetails.Text
			if e.ExceptionDetails.Exception != nil {
				s += " " + e.ExceptionDetails.Exception.Description
			}
			add(s)
		},
		func(e *proto.RuntimeConsoleAPICalled) {
			if e.Type != proto.RuntimeConsoleAPICalledTypeError && e.Type != proto.RuntimeConsoleAPICalledTypeWarning {
				return
			}
			s := "[console." + string(e.Type) + "]"
			for _, a := range e.Args {
				// 객체 인자(Error 포함)는 CDP가 value를 채우지 않는다 —
				// 예외 분기의 Description 사용과 대칭으로 폴백한다.
				if a.Value.Nil() {
					s += " " + a.Description
				} else {
					s += " " + a.Value.String()
				}
			}
			add(s)
		},
		// 네트워크 404·CORS·mixed content는 콘솔 API가 아니라 Log 도메인으로만
		// 온다. rod의 eachEvent가 구독 시 Log.enable을 자동 호출하므로(browser.go
		// eachEvent — 이벤트 도메인의 <domain>.enable 자동 실행) 명시 enable 불요.
		func(e *proto.LogEntryAdded) {
			if e.Entry.Level != proto.LogLogEntryLevelError && e.Entry.Level != proto.LogLogEntryLevelWarning {
				return
			}
			add("[log." + string(e.Entry.Level) + "] " + e.Entry.Text)
		},
	)
	// wait()가 이벤트를 소비한다 — until 신호 후 cancel로 종료시키고,
	// 콜백 실행이 끝난 것을 확인한 뒤에 결과를 확정한다 (경합 방지).
	consumed := make(chan struct{})
	go func() { wait(); close(consumed) }()
	<-until
	cancel()
	<-consumed
	mu.Lock()
	defer mu.Unlock()
	return lines
}

// SaveErrors는 수집분을 picks/<ts>-errors.txt로 저장하고 경로를 돌려준다.
func SaveErrors(stateDir string, lines []string, now time.Time) (string, error) {
	dir := filepath.Join(stateDir, "picks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, now.Format("20060102-150405")+"-errors.txt")
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if body == "" {
		body = "(수집된 에러 없음)\n"
	}
	return path, os.WriteFile(path, []byte(body), 0o644)
}
