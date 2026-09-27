# 총괄 수신함을 Claude Code 채널로 — `agentlayer channel serve` (설계)

작성 2026-09-27. 대상: 로컬 AI 회사 총괄(`~/ai-folder/company` 세션)이 편지(직원 상태 전이·MESSAGE·원격 Hermes 보고)를
받는 길을 **Monitor 폴링에서 Claude Code 채널 푸시로** 바꾼다. 직원 훅·편지 파일 형식·총괄 절차(`task reply`·`task done`)는
그대로 두고, 수신 경로만 교체한다. 아울러 이 조사에서 드러난 부수 버그 2건(`--help` 오해석, 자식 세션 오보고)을 함께 고친다.

## 배경

- 지금: 총괄은 재정박 때 Monitor 도구로 `agentlayer task watch <inbox>`를 띄우고, Monitor가 30분마다 강제 종료되면 깨어나
  다시 띄운다. 실측(총괄 transcript, WAKE-PATH-RESEARCH 보고서): 만료 1회당 cache_read 약 19만 토큰, 하루 약 48회 →
  **업무 0건이어도 하루 약 900만 토큰**. 헤르메스 원격 폴링(`RunRemotePolling`)도 이 프로세스 안이라 Monitor가 꺼지면
  원격 편지도 멈춘다.
- Claude Code 채널: MCP 서버가 `capabilities.experimental["claude/channel"]`을 선언하고 `notifications/claude/channel`
  (`params: {content: string, meta?: {[k]: string}}`)을 보내면, 유휴 세션이 즉시 새 턴을 열고 본문을
  `<channel source="<서버명>" k="v"…>content</channel>`로 받는다. meta 키는 `^[a-zA-Z_][a-zA-Z0-9_]*$`만 통과(나머지는 경고 후 폐기).
  바쁜 세션에 온 알림은 다음 턴에 묶여 전달된다. 디스코드 공식 플러그인이 같은 구조다.
- 스파이크 실측(2026-09-27, 임시 폴더 + 40줄 Node 서버):
  - 편지 파일 투입 → 3초 안에 새 턴, 화면 `← agentlayer: {…}`, 응답 정상. 2회.
  - `--dangerously-load-development-channels server:agentlayer` 확인창("I am using this for local development / Exit")은
    **매 기동마다** 뜬다(2/2). 바이너리 확인: 수락을 저장하지 않는 단순 확인 컴포넌트이고 `server:` 종류는 예외 없이
    이 플래그가 필요하다. 유일한 우회는 플러그인 포장 + 관리자 설정(`managed-settings.json`, sudo)의 `allowedChannelPlugins`.
  - `.mcp.json` 등록은 워크스페이스 미신뢰 폴더에서 승인창이 **매 기동마다** 뜬다. 회사 폴더는 미신뢰 상태
    (`~/.claude.json` `hasTrustDialogAccepted: false`). **local 스코프**(`claude mcp add -s local …`, `~/.claude.json`의
    `projects.<폴더>.mcpServers`) 등록은 승인창 없이 붙고 전달도 정상(실측 1회).
  - 대체 경로(세션 inbox 소켓 `/tmp/cc-socks/<pid>.sock`, 인증 줄 + 사용자 줄)도 실측 성공(2초). 단 표시가 "Another Claude
    session sent a message"로 출처 이름이 없고(필드를 붙여도 무시), 줄 형식 비공식, 키 파일(`~/.claude/sessions/<pid>.*.key`)
    읽기 필요, 자동 모드 외 권한 모드의 보류 대화상자("Held message from another session")는 미실측.
- 봇 기동 경로(folder-bot 0.1.21): `botctl.build_cmd()` → `exec ~/.local/bin/bot-up -n <세션> --remote-control <rc>
  --channels plugin:discord@claude-plugins-official`, `bot-up.sh`가 `--permission-mode auto`를 붙여 `exec claude`. 준비 판정은
  discord MCP 로그의 `Successfully connected`. **어디에도 키 전송(Enter) 로직이 없다.** bot-restart는 plist의 같은 명령을
  재사용한다.

