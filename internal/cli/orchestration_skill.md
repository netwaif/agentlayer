---
name: orchestration
description: AgentLayer로 멀티 에이전트 오케스트레이션. worker N개(claude·codex·gemini)를 각각 git worktree+tmux window에 띄워 태스크를 병렬로 dispatch하고, 완료를 기다렸다가 결과를 취합·비교한다. "worker 2개 만들어서 같은 태스크 시켜줘", "claude랑 codex한테 A/B로 시켜봐", "서로 다른 태스크 나눠서 병렬로", "/orchestration" 등으로 트리거. 에이전트 브라우저 관련 요청("브라우저에서 지목할게", "스크린샷 확인해봐", "스크린샷 폰으로 보내줘", "x.com 로그인 쿠키 가져와줘", "에이전트 브라우저에 뭐 들어 있어?", "x.com 쿠키 지워줘")도 이 스킬이 처리한다. 머지는 기본적으로 사용자 몫이다.
---

# AgentLayer Orchestration — 코디네이터 지침

너는 코디네이터다. worker 생성·지시·완료 감지·취합을 아래 절차대로 수행한다.
전제: 지금 tmux 세션 안에서 실행 중이고, `agentlayer`가 설치돼 hook 등록(`agentlayer init`)이 끝났고, 대상 폴더는 git 저장소다. 전제가 깨져 있으면 시작 전에 사용자에게 알린다.

## 0. 계획 복창

사용자 요청에서 다음을 정리해 한 문장으로 복창한 뒤 시작한다:
- worker 수와 종류 (claude | codex | gemini)
- 태스크 구성: 같은 태스크 A/B 비교인지, 서로 다른 태스크 분업인지
- 커밋·머지 방침 (기본: worker는 커밋하지 않고, base 브랜치는 아무도 건드리지 않고, 머지는 사용자가 선택)

## 1. worker 생성 — 태스크당 worktree 1개

```
agentlayer wt new <태스크이름> --agent <종류> [--repo <경로>] [--base <브랜치>] [--test '<명령>']
```

- 태스크 이름이 곧 브랜치(`agent/<이름>`)·tmux window 이름이 된다. A/B 비교면 `hero-dark-claude`, `hero-dark-codex`처럼 종류를 접미로 붙여라.
- worktree는 `<repo>/.agentlayer/worktrees/<이름>`에 생기고, window는 지금 tmux 세션에 열리며 CLI가 자동 기동된다.
- **부팅 대기**: 생성 직후 바로 dispatch하지 마라. 8초쯤 기다린 뒤
  `tmux capture-pane -p -t ':<태스크이름>.0' | tail -8`
  로 입력 프롬프트를 확인한다. 안 떠 있으면 5초 간격으로 재확인(최대 60초).
- 새 worktree 경로라 폴더 신뢰(trust)·온보딩 프롬프트가 뜰 수 있다. 화면을 읽고 같은 repo의 worktree임이 확실하면 Enter로 승인하고, 그 외의 프롬프트는 사용자에게 보고한다.

## 2. dispatch — send-keys 2단 규약 (한 번에 보내지 말 것)

```
tmux send-keys -t ':<태스크이름>.0' -l '<지시문>'
sleep 1
tmux send-keys -t ':<태스크이름>.0' Enter
```

- 텍스트와 Enter를 붙여 보내면 CLI가 텍스트를 처리하기 전에 Enter가 도착해 미제출로 남을 수 있다. 반드시 `-l`(리터럴)로 텍스트 → 1초 대기 → Enter 순서.
- 전송 후 capture-pane으로 입력줄에 지시문이 남아 있는지 확인하고, 남아 있으면 Enter만 재전송.
- 지시문에 작은따옴표·개행이 섞이면 이스케이프가 깨지기 쉽다. 그럴 땐 지시문을 파일로 쓰고:
  `tmux load-buffer <파일> && tmux paste-buffer -t ':<태스크이름>.0'` 후 Enter.
- **지시문 끝에 보고 규약을 붙인다**:
  > 작업이 끝나면 worktree 루트에 REPORT.md를 남겨라. 형식: `## Summary`(2~3문장) / `## Changed Files`(파일별 한 줄) / `## How to Verify`(실행·확인 방법) / `## Notes`(기존 파일 연결 필요 등 코디네이터가 알아야 할 것). 커밋은 하지 마라. base 브랜치는 절대 건드리지 마라.

