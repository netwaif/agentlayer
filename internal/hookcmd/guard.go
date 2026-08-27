package hookcmd

import (
	"path/filepath"
	"strings"
)

// hookPane은 hook이 기록해도 되는 pane ID를 돌려준다. 기본 tmux 서버가
// 아니면 ""(무시)다.
//
// pane ID 공간은 tmux 서버마다 독립인데 상태 저장소는 하나라, 별도
// 소켓(-L/-S) 서버의 hook이 쓰면 본 서버의 같은 번호 레코드를 오염시킨다.
// 실사고: e2e 테스트가 띄운 격리 서버에서 tmux-resurrect가 실세션 사본을
// 부활시켰고, 그 유령 claude들의 hook이 본 서버 봇 레코드의 session_id를
// 덮었다. 스캐너·restore가 기본 서버만 보므로 hook도 기본 서버만 받는다.
// TMUX 없이 TMUX_PANE만 남은 잔류 환경(서버 사망 후 생존 프로세스)도 걸러진다.
func hookPane(env func(string) string) string {
	pane := env("TMUX_PANE")
	if pane == "" {
		return ""
	}
	tmuxVar := env("TMUX") // "<소켓경로>,<서버pid>,<세션idx>"
	sock, _, ok := strings.Cut(tmuxVar, ",")
	if !ok || filepath.Base(sock) != "default" {
		return ""
	}
	return pane
}