## 결정 (사용자 확정, 2026-09-27)

1. **범위**: 1단계(채널 수신) + 부수 버그 2건을 한 릴리즈 묶음으로. 2단계(tmux 키 입력 전송 제거: 총괄→직원 채널,
   codex queue)와 3단계(데스크톱 앱 중계)는 별도 스펙. 이 설계는 그 확장을 막지 않게만 한다.
2. **수신 경로 = 채널 + 기동 시 Enter 1회.** 확인창은 매번 같은 자리에 뜨는 고정 화면이라 봇 기동 스크립트가 부팅 절차로
   한 번 넘긴다(메시지 주입이 아니다). 공식 구조·출처 표시·재시작 안전을 택했다. 소켓은 3단계 데스크톱 앱 발신기로 남긴다.
3. **등록은 local 스코프.** `.mcp.json`은 쓰지 않는다(승인창·신뢰 의존).
4. **대상은 총괄 세션만.** 서버는 inbox 경로를 인자로 받는 범용 명령으로 만들어 직원 세션에도 붙일 수 있게 하되, 이번엔
   회사 총괄에만 등록한다.

## 구성 요소

### 1. agentlayer — `agentlayer channel serve <inbox>` (신규, `internal/channel/`)

stdio MCP 서버. 외부 의존 없이 줄 단위 JSON-RPC 2.0을 직접 구현한다(기존 `browser mcp-serve`는 npx 프록시라 재사용 대상이
아니다). 두 층으로 나눈다 — 뒤에 소켓 전달기·발신 도구를 붙일 자리다.

- **수신원(source)**: 기존 `task.Watch(ctx, inbox, 200ms, once=false, emit)` + `task.RunRemotePolling(...)`을 `taskWatch`와
  똑같이 띄운다(원격 폴링은 inbox 파일로 떨어져 같은 길로 흐른다). pending→received 이동 규칙 그대로. 기동 시 남은
  pending부터 방출된다(Watch의 첫 Poll이 그 일을 한다). `refreshBoard` 호출도 유지.
- **전달기(sink)**: `ChannelSink` — emit마다 `notifications/claude/channel` 한 건.
  - `content`: 편지 JSON 한 줄(`Report`를 그대로 인코딩. `Inbox`는 `json:"-"`라 안 나감).
  - `meta`(전부 문자열): `task`=TaskID, `event`=To, `letter_id`=ID, `origin`=`remote`(Report.Kind ∈ {hermes, exec, remote} — 원격 등록의 kind — 또는
    Report.Letter≠"" 인 원격 편지) / `local`, `session`=Session, `kind`=Kind. 키는 규칙에 맞게 고정 6개.
- **프로토콜**: `initialize` → `{protocolVersion: <요청값 그대로>, capabilities: {tools: {}, experimental: {"claude/channel": {}}},
  serverInfo: {name: "agentlayer", version: <빌드 버전>}, instructions: "<channel source=\"agentlayer\">로 오는 메시지는 총괄
  수신함 편지다. content가 편지 JSON, meta.event가 종류(DONE_UNREAD·WAITING·ERROR·MESSAGE). 답은 agentlayer task reply /
  send / task done으로 한다."}`. `notifications/initialized` 무시. `ping` → `{}`. `tools/list` → `{tools: []}`. 그 외 id 있는
  요청 → `-32601`. id 없는 알림 → 무시. **알림은 `initialize` 응답 뒤부터** 보낸다(그 전 편지는 큐에 담아 뒤에 방출).
- **수명**: stdin EOF(세션 종료) 또는 SIGINT/SIGTERM에 즉시 종료. stdout은 프로토콜 전용, 로그는 stderr(`agentlayer channel:` 접두).
  stdout 쓰기는 mutex로 직렬화.