## 3. 완료 대기 — 화면 스크래핑 금지, 상태는 hook이 정본

```
agentlayer status | grep -F 'worktrees/<태스크이름>'
```

- 해당 행이 `[DONE]`이면 그 worker는 완료다. 모든 worker가 DONE이 될 때까지 30초 간격으로 폴링한다(백그라운드 until 루프 권장 — 사이사이 다른 준비 작업을 해도 된다).
- 한 worker가 오래 `[WORK]`에 머물면 capture-pane으로 화면을 확인한다 — 승인 대기 프롬프트면 내용을 사용자에게 보고하고 지시를 기다린다. 임의로 승인하지 않는다.
- `[WAIT]`(응답 필요)로 바뀐 worker도 화면을 확인해 질문이면 답을 보내거나 사용자에게 올린다.

## 4. 취합

각 worktree에서:
```
cat <worktree>/REPORT.md
git -C <worktree> diff --stat        # 필요하면 diff 본문까지
```

- **A/B 비교면**: 변경 규모·접근 방향·구성 차이를 표로 요약하고, 선택 관점(어떤 기준이면 어느 쪽인지)을 한 단락 덧붙인다. 머지는 하지 않는다 — 사용자가 고른다.
- **분업이면**: 각 보고를 요약하고, worker들이 Notes에 남긴 연결 작업(공용 파일 import 추가 등)은 코디네이터인 네가 **각 브랜치의 worktree 안에서** 직접 수정하고 커밋한다. base에서 하지 않는다.
- 미리보기가 필요하면 각 worktree에서 포트를 달리해 dev 서버를 띄워 비교하게 한다. 서버는 **그 worktree 폴더를 cwd로**(`cd <worktree> && python3 -m http.server <포트> --bind 127.0.0.1 &`) 띄운다 — 관제탑이 cwd(또는 `--directory` 인자)로 어느 브랜치인지 알아보고 ⎇브랜치 탭을 연다. codex worker는 샌드박스라 백그라운드 서버를 못 남기니 서버는 코디네이터가 띄운다.

## 5. 머지·정리 — 자동 머지는 없다

- 사용자가 브랜치를 고르면: worker 변경 커밋(REPORT.md는 커밋에서 제외) → `agentlayer wt merge <이름>` (검사 요약 후 y 확인을 받아야만 머지된다).
- 사용자가 "claude 거로 머지해줘"처럼 **어느 worker인지 말로 명시**했으면 그 말이 확인이다 → `agentlayer wt merge <이름> --yes` (Bash에서는 stdin이 없어 y를 받을 수 없다). 어느 쪽인지 안 정했으면 표를 보여주고 묻는다.
- 정리는 `agentlayer wt clean <이름>` — 미커밋·미병합이 있으면 거부되는 보존 우선 설계다. 거부되면 이유를 사용자에게 보여주고 지시를 기다린다.
- 폐기하는 브랜치도 사용자 확인 전에는 지우지 않는다.

## 6. 브라우저 — 전용 브라우저 하나를 셋이 나눠 쓴다

에이전트 전용 Chrome(로그인 세션 보존, `agentlayer init`이 chrome-devtools MCP로 연결)이 있다. 브라우저가 필요한 일은 전부 이 경로다.

