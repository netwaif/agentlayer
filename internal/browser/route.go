package browser

import (
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

	"github.com/netwaif/agentlayer/internal/state"
)

// RunLsof는 lsof 실행 주입점 (테스트 대체용).
type RunLsof func(args ...string) ([]byte, error)

// ExecLsof는 실제 lsof를 부른다.
func ExecLsof(args ...string) ([]byte, error) {
	return exec.Command("lsof", args...).Output()
}

// PortCWD는 localhost PORT를 리스닝하는 프로세스의 작업 폴더를 찾는다.
// dev 서버 → 작업 폴더 → 에이전트 라우팅의 핵심 고리.
func PortCWD(port int, run RunLsof) (string, error) {
	out, err := run("-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-Fp")
	if err != nil || len(out) == 0 {
		return "", fmt.Errorf("포트 %d 리스너 없음", port)
	}
	pid := ""
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "p") {
			pid = l[1:]
			break
		}
	}
	if pid == "" {
		return "", fmt.Errorf("포트 %d pid 파싱 실패", port)
	}
	out, err = run("-a", "-p", pid, "-d", "cwd", "-Fn")
	if err != nil {
		return "", err
	}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "n") {
			return l[1:], nil
		}
	}
	return "", fmt.Errorf("pid %s cwd 파싱 실패", pid)
}

// Candidates는 페이지 URL로 전송 대상 에이전트 후보를 좁힌다.
// localhost면 포트→cwd→최장일치, 아니면 산 에이전트 전원(오버레이에서 선택).
func Candidates(agents []*state.Agent, pageURL string, run RunLsof) []*state.Agent {
	alive := make([]*state.Agent, 0, len(agents))
	for _, a := range agents {
		if a.State != state.StateDead {
			alive = append(alive, a)
		}
	}
	u, err := url.Parse(pageURL)
	if err != nil {
		return alive
	}
	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" {
		return alive
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return alive
	}
	cwd, err := PortCWD(port, run)
	if err != nil {
		return alive
	}
	best, bestLen := []*state.Agent{}, 0
	for _, a := range alive {
		if a.CWD == "" {
			continue
		}
		p := strings.TrimSuffix(a.CWD, "/")
		if cwd != p && !strings.HasPrefix(cwd, p+"/") {
			continue
		}
		if len(p) > bestLen {
			best, bestLen = []*state.Agent{a}, len(p)
		} else if len(p) == bestLen {
			best = append(best, a)
		}
	}
	if len(best) == 0 {
		return alive
	}
	return best
}
