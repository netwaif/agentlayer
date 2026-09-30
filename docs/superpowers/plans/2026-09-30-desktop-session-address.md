# 데스크톱 앱 세션 주소 체계 (tmux pane → 프로세스 PID) 설계 메모

**목표:** Claude Code Desktop(앱)이나 맨 터미널에서 뜬 Claude 세션도 agentlayer의 관제(status·TUI·카드)와
전송(`send`·채널) 대상이 되게 한다. tmux pane 세션은 예전 그대로.

**범위 밖(다음 단계):** 웹 관제탑(로컬 HTTP), 태그·릴리즈, `send --from`, **채널 수신함의 pane 밖 키(아래 "방안"만, 구현 안 함 — 2026-09-30 사용자 지시)**.

## 막힌 점

1. 훅이 TMUX_PANE 없는 세션을 거부했다(`internal/hookcmd/claude.go` "tmux 밖 세션은 관제 대상이 아니다", `guard.go`).
2. 채널 수신함 키가 `inboxes/p<pane>` 하나뿐이었다(`internal/cli/channelcmd.go` `PaneInbox`). 채널 서버는 Claude가 띄운
   MCP 자식이라 환경에서 아는 것은 TMUX_PANE(있을 때)과 부모 PID뿐이고 Claude 세션 ID는 모른다.
3. `send`가 tmux 세션 이름(`ResolveTarget`)으로만 대상을 찾았다.
4. 앱 세션은 tmux 키 입력 폴백이 없다 — 채널·큐가 실패하면 되돌아갈 곳이 없다.

## 결정

- **좌표는 세션 ID가 아니라 에이전트 프로세스 PID** (`<kind>-pid<N>`).
  이유: (a) 훅·채널 서버·`send`가 조율 없이 같은 값을 얻는 유일한 것 — 훅은 `agentlayer ← sh ← claude`, MCP 서버는
  `agentlayer ← claude`, Bash 도구는 `agentlayer ← bash ← claude`라 `Getppid()`는 서로 다르지만 "조상 사슬에서 가장 가까운
  kind 프로세스"(`scan.FindAgentProcess`)는 같다. (b) `/clear`로 세션 ID가 바뀌어도 프로세스와 MCP 서버는 그대로다 —
  세션 ID 수신함이면 서버가 옛 수신함을 보게 된다. (c) 기동 직후 훅이 아직 안 돌았을 때도 서버가 수신함을 잡을 수 있다
  (상태 저장소 조회 불필요). (d) 코덱스 notify 경로는 세션 ID를 주지 않는다.
  세션 ID·`-n` 이름은 레코드 필드(`SessionID`·`Name`)로 남겨 **send 대상 해석과 표시**에 쓴다 — 사용자가 부르는 주소는
  세션 ID(접두)·이름이고, PID는 내부 좌표다. (a)~(d)는 채널 수신함 키를 정할 때도 그대로 적용된다(아래 방안).
- **잔류 TMUX_PANE·별도 tmux 서버 방어는 그대로**: TMUX_PANE이 있는데 기본 서버가 아니면 예전대로 무시. TMUX_PANE이
  아예 없을 때만 tmux 밖 세션으로 잡는다(`hookcmd.locate`).
- **생사 판정**: pane 없는 레코드는 `scan.SyncDetached`가 프로세스 표로 본다. 프로세스가 사라졌거나 그 PID에 다른 종류가
  앉아 있으면(재사용) 레코드를 **지운다**(DEAD 보존 안 함 — restore할 자리가 없다). ps를 못 읽으면(빈 표) 건드리지 않는다.
  `scan.Sync`의 pane 소실 판정은 pane 없는 레코드를 건너뛴다. 세션 ID가 바뀌어도(/clear) 레코드는 하나로 유지된다.
- **`scan.KindFromArgs` basename 비교를 대소문자 구분으로**: 데스크톱 앱 실행 파일이 `/Applications/Claude.app/Contents/MacOS/Claude`
  라 무시하면 앱 자체가 claude 에이전트로 잡혀 (1) 앱 안 CLI가 전부 "중첩 claude"로 무시되고 (2) 조상 탐색이 앱을 집는다.
- **전송 규칙 불변**: Claude는 채널, 코덱스는 큐가 정본. pane 세션은 예전대로 tmux 폴백. 앱 세션은 `deliver`가 `tmuxOK`를
  끄고, 채널·큐가 실패하거나 둘 다 불가능하면 `errNoFallback`으로 끝낸다(키 입력 시도 없음). 채널 수신함 키가 아직 없으므로
  **앱 Claude 세션에는 지금 `send`가 항상 오류**(`canClaudeChannel`이 pane 없는 레코드에 false)이고, 앱 코덱스는 큐로 간다.