- **도구는 chrome-devtools MCP**(`list_pages`·`new_page`·`navigate_page`·`click`·`fill`·`evaluate_script`·`get_console_message`·`get_network_request`). 브라우저가 안 떠 있어도 MCP 서버(`agentlayer browser mcp-serve`)가 띄우므로 기동 명령은 없다.
- **자기 탭에서만 작업한다** — 시작할 때 `new_page`로 탭을 만들고 그 pageId만 쓴다. 다른 에이전트(claude·codex·gemini)가 같은 브라우저를 쓰고 있으므로 남의 탭을 이동·닫지 않는다.
- 내장 브라우저 스킬(`browser:control-in-app-browser` 등 앱 자체 런타임)은 쓰지 않는다 — 그건 이 브라우저를 모른다.
- worker가 dev 서버를 띄우면 `http://localhost:<포트>`를 한 줄로 찍는다. 사용자는 관제탑 `p`(프리뷰)나 터미널 링크 ⌘-클릭으로 같은 브라우저에서 본다.
- 사용자가 화면을 봐야 하는데 원격이거나 "스크린샷 폰으로 보내줘"라고 하면 `agentlayer browser shot <url>`로 찍는다 — stdout 첫 줄이 png 절대경로다. **디스코드에서 온 지시면 그 경로를 그 채널 답글(discord `reply`의 `files`)에 첨부한다** — 사용자가 지시한 화면에 결과가 온다. SSH처럼 답글 첨부 수단이 없으면 `--notify`를 붙여 알림 웹훅(폰 Discord 알림 채널)으로 보낸다. 봇 세션처럼 PATH가 최소면 `~/.local/bin/agentlayer`로 부른다.
- **"뭔가 잘못된 것 같은데"·"콘솔 에러 확인해봐"처럼 증상만 말하면** 사람에게 F12나 로그를 요구하지 말고 직접 읽는다 — chrome-devtools `list_console_messages`(JS 예외·console.error)와 `list_network_requests`(404·CORS)로 자기 탭을 보거나, `agentlayer browser errors --reload`(활성 탭을 리로드해 5초 수집, 예외·404를 줄 단위로 찍고 txt 경로를 stdout에 남김)를 쓴다. `--reload` 없는 `errors`는 사람이 Enter를 칠 때까지 기다리므로 에이전트가 부르면 안 된다. 화면에 보이는 증상 하나에 콘솔 에러가 여럿인 경우가 흔하니 **에러를 전부 짚고** 원인별로 고친다.
- 사용자가 관제탑 `b`로 지목한 요소는 "셀렉터·컨텍스트 md·png 경로"가 한 줄로 들어온다 — md를 읽고 고친 뒤 같은 탭을 리로드해 스스로 확인한다.
- **지목·스크린샷도 말로 시킬 수 있다** — 사용자가 "브라우저에서 지목할게"라고 하면 `agentlayer browser pick --once`를 실행한다(Bash timeout 5분 — 사용자가 클릭하고 지시를 적을 때까지 기다린다). 먼저 "에이전트 브라우저에서 요소를 클릭하고 뜨는 입력창에 지시를 적어 주세요"라고 안내한다. 사용자가 Enter를 치면 stdout에 `브라우저 요소 수정 요청: "…" — 맥락 파일을 읽고 반영해줘: <md 경로>` 한 줄이 나온다 → md를 읽고 그 요소만 고친 뒤 자기 탭을 리로드해 확인한다. "스크린샷 확인해봐"는 `agentlayer browser shot [url]`이 찍은 png 경로를 Read로 보고 판단한다(chrome-devtools `take_screenshot`도 된다). 관제탑 `b`/`s`는 여러 에이전트 중 대상을 고를 때 쓰는 같은 기능이다.
- **로그인 쿠키는 사용자가 말로 시키고 에이전트가 명령을 대신 친다** — "OO 로그인 쿠키 가져와줘" → `agentlayer browser cookies import <도메인>` (실사용 Chrome에서 그 도메인 쿠키만 읽어 에이전트 브라우저에 넣는다. macOS Keychain 팝업이 뜨니 사용자에게 "항상 허용"을 누르라고 먼저 말한다), "에이전트 브라우저에 뭐 들어 있어?" → `cookies list` (호스트별 개수) / `cookies list <도메인>` (이름·만료, 값은 안 나오므로 출력을 그대로 보여줘도 된다), "OO 쿠키 지워줘" → `cookies clear <도메인>` (에이전트 브라우저에서만 지운다). 실사용 Chrome은 import 때 읽기만 하고 절대 바꾸지 않는다.

## 하지 말 것

- **별도 tmux 서버(-L/-S) 금지** — 상태 저장소가 공유라 pane ID가 충돌해 다른 세션 레코드를 오염시킨다.
- 화면 파싱으로 완료 판정 금지 — 완료는 `agentlayer status`의 상태로만 판단한다.
- 자동 머지 금지, base 브랜치 직접 수정 금지, worker의 승인 프롬프트 임의 승인 금지.
- 브라우저는 남의 탭 조작 금지, 앱 내장 브라우저 런타임 사용 금지 — chrome-devtools MCP·자기 탭만.