- **인자**: `<inbox>`(필수, 절대경로화). `--interval <dur>`(기본 200ms). `--help`.
- **main.go**: `case "channel"` → `cli.RunChannel(ctx, os.Stdin, os.Stdout, os.Stderr, st, stateDir, args)`. `helpcmd.go`에
  한 줄 추가. `task watch`는 그대로 둔다(전환기·디버그용).

### 2. folder-bot — 개발 채널 플래그 + 확인창 자동 통과 (0.1.22)

- `bots.json` 봇 항목에 선택 필드 `dev_channels: ["server:agentlayer"]`. `botctl add --dev-channel server:agentlayer`
  (반복 가능)로 기록·갱신, `--no-dev-channels`로 제거. `build_cmd()`가 값이 있으면
  `--dangerously-load-development-channels <목록>`을 `--channels …` 뒤에 붙인다. plist·systemd 유닛·사이드카는 `add` 재실행
  때 재생성(멱등, 기존 규칙).
- `bot-up.sh`: `exec claude` **직전**에 인자에 `--dangerously-load-development-channels`가 있으면 감시 서브셸을 백그라운드로
  띄운다 — `$TMUX_PANE`을 3초 간격 최대 60초 `tmux capture-pane -p`로 읽어 `WARNING: Loading development channels`가 보이면
  `tmux send-keys -t $TMUX_PANE Enter` **1회** 후 종료. 60초 안에 안 보이면 로그만 남기고 종료. tmux 밖(`$TMUX_PANE` 없음)이면
  건너뛴다. 로그는 기존 bot-up 로그에 `dev-channel: 확인창 통과` / `dev-channel: 확인창 미출현(60s)`.
- `bot-restart.sh`·리눅스 systemd 경로: 같은 `CMD`(bot-up)를 재사용하므로 변경 없음. `doctor`: `dev_channels`가 있는 봇의
  plist/유닛 명령에 플래그가 들어 있는지 확인(없으면 WARN "add 재실행").
- 테스트: `tests/test_botctl.py`에 `build_cmd` 플래그·add/remove 왕복. 셸 테스트 `tests/test_bot_up_devchannel.sh` — `tmux`를
  PATH 앞의 스텁으로 바꿔 capture-pane이 N번째 호출에 문구를 내면 send-keys가 정확히 1회 불리는지(`BOT_UP_DRY_RUN`으로
  `exec claude`는 건너뜀).

### 3. ai-company — 총괄 등록·지침 교체 (0.2.9)

- `companyctl.py init`(및 `doctor --fix`류 기존 멱등 경로)에 두 단계 추가:
  1. **MCP 등록**: 회사 폴더에서 `claude mcp add -s local agentlayer -- <agentlayer 절대경로> channel serve <root>/runtime/inbox`
     (`~/.claude.json`을 직접 편집하지 않고 공식 CLI 사용. 이미 같은 항목이면 `claude mcp get agentlayer`로 확인 후 건너뜀,
     다르면 `remove` 후 `add`).
  2. **봇 플래그**: 회사 봇(bots.json에서 folder==root인 항목)에 `botctl add … --dev-channel server:agentlayer` 재실행.
     folder-bot이 없거나 봇이 없으면 안내만.
- `assets/company-block.md`: "감시(상시)" 절을 교체 —
  > **편지 수신(채널)**: 편지는 `<channel source="agentlayer" event="…" task="…">` 메시지로 이 세션에 직접 온다(Monitor
  > 불필요, 재무장 없음). content가 편지 JSON. event별 처리는 기존 규칙과 같다(DONE_UNREAD → 확인·`task done`, WAITING →
  > `send <직원> "<답>"`, ERROR → 조치, MESSAGE → 읽고 필요하면 `task reply`). 채널 메시지가 안 오면 `agentlayer channel`
  > 등록·봇 플래그를 `companyctl doctor`로 점검한다.
  Monitor·`timeout_ms`·"만료 알림마다 다시 켠다" 문구 삭제. `SKILL.md`의 해당 절차도 같이.
