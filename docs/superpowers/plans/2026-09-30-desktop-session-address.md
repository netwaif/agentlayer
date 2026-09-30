# 데스크톱 앱 세션 주소 (tmux 밖 세션) — 설계 메모

**원래 목표:** Claude Code Desktop 앱 안에서 연 Claude 세션도 agentlayer의 관제(status·TUI)와 전송(`send`·채널) 대상이 되게
주소 체계를 tmux pane 밖으로 넓힌다(SESSION.md 다음 단계 0번 ⑤).

**2026-09-30 범위 변경(맥 실기 결과):** 앱에서 새로 연 Claude 세션은 **채널도 세션 간 메시지도 받지 못한다** — 앱이
`--dangerously-load-development-channels`(채널 플래그)를 넘기지 않고, 세션 간 메시지도 앱 화면에 나타나지 않는다. 기록·주소화해도
지시를 넣을 길이 없으므로 앱 안의 Claude 노드는 **"tmux 세션을 리모트 컨트롤(`--remote-control`)로 보는 창"**으로 정하고,
tmux 밖 Claude 세션을 기록·주소화하는 작업(아래 1·2번)은 **보류**한다. 남긴 것은 **코덱스 앱 세션에 세션 ID로 바로 보내기**(3번)다.

## 보류 — tmux 밖 Claude 세션 기록·주소화 (1·2번)

- **보류 사유:** 위 실기 결과. 레코드가 생겨도 `send`가 닿을 경로(채널)가 없다. tmux 키 입력 폴백도 없다.
- **만들어 둔 코드:** 브랜치 `desktop-sessions-hold`(origin에 푸시, PR에서는 제외). 커밋 b502d3f(훅이 TMUX_PANE 없이 조상 사슬의
  에이전트 프로세스 PID로 `<kind>-pid<N>` 레코드 기록, `state.Agent.Name`·`Label()`·`Where()`, `scan.SyncDetached`가 프로세스 표로
  생사 판정, `scan.KindFromArgs` basename 대소문자 구분) → 5bb2a64(채널 서버가 tmux 밖이면 `inboxes/pid<N>` 수신함, 이후 되돌림)
  → b579992(`send` 대상을 `-n` 이름·세션 ID 접두로도, pane 없으면 폴백 없이 오류) → 474896b(status/TUI `이름 (app)` 표시) → 문서.
  테스트까지 통과한 상태다.
- **되살릴 때 알아둘 것:**
  - 좌표는 세션 ID가 아니라 에이전트 프로세스 PID로 잡았다. 훅(`agentlayer ← sh ← claude`)·MCP 서버(`agentlayer ← claude`)·Bash
    도구(`agentlayer ← bash ← claude`)의 `Getppid()`는 서로 다르지만 "가장 가까운 kind 조상"은 같고, `/clear`로 세션 ID가 바뀌어도
    프로세스는 그대로이며, 코덱스 notify 경로는 세션 ID를 주지 않는다.
  - 데스크톱 앱 실행 파일이 `/Applications/Claude.app/Contents/MacOS/Claude`라 `KindFromArgs`가 대소문자를 무시하면 앱 자체가 claude
    에이전트로 잡혀 앱 안 CLI가 전부 "중첩 claude"로 무시된다 — 이 수정(대소문자 구분)은 앱 안에서 tmux 세션을 `--remote-control`로
    볼 때 훅에도 영향이 있을 수 있으니, 그 경로에서 `NestedClaude`가 오판하는지 맥에서 확인할 가치가 있다.
  - 선결 조건: 앱 세션이 채널(또는 다른 입력 경로)을 받게 되는 것. 그때 채널 수신함 키는 PID 수신함(`inboxes/pid<N>`)이 권장 —
    상태 저장소 조회 없이 서버가 잡을 수 있고 `/clear`에도 어긋나지 않는다. 세션 ID 수신함은 훅이 먼저 돌아야 하고 `/clear`에
    갈아타야 한다.

## 결정 — 코덱스 앱 세션은 세션 ID로 바로 보낸다 (3번, 구현)

- 코덱스 데스크톱 앱 세션은 훅도 tmux도 없어 상태 저장소에 레코드가 없다. 그러나 rollout(`~/.codex/sessions`, `usage.CodexSessionsRoot`)은
  남기고 `codex queue --thread <ID>`는 떠 있는 어떤 세션에든 들어간다.
- `agentlayer send <세션 ID> <메시지>`: 레코드에서 못 찾고(`ResolveTarget`) 인자가 세션 ID 형식(`LooksLikeSessionID` — UUID 전체 또는
  16진수·하이픈 앞자리 접두 8자 이상)이면 `sendCodexDirect`가 큐로 바로 보낸다.
  - 접두면 rollout에서 전체 ID·작업 폴더를 찾는다(`usage.CodexSessionByPrefix`, 최근 400개). 둘 이상이면 후보를 보이고 거부, 없으면 오류.
  - 전체 UUID면 rollout이 없어도 그대로 보낸다(작업 폴더는 `--cwd`, 없으면 rollout, 그것도 없으면 빈 값 = 현재 폴더).
  - `--cwd <폴더>`가 rollout의 cwd보다 우선한다.
  - 성공 시 `via=queue`, JSON 출력 유지(`session`·`session_id`·`kind`·`cwd`·`recorded:false`·`state:""`). 실패 시 오류(폴백 없음).
  - 상태를 모르니 관문(작업 중·승인 대기)은 없다. 큐는 현재 턴 뒤에 처리되고 승인창이 떠 있으면 그 뒤에 처리된다.
  - 설정 `codex_queue: false`면 기록 없는 세션에는 보낼 길이 없어 오류.
- 훅이 세션 ID를 남긴 tmux 코덱스도 세션 ID 접두로 찾을 수 있다(`ResolveTarget` 확장) — 이 경우는 예전 경로(관문·큐·tmux 폴백)다.
  산 레코드 우선, 둘 이상이면 후보를 보이고 거부.
- 전송 규칙은 그대로: Claude는 채널, 코덱스는 큐가 정본, tmux 키 입력은 폴백. 기록 없는 세션은 tmux 좌표가 없으니 폴백이 없다.

## 확인 못 한 것(맥 실기 필요)

- 코덱스 데스크톱 앱 세션이 CLI와 같은 rollout 폴더(`~/.codex/sessions`)를 쓰는지, `codex queue --thread <ID>`가 앱 세션에 들어가는지.
  `agentlayer status --json`에 없는 앱 세션 ID로 `agentlayer send <ID 앞 8자> 안녕`이 `(codex queue)`로 끝나고 앱 화면에 나타나는지.
- 앱 세션의 rollout `cwd`가 앱이 연 폴더와 같은지(`--cwd` 없이 큐가 그 폴더에서 실행되는지).
- 세션 ID 접두가 다른 세션과 겹칠 때의 오류 문구가 실사용에서 읽히는지.
