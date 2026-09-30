# 데스크톱 앱 세션 주소 (tmux 밖 세션) — 설계 메모

**원래 목표:** Claude Code Desktop 앱 안에서 연 Claude 세션도 agentlayer의 관제(status·TUI)와 전송(`send`·채널) 대상이 되게
주소 체계를 tmux pane 밖으로 넓힌다(SESSION.md 다음 단계 0번 ⑤).

**2026-09-30 범위 변경(맥 실기 결과, 최종):** 앱에서 새로 연 Claude 세션은 **채널도 세션 간 메시지도 받지 못한다** — 앱이
`--dangerously-load-development-channels`(채널 플래그)를 넘기지 않고, 세션 간 메시지도 앱 화면에 나타나지 않는다. 앱 안의 Claude
노드는 **"tmux 세션을 리모트 컨트롤(`--remote-control`)로 보는 창"**으로 정하고, tmux 밖 Claude 세션을 기록·주소화하는 작업은
**보류**한다. 대신 **기존 입력의 결과를 하나도 바꾸지 않는 원칙** 아래 네 경로를 만들었다: A 코덱스 세션 ID 직송, E `inbox wait`,
F 주소록 이름으로 보내기, G 업무 등록 없는 원격 직송. 상태 저장소(state)·훅(hookcmd)·status/UI 코드는 손대지 않았고, 새 기능은
새 파일(`sendcodex.go`·`inboxcmd.go`·`sendremote_direct.go`)에 두었으며 `RunSend`에는 "기존 해석이 실패한 뒤에만 타는 분기"만 붙였다.

## 보류 — tmux 밖 Claude 세션 기록·주소화

- **사유:** 위 실기 결과. 레코드가 생겨도 `send`가 닿을 경로(채널)가 없고 tmux 키 입력 폴백도 없다.
- **만들어 둔 코드:** 브랜치 `desktop-sessions-hold`(origin에 푸시, PR 제외, 테스트 통과 상태). 훅이 TMUX_PANE 없이 조상 사슬의
  에이전트 프로세스 PID로 `<kind>-pid<N>` 레코드를 기록(b502d3f), 채널 서버가 tmux 밖이면 `inboxes/pid<N>` 수신함(5bb2a64),
  `send` 대상을 `-n` 이름·세션 ID 접두로도(b579992), status/TUI `이름 (app)` 표시(474896b).
