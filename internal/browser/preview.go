// preview: worktree 안에서 도는 dev 서버를 찾아 브랜치 라벨 창으로 연다.
package browser

import (
	"fmt"
	"strconv"
	"strings"

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
	page, err := b.Page(proto.TargetCreateTarget{
		URL: fmt.Sprintf("http://localhost:%d", s.Port), NewWindow: true,
	})
	if err != nil {
		return err
	}
	if err := page.WaitLoad(); err != nil {
		return err
	}
	_, err = page.Eval(`(b) => { document.title = '⎇' + b + ' — ' + document.title }`, s.Branch)
	return err
}
