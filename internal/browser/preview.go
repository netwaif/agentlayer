// preview: worktree 안에서 도는 dev 서버를 찾아 브랜치 라벨 창으로 연다.
package browser

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// DevServer는 worktree 안에서 리스닝 중인 dev 서버 하나.
type DevServer struct {
	Port        int
	CWD, Branch string
}

// DevServers는 localhost 리스너를 전수 스캔해 worktree 경로 아래 것만 추린다.
// wtPaths는 경로→브랜치 맵. lsof 실패는 조용히 빈 결과로 처리한다.
func DevServers(run RunLsof, wtPaths map[string]string) []DevServer {
	out, err := run("-nP", "-iTCP", "-sTCP:LISTEN", "-Fpn")
	if err != nil {
		return nil
	}
	pid, seen := "", map[string]bool{}
	pids := []string{}          // 발견 순서 보존 — map 순회의 비결정성 회피
	ports := map[string][]int{} // pid → ports
	for _, l := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(l, "p"):
			pid = l[1:]
		case strings.HasPrefix(l, "n"):
			addr := l[1:]
			// IPv6([::1]:3000)도 마지막 콜론 뒤가 포트다.
			i := strings.LastIndex(addr, ":")
			if i < 0 {
				continue
			}
			port, err := strconv.Atoi(addr[i+1:])
			key := pid + ":" + addr[i+1:] // 같은 pid의 IPv4/IPv6 이중 리스닝 중복 제거
			if err == nil && !seen[key] {
				seen[key] = true
				if len(ports[pid]) == 0 {
					pids = append(pids, pid)
				}
				ports[pid] = append(ports[pid], port)
			}
		}
	}
	var res []DevServer
	for _, pid := range pids {
		out, err := run("-a", "-p", pid, "-d", "cwd", "-Fn")
		if err != nil {
			continue // 죽었거나 권한 없는 pid — 건너뛴다
		}
		cwd := ""
		for _, l := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(l, "n") {
				cwd = l[1:]
			}
		}
		for wp, branch := range wtPaths {
			p := strings.TrimSuffix(wp, "/")
			if p == "" {
				continue // 빈/루트 경로(손상된 meta)는 전 경로에 매칭되므로 무시
			}
			if cwd == p || strings.HasPrefix(cwd, p+"/") {
				for _, port := range ports[pid] {
					res = append(res, DevServer{Port: port, CWD: cwd, Branch: branch})
				}
			}
		}
	}
	return res
}

// OpenPreview는 dev 서버를 새 창으로 열고 제목에 ⎇브랜치를 새긴다.
func OpenPreview(b *rod.Browser, s DevServer) error {
	// worktree(⎇브랜치) 프리뷰는 같은 창의 탭으로 — 창 3개가 화면을 덮으면 터미널까지 가려
	// 촬영·비교가 오히려 어렵다(2026-09-02 사용자 판단). 탭 제목의 ⎇로 구분하고, 관제탑
	// b/s는 그 worker의 탭을 앞으로 가져온다. 일반 폴더의 첫 프리뷰만 새 창.
	page, err := b.Page(proto.TargetCreateTarget{
		URL: fmt.Sprintf("http://localhost:%d", s.Port), NewWindow: s.Branch == "",
	})
	if err != nil {
		return err
	}
	if err := page.WaitLoad(); err != nil {
		return err
	}
	if s.Branch == "" {
		return nil // worktree가 아닌 일반 폴더 — 제목은 그대로
	}
	return MarkBranch(page, s.Branch)
}

// IsHTMLServer는 포트가 진짜 웹 화면(dev 서버)인지 HTTP로 확인한다 — text/html이고 4xx/5xx가
// 아니어야 한다. 에이전트 CLI(agy 등)가 여는 내부 포트는 404 text/plain이라 걸러진다
// (실측 2026-09-03: gemini worker의 51871이 프리뷰로 열렸다).
func IsHTMLServer(port int) bool {
	c := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 400 && strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html")
}

// FilterHTML은 dev 서버 후보 중 IsHTMLServer(또는 주입한 probe)를 통과한 것만 남긴다.
func FilterHTML(servers []DevServer, probe func(port int) bool) []DevServer {
	if probe == nil {
		probe = IsHTMLServer
	}
	var out []DevServer
	for _, s := range servers {
		if probe(s.Port) {
			out = append(out, s)
		}
	}
	return out
}

// MarkBranch는 탭 제목에 ⎇브랜치를 붙이고, 리로드·이동 뒤에도 유지되게 새 문서마다
// 다시 붙이는 스크립트를 심는다(Page.addScriptToEvaluateOnNewDocument — 탭 수명 동안 유효).
func MarkBranch(page *rod.Page, branch string) error {
	if branch == "" {
		return nil
	}
	fn := fmt.Sprintf(`() => { const b = %q; const mark = () => { if (!document.title.startsWith('⎇')) document.title = '⎇' + b + ' — ' + document.title; };
		if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', mark); else mark(); }`, branch)
	if _, err := (proto.PageAddScriptToEvaluateOnNewDocument{Source: "(" + fn + ")()"}).Call(page); err != nil {
		return err
	}
	_, err := page.Eval(fn) // rod Eval은 함수식을 받는다
	return err
}
