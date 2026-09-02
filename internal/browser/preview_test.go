package browser

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// worktree 경로 아래에서 리스닝하는 dev 서버만 추려야 한다.
func TestDevServersFiltersByWorktree(t *testing.T) {
	run := func(args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "cwd" { // pid별 cwd 질의
				if args[2] == "11" {
					return []byte("p11\nfcwd\nn/Users/x/wts/feat-a\n"), nil
				}
				return []byte("p22\nfcwd\nn/Users/x/elsewhere\n"), nil
			}
		}
		// 전체 리스너 스캔: pid 11은 3000, pid 22는 9999
		return []byte("p11\nf3\nn*:3000\np22\nf4\nn127.0.0.1:9999\n"), nil
	}
	wts := map[string]string{"/Users/x/wts/feat-a": "feat-a"}
	got := DevServers(run, wts)
	if len(got) != 1 || got[0].Port != 3000 || got[0].Branch != "feat-a" {
		t.Fatalf("worktree 아래 dev 서버만: %+v", got)
	}
	if got[0].CWD != "/Users/x/wts/feat-a" {
		t.Errorf("CWD가 실제 작업 폴더여야: %+v", got[0])
	}
}

// IPv6 주소([::1]:3000)도 포트를 파싱하고, 같은 pid의 같은 포트가
// IPv4/IPv6로 중복 나열돼도 한 번만 세야 한다. 같은 pid의 다른 포트는 모두.
func TestDevServersIPv6AndDedup(t *testing.T) {
	run := func(args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "cwd" {
				return []byte("p11\nfcwd\nn/wts/feat-a\n"), nil
			}
		}
		// 3000은 IPv4·IPv6 이중 리스닝(중복 1회), 5173은 추가 포트
		return []byte("p11\nf3\nn*:3000\np11\nf4\nn[::1]:3000\np11\nf5\nn127.0.0.1:5173\n"), nil
	}
	got := DevServers(run, map[string]string{"/wts/feat-a": "feat-a"})
	if len(got) != 2 {
		t.Fatalf("중복 제거 후 포트 2개여야: %+v", got)
	}
	ports := map[int]bool{}
	for _, s := range got {
		ports[s.Port] = true
	}
	if !ports[3000] || !ports[5173] {
		t.Errorf("3000·5173 둘 다 있어야: %+v", got)
	}
}

// worktree 경로에 뒤따르는 /가 있어도 매칭돼야 하고, 접두사가 겹치는
// 다른 폴더(feat-a-extra)는 오매칭하면 안 된다.
func TestDevServersPathBoundary(t *testing.T) {
	run := func(args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "cwd" {
				if args[2] == "11" {
					return []byte("p11\nfcwd\nn/wts/feat-a/sub\n"), nil
				}
				return []byte("p22\nfcwd\nn/wts/feat-a-extra\n"), nil
			}
		}
		return []byte("p11\nf3\nn*:3000\np22\nf4\nn*:4000\n"), nil
	}
	got := DevServers(run, map[string]string{"/wts/feat-a/": "feat-a"})
	if len(got) != 1 || got[0].Port != 3000 {
		t.Fatalf("경계 매칭 실패: %+v", got)
	}
}

// 빈 경로("")·루트("/") 키는 손상된 meta의 흔적 — TrimSuffix 후 ""가 되어
// 모든 절대경로에 접두사 매칭돼 버리므로 무시해야 한다.
func TestDevServersIgnoresEmptyAndRootPath(t *testing.T) {
	run := func(args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "cwd" {
				return []byte("p11\nfcwd\nn/Users/x/anywhere\n"), nil
			}
		}
		return []byte("p11\nf3\nn*:3000\n"), nil
	}
	wts := map[string]string{"": "broken-empty", "/": "broken-root"}
	if got := DevServers(run, wts); len(got) != 0 {
		t.Fatalf("빈/루트 worktree 경로는 매칭되면 안 됨: %+v", got)
	}
}

// lsof 실패(리스너 0개 포함)면 조용히 빈 결과.
func TestDevServersLsofError(t *testing.T) {
	run := func(args ...string) ([]byte, error) { return nil, fmt.Errorf("lsof 실패") }
	if got := DevServers(run, map[string]string{"/wts/a": "a"}); got != nil {
		t.Fatalf("에러면 nil이어야: %+v", got)
	}
}

// cwd 질의가 실패한 pid는 건너뛴다.
func TestDevServersCWDErrorSkipsPid(t *testing.T) {
	run := func(args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "cwd" {
				return nil, fmt.Errorf("죽은 pid")
			}
		}
		return []byte("p11\nf3\nn*:3000\n"), nil
	}
	if got := DevServers(run, map[string]string{"/wts/a": "a"}); len(got) != 0 {
		t.Fatalf("cwd 실패 pid는 제외: %+v", got)
	}
}

// OpenPreview는 dev 서버를 새 창으로 열고 제목에 ⎇브랜치를 새겨야 한다.
func TestOpenPreviewLabelsTitle(t *testing.T) {
	bin, ok := launcher.LookPath()
	if !ok {
		t.Skip("Chrome 없음")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>데브서버</title></head><body>ok</body></html>`)
	}))
	defer srv.Close()
	_, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	u := launcher.New().Bin(bin).Headless(true).MustLaunch()
	b := rod.New().ControlURL(u).MustConnect()
	t.Cleanup(func() { b.MustClose() })

	if err := OpenPreview(b, DevServer{Port: port, Branch: "feat-a"}); err != nil {
		t.Fatal(err)
	}
	pages, err := b.Pages()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range pages {
		info, err := p.Info()
		if err != nil || !strings.Contains(info.URL, ":"+portStr) {
			continue
		}
		found = true
		if want := "⎇feat-a — 데브서버"; info.Title != want {
			t.Errorf("제목 라벨: got %q want %q", info.Title, want)
		}
	}
	if !found {
		t.Fatalf("포트 %d 페이지가 열려야 함", port)
	}
}

func TestFilterHTMLDropsNonHTML(t *testing.T) {
	in := []DevServer{{Port: 8101}, {Port: 51871}, {Port: 8102}}
	got := FilterHTML(in, func(port int) bool { return port != 51871 })
	if len(got) != 2 || got[0].Port != 8101 || got[1].Port != 8102 {
		t.Errorf("HTML 아닌 포트는 빠져야 함: %v", got)
	}
}

// ⎇ 제목은 리로드 뒤에도 남아야 한다 — worker가 자기 탭을 리로드해도 구분이 유지되게.
func TestMarkBranchSurvivesReload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<title>앱</title><h1>x</h1>`))
	}))
	defer srv.Close()
	p := headlessPage(t, `<p>init</p>`)
	p.MustNavigate(srv.URL).MustWaitLoad()
	if err := MarkBranch(p, "agent/hero"); err != nil {
		t.Fatal(err)
	}
	if ti := p.MustInfo().Title; !strings.HasPrefix(ti, "⎇agent/hero") {
		t.Fatalf("즉시 붙어야 함: %q", ti)
	}
	p.Timeout(15 * time.Second).MustNavigate(srv.URL).MustWaitLoad()
	if ti := p.MustEval(`() => document.title`).Str(); !strings.HasPrefix(ti, "⎇agent/hero") {
		t.Errorf("다시 로드한 뒤에도 남아야 함: %q", ti)
	}
}
