# 리눅스(WSL2) 검증 기록

시청자 배포물이 리눅스에서 도는지 실측한 기록. 목적지는 Windows WSL2, 1차 실험대는 Ubuntu 24.04 VM(`ubuntu-agent`, 화면 없음 = WSL2·VPS와 같은 조건).
갱신 규칙: 날짜별로 아래에 추가. 되는 것/안 되는 것을 도구별로 적는다.

## 2026-09-07 — 1차: Ubuntu VM, 헤드리스(로그인 없이 되는 범위)

### 환경
Ubuntu 24.04.4 server, Node 24.20(nvm), Claude Code 2.1.263, Codex 0.153.4, Gemini CLI 0.58.0, bun 1.4.2, agy 1.1.27, python 3.12, jq·git·tmux 있음.
비대화형 SSH에서도 잡히도록 `/usr/local/bin/{claude,codex,gemini,bun,agy}` 링크.

### 되는 것
- **플러그인 마켓 추가·설치가 로그인 없이 된다**: `claude plugin marketplace add netwaif/loadout` → `claude plugin install loadout@loadout`. multi-agent-starter·claude-plugins-official(discord)·discord-harness-installer 전부 동일. `claude plugin list`에 4개 enabled.
- **loadout 0.5.1**: `store.py --list`, `--pick karpathy,agent-loop --yes`(CLAUDE.md+딸린 파일), `--doctor`(구조 이상 없음), `--remove agent-loop`, `--flavor codex`(AGENTS.md) 전부 exit 0. macOS 전용 코드 없음.
- **multi-agent-starter 3.6.0**: `init.py --flavor claude|codex|antigravity --target … --yes` 셋 다 validate PASS(13·14·15개). `call_worker.sh` 전제(bash·jq·timeout·mktemp) 전부 있음, `bash -n` 통과. KNOWN_ISSUES KI-3(네이티브 Windows)는 WSL2에선 해당 없음.
- **discord 플러그인 0.0.4**(공식): bun 설치 후 `bun install` 114 packages, `bun run start`가 토큰 요구 메시지까지 정상 도달(`~/.claude/channels/discord/.env`).
- **discord-harness-installer 0.1.14** `harnessctl.py preflight`: macOS 한 줄만 FAIL, git·tmux·node·bun·claude·codex·agy·discord 플러그인 전부 OK. `fetch`로 정본 3레포 핀 체크아웃 정상(discord-multiagent v0.1.1, codex-discord v0.1.6, usage-coach v0.1.2 → `~/.local/share/discord-harness/repos`).
- **agy(Antigravity CLI)**: 공식 설치기 `curl -fsSL https://antigravity.google/cli/install.sh | bash`가 리눅스 x86_64 바이너리를 `~/.local/bin/agy`에 놓는다(210MB, 1.1.27 = 맥과 같은 버전).

### 안 되는 것 / 포팅 필요
- **디스코드 하네스는 리눅스에서 설치 불가(현재)**. 두 층에 macOS 의존:
  1. `harnessctl.py:88` `sys.platform == "darwin"` 아니면 preflight FAIL(설치 시작 자체가 막힘).
  2. 정본 3레포가 봇 자동 기동을 **launchd LaunchAgent**(plist + `launchctl bootstrap gui/$(id -u)`)로 한다 — `discord-multiagent/scripts/install-autostart.sh`, `codex-discord/scripts/install.sh`(plist 3종 tui·daemon·gemini)·`uninstall.sh`, `usage-coach/scripts/install.sh`·`uninstall.sh`. `bot-restart.sh`는 `~/Library/LaunchAgents/*.plist`를 훑는다. harnessctl 자체도 `plistlib`로 수다 클로드 plist를 만든다.
  → 리눅스 대응 = systemd user unit(`~/.config/systemd/user/*.service`, `systemctl --user enable --now`, `loginctl enable-linger`)으로 분기. WSL2는 systemd가 기본 꺼져 있을 수 있음(`/etc/wsl.conf` `[boot] systemd=true`) — WSL2 단계에서 확인.
  3. 사소: tmux 경로 후보 `/opt/homebrew/bin/tmux`(리눅스에선 which로 잡히므로 무해), 힌트 문구 `brew install …`, 매뉴얼의 `pbpaste > .bot-token-*`(리눅스는 `xclip -o`/`wl-paste` 또는 편집기로 저장 안내 필요).