- **`send` 대상 해석 순서**: tmux 세션 이름(`:창` 포함) → `-n` 이름 정확 일치 → 세션 ID 접두(4자 이상). 산 레코드가 있으면
  죽은 것은 뺀다. 둘 이상이면 `TargetLabel`(pane: `세션:창(%pane)`, 앱: `이름 (app, 세션 <앞 8자리>)`)을 보이고 거부.
- **표시**: `Agent.Label()`(tmux 세션 → `-n` 이름 → 세션 ID 앞 8자리 → ID), status/TUI SESSION 열 `이름 (app)`, info 카드
  `위치 app — tmux 밖 세션 (pid N)`, 카드 `**이름** (app)`. TUI enter는 이동할 pane이 없다고 안내, 미리보기 없음.

## 방안(미구현) — 채널 수신함의 pane 밖 키

제약: 채널 서버(`agentlayer channel serve --self`)는 Claude가 띄운 MCP 자식이라 환경에서 아는 것은 TMUX_PANE(있을 때)과
부모 PID(`os.Getppid()`)뿐이고 Claude 세션 ID는 모른다. 지금은 TMUX_PANE이 없으면 수신함 없이 passive로 머문다(`RunChannel`).

1. **PID 수신함 `inboxes/pid<N>`(권장)**: 서버가 `scan.FindAgentProcess(LoadProcTable(), os.Getppid(), "claude")`로 자기 Claude
   PID를 얻어 그 수신함을 쥔다. 훅이 레코드에 같은 규칙으로 PID를 남기므로 `send`는 `a.PID`로 같은 경로를 만든다
   (`AgentInbox(stateDir, a)` = pane이면 `p<pane>`, 아니면 `pid<N>`). 상태 저장소 조회가 필요 없어 기동 직후(훅 전)에도 잡히고,
   `/clear`(세션 ID 변경)에도 어긋나지 않으며, 총괄 모드 서버도 같은 자리에서 함께 쥘 수 있다. `purgeStale`은 PID 재사용(재부팅)에
   pane 재사용과 같은 취지로 동작한다. 이 방안은 한 번 구현해 테스트까지 통과시킨 뒤 사용자 지시로 되돌렸다(커밋 5bb2a64 → revert).
   되살릴 때 손댈 곳: `channelcmd.go`(`ProcessInbox`·`AgentInbox`·`selfProcessFn`·`RunChannel`의 `pane` 결정),
   `codexqueue.go`(`canClaudeChannel`·`deliver`가 `AgentInbox`를 쓰게), `claudechannel_test.go`의 `startSelfServer`(pane ""이면 PID 수신함 대기).
2. **세션 ID 수신함 `inboxes/s<session_id>`**: 서버가 부모 PID로 상태 저장소에서 자기 레코드를 찾아 `SessionID`를 얻는다.
   사람이 보는 주소와 폴더 이름이 일치하는 장점이 있지만, (a) 훅이 먼저 돌아야 레코드가 있어 기동 직후 재시도가 필요하고
   (b) `/clear`로 세션 ID가 바뀌면 서버가 옛 수신함을 보게 되어 주기적으로 레코드를 다시 읽어 수신함을 갈아타야 한다.
3. **둘 다**: PID 수신함을 쥐고, 레코드가 생기면 세션 ID 수신함을 심볼릭 링크로 덧붙인다. 잠금·purge는 PID 쪽 하나만.

어느 방안이든 확인 순서: 앱 세션이 개발 채널 플래그 없이 채널 알림을 받는지(못 받으면 수신함 키가 있어도 전달되지 않는다)
→ 훅·서버가 같은 PID를 얻는지 → `send <이름>`이 `(채널)`로 끝나는지.

## 확인 못 한 것(맥 실기 필요)

- 앱에서 연 세션이 개발 채널 플래그(`--dangerously-load-development-channels server:agentlayer`) 없이 채널 알림을 받는지.
  못 받으면 서버는 `HasChannelFlag` 규칙대로 passive로 남고 앱 세션에는 지시가 오류로 끝난다(폴백 없음).
- 앱이 띄운 CLI 프로세스의 ps 명령행이 `claude`로 인식되는지(`scan.KindFromArgs` — basename `claude` 또는
  `@anthropic-ai/claude-code` 경로). `node …/cli.js` 형태면 훅이 조상을 못 찾아 예전처럼 무시된다(레코드 없음).
- 훅이 실제로 `sh -c`를 거쳐 뜨는지와 MCP 서버가 직접 뜨는지 — 수신함 키를 구현할 때 조상 탐색으로 같은 PID가 나와야 한다.
  지금은 `agentlayer status --json`의 `pid`가 앱 세션의 `claude` 프로세스(ps)와 같은지만 본다.
- `-n` 이름이 ps 명령행에 남는지(`claude -n 이름`). `/rename`으로 붙인 이름은 명령행에 없어 잡지 못한다 → 세션 ID 접두로 부른다.
- `/clear` 뒤에도 MCP 서버가 살아남는지 — 수신함 키 방안 1·2의 우열을 가르는 조건.