- **되살릴 때 알아둘 것:** 좌표는 세션 ID가 아니라 에이전트 프로세스 PID(훅·MCP 서버·Bash 도구의 `Getppid()`는 다르지만 "가장
  가까운 kind 조상"은 같다; `/clear`에도 프로세스는 그대로; 코덱스 notify는 세션 ID를 안 준다). 데스크톱 앱 실행 파일이
  `/Applications/Claude.app/Contents/MacOS/Claude`라 `scan.KindFromArgs`(basename 대소문자 무시)가 앱 자체를 claude로 볼 수 있다 —
  `NestedClaude`·`SessionPID`가 앱을 잡지 않는지 맥에서 볼 것. 선결 조건은 앱 세션이 채널(또는 다른 입력 경로)을 받게 되는 것.

## 결정

### A. `send <UUID>` — 기록 없는 코덱스 세션 직송 (`internal/cli/sendcodex.go`)
- `ResolveTarget`이 실패한 뒤, 인자가 세션 ID 형식(`LooksLikeSessionID`: UUID 전체 또는 16진수·하이픈 앞자리 접두 8자 이상)이면
  `codex queue --thread <ID>`로 보낸다. 접두는 rollout(`usage.CodexSessionByPrefix`, 최근 400개)에서 전체 ID·cwd를 찾고 둘 이상이면
  후보를 보이고 거부, 없으면 오류. 전체 UUID는 rollout이 없어도 보낸다. `--cwd`가 rollout cwd보다 우선. 성공 `via=queue`, 실패 오류
  (폴백 없음). 상태를 모르니 관문 없음. `codex_queue: false`면 오류.
- `ResolveTarget` 자체는 손대지 않았다(레코드의 세션 ID 접두 매칭도 넣지 않음) — 기존 입력 불변 원칙. tmux 세션 이름이
  UUID 모양이면 tmux가 우선한다.

### E. `inbox wait` (`internal/cli/inboxcmd.go`, main.go `case "inbox"`)
- 세션 PID = 부모 사슬에서 인자에 claude가 있는 첫 조상(`SessionPID`, `scan.LoadProcTable`·`KindFromArgs`). 못 찾으면 자기 PID
  (기다리는 동안은 살아 있어 send의 생사 판정이 맞다)와 stderr 안내.
- 주소록 `addresses/<이름>.json` {name, pid, cwd, inbox, registered_at}. 이름 기본 폴더명, 다른 산 세션과 겹치면 `-<pid 끝 4자리>`,
  죽은 항목은 덮어쓴다. 수신함 `inboxes/a<pid>/`(pane 수신함 `p<pane>`과 접두가 다르다). 편지는 `task.Report`·`task.Watch`(once)·
  `DirectiveEvent`를 재사용하고 시작 시 `purgeStale`. 편지 한 통 → stdout `from: X` + 빈 줄 + 본문, exit 0. 타임아웃 →
  `ErrInboxTimeout`("답 없음(기간)")을 main이 exit 2로. SIGINT/SIGTERM은 ctx 취소 → defer가 주소록을 지운다.
- 상태 저장소에 넣지 않는다(status·TUI 불변). `.channel.lock`도 쥐지 않는다 — 생사는 PID로 본다(지시 그대로).

### F. `send <이름>` — 주소록 (`sendToAddress`)
- `ResolveTarget` 실패 뒤, 세션 ID 형식이 아닐 때만 주소록을 본다. PID가 살아 있으면 `SendViaChannel`(넣고 3초 안에 집어 가야
  성공, 아니면 회수) → `via=inbox`. 죽었으면 항목 삭제 후 오류. 주소록에도 없으면 예전 오류 그대로. 원격 이름·tmux 세션 이름이
  우선(순서: remote.Load → ResolveTarget → UUID → 주소록).

### G. `send <원격> [--file]...` — 업무 등록 없는 원격 직송 (`internal/cli/sendremote_direct.go`)
- `remote.Load` 성공 + `task.Load(RemoteAgentID)` 없음일 때만. 어댑터는 수정하지 않고 `Dispatch`만 쓴다(임시 업무ID
  `MSG-<NewID 8자 대문자>`, 제목 = 본문 첫 줄 60자, Attempt = 시각). 업무 등록은 만들지 않는다 → `task watch` 추적 없음, 답은
  편지함(Mailbox)으로.
- 첨부: `Answer`는 편지 카드를 `complete`로 닫아 버려 새 카드에 쓸 수 없고 `upload`는 비공개다. 그래서 Hermes(`*remote.Hermes`)의
  공개 필드(`R` Runner·`WorkspaceRoot`)와 `remote.ShellQuote`로 `upload`과 같은 셸 한 줄(`mkdir -p … && cat > …`, 표준입력)을
  카드 작업 폴더 `<WorkspaceRoot>/<MSG-id>/from-company/`에 실행하고 본문 끝에 `첨부: <경로>`를 적는다. exec 원격에 `--file`은 오류.
- `--file`은 이 경로에서만 유효 — 업무 등록된 원격·tmux·채널·큐·주소록 경로에 주면 `errNoFileHere`.

## 기존 동작이 바뀐 입력
- 없음. 오류였던 두 입력이 동작으로 바뀌었다: (1) 레코드 없는 UUID 대상(`send <UUID>`) → 코덱스 큐 직송, (2) 업무 등록 없는
  원격 대상 → 직송. 새 플래그 `--cwd`·`--file`은 예전에 "알 수 없는 플래그" 오류였다. 새 명령 `inbox wait`는 예전에 "알 수 없는
  명령"이었다. 주소록 이름은 예전 해석이 실패한 이름에만 붙는다.

## 확인 못 한 것(맥 실기 필요)
- 앱 Claude 세션의 Bash에서 `agentlayer inbox wait … &`로 띄웠을 때 `SessionPID`가 앱의 CLI claude를 찾는지(`Claude.app`
  실행 파일을 먼저 잡지 않는지) — 주소록 json의 pid를 ps와 대조.
- 백그라운드 Bash가 끝난 뒤 그 stdout(from:/본문)이 세션에 결과로 들어오는지, 30분 타임아웃 뒤 재실행 흐름이 실사용에 맞는지.
- 코덱스 앱 세션이 CLI와 같은 rollout 폴더를 쓰는지, `codex queue --thread <ID>`가 앱 세션 화면에 나타나는지, rollout `cwd`가 앱이
  연 폴더와 같은지(`--cwd` 없이 동작).
- Hermes 직송: `MSG-…` 카드가 만들어져 기동되는지, `from-company/` 첨부 경로가 원격 워커에게 읽히는지.