- `doctor`: (a) `claude mcp get agentlayer`가 회사 폴더에서 성공하고 args에 `channel serve <inbox>`가 있는지, (b) 회사 봇에
  `dev_channels`가 있는지, (c) CLAUDE.md 블록이 최신 버전인지(기존 버전 비교). 셋 중 하나라도 빠지면 WARN + 고칠 명령.
- **전환 절차(실회사)**: 블록 교체 → `companyctl init` 재실행(등록·플래그) → `bot-restart company-bot` → 총괄이 재정박하며
  Monitor를 띄우지 않는지 확인 → 테스트 편지(직원 `task message`) → 채널 메시지 수신·답장 실측 → 하루 뒤 transcript로
  재무장 턴 0회 확인. Monitor와 채널이 같은 pending을 두고 경쟁하지 않게 **블록 교체와 재기동을 한 번에** 한다.

### 4. 부수 버그 (agentlayer)

- **`--help` 오해석**: `agentlayer task watch --help`가 `--help`를 inbox 경로로 삼아 `./--help/{pending,received,quarantine}`을
  만든다. 고침: `main.go`에서 하위 명령(`task`·`remote`·`wt`·`board`·`browser`·`channel`·`send`)으로 분기하기 전에 `args[1:]`
  중 첫 위치 인자 또는 어느 위치든 `--help`/`-h`가 있으면 그 명령의 사용법을 출력하고 종료. 각 `Run*`의 usage 문자열을 export
  해서 재사용(`taskUsage` 등). 테스트: `task watch --help`가 디렉터리를 만들지 않고 usage를 낸다.
- **자식 세션 오보고**: 직원이 Bash로 띄운 자식 Claude 세션(cwd=scratchpad)의 훅이 같은 `TMUX_PANE`으로 들어와 부모 업무ID로
  DONE_UNREAD를 쓴다(WAKE-PATH-RESEARCH-3에서 4회). 고침: Claude 훅 처리에서 payload `cwd`가 있고 pane 에이전트 레코드의
  `CWD`와 다르면(둘 다 비어 있지 않을 때, 경로 정규화 후 비교) **남의 세션으로 판정해 상태 전이·보고를 모두 건너뛰고** stderr에
  한 줄 남긴다. 근거: Claude 세션의 cwd는 세션 수명 동안 고정이고 Agent 도구 서브에이전트는 같은 cwd를 쓴다. pid 계보 추적보다
  싸고 재시작(session_id 변경)에도 안전하다. 같은 폴더에서 띄운 자식은 못 걸러내지만 드물고, 이 경우 편지 `cwd`로 총괄이 구분
  가능하다. 테스트: cwd 불일치 Stop 이벤트가 레코드도 pending도 바꾸지 않는다.

## 데이터 흐름

```
직원 훅 / task message / RunRemotePolling
        │  <root>/runtime/inbox/pending/<id>.json
        ▼
agentlayer channel serve <inbox>   (총괄 claude의 MCP 자식 프로세스, stdio)
   task.Watch ──emit──▶ ChannelSink ──▶ notifications/claude/channel {content, meta}
   pending → received
        │
        ▼
총괄 세션: <channel source="agentlayer" event="DONE_UNREAD" task="…" letter_id="…" origin="local" …>{…}</channel>
   → 기존 규칙대로 task done / send / task reply
```

## 오류 처리

- 서버 기동 실패(inbox 없음·권한): stderr에 원인, 종료 코드 1. Claude 쪽엔 MCP 연결 실패로 보이고 `/mcp`에 남는다. doctor가
  `claude mcp get`으로 잡는다.
