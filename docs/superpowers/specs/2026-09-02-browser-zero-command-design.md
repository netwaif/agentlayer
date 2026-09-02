# 브라우저 기능 "명령 0개" 설계 (2026-09-02)

## 배경
에이전트 전용 브라우저(browser 서브커맨드 8개 + chrome-devtools MCP)는 기능이 갖춰졌으나
시청자는 명령을 외우기 어려워한다. 원칙: **관제탑(TUI)이 표면, CLI는 배관**. 명령은
init·TUI·스킬이 대신 부르고 사람은 키 하나만 누른다.

## 1. init 자동화 — 에이전트 제어를 명령 0개로
- `agentlayer browser mcp-serve`: Chrome이 없으면 기동(browser.Connect) 후
  `npx chrome-devtools-mcp@latest --browserUrl=http://127.0.0.1:<port>`로 exec(stdio 승계).
  MCP 클라이언트가 이 명령을 서버로 띄우므로 "브라우저 기동" 단계가 사라진다.
- `agentlayer init`이 세 CLI에 MCP 서버 `chrome-devtools`를 등록한다(설정 파일 직접 편집,
  기존 훅 설치와 같은 패턴: 백업 `.agentlayer.bak`·멱등·기존 설정 우선·dry-run).
  - claude: `~/.claude.json` `mcpServers.chrome-devtools = {type:stdio, command:<bin>, args:[browser, mcp-serve]}`
  - codex: `~/.codex/config.toml` `[mcp_servers.chrome-devtools]` 섹션 append
  - gemini: `~/.gemini/settings.json` `mcpServers.chrome-devtools`
  - 해당 CLI 설정 디렉터리가 없으면 건너뜀. `browser mcp`는 수동 안내용으로 유지.

## 2. 관제탑 키
- `b` 지목: 선택 에이전트를 대상으로 `browser pick --agent <id>`를 tea.ExecProcess로 실행.
  대상이 정해져 있어 후보 선택 프롬프트가 없다. 탭이 없으면 에러 표시.
- `s` 캡처: 활성 탭 스크린샷을 선택 에이전트 pane으로 전송(`shot --send --agent <id>`).
- `p` 프리뷰: 선택 에이전트의 CWD 아래에서 listen 중인 dev 서버를 ⎇브랜치 창으로 연다.
  행에 `🌐:3000` 뱃지 표시. 감지는 lsof 스캔, refresh마다가 아니라 10초 간격.
- errors·cookies·mcp는 CLI에만 남긴다(에이전트는 MCP 콘솔 도구, cookies는 드묾).

## 3. 스킬 문단
orchestration 스킬(init이 설치)에 "브라우저" 절 추가: chrome-devtools MCP를 자기 탭에서 쓸 것,
내장 브라우저 스킬(codex control-in-app-browser) 금지, localhost 주소는 링크로 찍을 것
(사용자가 ⌘-클릭), 사용자가 원격이면 `agentlayer browser shot --notify`.

## 4. iTerm2 링크 라우팅
init이 `defaults read com.googlecode.iterm2 "New Bookmarks"`에서 `agentlayer browser open`
액션 유무를 감지해, 없으면 수동 절차(Smart Selection 규칙 4단계)를 출력한다.
자동 주입은 iTerm2가 실행 중이면 plist를 되써 덮이므로 **하지 않는다**(2026-09-02 결정).
필요해지면 `agentlayer init --iterm2`(iTerm2 종료 확인 + plist 백업)로 별도 추가.

## 범위 밖
Playwright MCP 등록, 프로필 다중화, 원격 스트리밍.