- **세션 단위 검증(스킬을 말로 호출)은 로그인 뒤**: VM에 `claude`·`codex`·`agy` 로그인이 없다(`claude auth status` loggedIn=false). 화면 없는 VM이라 브라우저 OAuth는 사용자 pane에서 URL 열기·코드 붙여넣기로 진행해야 함. 맥 세션 복사 금지(구글은 회전 충돌).

### 다음
- 사용자가 VM에서 로그인(claude → codex → agy) 후: multi-agent-starter "멀티 에이전트 시스템 구성해줘"·loadout "구성 골라 담아줘" 세션 실행, 클로드 워커 1회 dispatch.
- 디스코드 하네스 리눅스 분기(systemd)는 별도 작업으로 — 정본 3레포+설치기 4곳.
- WSL2(Boot Camp Win10)에서 같은 절차 재확인 → 그 뒤 멤버 공지.

## 2026-09-07 — 2차: 로그인 뒤 세션 단위(codex·agy)

- **codex 로그인** Device Code 방식으로 완료(`codex login status` = Logged in using ChatGPT). **agy 로그인** 완료(`~/.gemini/antigravity-cli/antigravity-oauth-token`). **claude 로그인은 안 됨** — `~/.claude/.credentials.json` 없음, `claude auth status` loggedIn=false. 재시도 필요.
- **agy 제미나이 워커 경로 OK**: 격리 tmp에서 `agy --prompt "…"`(backends.json gemini cli 정의 그대로) → 응답 정상, exit 0. 리눅스 x86_64 바이너리로 맥과 동일 동작.
- **codex 오케스트레이터(flavor codex) 읽기 OK**: `codex exec --skip-git-repo-check` 비대화형으로 AGENTS.md를 읽고 워커 3종 역할 요약. 주의 2가지 — SSH 비TTY에서는 `</dev/null` 필요(stdin 대기), git 저장소가 아니면 `--skip-git-repo-check` 또는 `git init`.
- **codex 셸 샌드박스 리눅스 차단(해결됨, 아래 4차)**: 셸 명령 실행 시 `bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted`. 원인 = Ubuntu 24.04 `kernel.apparmor_restrict_unprivileged_userns=1`(비특권 user namespace 제한). `apt install bubblewrap`(0.9.0)만으로는 해결 안 됨. 후보: `sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0`(+`/etc/sysctl.d/`에 영구화) 또는 bwrap용 AppArmor 프로필. **WSL2 커널은 AppArmor 제한이 다르므로 WSL2에서 재확인 필수** — 멤버 공지문에 "codex 셸이 막히면 이 sysctl" 문구 후보.
- 도구 검증 중 자동 승인 우회 문구가 든 `codex exec --full-auto` 실행은 이 세션(auto mode)에서 차단됨 — 오케스트레이터 1사이클 실행(승인 포함)은 사용자 pane에서 대화형으로 하는 게 맞다.

## 2026-09-07 — 3차: claude 로그인 뒤 세션 단위(스킬을 말로 호출)

- claude 재로그인 완료(`~/.claude/.credentials.json`, authMethod claude.ai). 비대화형 `claude -p … --permission-mode acceptEdits --allowedTools "Skill,Bash,Read,Write,Edit,Glob,Grep"`로 검증.
- **multi-agent-starter 스킬 OK**: "멀티 에이전트 시스템 구성해줘"(flavor claude, 대상 `~/lab/s-mas`) → 33개 파일, validate 13개 PASS. 사용자 플러그인 스킬이 리눅스 세션에서 정상 트리거·실행.
- **loadout 스킬 OK**: "CLAUDE.md 구성 골라 담아줘"(karpathy·session-handoff, 대상 `~/lab/s-loadout`) → CLAUDE.md+SESSION.template.md, doctor 이상 없음.
- 남은 세션 검증: 워커 dispatch 1사이클(승인 포함)은 gate G0가 인터랙티브 전용이라 사용자 pane 대화형에서. codex 셸 샌드박스는 sysctl 적용 뒤 재확인.