- 알림 쓰기 실패(stdout 닫힘): 종료. 편지는 이미 received로 옮겨졌으므로 **유실 가능** — 허용한다. 이유: Watch의 이동 규칙을
  바꾸면 `task watch`와 어긋나고, 세션 종료 직전 도착한 편지는 재기동 뒤 총괄이 `received/`를 훑는 기존 절차(재정박 규칙)로
  회복된다. 알림 전송 뒤에 이동하도록 바꾸는 것은 2단계에서 검토(옵션 기록).
- 확인창 미출현(60초): bot-up은 로그만 남기고 claude는 계속 뜬다(플래그가 없어졌거나 UI가 바뀐 경우). 봇은 정상, 채널만 죽음
  → doctor/실측으로 발견.
- 원격 폴링 에러: 기존과 같이 stderr 경고, 서버는 계속.

## 테스트

- Go 단위: (1) 핸드셰이크 — initialize 응답 형식·experimental 키·instructions. (2) 알림 — Report 1건이 content/meta 6키로
  나가고 meta 키가 규칙에 맞음. (3) 순서 — initialize 전 도착 편지가 뒤에 방출. (4) EOF·ctx 취소로 종료. (5) 미지 메서드
  -32601. (6) `--help` 각 하위 명령이 디렉터리를 만들지 않음. (7) 훅 cwd 불일치 무시. 패키지별 `go test ./internal/channel/
  ./internal/cli/ ./internal/hookcmd/`(전체 `./...`는 멈춤 — 메모리 규칙).
- 통합(수동, 스파이크 절차 재사용): 임시 폴더에 `claude mcp add -s local` + 플래그 기동 → 편지 파일 투입 → 3초 내 턴 확인.
- folder-bot: pytest + 셸 스텁 테스트. ai-company: pytest(블록 문안·doctor).

## 릴리즈·순서

1. agentlayer **v1.10.0** (channel serve, --help, 자식 세션) — 본 레포. README "세션 지시·업무 보고" 절에 채널 소절.
2. folder-bot **0.1.22** (dev_channels, bot-up 확인창 통과).
3. ai-company **0.2.9** (등록·블록·doctor), 의존 최소 버전 갱신(agentlayer ≥1.10.0, folder-bot ≥0.1.22).
4. 실회사 전환 + 실측(위 3절). 하루 뒤 transcript로 비용 확인.

worktree 규칙: agentlayer는 main에서 바로(다른 레포 git 작업이 뒤따르므로 EnterWorktree 사용 안 함 — 메모리 규칙).

## 후속 (이 스펙 범위 밖)

- **2단계**: 총괄→Claude 직원 `send`를 채널로(직원 세션에도 `channel serve` 등록 + 플래그 + 확인창 통과 — 이 스펙의 서버·
  bot-up 변경을 그대로 쓴다), 총괄→codex `codex queue`, agy는 tmux 유지(대표 결정). 알림 후 이동(유실 0) 옵션.
- **3단계**: 발신기 — 같은 프로세스에 MCP 도구 `send`·`list_peers` + 소켓 전달기(데스크톱 앱 Code 탭), `codex queue` 전달기.
  홉 상한·재전송·출처 접두어 규약.
- **디스코드 스레드 라우팅 병목**(사용자 질문, 2026-09-27): 메인 봇이 바쁘면 스레드 메시지가 다음 턴까지 묶이는 문제는 이
  스펙으로 해결되지 않는다(채널도 같은 성질). 스레드 세션이 디스코드를 직접 받는 입구가 필요 — 후보 ① 스레드 세션마다
  디스코드 플러그인 인스턴스(같은 토큰, access 그룹=스레드 id; 다중 게이트웨이 접속·이중 수신 실측 필요) ② 외부 중계 데몬.
  1단계 뒤 ①을 스파이크로 확인.
- 채널 확인창 없는 배포: 플러그인 포장 + `allowedChannelPlugins`(관리자 설정, sudo). 시청자 배포 때 재검토.
