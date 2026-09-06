---
name: agent-browser
description: AgentLayer 에이전트 전용 브라우저(chrome-devtools MCP로 연결된 전용 Chrome)로 웹 화면을 열고·보고·조작하고·진단하는 지침. 트리거 — "브라우저에서 열어봐/확인해봐", "스크린샷 확인해봐", "스크린샷 폰으로 보내줘", "콘솔 에러 확인해봐"·"뭔가 잘못된 것 같은데"(웹 화면 증상), "브라우저에서 지목할게", "x.com 로그인 쿠키 가져와줘", "에이전트 브라우저에 뭐 들어 있어?", "x.com 쿠키 지워줘", "claude.ai 세션 키 꺼내서 .env에 넣어줘"·"유튜브 쿠키 파일 만들어줘"(cookies export), "/agent-browser". 브라우저가 필요한 일은 전부 이 경로다 — 앱 내장 브라우저 런타임은 쓰지 않는다.
---

# AgentLayer 에이전트 브라우저 — 사용 지침

에이전트 전용 Chrome이 있다. 로그인 세션이 보존되고, 사람이 같은 창을 보며, 여러 에이전트(claude·codex·gemini)가 하나를 나눠 쓴다. `agentlayer init`이 이 브라우저를 chrome-devtools MCP로 붙여 놓았다.

## 공통 규칙

- **도구는 chrome-devtools MCP** — `list_pages`·`new_page`·`navigate_page`·`take_snapshot`·`take_screenshot`·`click`·`fill`·`evaluate_script`·`list_console_messages`·`list_network_requests`. 브라우저가 안 떠 있어도 MCP 서버(`agentlayer browser mcp-serve`)가 띄우므로 **기동 명령은 없다**.
- **자기 탭에서만 작업한다** — 시작할 때 `new_page`로 탭을 만들고 그 pageId만 쓴다. 다른 에이전트가 같은 브라우저를 쓰고 있으므로 남의 탭을 이동·닫지 않는다.
- 앱 내장 브라우저 스킬(`browser:control-in-app-browser` 등 자체 런타임)은 쓰지 않는다 — 그건 이 브라우저를 모른다.
- **창 크기를 바꾸지 않는다** — `resize_page`·`emulate`는 사람이 보고 있는 창을 흔든다. 반응형 확인이 필요하면 먼저 사용자에게 묻고, 끝나면 원래 크기로 되돌린다.
- 아래 셸 명령은 `agentlayer browser …`다. 봇 세션처럼 PATH가 최소면 `~/.local/bin/agentlayer`로 부른다.

## 보기·찍기 — "스크린샷 확인해봐", "폰으로 보내줘"

- 스스로 확인할 때는 `take_screenshot`(자기 탭) 또는 `agentlayer browser shot [url]`(전체 페이지, stdout 첫 줄이 png 절대경로)로 찍고 Read로 보고 판단한다.
- 사용자가 봐야 하는데 원격이거나 "스크린샷 폰으로 보내줘"라고 하면 `agentlayer browser shot <url>`로 찍는다. **디스코드에서 온 지시면 그 png 경로를 그 채널 답글(discord `reply`의 `files`)에 첨부한다** — 사용자가 지시한 화면에 결과가 온다. SSH처럼 답글 첨부 수단이 없으면 `--notify`를 붙여 알림 웹훅(폰 Discord 알림 채널)으로 보낸다.

## 증상 진단 — "뭔가 잘못된 것 같은데", "콘솔 에러 확인해봐"

- 사람에게 F12나 로그를 요구하지 말고 직접 읽는다. 자기 탭이면 `list_console_messages`(JS 예외·console.error)와 `list_network_requests`(404·CORS). 사람이 보고 있는 활성 탭이면 `agentlayer browser errors --reload`(탭을 리로드해 5초 수집, 예외·404를 줄 단위로 찍고 txt 경로를 stdout에 남김).
- `--reload` 없는 `errors`는 사람이 Enter를 칠 때까지 기다리므로 에이전트가 부르면 안 된다.
- 화면에 보이는 증상 하나에 콘솔 에러가 여럿인 경우가 흔하다 — **에러를 전부 짚고** 원인별로 고친 뒤 같은 탭을 리로드해 스스로 확인한다.

