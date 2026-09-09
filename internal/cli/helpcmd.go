package cli

import "runtime"

// TerminalLabel은 도움말 첫 줄의 터미널 이름. iTerm2 링크 라우팅은 macOS에만 있다.
func TerminalLabel(goos string) string {
	if goos == "darwin" {
		return "iTerm2+tmux"
	}
	return "tmux"
}

// HelpText는 `agentlayer help`(-h/--help) 출력을 만든다.
// main.go run()의 switch와 명령 목록이 어긋나지 않게 helpcmd_test.go가 감시한다.
func HelpText() string { return helpText(runtime.GOOS) }

func helpText(goos string) string {
	return "agentlayer — " + TerminalLabel(goos) + ` 멀티 에이전트 관제탑

사용법: agentlayer [명령] [플래그]
  인자 없이 실행하면 TUI가 뜬다 (tmux 안에서).

명령:
  status         에이전트 목록·상태 출력  [--json]
  info <이름|id> 세션 하나 상세 (모델·ctx·hook 배선 등)
  card           Discord 대시보드 카드 게시  [--out 출력만] [--event 전이 트리거 모드]
  init           hook·tmux 바인딩·스킬 설치 (멱등)  [--dry-run]
  resume [id]    죽은 세션의 대화를 새 window에서 재개 (비상 복구용)
  restore        죽은 세션 배치 부활 — 체크리스트로 골라 실행  [--resume 대화째] [--yes 전부] [--dry-run] [id ...]  (자동 기동 봇 제외)
  wake-all       전 세션에 "이어서하자" 전달  [--yes] [--except 이름,..] [--watch] [--timeout 10m]
  close-all      전 세션에 "세션 마감" 전달  (플래그는 wake-all과 동일)
  broadcast <메시지>  전 세션에 임의 메시지 전달
  wt <명령>      worktree 워커 관리 (new·list·diff·test·review·send·merge·clean) — 'agentlayer wt'로 상세
  browser [open|pick|shot|errors|preview|cookies|mcp]  에이전트 전용 브라우저 (기동/탭 열기/요소찍기/캡처/콘솔에러/wt 프리뷰/쿠키 import·list·clear·export/MCP 연동 명령)
  version        버전 정보 (-v/--version)
  help           이 도움말 (-h/--help)
`
}