## 2026-09-07 — 4차: codex 셸 샌드박스 해결

- `sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0` + `/etc/sysctl.d/99-codex-bwrap.conf` 영구화(사용자 pane에서 실행) → `codex exec`가 셸 명령(`ls _shared/adapters`) 정상 실행. bubblewrap 0.9.0은 설치돼 있음(없으면 codex가 번들 bwrap 사용, 경고만).
- 결론: Ubuntu 24.04에서 codex 워커를 쓰려면 이 sysctl 한 줄이 필수. 멤버 공지·매뉴얼 후보 문구: "codex가 `bwrap: loopback: Failed RTM_NEWADDR`로 막히면 위 sysctl".
- 이로써 loadout·multi-agent-starter는 리눅스에서 생성기·스킬 세션·백엔드 3종(claude 로그인, codex 셸, agy 응답) 전부 확인. 미확인은 워커 dispatch 1사이클(대화형, 사용자 pane)뿐.

## 2026-09-07 — 5차: agentlayer 리눅스 빌드(브랜치 linux-port)

스펙 `docs/superpowers/specs/2026-09-07-linux-port-design.md`, 계획 `docs/superpowers/plans/2026-09-07-linux-port.md`. 크로스빌드(`GOOS=linux GOARCH=amd64`, -s -w) 바이너리를 VM `~/.local/bin/agentlayer`에 넣어 실측.

### 되는 것
- `agentlayer version`(linux/amd64), `help` 첫 줄 "agentlayer — tmux 멀티 에이전트 관제탑"(iTerm2 문구 없음).
- `agentlayer init`: claude hook 3종·codex notify+hooks.json+AGENTS.md·gemini GEMINI.md+hooks.json+settings.json·chrome-devtools MCP 3종·스킬 2개 설치. iTerm2 절은 앱이 없어 건너뜀. tmux 팝업 두 줄 안내는 그대로.
- **hook 전이**: VM tmux 세션 `t2`에서 `claude` 대화형 실행 → `agentlayer status`에 `[idle] claude t2 ~/lab/s-mas` → 질문 뒤 `[DONE]` → `/exit` 뒤 `[dead]`. `claude -p`도 기록 파일(`agents/claude-0.json`) 생성 후 dead. 관제탑 핵심 경로가 리눅스에서 동작.
- **엔진 다운로드**: `agentlayer browser`가 Chrome for Testing 152.0.7977.82 linux64(약 290MB 실행 파일)를 받아 Go unzip으로 풀고 실행 권한까지 정상(`chrome-linux64/chrome`).
- 리눅스 전용 문구 두 가지 실측: `cookies import x.com` → "cookies import는 macOS 전용입니다 … 에이전트 브라우저 창에서 직접 로그인" (브라우저를 띄우기 전에 거름). `agentlayer browser`(무화면) → "에이전트 브라우저는 창이 필요합니다 — 디스플레이가 없습니다(DISPLAY·WAYLAND_DISPLAY 비어 있음). WSL2는 WSLg…" (첫 실측에선 Chrome X 오류 원문이 그대로 나와 사전 검사를 추가함).
- `go test ./...` 맥 통과, `GOOS=linux` amd64·arm64 빌드 통과. `install.sh` dry-run이 릴리즈 URL을 올바르게 조립(`AGENTLAYER_VERSION`·`AGENTLAYER_DRY_RUN`·`AGENTLAYER_BIN_DIR`).

### 미확인(WSL2에서)
- 브라우저 창 실제 기동·FX·pick·shot(WSLg 필요). 데스크톱 알림 `notify-send`(WSL2엔 보통 없음 → 조용히 생략되는지).
- `install.sh` 실설치는 linux 자산이 포함된 다음 릴리즈 뒤에.

## 2026-09-07 — 6차: 디스코드 하네스 리눅스 서비스 층(systemd)

정본 4레포에 launchd→systemd 사용자 유닛 분기. 태그 discord-multiagent v0.1.2, codex-discord v0.1.7, usage-coach v0.1.4, 설치기 0.1.15(pins 갱신). 스펙·계획은 discord-harness-installer `docs/superpowers/`.

