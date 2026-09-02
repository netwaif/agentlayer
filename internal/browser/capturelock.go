package browser

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// 화면 잠자기 잠금 정리.
//
// chrome-devtools MCP(puppeteer)로 스크린샷을 찍으면 Chrome이 "Capturing"이라는
// NoDisplaySleep 잠금을 걸고, 어떤 경우엔 다음 캡처가 완료될 때까지 풀지 않는다 —
// 실측 2026-09-02: 15:56에 걸린 잠금이 7시간 넘게 남아 모니터가 안 꺼졌다. 같은
// 페이지에 캡처를 한 번 더 완료시키면 풀린다(rod shot으로 실측). 그래서 hook이 돌 때
// 에이전트 브라우저에 그런 잠금이 있으면 웹 탭마다 1×1 캡처를 완료시켜 풀어 준다.

var captureLockRe = regexp.MustCompile(`pid (\d+)\(Google Chrome\): \[[^\]]*\] (\d+):(\d+):(\d+) NoDisplaySleepAssertion named: "Capturing"`)

// ParseCaptureLocks는 `pmset -g assertions` 출력에서 Chrome의 Capturing 잠금을
// pid → 가장 오래된 잠금 나이로 돌려준다.
func ParseCaptureLocks(out []byte) map[int]time.Duration {
	res := map[int]time.Duration{}
	for _, m := range captureLockRe.FindAllStringSubmatch(string(out), -1) {
		pid, _ := strconv.Atoi(m[1])
		h, _ := strconv.Atoi(m[2])
		mi, _ := strconv.Atoi(m[3])
		s, _ := strconv.Atoi(m[4])
		age := time.Duration(h)*time.Hour + time.Duration(mi)*time.Minute + time.Duration(s)*time.Second
		if cur, ok := res[pid]; !ok || age > cur {
			res[pid] = age
		}
	}
	return res
}

// ChromePID는 CDP 포트를 listen 중인 프로세스 pid(에이전트 브라우저). 없으면 0.
func ChromePID(port int, run RunLsof) int {
	out, err := run("-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-Fp")
	if err != nil {
		return 0
	}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "p") {
			pid, _ := strconv.Atoi(l[1:])
			return pid
		}
	}
	return 0
}

// RunPmsetAssertions는 macOS pmset -g assertions 출력.
func RunPmsetAssertions() ([]byte, error) { return exec.Command("pmset", "-g", "assertions").Output() }

// HasCaptureLock은 에이전트 브라우저(port)가 Capturing 잠금을 들고 있는지.
func HasCaptureLock(port int, lsof RunLsof, pmset func() ([]byte, error)) bool {
	pid := ChromePID(port, lsof)
	if pid == 0 {
		return false
	}
	out, err := pmset()
	if err != nil {
		return false
	}
	_, ok := ParseCaptureLocks(out)[pid]
	return ok
}

// ReleaseCaptures는 웹 탭마다 1×1 캡처를 완료시켜 걸려 있던 Capturing 잠금을 푼다.
// 캡처는 읽기 전용이라 페이지에 영향이 없다. 푼 탭 수를 돌려준다.
func ReleaseCaptures(b *rod.Browser) int {
	pages, err := b.Pages()
	if err != nil {
		return 0
	}
	n := 0
	for _, p := range pages {
		info, err := p.Info()
		if err != nil || !IsWebURL(info.URL) {
			continue
		}
		_, err = (proto.PageCaptureScreenshot{
			Format: proto.PageCaptureScreenshotFormatPng, FromSurface: true,
			Clip: &proto.PageViewport{X: 0, Y: 0, Width: 1, Height: 1, Scale: 1},
		}).Call(p.Timeout(5 * time.Second))
		if err == nil {
			n++
		}
	}
	return n
}

// ThrottleOK는 dir/<name>.throttle의 mtime으로 최소 간격을 강제한다 — hook마다 불리는
// 정리 작업이 폭주하지 않게. 통과하면 파일을 갱신하고 true.
func ThrottleOK(dir, name string, min time.Duration, now time.Time) bool {
	path := filepath.Join(dir, name+".throttle")
	if st, err := os.Stat(path); err == nil && now.Sub(st.ModTime()) < min {
		return false
	}
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(path, nil, 0o600)
	_ = os.Chtimes(path, now, now)
	return true
}