## 지목 받기 — "브라우저에서 지목할게"

- `agentlayer browser pick --once`를 실행한다(Bash timeout 5분 — 사용자가 요소를 클릭하고 지시를 적을 때까지 기다린다). 실행 전에 "에이전트 브라우저에서 요소를 클릭하고 뜨는 입력창에 지시를 적어 주세요"라고 안내한다.
- 사용자가 Enter를 치면 stdout에 `브라우저 요소 수정 요청: "…" — 맥락 파일을 읽고 반영해줘: <md 경로>` 한 줄이 나온다. md(셀렉터·주변 HTML·스크린샷 경로)를 읽고 **그 요소만** 고친 뒤 자기 탭을 리로드해 확인한다.
- 사용자가 관제탑 `b`로 지목하면 같은 형식의 한 줄이 pane 입력으로 들어온다 — 처리는 같다. 관제탑 키는 여러 에이전트 중 대상을 고를 때 쓰는 같은 기능이다.

## 로그인 — 쿠키는 사용자가 말로 시키고 에이전트가 명령을 친다

- "OO 로그인 쿠키 가져와줘" → `agentlayer browser cookies import <도메인>` — 실사용 Chrome에서 그 도메인 쿠키만 읽어 에이전트 브라우저에 넣는다. macOS Keychain 팝업이 뜨니 사용자에게 "항상 허용"을 누르라고 **먼저** 말한다.
- "에이전트 브라우저에 뭐 들어 있어?" → `cookies list`(호스트별 개수) / `cookies list <도메인>`(이름·만료). 값은 안 나오므로 출력을 그대로 보여줘도 된다.
- "OO 쿠키 지워줘" → `cookies clear <도메인>` — 에이전트 브라우저에서만 지운다.
- "OO 로그인 쿠키를 XX 설정에 넣어줘"·"세션 키 꺼내서 .env에 넣어줘"·"yt-dlp용 쿠키 파일 만들어줘" → `cookies export`. 브라우저에선 되는데 자동화만 하면 로그인에서 막히는 도구(사용량 모니터·yt-dlp·notebooklm CLI·감시 봇)에 세션을 넘겨주는 통로다.
  - 값 하나: `cookies export claude.ai sessionKey --to ~/x/key.txt` (값 한 줄) / `.env` 주입: `cookies export claude.ai sessionKey --env ~/bot/.env CLAUDE_SESSION_KEY` (그 KEY 줄만 갱신, 나머지 보존)
  - 도메인 전체: `cookies export youtube.com --to ~/x/cookies.txt` (Netscape cookies.txt — yt-dlp `--cookies`·curl `-b`) / `--format json`
  - 값은 파일(0600)로만 가고 화면·로그·Discord에 안 나온다. 명령 출력 요약("… → 파일 기록, 만료 날짜")만 전하고, **값을 읽어 보여주거나 채팅에 옮기지 않는다**. 어느 쿠키인지 모르면 `cookies list <도메인>`으로 이름부터 보고, 없다고 나오면 `cookies import <도메인>` 뒤에 다시 한다.
- 실사용 Chrome은 import 때 읽기만 하고 **절대 바꾸지 않는다**.

## dev 서버를 사람에게 보여주기

- 서버를 띄웠으면 `http://localhost:<포트>`를 한 줄로 찍는다. 사용자는 관제탑 `p`(프리뷰)나 터미널 링크 ⌘-클릭으로 같은 브라우저에서 본다. 처음 보는 서버는 관제탑이 자동으로 열기도 한다(`preview_auto`).
- 서버는 **그 작업 폴더를 cwd로** 띄운다(`cd <폴더> && python3 -m http.server <포트> --bind 127.0.0.1 &`) — 관제탑이 cwd로 어느 폴더·브랜치인지 알아본다.

## 하지 말 것

- 남의 탭 이동·닫기 금지. 자기 탭(pageId)만.
- 앱 내장 브라우저 런타임 사용 금지 — chrome-devtools MCP만.
- 묻지 않고 `resize_page`·`emulate`로 창 크기·기기 흉내 바꾸기 금지.
- `errors`를 `--reload` 없이 부르기 금지(사람 입력을 기다리며 멈춘다).
- 실사용 Chrome의 쿠키·프로필 변경 금지.
