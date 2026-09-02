package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// autoPreviewThrottle은 hook마다 불리는 스캔의 최소 간격 — lsof 전수 스캔 폭주 방지.
const autoPreviewThrottle = 5 * time.Second

type previewSeen struct {
	LastScan time.Time            `json:"last_scan"`
	Seen     map[string]time.Time `json:"seen"` // "cwd:port" → 처음 본 시각
}

func previewSeenPath(dir string) string { return filepath.Join(dir, "preview-seen.json") }

func loadPreviewSeen(dir string) previewSeen {
	var s previewSeen
	if b, err := os.ReadFile(previewSeenPath(dir)); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	if s.Seen == nil {
		s.Seen = map[string]time.Time{}
	}
	return s
}

func savePreviewSeen(dir string, s previewSeen) {
	if b, err := json.Marshal(s); err == nil {
		_ = os.WriteFile(previewSeenPath(dir), b, 0o600)
	}
}

// AutoPreview는 paths(에이전트 폴더→브랜치) 아래 dev 서버를 스캔해 처음 보는 것을
// 전용 브라우저에 한 번 연다. 상태는 파일(preview-seen.json)에 남겨 짧게 사는
// hook 프로세스들 사이에서도 "한 번만"이 지켜진다. 사라진 서버는 기록에서 지워
// 재시작하면 다시 열리고, 이미 탭이 있으면 열지 않되 본 것으로 친다(사용자가
// 닫은 탭을 되살리지 않기 위해). 5초 안의 재호출은 스캔 없이 돌아간다.
func AutoPreview(dir string, paths map[string]string, scan func(map[string]string) []DevServer,
	hasTab func(port int) bool, open func(DevServer) error, now time.Time) []DevServer {

	st := loadPreviewSeen(dir)
	if now.Sub(st.LastScan) < autoPreviewThrottle {
		return nil
	}
	st.LastScan = now
	servers := scan(paths)
	alive := map[string]bool{}
	var opened []DevServer
	for _, s := range servers {
		key := s.CWD + ":" + strconv.Itoa(s.Port)
		alive[key] = true
		if _, seen := st.Seen[key]; seen {
			continue
		}
		st.Seen[key] = now
		if hasTab(s.Port) {
			continue
		}
		if err := open(s); err == nil {
			opened = append(opened, s)
		}
	}
	// 이번 스캔 범위(paths) 안의 서버 중 사라진 것만 잊는다
	for key := range st.Seen {
		if alive[key] {
			continue
		}
		for p := range paths {
			p = strings.TrimSuffix(p, "/")
			if p != "" && (strings.HasPrefix(key, p+":") || strings.HasPrefix(key, p+"/")) {
				delete(st.Seen, key)
				break
			}
		}
	}
	savePreviewSeen(dir, st)
	return opened
}