### 되는 것(VM E2E, systemd --user running)
- preflight: `[OK] OS: linux`, `[OK] systemd --user: running`, 도구 전부 OK. (WSL2면 systemd 미가동 시 FAIL+wsl.conf 안내, WSL 감지 시 "터미널 열어 두기/VPS 권장" INFO)
- fetch 새 핀, plugins(multi-agent-starter·folder-bot) 설치, pair(가짜 토큰) .env 조립.
- install --autostart --dashboard: 유닛 6개 생성·enable·심링크, `loginctl enable-linger` = yes. orchestrator·chat-claude tmux 유닛 active, usage-coach 타이머 active(waiting), codex-discord-daemon `activating (auto-restart)` = Restart=always 증명.
- remove: 유닛·사이드카·심링크·repos 전부 정리, .env·chat/·tasks/·~/.config/usage-coach 보존.
- 각 레포 테스트 리눅스 케이스 추가 통과(usage-coach 8, codex-discord install-linux+uninstall+57 node, discord-multiagent autostart-linux+manifest, 설치기 44). 테스트에 launchctl/systemctl 호출 시 즉시 실패하는 안전장치(실기기 오염 방지 — 실제로 첫 시도에서 맥 LaunchAgent를 내려 복구함).

### 봇 연결 실패는 전부 하위 원인(리눅스 포팅 무관), 실측 확인
- codex-discord-daemon/gemini 크래시 = 가짜 Discord 토큰(진짜 토큰이면 맥에서 "로그인: Codex Bot" 확인됨).
- usage-coach-dashboard 실패 = 가짜 웹훅 → 카드 POST에서 HTTP 404(코드는 끝까지 정상 실행, discord_dash.py:452). 진짜 웹훅이면 동작.
- codex-discord-tui 실패 = **codex TUI 첫 실행 신뢰 프롬프트**. 새 CODEX_WORKDIR에서 codex TUI가 "Do you trust the contents of this directory?"를 띄우고 입력 대기 → tui-up.sh가 보낸 더미 턴 첫 글자를 프롬프트가 삼켜 롤아웃 미생성 → 180초 타임아웃. **OS 무관·프레시 설치 공통**(맥은 그 폴더를 과거에 한 번 신뢰해 안 겪음). `[projects."/home/soonho"] trust_level="trusted"`는 하위 폴더를 안 덮음(codex 신뢰는 경로 접두어가 아님). → 후속: codex-discord install/tui-up이 CODEX_WORKDIR를 codex 신뢰 목록에 선등록(또는 tui-up이 트러스트 프롬프트에 자동 응답). 이번 커밋 범위 밖(사용자 보고).

### 곁다리로 잡은 agentlayer 버그(고침, b83aeb1)
- `agentlayer init`의 codex notify 삽입이 최상위 키 다음 줄이 바로 `[section]`일 때 앞 줄에 붙어 `approvals_reviewer = "auto_review"notify = [...]`로 config.toml을 깨뜨림 → codex가 config 로드에서 죽음(TUI·exec 전부). 삽입 위치를 헤더 줄 시작으로 옮겨 수정, 재현 테스트 추가. VM config는 수동 복구함.

### 6차 후속: codex 신뢰 프롬프트 해결(codex-discord v0.1.8)
- install.sh가 CODEX_WORKDIR를 `~/.codex/config.toml`에 `[projects."<wd>"] trust_level="trusted"`로 선등록(섹션 EOF 추가라 안전·멱등·.bak). tui-up.sh가 `--dangerously-bypass-hook-trust`를 붙인다(hooks.json 있을 때). 양 OS 공통(프레시 설치는 맥도 겪음).
- VM 실측: config 선등록+플래그 조합으로 codex TUI가 디렉터리·hooks 프롬프트 없이 "Ask Codex to do anything" 입력창까지 기동. 설치기 pins codex-discord v0.1.8, 0.1.16.

## 2026-09-07 — 7차: Win10 WSL2 실기(사용자 세션, v1.4.0-rc1 설치본)

