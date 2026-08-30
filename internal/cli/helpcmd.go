package cli

// HelpText는 `agentlayer help`(-h/--help) 출력을 만든다.
// main.go run()의 switch와 명령 목록이 어긋나지 않게 helpcmd_test.go가 감시한다.
func HelpText() string {
	return `agentlayer — iTerm2+tmux 멀티 에이전트 관제탑

사용법: agentlayer [명령] [플래그]
  인자 없이 실행하면 TUI가 뜬다 (tmux 안에서).

명령:
  status         에이전트 목록·상태 출력  [--json]
  info <이름|id> 세션 하나 상세 (모델·ctx·hook 배선 등)
  card           Discord 대시보드 카드 게시  [--out 출력만] [--event 전이 트리거 모드]
  init           hook·tmux 바인딩·스킬 설치 (멱등)  [--dry-run]
  resume [id]    죽은 세션의 대화를 새 window에서 재개 (비상 복구용)
  restore        죽은 세션 배치 부활 — 새 CLI 기동 후 wake-all 권장  [--resume 대화째] [--dry-run] [id ...]
  wake-all       전 세션에 "이어서하자" 전달  [--yes] [--except 이름,..] [--watch] [--timeout 10m]
  close-all      전 세션에 "세션 마감" 전달  (플래그는 wake-all과 동일)
  broadcast <메시지>  전 세션에 임의 메시지 전달
  wt <명령>      worktree 워커 관리 (new·list·diff·test·review·send·merge·clean) — 'agentlayer wt'로 상세
  version        버전 정보 (-v/--version)
  help           이 도움말 (-h/--help)
`
}
