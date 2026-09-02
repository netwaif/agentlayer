# 에이전트 브라우저 영상 시연 4종 (2026-09-02, v1.3.0 후보 = browser-mcp 브랜치)

공통 전제
- 로컬 바이너리 최신(`make install`), `agentlayer init` 완료(hook·MCP 3사·스킬), `C-b a` 팝업 + client-resized 훅.
- 전용 Chrome은 첫 도구 호출 때 자동 기동. 촬영 전 `pkill -f browser-profile`로 깨끗이 시작하면 "자동으로 뜬다"가 화면에 남는다.
- 촬영 가이드(SESSION.md 2026-08-26): 별도 tmux 서버 금지, 더미 폴더 세션 사용.
- 시연 저장소: `~/ai-folder/demo/browser-demo` (정적 index.html, 파란 CTA `.cta.primary`가 지목 대상, 브랜치 master).
- 실사용 Chrome과 구분: 전용 Chrome은 다크 grayscale 테마 + 프로필명 AgentLayer. 북마크바로도 구분 가능.

## A. 기본 루프 — 링크 → 지목 → 수정 → 스스로 확인 (본편)
준비: 더미 세션 `demo-a`에서 `cd ~/ai-folder/demo/browser-demo && claude` (hook이 등록하므로 관제탑에 뜸).
1. 프롬프트: `이 폴더 dev 서버 8100으로 백그라운드로 띄우고 주소 알려줘`
   기대: 몇 초 안에 전용 Chrome에 `localhost:8100` 탭 자동 오픈(hook autopreview). 관제탑엔 `🌐:8100` 뱃지.
   (안 뜨면 `C-b a` → 행 선택 → `p`. 백업: 터미널 주소 ⌘-클릭 → Smart Selection 규칙으로 같은 브라우저.)
2. `C-b a` → `demo-a` 행 → `b` → 브라우저에서 파란 "지금 시작하기" 버튼 클릭 → 오버레이에 `브랜드 액센트 색으로 바꾸고 모서리 더 둥글게` → Enter.
   기대: 에이전트 pane에 "브라우저 요소 수정 요청: … (md·png 경로)" 한 줄. 에이전트가 md 읽고 CSS 수정 → chrome-devtools MCP로 탭 리로드해 확인.
3. `C-b a` → 같은 행 → `s` → 캡처가 에이전트에게 감 → "확인했다" 답.
포인트 멘트: 사람이 보는 브라우저 = 에이전트가 조작하는 브라우저. pick 산출물은 `~/.local/state/agentlayer/picks/`.
리스크: pick 진입 뒤 돌아가기 = 터미널 esc/q 또는 오버레이 Esc. 요소 png는 문서 좌표 clip(레티나 수정 완료).

## B. 3사 A/B — 브라우저 하나를 셋이 나눠 쓴다 (훅)
준비: 코디네이터 세션(`demo-b`, `~/ai-folder/demo/browser-demo`에서 claude). codex·agy 로그인 상태 확인. gemini는 agy(무료 티어 stock은 죽음).
1. 프롬프트(`/orchestration`):
   `worker 3개(claude·codex·gemini)로 같은 태스크 A/B: "hero 섹션을 더 대담하게 — 헤드라인 크기·CTA·카드 톤 자유". 각자 자기 worktree에서 python3 -m http.server를 백그라운드로 띄우되 포트는 claude 8101, codex 8102, gemini 8103. 완료 후 주소를 REPORT.md에 적어.`
   기대: `wt new` 3개 → 관제탑에 worker 3행(⎇ agent/…) → 각 서버가 뜨는 대로 ⎇브랜치 제목 창이 자동으로 열림(⎇가 제목에 보이는 게 B의 핵심 컷).
2. 세 창을 나란히 비교 → 마음에 드는 worker 행에서 `b`로 세부 수정 지시(라우팅이 그 worker pane으로).
3. `agentlayer wt merge <이름>`(y 확인) → `wt clean`.
리스크: worker 부팅 8~60초(편집 컷). 스킬이 "자기 탭에서만" 규칙을 알고 있음. 서버 포트 충돌 시 lsof로 정리. 시간 없으면 claude·codex 2개로 축소.

## C. 폰에서 원격 디버깅 — 디스코드 하네스 (훅)
준비: 디스코드 멀티에이전트 봇 세션 살아 있는지(`agentlayer status`에 claude-discord/orchestrator). 그 세션의 Claude는 user scope MCP라 chrome-devtools 사용 가능. 알림 웹훅(폰 Discord) 설정돼 있음. Mac 화면은 잠금 OK, 로그아웃 NO.
1. 폰 Discord에서: `chrome-devtools MCP로 http://localhost:8100 열고, 첫 화면의 CTA 버튼 색이 브랜드 색(#d97757)인지 확인해. 확인 결과를 /Users/soonho/.local/bin/agentlayer browser shot --notify 로 보내줘`
   기대: 에이전트가 MCP로 탭 열고 스타일을 읽음 → 스크린샷이 폰 Discord 알림 채널에 첨부로 도착.
2. 폰에서 후속: `그 버튼을 브랜드 색으로 고치고 다시 shot --notify` → 수정본 스크린샷 도착.
포인트 멘트: Orca 같은 앱은 원격 스트리밍이 필요하지만, 여기선 스크린샷 한 장이 폰으로 온다. 내장 브라우저 런타임(node_repl)이 아니라 chrome-devtools를 지목하는 게 요령.
리스크: 봇 세션의 PATH가 최소라 `agentlayer`는 절대 경로로. 디스코드 응답 지연은 편집.

## D. (제외)

## E. 로그인 유지 실전 — x.com에서 Anthropic 최신 소식 (훅 + 실용)
준비(촬영 전, 1회): `agentlayer browser cookies import x.com` → macOS Keychain 팝업 "항상 허용". 실사용 Chrome Default 프로필에 x.com 쿠키 75개 확인됨(2026-09-02). 가져온 뒤 전용 Chrome에서 x.com이 로그인 상태인지 눈으로 확인. 계정 화면 노출 부분은 마스킹.
1. 아무 클로드 세션(예: demo-a)에 프롬프트:
   `chrome-devtools MCP로 https://x.com 열어(이미 로그인돼 있음). 내 팔로잉 중 Anthropic 계정(@AnthropicAI)을 찾아 들어가서 최근 게시물 중 Claude 관련 소식 5개를 날짜·요지·링크로 정리해줘. 다른 탭은 건드리지 마.`
   기대: 에이전트가 검색/프로필 이동/스크롤을 MCP로 수행, 정리본 출력.
2. 선택: `b`로 특정 게시물을 찍어 `이 글 한국어로 요약해` → 지목 라우팅으로 요약.
포인트 멘트: 2FA 재로그인 없이 쿠키만 가져온다(전체 프로필 복사 아님). 에이전트가 로그인된 웹을 다룰 수 있다는 것.
리스크: x.com 봇 감지로 로그인이 풀릴 수 있음(그때는 전용 Chrome에서 수동 로그인 1회 — 프로필에 보존됨). 스크롤 많은 페이지라 MCP 호출이 여러 번 → 편집.

## 촬영 중 버그 창구
agentlayer-dev 세션(browser-mcp 브랜치)에 증상 붙여넣기. 고치면 `make install`로 즉시 반영(재기동 필요한 건 전용 Chrome `pkill -f browser-profile`뿐).