환경: Win10 Pro 19045 + 스토어판 WSL 2.5.7(WSLg 1.0.66) + Ubuntu-24.04(새 설치, systemd 기본 on). 원 보고서 = NAS `/Volumes/private/mac-to-win10/RESULT-wsl2-20260907.md`(스크린샷 7장 동봉).

### 되는 것
- install.sh(`AGENTLAYER_VERSION=v1.4.0-rc1`) → `~/.local/bin/agentlayer`, init(claude hook 5종·codex notify+hooks·GEMINI.md·gemini settings.json·MCP 3종·스킬 2개), tmux 팝업 `C-b a`, TUI `s`·`b`.
- claude hook 전이 idle→WORK→DONE→dead 전부.
- 에이전트 브라우저: Chrome for Testing 152 linux64 창이 **윈도우 데스크톱에 뜸**(WSLg X11). agent-browser 스킬로 열기·스크린샷·콘솔·지목(`pick --once`)·FX(테라코타 테두리+배지) 동작.
- 멤버 배포물: loadout 0.5.1·multi-agent-starter 3.6.0 스킬 세션 PASS(validate 13). codex 셸 샌드박스 **sysctl 불필요**(WSL2 커널은 AppArmor userns 제한 없음, bubblewrap 번들 사용).
- 디스코드 하네스: preflight 전부 OK, systemd 사용자 유닛 6개+timer, codex 봇 신뢰 프롬프트 없이 기동, remove 뒤 잔존 0. `/etc/wsl.conf` 수정·`wsl --shutdown` 불필요.

### 고친 것(이 커밋)
- **npm으로 깐 codex가 관제탑에서 영원히 `[dead]`** — pane 전면 프로세스가 `node` 래퍼라 `DetectKind`가 못 알아봄. `internal/scan/proc.go`: 래퍼 pane이면 `ps -axo pid=,ppid=,args=`로 pane_pid 자신·자식의 인자(`bin/codex`, `@openai/codex`, `bin/gemini`…)로 2차 판정. npm gemini-cli도 같이 해결.
- **agy 훅 미등록** — init이 `~/.gemini/config/`가 있을 때만 `config/hooks.json`을 썼는데 갓 설치한 agy는 그 폴더가 없다. `main.go agyInstalled`: `config/`·`antigravity-cli/`·PATH의 `agy` 중 하나면 등록(폴더 생성). agy는 `/hooks`도 같은 파일에 쓴다(agy 릴리즈 노트 확인). WSL 쪽은 `agentlayer init` 재실행이면 된다.
- **`browser`가 디스플레이 검사 전에 엔진 200MB를 받음** — `displayAvailable`을 `EnsureEngine` 앞으로.
- **CfT 공유 라이브러리 누락** 첫 기동 실패에 apt 한 줄 힌트(`launchHint`), README 리눅스 절에 apt+`fonts-noto-cjk` 안내(FX 배지 한글이 □로 나오던 것).
- [discord-multiagent v0.1.3] `scripts/bot-up.sh` BSD `stat -f %m` → 리눅스 `stat -c %Y`, MCP 로그 경로 `~/.cache/claude-cli-nodejs` 분기(chat-claude 봇이 `line 44: File: unbound variable`로 즉사하던 것). 설치기 pins 갱신.

### WSL2에서만 다른 점(참고)
- 세 CLI 로그인 모두 "브라우저 열기"가 WSL 안에서 안 뜸(xdg-open 없음) → URL을 윈도우 브라우저에 복사. `127.0.0.1:<port>` 콜백은 localhost 포워딩으로 정상.
- nvm이 `~/.bashrc`에만 PATH를 넣어 비대화형 `bash -lc`에서 node/claude가 안 보임 → `~/.profile`에도. 훅·유닛은 절대 경로라 무관.
- `wsl.exe -- bash -lc`(비대화형)에서는 `systemctl --user`가 "Failed to connect to bus" — tmux/로그인 셸 안에서는 running.
- CfT 툴바에 프로필 아바타가 없어 "AgentLayer" 프로필 이름이 안 보임(맥과 동일한지 미확인). `agentlayer browser`(인자 없음)를 브라우저가 떠 있을 때 다시 실행하면 붙어 있음, `browser errors`는 Enter 대기 — 자동화 스크립트에서는 timeout 필요.

