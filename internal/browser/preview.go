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
	page, err := b.Page(proto.TargetCreateTarget{
		URL: fmt.Sprintf("http://localhost:%d", s.Port), NewWindow: true,
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
	if _, err = page.Eval(`(b) => { document.title = '⎇' + b + ' — ' + document.title }`, s.Branch); err != nil {
		return err
	}
	tilePreviewWindow(b, page)
	return nil
}

// previewCols는 ⎇브랜치 프리뷰 창을 가로로 몇 칸에 나눠 배치할지 — A/B 비교는 보통 2~3개.
const previewCols = 3

// TileBounds는 n번째(0부터) 프리뷰 창의 위치·크기. 가로 previewCols칸, 넘치면 다음 줄.
func TileBounds(availLeft, availTop, availW, availH, n, cols int) (left, top, w, h int) {
	if cols < 1 {
		cols = 1
	}
	w = availW / cols
	rows := 1
	if n >= cols {
		rows = 2
	}
	h = availH / rows
	left = availLeft + (n%cols)*w
	top = availTop + (n/cols)*h
	return
}

// tilePreviewWindow는 ⎇ 제목이 붙은 다른 프리뷰 창 수를 세어 새 창을 빈 칸에 놓는다 —
// worker 3개가 띄운 서버가 겹치지 않고 나란히 뜨게(사람이 창을 옮기지 않아도 비교 가능).
// 실패는 조용히 무시(배치는 보조 기능).
func tilePreviewWindow(b *rod.Browser, page *rod.Page) {
	win, err := proto.BrowserGetWindowForTarget{TargetID: page.TargetID}.Call(b)
	if err != nil {
		return
	}
	others := map[proto.BrowserWindowID]bool{}
	if pages, err := b.Pages(); err == nil {
		for _, p := range pages {
			if p.TargetID == page.TargetID {
				continue
			}
			info, err := p.Info()
			if err != nil || !strings.HasPrefix(info.Title, "⎇") {
				continue
			}
			if w, err := (proto.BrowserGetWindowForTarget{TargetID: p.TargetID}).Call(b); err == nil && w.WindowID != win.WindowID {
				others[w.WindowID] = true
			}
		}
	}
	res, err := page.Eval(`() => [screen.availLeft, screen.availTop, screen.availWidth, screen.availHeight]`)
	if err != nil {
		return
	}
	a := res.Value.Arr()
	if len(a) != 4 {
		return
	}
	left, top, w, h := TileBounds(a[0].Int(), a[1].Int(), a[2].Int(), a[3].Int(), len(others), previewCols)
	_ = proto.BrowserSetWindowBounds{WindowID: win.WindowID, Bounds: &proto.BrowserBounds{
		Left: &left, Top: &top, Width: &w, Height: &h, WindowState: proto.BrowserWindowStateNormal,
	}}.Call(b)
}
