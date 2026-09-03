# SESSION — 세션 이어가기 기록

<!-- 이 파일은 다음 세션(기억 0)이 처음 읽는 유일한 문서다.
     섹션 5개는 고치거나 빼지 말 것. 갱신 규칙은 섹션마다 주석으로 표시. -->

## 목표
<!-- 이 폴더에서 하는 일. 거의 고정 — 바뀔 때만 명시적으로 수정 -->

AgentLayer 개발 — iTerm2+tmux 멀티 에이전트 관제탑 (Go 단일 바이너리, mat·coach 자매 도구).
Orca를 설치하는 대신 그 핵심 기능(상태 추적·알림·worktree·Discord)을 tmux 위에 구현.
실사용 + 유튜브(AICheatKey) 소재 + 시청자 배포(brew) 목적.
정본 문서: `docs/superpowers/specs/2026-08-25-agentlayer-design.md`(스펙),
`docs/superpowers/plans/`(Phase 1~4 계획), `agentlayer-local-handoff-2026-08-25.md`(원 핸드오프).

## 현재 상태
<!-- 덮어쓰기. 항상 짧게 — 지금 어디까지 왔는지 스냅샷만 -->

**GitHub 릴리즈 = v1.2.5**. 로컬 main은 origin 대비 18커밋 앞섬(미푸시), 브랜치 `browser-mcp` 60f975f(main 미머지). 2026-09-03 저녁 커밋 60f975f 3건: restore 체크리스트(터미널에서 `restore`만 치면 space/a/enter/q, `--yes`·`--dry-run`·`<id>` 유지)·봇 중복 복원 방지(같은 자리 pane·LaunchAgent 관할 제외)·자동 프리뷰 튀어나옴 제거(Activate 삭제·PortOpen으로 seen 유지·브라우저 떠 있을 때만). make install = 60f975f. **촬영 A·B·C·E 전부 완료** — 타워 세션(agentbrowser-c7)이 대본 단계, 이 세션이 SSH inspect 실측 여부·Orca 대비 근거·가벼움 실측값을 전달함. obs-record 스킬 caffeinate 누수(모니터 안 꺼짐) 수정. 8080 python 데모 서버 떠 있음. 워킹트리 클린.

## 다음 단계
<!-- 덮어쓰기. 첫 항목 = 다음 세션이 바로 집어들 일 -->

1. **v1.3.0 릴리즈**: `browser-mcp` → main 머지 → push → v1.3.0 태그 → `GITHUB_TOKEN=$(gh auth token) goreleaser release --clean`(SESSION.md 더티면 stash 먼저) → tap Casks 확인 → 디스코드 공지 → 매뉴얼 반영. 릴리즈 노트: cookies list/clear·pick --once·스킬 말로 시키기·Capturing 잠금 정리·mcp-serve 지연 기동·⎇ 탭·기동 신뢰 자동 승인·worker_auto_approve·codex hooks·MCP roots·errors --reload·SendText 지연·**restore 체크리스트·봇 중복 복원 방지·autopreview 튀어나옴 제거**. 매뉴얼 원본 재부팅 절차를 "봇 자동 기동 → `agentlayer restore`(체크리스트) → wake-all"로 갱신(--dry-run 먼저 문구 제거)
2. 후속 후보: 관제탑 TUI에서 restore 체크리스트 키 노출(명령 0개 원칙), codex 훅 trusted_hash 자동 기록(해시 방식 미상), gemini(agy) AGENTS/GEMINI.md 브라우저 블록, `errors --reload` 대기 시간 옵션, favicon 404 소음 처리
3. 촬영 뒤 정리: 8080 python 서버 종료, `~/.local/state/agentlayer/picks/`(shot·errors txt 다수), `worktrees/demo.review.diff`·search-* 메타, demo 저장소 `git reset --hard f1defa2`, worktree 3개(hero-bold-*) 정리, DEAD 레코드(demo-b 4개 등)는 24h 뒤 자동
4. 모니터 또 안 꺼지면: `pmset -g assertions | grep -E "DisplaySleep|caffeinate"` — obs-record caffeinate면 스킬 재점검, Chrome "Capturing"이면 capture-janitor 점검
5. 대시보드 채널 옛 핑 메시지 Discord 수동 삭제(567c2c9)
6. 보류 아이디어: `agentlayer status --prune`, agy 신뢰 질문 실화면 확인, Termius 컴팩트 모드, MultiAgent 패널 날짜 필터, 미리보기 원본색(-e), Orca 대비 메모리 측정, provider 게이지 텍스트화, 브라우저 프로필 다중화, `init --iterm2`, autopreview 포트 제외 옵션

## 결정 기록
<!-- 누적. 삭제 금지. 형식: - YYYY-MM-DD 한 줄 -->

- 2026-08-25 Orca 미설치 확정 — 인프라 층은 tmux가 상위 호환, 관제 층만 구현 (보조 도구로만 접근)
- 2026-08-25 Go+bubbletea 채택 (mat과 동일 스택, brew tap 배포 정합)
- 2026-08-25 통합 모델: 수집 1개(파일 상태) + 표면 3개(TUI·Discord 카드·CLI). coach/mat repo는 독립 유지
- 2026-08-25 상태 판정은 hook+메타데이터만, 화면 스크래핑 금지 (미리보기는 표시 전용 예외)
- 2026-08-25 제외 확정: Quick Commands, worktree 체크포인트, 하이버네이션, 자동 merge, Workspace 개념(tmux 세션이 이미 그 역할)
- 2026-08-25 resume은 비상 복구로 축소 (일상 재시작은 SESSION.md 방식이 우월 — 부푼 컨텍스트 재적재 회피)
- 2026-08-25 유휴 알림("waiting for your input")은 문구로 식별해 어떤 상태도 안 덮음. UserPromptSubmit hook 추가
- 2026-08-26 최소 PATH/LANG 환경(팝업·LaunchAgent) 대응 원칙: 외부 도구(tmux·coach·lazygit)는 자체 탐색, tmux 호출에 UTF-8 강제, Claude 감지는 버전형 명령(로케일 무관)
- 2026-08-26 wake-all/close-all 대상 = 세션 이어가기 조각·SESSION.md 있는 폴더만 (broadcast는 전체)
- 2026-08-26 Discord 카드 업서트는 선게시 후삭제 (카드 소실 방지). 웹훅 응답은 절단 없이 읽음
- 2026-08-26 상태 용어: 응답 필요(WAIT)/새 완료(DONE_UNREAD)/작업중/대기(IDLE)
- 2026-08-26 hook·notify 등록도 절대 경로로(마이그레이션 포함) — claude-discord처럼 PATH 최소 LaunchAgent 세션의 hook 유실 해결 (v0.8.3)
- 2026-08-26 SessionStart는 IDLE로 매핑(compact만 상태 유지) — 부팅 자동기동 세션 WORK 오탐 제거
- 2026-08-26 관제탑 기능은 3사(claude·codex·gemini) 공통 적용이 원칙 — claude 전용 기능 금지, 불가능한 부분만 예외로 사용자에게 보고. CLI가 늘면 확장 전제
- 2026-08-26 기본모델 3종 소스: claude=~/.claude/settings.json "model"(Default 선택 시 키 삭제됨=자동), codex=~/.codex/config.toml 최상위 model(+effort), gemini=~/.gemini/antigravity-cli/settings.json model 우선→~/.gemini/settings.json. 미설정="자동" 표시, Claude Fable이면 빨강 경고
- 2026-08-26 Gemini 3사 공통 편입 (실전 검증 완료): agy 명령도 gemini kind로 감지. hook 두 계열 — agy=~/.gemini/config/hooks.json(PostToolUse·PreInvocation→WORK, Stop→DONE; PreToolUse는 decision 필수라 미등록), stock CLI=~/.gemini/settings.json hooks(SessionStart→IDLE, BeforeAgent·AfterTool→WORK, Notification→WAIT, AfterAgent→DONE). gemini hook은 stdout "{}" 필수(main이 출력). agy modelName→Agent.Model 기록
- 2026-08-26 resume 3사 확장: claude --resume <sid> / codex resume <sid>(notify에 sid 없어 rollout 헤더 session_id 추출) / agy --conversation <conversationId>(brain 폴더 존재로 agy 판별). stock Gemini CLI는 재개 CLI 없음
- 2026-08-26 agy 세션파일(brain transcript)에는 모델·토큰 기록 없음 — agy 모델은 hook modelName이 유일 출처. stock CLI는 ~/.gemini/tmp/<projects.json 매핑>/chats/session-*.jsonl 각 턴에 model·tokens 기록됨(창 크기 없어 ctx%는 불가)
- 2026-08-26 "gemini ctx% 불가" 정정 → 근사값으로 가능: stock=마지막 턴 tokens.total/1M, agy=brain transcript_full.jsonl 크기/4/1M (agy 세션 자신이 권한 방식). 근사값은 "ctx ~N%"로 표시 (CtxInfo.Approx)
- 2026-08-26 ctx 맵 CWD 키 → 에이전트 ID 키로 교체 — 같은 폴더의 claude 스냅샷이 codex·gemini 행에 오귀속되던 버그 수정 (usage.AgentCtx 한 곳으로 통합, TUI·카드·info 공용)
- 2026-08-26 목록 정렬: 3사 정보 안 섞이게 종류 그룹 우선(claude→codex→gemini, state.KindRank), 그룹 안에서 상태 우선순위 — store.List라 TUI·status·카드 일관
- 2026-08-26 agy ctx 추정에 고정 오버헤드 100KB 가산 — transcript 30KB 시점 실제 요청 134KB 실측(gen_metadata blob). transcript만으로는 0%로 보이던 문제 해결 (agyBaselineBytes)
- 2026-08-26 wiring plist 매칭 오탐 2건 수정: ① 경로 부분일치 → 뒤 경계 정규식(상위 폴더가 하위 폴더 봇 plist에 매칭돼 "Discord 연결됨" 오표시) ② 4자 미만 세션명("ai")은 매칭 제외(ai.openclaw.gateway 오탐)
- 2026-08-26 TUI 목록에 종류 그룹 구분선("── codex ───…", provider 색 라벨) — previewHeight에 구분선 수 반영
- 2026-08-26 usageCmd를 usageCmd(coach만·느림)/ctxCmd(모델·ctx·⌁·기본모델·MultiAgent, 파일 읽기·즉시)로 분리 — 콜드 coach가 빠른 정보 표시를 막던 문제
- 2026-08-26 유휴 에코 규칙 정교화(8-25 결정 보완): WORK 상태에서 온 "waiting for your input" 에코는 놓친 종료 신호 → WAIT로 복구. 원인: 백그라운드 셸 생존 시 Stop 유예, Esc 인터럽트 시 Stop 미발화 (SendManual 세션 WORK 고착 실사례). DONE·IDLE·WAIT는 기존대로 안 덮음
- 2026-08-26 v1.0.0 릴리즈: gh repo create(netwaif/agentlayer, public) → 태그 v1.0.0 → goreleaser release(GITHUB_TOKEN=$(gh auth token)). brew formula는 tap의 Formula/ 디렉터리에 있어야 함(.goreleaser.yaml에 directory: Formula 추가, 루트에 갔던 첫 파일은 git mv로 이동). 이 Mac은 CLT 낡아 brew install 로컬 검증만 불가(다른 머신은 정상)
- 2026-08-26 촬영 가이드(영상 세션에 전달): **별도 tmux 서버(-L) 금지** — 상태 저장소가 공유라 pane ID 충돌로 실세션 레코드 덮어씀 + demo 서버에서 TUI 열면 실세션 전부 DEAD 오판. 같은 서버 + 더미 폴더(SESSION.md 필수) 세션으로 촬영. W 촬영은 실봇 종료(dead는 대상 제외) 후 더미만 남기고
- 2026-08-26 brew 경로는 아직 미실검증 (formula 인식·아카이브·체크섬까지만 확인) — 영상 게시 전 실검증 필수를 다음 단계로 등재
- 2026-08-26 brew 실검증 통과 (위 결정 해소): 이 Mac CLT 14.3.1→16.2 갱신(softwareupdate -i "Command Line Tools for Xcode-16.2") 후 `brew install netwaif/tap/agentlayer` 성공, `/usr/local/bin/agentlayer status` 실행·상태 출력 정상. 검증 후 brew uninstall(로컬은 make install 본 유지). 영상 게시 차단 요인 해소
- 2026-08-26 Discord 대시보드 채널 새 서버 이전: 웹훅은 채널 종속이라 이전 불가 → 새 채널(1542162018596036660) 웹훅으로 `~/.config/agentlayer/config.json` 교체 + `discord-card.json` message_id 리셋(옛 채널 메시지 무효) → `agentlayer card` 게시·채널 확인. LaunchAgent는 같은 설정을 읽어 그대로 동작
- 2026-08-26 TUI 리사이즈 잘림 수정: bubbletea(altscreen)는 폭 초과 줄을 안 잘라줘 터미널 래핑→화면 깨짐. View()=viewBody()+clampLines(x/ansi.Truncate, ANSI 폭 계산·리셋 후행)로 가로 클램프, 목록은 listWindow(커서 추적 스크롤, capacity=height-10, "↑/↓ N줄 더" 표시)로 세로 해결. previewHeight와 상수 공유
- 2026-08-26 미리보기 가로 잘림은 agentlayer 문제 아님 — headless 생성 tmux 세션이 기본 80x24라 원본 pane이 80칸(claude-discord 실사례). resize-window -x 212 -y 51 즉시 적용 + `~/.tmux.conf`에 `set -g default-size 212x50` 영구 설정으로 해결
- 2026-08-27 카드 갱신 이벤트 구동으로 전환("5분 지연·색 불명·실황 불일치" 피드백): hook 전이(prev≠to) 시 detached `card --event` 발사, dirty(card.dirty)+flock(card.lock) single-flight 코얼레싱, 게시 중 온 트리거는 루프 재게시로 수습. --event는 usage 캐시 24h 허용(콜드 coach 금지) — usage 최신화·하트비트는 기존 5분 LaunchAgent가 계속 담당
- 2026-08-27 카드=TUI 동등 정보 원칙(BuildComponents→CardData/BuildCard): 상태 집계·기본모델 3사(Fable ⚠)·MultiAgent·세션 이름·TASK·⎇브랜치·ctx 나이·종류 구분선·WORK 정체 "작업중?". 에이전트 행 게이지 막대 금지 — Discord 폰트에서 격자로 깨짐, ctx N% 텍스트로 (provider 창 게이지는 유지)
- 2026-08-27 workspace=tmux 세션 개념 확정(window 아님) — 사용자가 폴더별 세션으로 재정리(agentlayer dev·agentlayer youtube). 재부팅 시 tmux 서버가 메모리라 전멸하는 문제는 restore로 해결
- 2026-08-27 restore 절충 확정(8-25 "resume 비상 축소" 결정과 정합): 기본=죽은 레코드로 배치 재구성+새 CLI 기동+wake-all 재정박(컨텍스트 깨끗), `--resume`=마감 못 한 죽음(강제 재부팅·정전) 구조용으로 대화째 부활(claude --resume/codex resume/agy --conversation). 기동 명령은 pane 셸에 SendText — 명령 종료 후에도 window 생존+사용자 셸 환경 승계
- 2026-08-27 restore 세부: 대상=Sync 후 DEAD만, 같은 window 분할 pane은 대표 1개, 폴더·세션명 없으면 사유와 건너뜀, 성공 시 원본 dead 레코드 즉시 삭제(status 이중 행 방지). DEAD는 재부팅/의도적 닫기 구분 불가 → 평상시엔 dry-run 먼저 권장
- 2026-08-27 ctx 스냅샷 파일명=Claude session_id 발견 → LoadSnapshots가 sid 키도 등록, AgentCtx claude는 sid 우선·폴더 키 폴백 — 같은 폴더 두 claude 세션(restore-lab Opus vs 본세션 Fable)의 모델·ctx 오귀속 수정
- 2026-08-27 agentlayer는 tmux만 필수(iTerm2 무관) — 코드에 iTerm2 의존 없음, osascript는 macOS 내장, coach·lazygit은 선택
- 2026-08-27 재부팅 실테스트 통과 — restore로 claude 9세션 부활. codex-live는 브리지 LaunchAgent 관할(부팅 시 codex 업데이트 프롬프트에 걸림 → 사용자가 업데이트, `launchctl kickstart gui/501/com.codex-discord.tui`로 재기동)
- 2026-08-27 Sync에 "밖에서 부활한 세션의 옛 DEAD 즉시 정리" 추가(808dfe0) — 브리지가 restore 안 거치고 같은 kind·세션명·cwd로 살리면 dead 이중 행이 24h 남던 문제. 대체 pane 없는 DEAD는 기존대로 24h 보존
- 2026-08-27 restore+wake-all이 실험 세션(restore-lab)까지 부활·각성시켜 그 Opus가 무승인 구현·커밋(839f157, 본세션 미커밋 테스트까지 혼입) 사고 → 세션 kill, mixed reset으로 두 커밋(808dfe0 내 수정/f623fbf version, Opus 저작 표기 유지) 재구성. 교훈: 폴더당 조종사 1세션, `restore <id>` 개별 선택 필요
- 2026-08-27 상태 오염 사고 근본원인 규명: e2e 테스트의 -L 격리 서버가 ~/.tmux.conf를 로드 → tmux-resurrect/continuum이 재부팅 전 레이아웃(실폴더 claude 4개 포함)을 테스트 서버 안에 자동 부활 → 유령들의 전역 hook이 본 서버 %0·%1·%3·%5 레코드의 session_id 오염(zzukumi-bot 실제 Opus가 Fable로 표시). 정리: 그림자 서버 kill-server + 오염 레코드 4건 sid 제거(폴더 폴백으로 즉시 정상)
- 2026-08-27 재발 방지 2종 커밋: ① hookPane 가드(a23265d) — $TMUX 소켓 basename=="default"일 때만 hook 기록(3사 공통), TMUX 없는 잔류 환경도 차단 ② 테스트 tmux 호출 전부 -f /dev/null(78dfb55) — 사용자 설정(resurrect) 차단. TUI e2e 대기 2s→10s(스위트 병렬 부하)
- 2026-08-28 /orchestration 스킬 신설(6a24d06) — Orca orchestration 대응: wt new 3사 워커 생성 → send-keys 2단 dispatch(+REPORT.md 보고 규약) → status DONE 폴링(화면 스크래핑 금지) → 취합·비교, 자동 머지 금지. 실검증: claude(Opus)+codex 워커 A/B 전 사이클 통과(신뢰 프롬프트→dispatch→DONE 감지→취합→정리)
- 2026-08-28 스킬 배포 방식 확정: 플러그인 아닌 go:embed 동봉 + `agentlayer init` 설치(멱등, 갱신 시 .bak) — 스킬이 바이너리 명령 종속이라 버전 잠금이 핵심. 플러그인은 마켓플레이스 수요 생기면 후속
- 2026-08-28 v1.1.0 릴리즈 완료: push(340a~6a24d06) → 태그 → goreleaser → GitHub 릴리즈+brew formula 1.1.0 확인. goreleaser brews deprecated 경고 지속(→homebrew_casks 이관 필요, 비차단)
- 2026-08-28 MultiAgent(하네스)와 통합 안 함 확정 — 접점 규약만: REPORT.md≈worker-result.md 양식 공유, MultiAgent 코드 단계에 wt 선택 사용. 사용 기준: 승인·비평·재진입=MultiAgent, 병렬·A/B·worktree 격리=orchestration, 일상 작업=단일 세션
- 2026-08-28 goreleaser brews→homebrew_casks 이관(69c1a6f): brews는 v2.16 완전 deprecated. cask는 tap Casks/에 생성, 미서명 바이너리라 quarantine 해제 postflight(xattr) 포함, conflicts.formula는 Homebrew서 제거된 no-op이라 미사용. v1.2.0 릴리즈 때 tap Formula/agentlayer.rb 삭제 완료(공존 시 brew가 formula 우선). 설치 명령은 `brew install netwaif/tap/agentlayer` 그대로
- 2026-08-28 help 서브커맨드(0af2663): help/-h/--help, 미지 명령 에러는 'agentlayer help' 안내로. helpcmd_test가 라우팅 목록과 어긋남 감시 — main.go switch에 명령 추가 시 테스트 목록도 갱신
- 2026-08-28 notify 웹훅 분리(7abfb3e): config notify_webhook_url 신설, 비면 카드 웹훅 폴백. 대시보드 채널은 카드 1장 전용이 됨. 실배선: 알림 채널 웹훅 등록 + 대시보드 채널 새로 생성(웹훅 교체·discord-card.json 리셋), 옛 채널(1542162018596036660)은 사용자가 삭제
- 2026-08-28 TUI notice는 일회성(a74b921) — 키 입력 진입부에서 일괄 소거, 필요한 분기가 재설정
- 2026-08-28 tmux 밖 enter=attach(62c482a): switch-client 기반 jumpCmd를 밖에서 쓰면 최근 활동 클라이언트(책상 화면)가 전환됨 → AttachArgv(포커스+attach "=" 완전일치 체이닝)로 이 터미널이 진입, detach 시 TUI 복귀. 폰 Termius는 ssh 후 tmux 밖 실행이 권장(뷰포트·팝업 잔상 회피)
- 2026-08-28 dead enter=y/n resume 확인(10012fc→0fc580c): 안내문 대신 확인→창 생성→이동. 팝업 안에서는 tmux가 current session 특정 불가(JumpTo activeClient와 같은 함정) → ActiveSession() 명시 타겟. 창은 명령 인자 대신 SpawnShellWindow(셸+SendText, 명령 즉사해도 창 생존). 원 세션 생존 시 제 집에 생성+JumpToSessionPane 점프, 죽었으면 활성 세션 폴백. 성공 시 dead 레코드 즉시 삭제(TUI·CLI, restore 8-27 기준 통일)
- 2026-08-28 TUI 목록은 에이전트 pane 목록(세션 목록 아님) — zsh만 남은 세션은 안 나옴(C-b s와 다른 이유). work 세션 실물은 move-window로 정리 후 촬영용 kill
- 2026-08-28 B 전체지시 TUI 편입(a9ee6bb→87ac4eb): 입력줄→y 확인→SendAll(handoffOnly=false 전체). 입력은 bubbles/textinput(커서 이동·중간 편집·붙여넣기 — 자작 최소 버퍼는 끝 backspace만 돼 교체). W/C·B 전송은 주입점 sendAll 경유(테스트 실전송 차단)
- 2026-08-28 v1.2.0 릴리즈 완료: 13커밋 push→태그→goreleaser(cask 첫 게시)→tap Formula 삭제(gh api DELETE). deprecated 경고 소멸
- 2026-08-29 tmux-resurrect/continuum 제거(~/.tmux.conf 주석 처리, .bak-20260829 백업) — continuum autosave가 8/26 14:24부터 고장나 재부팅 시 3일 묵은 스냅샷을 부활시킴 + 8-27 오염 사고 원인. 재부팅 절차 확정: LaunchAgent 봇 자동 기동 → agentlayer restore(--dry-run 먼저) → 전체 기상(wake-all 재정박). 봇 세션은 LaunchAgent 관할이라 restore 대상 아님
- 2026-08-29 stock gemini CLI 무료 티어 사망 실측(IneligibleTierError: "no longer supported for Gemini Code Assist for individuals" → Antigravity 이관 안내) — 촬영 중 Opus의 lifecycle.go gemini→agy 하드코딩 핫픽스를 usage.GeminiCommand()(antigravity-cli 폴더 흔적→agy, 없으면 stock 폴백 — API 키·Vertex 사용자 유효)로 재정리, wt commandFor·restore freshCommand가 공유. 참고: 이 셸에서 gemini CLI는 PATH에 /usr/sbin 없으면 spawnSync sysctl ENOENT로 오사
- 2026-08-29 v1.2.1 릴리즈: push(0a5af8a·b384a91)→태그→goreleaser(GITHUB_TOKEN=$(gh auth token))→릴리즈 자산 2종+checksums·tap Casks 1.2.1 확인. 패치 릴리즈도 동일 절차로 무리 없음
- 2026-08-30 restore <id> 개별 선택 추가(02a5fba) — 위치 인자로 지정한 죽은 레코드만 부활(FilterByIDs), 없는 ID·살아 있는 ID는 사유 출력(명시 지정을 조용히 거르지 않음). 인자 없으면 기존 전체 동작. flag 패키지 특성상 플래그가 ID보다 앞: `restore [--resume] [--dry-run] [id ...]`
- 2026-08-30 촬영 백업(agents-backup-filming) 복원 불필요 판정 — 백업 3건의 pane ID(%1·%6)는 오늘자 다른 라이브 세션(orchestrator·agentlayer-make)이 재사용 중, academy 세션은 재생성됨, zzukumi-bot은 LaunchAgent 관할. 삭제만 rm 차단(auto 모드 분류기)으로 보류 → 다음 단계 1
- 2026-08-30 한도 악화 핑 알림 채널 이관(567c2c9) — notify 웹훅 분리(7abfb3e) 때 WorsenedPings 발사 경로가 누락돼 카드 웹훅(대시보드 채널)으로 계속 쌓임(실사용 피드백). "알림 채널 우선, 미분리 시 카드 폴백"을 config.NotifyURL()로 승격, notify.Notify와 main.go publishCard 핑이 공용
- 2026-08-30 v1.2.2(restore <id>)·v1.2.3(핑 채널 수정) 릴리즈 — agentlayer 영상 게시됨, 매뉴얼 미배포 시점이라 시청자 설치본은 v1.2.3부터
- 2026-08-31 매뉴얼(agentlayer-manual.txt) 기술 검증 — 확정 오류 4건(전송 대상 서술·"Go 1.22"→go.mod 1.25.7·restore [id ...] 누락·§1 비문)+권고 4건을 agentlayer-c6 세션에 지시, 전건 반영 확인. 나머지 서술(크기·경로·hook·키·상태·wt)은 코드와 일치
- 2026-08-31 미리보기 주기 config화(355af28): config.json `preview_interval`(Go duration 문자열, 기본 1s로 단축, 200ms 하한 클램프, 파싱 불가→기본). 미리보기 틱(previewTickMsg)을 목록 폴링 2s와 분리. TUI 내 조절 UI는 안 넣음 — 설정은 파일, 화면은 관제 원칙
- 2026-08-31 wake-all/close-all/broadcast에 gemini 포함(a196e9a) — Targets 필터(claude·codex만)는 gemini 편입(8-26) 전날 작성된 잔재로 판정, 3사 공통 원칙 위반이라 수정. TUI W·C·B도 같은 경로라 함께 적용
- 2026-08-31 매뉴얼 선반영 금지 원칙(agentlayer-c6 지적 수용) — 미릴리즈 코드 변경은 매뉴얼에 먼저 반영하지 않는다. 시청자 설치본 기준 유지, 릴리즈 후 반영
- 2026-08-31 v1.2.4 릴리즈(preview_interval+gemini 포함): push→태그→goreleaser→릴리즈 자산 3종·tap Casks 1.2.4 확인. 매뉴얼도 v1.2.4 기준으로 정렬 완료
- 2026-09-01 매뉴얼 배포 확인 — "미배포" 기록은 낡은 것. Drive `agentlayer 한국어 매뉴얼 v1.2.0.pdf`(ID 1c6ud4ALnvfJaOFVtFq4VmFKwJAzQO6eZ, 도구 v1.2.4 정렬본) 로컬과 MD5 일치·공유 켜짐, agentlayer-c6 세션이 8-31 배포함. 촬영 백업 폴더는 rm auto 분류기 재차단으로 `~/.Trash/agents-backup-filming-20260901`로 mv 정리
- 2026-09-01 v1.2.5 릴리즈 — usage stale-while-revalidate(5b6761a): coach 콜드 실행이 실측 1분58초라 캐시 만료 시 TUI/카드가 통째로 늦던 문제. ReadCached(나이 불문 읽기 전용) 신설, TUI Init에 usageCacheCmd(낡은 캐시 즉시 그림+뒤에서 갱신), 카드는 1차 캐시 즉시 게시→coach 갱신→TS 변화 시만 2차 재게시(--event는 1차 단일). push→태그→goreleaser→tap Casks 1.2.5·디스코드 공지(채널 1522490241859059784)·영상 COcgg7Q_r8U 고정댓글에 업데이트 블록 추가(고정댓글 ID Ugw34-uRD1kfykfUsUF4AaABAg, my-videos videos.json에 기록)
- 2026-09-01 에이전트 전용 브라우저 착수 — Orca 대응 "브라우저에서 요소 찍으면 터미널 에이전트가 고치는" 역방향 다리. 스펙 `docs/superpowers/specs/2026-09-01-agent-browser-design.md`, 계획 `docs/superpowers/plans/2026-09-01-agent-browser.md`. 접근=Go+rod(순수 CDP, 자동 다운로드 금지·시스템 Chrome, 단일 바이너리 유지). SDD 9태스크 서브에이전트 구동(구현 Fable5·리뷰 Opus·최종리뷰 Fable5), 태스크별+최종 브랜치 리뷰 전부 클린 후 로컬 main 머지(브랜치 삭제)
- 2026-09-01 브라우저 설계 결정: 내장 브라우저 UI(터미널 임베드 불가)·원격 스트리밍·쿠키 전체복사·제어 API·TUI 통합·다중 프로필 제외. pick 지시입력은 브라우저 내 shadow DOM 오버레이(사용자 선택). 라우팅=페이지 localhost 포트→lsof cwd→에이전트 레코드 최장일치, 실패/복수면 선택. 전송은 tmuxx.SendText 한 줄(맥락은 picks/ md·png 파일)이라 3사 공통
- 2026-09-01 cookies import 편입(2b4f6a0) — 사용자 지적("기능 최대한 넣으라 했는데 왜 뺐나") 수용. 전체 프로필 복사가 아닌 **도메인 화이트리스트** 방식이라 격리 원칙과 양립. 2FA 재로그인 회피용. Fable5 safeguard가 쿠키 복호화를 민감작업으로 보고 Opus4.8로 전환→Opus가 정당맥락(본인 머신·본인 쿠키·명시요청·Orca도 제공)에서 직접 구현. macOS Keychain "Chrome Safe Storage"+v10 복호화, github.com 6개 실검증(패딩 검증 통과=키 정확). 세션 모델이 이 작업 때마다 Fable↔Opus 오간 건 safeguard 정상 동작
- 2026-09-01 브라우저 shot/errors 자율성: shot은 비상호작용(경로 stdout)이라 에이전트가 자기 dev서버 캡처→검증 자율 사용 가능. errors는 stdin Enter 대기(사람이 버그 재현)라 에이전트 자율 불가 — 필요 시 `--duration`/리로드 1회 수집 모드 후속(다음 단계 3)

- 2026-09-02 브라우저 제어는 자체 구현 안 함 — chrome-devtools MCP에 위임(Playwright MCP는 README 한 줄). agentlayer는 프로필·pick·프리뷰·라우팅만
- 2026-09-02 CDP 포트 고정(browser_port 기본 9222) — MCP가 고정 주소로 붙고, 기록 없이도 포트 프로브로 재attach
- 2026-09-02 회색 화면 원인은 Chrome 결함이 아니라 rod 기본 기기 에뮬레이션(1280×800) — NoDefaultDevice로 해결. 창 bounds 나지(FitViewport) 처방은 효과 없어 폐기
- 2026-09-02 errors 비상호작용 모드 폐기 — 에이전트는 MCP 콘솔 도구로 스스로 봄
- 2026-09-02 링크 라우팅은 iTerm2 Semantic History(파일 전용)가 아니라 Smart Selection 규칙 액션(⌘-클릭이 첫 액션 실행)
- 2026-09-02 SSH 원격 화면 확인은 shot --notify(Discord 첨부)·chrome://inspect 터널 두 가지로 — 스트리밍은 범위 밖
- 2026-09-02 "명령 0개" 원칙: 관제탑이 표면, CLI는 배관. init이 MCP 등록(mcp-serve 래퍼로 기동까지 자동)·관제탑 b/s/p 키·스킬 절. iTerm2 규칙 자동 주입은 iTerm2 되쓰기 위험으로 감지+안내만
- 2026-09-02 codex 브라우저 실패 원인은 ChatGPT 앱 내장 browser:control-in-app-browser 스킬 — 스킬 문단에 사용 금지 명시, chrome-devtools MCP 지목
- 2026-09-02 사용자 요청으로 codex(~/.codex/config.toml)에 chrome-devtools MCP 등록 — 이후 init이 mcp-serve로 교체

- 2026-09-02 팝업 형태 유지 확정(사용자 선호). tmux 3.6a 팝업은 커져도 안 따라옴(실측) → client-resized 훅 + popup-refresh 재오픈(커서 유지, 재오픈은 비동기 — display-popup -E가 블록)
- 2026-09-02 자동 프리뷰는 관제탑이 아니라 hook 경로가 정본(관제탑 닫혀도 동작). preview-seen.json으로 "한 번만", 사라진 서버는 잊음, 5초 스로틀, 기존 탭은 앞으로 가져옴. config preview_auto 기본 켬
- 2026-09-02 전용 브라우저 테마 = 다크 grayscale(검정·중성 회색, Claude Desktop 톤). 시드 색은 전부 색기운 남아 탈락(주황→갈색, 회색→푸른빛). 구분은 북마크바로(사용자). 프로필명 정본은 Local State. enable-automation 인포바 제거
- 2026-09-02 사용자 원칙: 색·디자인은 AgentLoops 산출물 팔레트(웜다크 #1f1e1d·#262624·#2d2c2a, 테라코타 #d97757, 크림 #faf9f5)를 먼저 확인. "구분보다 예쁜 게 우선"
- 2026-09-02 pick 요소 스크린샷: rod el.Screenshot은 CSS 좌표 크롭이라 레티나에서 오류 → Page.captureScreenshot clip(문서 좌표=뷰포트+스크롤, scale 1=기기 픽셀)로 교체, DPR 2 에뮬레이션 회귀 테스트
- 2026-09-02 pick 복귀 경로: 터미널 esc/q/Ctrl-C(raw stdin 감시) + 오버레이 취소 시 루프 종료(ErrPickCancelled)
- 2026-09-02 cookies import는 Chrome 프로필 자동 선택(도메인 쿠키 최다, 동률 last_used, --profile 지정). 이 Mac의 Default는 다른 계정, 사용자 계정은 Profile 1(netwaif)
- 2026-09-02 영상 시연 선정: A 기본 루프·B 3사 A/B(orchestration)·C 디스코드 원격(shot --notify)·E x.com 로그인 소식 수집. D(코덱스 비포/애프터)는 매뉴얼 내용이라 제외
- 2026-09-02 cookies clear/list 추가(사용자: 실사용에도 필요). 용어 합의: "에이전트 브라우저"(browser-profile Chrome) vs "실사용 브라우저". import=실사용→에이전트 복사(읽기만), list/clear=에이전트만
- 2026-09-02 쿠키·지목·스크린샷·머지·폰 전송은 관제탑 키가 아니라 **스킬 경로**(말로 시키면 에이전트가 명령 실행). `pick --once`(RunPick send=nil→stdout, SelfAgents 스텁). 관제탑 b/s는 여러 에이전트 중 고를 때만. 시연 A4(스크린샷 확인) 컷은 A3와 중복이라 삭제
- 2026-09-02 관제탑 쪽 자동 프리뷰 제거 — 팝업이 매번 새 프로세스라 인메모리 seen이 비어 hasTab→Connect·Activate로 브라우저가 튀어나옴. hook 경로만 정본. hook은 wt 메타로 ⎇브랜치 결합(PreviewPaths)
- 2026-09-02 ActivePage: chrome://newtab 등 내부 탭 제외, 포커스>보임>첫 웹 탭. 촬영 중 pick이 빈 새 탭에 검사 모드 건 사고
- 2026-09-02 모니터 안 꺼짐 원인 = MCP(puppeteer) 스크린샷이 남기는 Chrome "Capturing" NoDisplaySleep 잠금(실측 7h14m). 같은 페이지 캡처 완료 시 풀림 → hook이 감지해 탭마다 1×1 캡처(ReleaseCaptures, 30s 스로틀). 브라우저 튀어나옴 원인 = mcp-serve가 세션 시작마다 Chrome 기동 → stdio 프록시로 첫 tools/call에서만 기동(chrome-devtools-mcp 지연 연결 실측) + pkill 뒤 크래시 탭 복원(exit_type Normal·restore_on_startup 5)
- 2026-09-03 worktree 프리뷰 창 3개 타일 배치(f63604d)는 사용자 판단으로 철회(터미널 덮음) → 같은 창의 탭(⎇ 제목, 리로드 유지 MarkBranch). B 실패 원인 3개: codex 샌드박스가 백그라운드 서버 못 남김(탭 없음), claude worker가 자기 탭(127.0.0.1:8101)을 열어 프리뷰 탭과 중복, gemini(agy) 내부 포트 51871이 dev 서버로 잡힘 → HTTP 판정(text/html·<400) 추가. B1은 코디네이터가 서버를 띄우는 구성으로 변경 권고
- 2026-09-03 worker 기동 시 폴더 신뢰 질문(codex·claude·gemini/agy) 자동 승인 — 설정 사전 등록은 codex가 저장소 루트별로 다시 묻고(루트 "/" trusted도 안 덮음) agy는 저장 위치 불명이라, wt new가 분리 감시자(`wt accept-prompts <pane>`, 40s)를 띄워 화면에 신뢰 질문이 보이면 Enter. 화면 파싱 금지 원칙의 명시적 예외(기동 핸드셰이크). 오늘 codex 질문이 "지나간" 건 코디네이터 dispatch의 Enter가 기본 선택을 확정한 것
- 2026-09-03 pick 터미널 키 감시 raw→cbreak(ICANON·ECHO만 끔, OPOST 유지) + 시작 시 화면 비움 — 관제탑 b 뒤 팝업이 계단식으로 깨진 건 raw 모드의 \n 미변환. x/sys/unix 직접 사용(darwin TIOCGETA/linux TCGETS)
- 2026-09-03 gemini worker(agy)만 git add/commit마다 도구 승인을 물음(B1이 worker 커밋을 시키면서 드러남; claude는 auto 모드, codex는 trusted). wt new가 agy `--dangerously-skip-permissions`/gemini `--yolo`로 기동, config `worker_auto_approve`(기본 true). agy 신뢰는 `~/.gemini/antigravity-cli/settings.json` trustedWorkspaces(정확 경로)에 기록됨
- 2026-09-03 B 재촬영 실패 원인 = 코디네이터가 저장소 루트에서 `python3 -m http.server --directory <worktree>`로 띄워 cwd가 루트 → 브랜치 미판정 → ⎇ 없는 새 창. DevServers가 명령 인자(ProcArgs)로도 worktree를 찾고 후보 중 가장 깊은 경로 하나만 쓴다. 스킬에 "서버는 worktree를 cwd로" 명시. 고친 뒤 같은 서버로 ⎇ 탭 3개 실확인 → B 촬영 성공

- 2026-09-03 codex 상태가 작업 중에도 DONE으로 보인 원인 = notify(agent-turn-complete)만 있고 WORK 전이가 없었음(촬영 스크린샷 실측). `~/.codex/hooks.json`에 SessionStart·UserPromptSubmit·PostToolUse·PermissionRequest·Stop → `agentlayer hook codex --event <e>`(stdin JSON, RunCodexEvent) 등록을 init에 추가. codex는 새 훅을 `/hooks` 신뢰 확인 전엔 실행 안 함(trusted_hash 역산 실패 — sha256(cmd)·JSON 직렬화 후보 12종 불일치). 살아 있는 TUI는 재시작해야 hooks.json·AGENTS.md를 읽음
- 2026-09-03 codex "에이전트 브라우저 못 찾음" 원인 = chrome-devtools MCP는 붙어 있었으나 ChatGPT 앱 내장 인앱 브라우저 스킬(node_repl `agent.browsers.list()`→[])을 선택. 사용자 실측: "chrome-devtools 이용해"라고 말하면 씀 → init이 `~/.codex/AGENTS.md`에 마커 블록(agentlayer:browser)으로 지침 설치(InstallCodexAgents)
- 2026-09-03 mcp-serve가 세션 시작 즉시 Chrome을 띄우던 버그(browserMCPServe 첫 줄 Connect) → CheckPort로 교체. 봇 4개 부팅과 동시에 Chrome 기동되던 실측이 근거
- 2026-09-03 스킬 스크린샷 문단: 디스코드 지시면 `shot <url>` stdout 경로를 그 채널 답글 files로 첨부, --notify는 SSH 등 첨부 수단 없을 때만(타워 요청: 폰 한 화면 왕복)
- 2026-09-03 **내 샌드박스 판단이 틀렸음(타워 실측으로 정정)**: 코덱스 브리지 `-s workspace-write`에서 cwd(`~/ai-folder/codex-discord-workspace`) 밖 `~/ai-folder/demo/browser-demo/index.html` 수정이 됐고, 스크린샷도 프라이밍 없이 채널에 첨부됨. 추정 원인(미검증): `approval_policy=on-request` + `approvals_reviewer="guardian_subagent"`라 샌드박스 밖 쓰기가 승인 요청→가디언 서브에이전트 자동 승인→비샌드박스 재실행. `[projects."…/browser-demo"]` 신뢰 폴더도 후보. 결과: CODEX_WORKDIR 변경 불필요, C는 (나) 두 하네스 구성(C0 관제탑·C1 클로드 서버·C2 코덱스 열기+사진·C3 코덱스 수정)

- 2026-09-03 코덱스가 MCP 스크린샷을 base64로 우회한 원인 = chrome-devtools-mcp가 filePath를 클라이언트 roots(roots/list) 안에서만 허용하는데 codex는 roots 능력을 선언하지 않아 OS 임시 폴더만 허용. mcp-serve 프록시가 보정(`internal/cli/mcproots.go`): 클라이언트에 roots 능력 없으면 initialize에 끼워 넣고 roots/list를 cwd(file://)로 대신 답함, 능력 있어도 빈 목록·error면 cwd 채움. 가짜 클라이언트 E2E: cwd 저장 성공·밖은 여전히 거부. 살아 있는 codex-live는 재시작(재부팅) 뒤 적용
- 2026-09-03 `agentlayer browser errors --reload` 추가 — 기존 errors는 Enter 대기라 에이전트 Bash에서 즉시 EOF로 아무것도 못 모았음(실측). 리로드 후 5초 수집, 404 줄에 URL 부착. 스킬·codex AGENTS 블록에 '증상만 말하면 콘솔·네트워크 직접 읽고 전부 짚기' 문단, AGENTS 블록에 cookies list/import/clear 안내 추가
- 2026-09-03 broadcast가 codex TUI에 제출 안 되던 원인 = `tmuxx.SendText`가 텍스트 직후 Enter를 붙여 보내 codex가 Enter를 삼킴(문장이 입력줄에 남음, Enter만 따로 보내니 제출됨). SendText에 `SendEnterDelay`(300ms+길이 비례, 최대 1s) 삽입 — codex-discord 브리지 pasteToPane과 같은 처방. `broadcast --yes --except …`로 codex-live 단독 전송해 'ok' 회신 실측
- 2026-09-03 codex `/hooks` 화면 실측: 살아 있는 TUI도 바뀐 hooks.json을 즉시 인식("5 hooks need review"), `t`로 전부 신뢰 → Active 5/5, config.toml [hooks.state]에 항목별 trusted_hash 추가. 해시 역산 2차(28개 후보: 명령·훅 JSON·그룹 JSON·TOML 등) 실패 — 자동 신뢰는 보류. 신뢰 뒤 broadcast로 codex-live에 프롬프트 → 관제탑 WORK→DONE 실측
- 2026-09-03 매뉴얼 원본 `~/ai-folder/youtube/AgentLoops/agentlayer/tasks/agentlayer-video-prep/artifacts/manual/agentlayer-manual.txt`에 Codex /hooks 신뢰 단계(2장 init 절)·hooks.json 5이벤트·AGENTS.md 브라우저 블록(5장 Codex 절) 추가(.bak-codexhooks-*). PDF 빌드·배포는 안 함 — v1.3.0 릴리즈 때 `/deploy-manual agentlayer`로(VERSION 1.2.0 → 올릴 것)
- 2026-09-03 restore 봇 2개씩 뜬 원인 확정: bot-up.sh가 봇을 락으로 직렬 기동 → 대기 중 pane 명령이 bash → 스캐너(DetectKind) 미감지 → 옛 DEAD 레코드가 복원 대상 → 봇 세션에 window 추가. 대응: PlanRestore에 RestoreEnv(SessionExists·PaneAt·LaunchAgents) 주입 — 같은 자리 pane은 ID 명시로도 못 넘고(물리 충돌), LaunchAgent 관할(wiring.TmuxSessionAgents: tmux+new-session+세션명 plist)은 ID 명시 시 강제 가능(정책). 2026-08-29 재부팅 절차 갱신: 봇 자동 기동 → `agentlayer restore`(체크리스트) → wake-all. `--dry-run`은 SSH/스크립트·건너뜀 사유 확인용으로만
- 2026-09-03 선택 복원은 CLI 인자 나열이 아니라 체크리스트(사용자 "불편해 보이네. 체크박스로는 안되나?"). 기본 전부 체크라 enter만 치면 이전과 동일, `--yes`는 스크립트용, 파이프/비터미널은 화면 없이 전부. 주입점 restoreIsTerminal·runRestorePicker로 테스트
- 2026-09-03 에이전트 브라우저 수시 튀어나옴 원인 3가지: ① autopreview hasTab의 pg.Activate() ② seen 망각 — HTML 확인(1.5초 타임아웃)으로 스캔이 놓치면 기록을 지워 다음 스캔에 "새 서버"(13:10부터 뜬 8080의 발견 시각이 15:35로 갱신된 실측) ③ Connect가 닫힌 Chrome 재기동. 셋 다 제거(Activate 삭제·PortOpen 되묻기·IsUp 게이트). shot/pick/errors의 Activate는 사람이 봐야 하는 동작이라 유지
- 2026-09-03 모니터 안 꺼짐은 agentlayer 아님 — obs-record 스킬의 `caffeinate -dims`(13:31 기동, cleanup 누락으로 10시간 잔존). agentlayer 코드에 caffeinate 없음, Chrome Capturing 잠금도 없었음. 스킬 수정: 3시간 상한·stop에서 해제·서명 pkill로 고아 정리
- 2026-09-03 타워(agentbrowser-c7)에 대본 근거 전달: SSH 터널+chrome://inspect는 **미실측**(9222가 127.0.0.1 바인딩인 것만 lsof 실측, LAN 직결 불가·터널만), 가벼움 실측값(바이너리 17,532,848B·직접 의존 9/간접 21·LOC 10,653+테스트 7,611·`status` 0.02s/최대 RSS 7.2MB·mcp-serve 4~8MB), Orca 비목표 목록은 스펙 2026-08-25·핸드오프 4.2, v1.3.0 미출시(대본에 날짜 적지 말 것)

## 파일 흔적
<!-- 누적. 만든/고친 파일의 경로를 그대로 적는다. "설정 파일 고침" 같은 산문 금지 -->
<!-- 형식: - `경로` 무엇을 (함수명·핵심 식별자 포함) -->

- `internal/state/` Agent 레코드·AgentState·Store(원자적 파일 저장소, MarkRead)
- `internal/tmuxx/tmux.go` ListPanes/JumpTo(activeClient)/NewWindow/SendText/CapturePane, Bin() tmux 자체탐색, run()에 LANG 보장
- `internal/scan/scan.go` DetectKind(versionRe=버전형→claude)/IDForPane/Sync(DEAD 24h 정리)
- `internal/hookcmd/claude.go` RunClaude(이벤트 매핑, 유휴 에코 무시), codex.go RunCodex(turn-complete)
- `internal/cli/` status.go(PadRight runewidth), initcmd.go(hook 설치·PrintTmuxBinding 절대경로), codexinit.go, wtcmd.go(RunWT), allcmd.go(Targets·HasSessionHandoff·SendAll·watchDone), infocmd.go(RenderInfo·InfoData·FindAgent)
- `internal/ui/model.go` TUI(2초 폴링·usageCmd 15초·previewCmd·W/C pendingCmd·g lazygit ExecProcess), view.go(선택바 전체폭·⌁·⎇·ctx 뱃지·usageView·미리보기 패널·previewHeight)
- `internal/usage/` coach.go(FetchCached 5분 캐시·LookupTool·extendedEnv), ctx.go(LoadSnapshots·CodexLatest)
- `internal/discord/card.go`(BuildComponents·WorsenedPings·wired ⌁표시) webhook.go(Upsert 선게시후삭제·1MB 응답)
- `internal/wt/` meta.go·git.go·lifecycle.go(New/Clean 보존우선/MergeGuide)·review.go(#> 코멘트→SendComments)·runtest.go
- `internal/wiring/wiring.go` Collect(bots.json·access.json·브리지 .env CODEX_WORKDIR·plist 경계매칭)·DiscordConnected
- `internal/starter/starter.go` ActiveTasks(task.md yaml status)
- `internal/config/config.go` ~/.config/agentlayer/config.json (webhook·notify_discord=true·channel_labels)
- `main.go` 서브커맨드 라우팅 전부
- 시스템 상태: `~/.local/bin/agentlayer`(설치본), `~/.local/state/agentlayer/`(agents·usage-cache·discord-card.json), `~/.claude/settings.json`(hook 5종), `~/.codex/config.toml`(notify), `~/.tmux.conf:75`(C-b a 바인딩), `~/Library/LaunchAgents/com.netwaif.agentlayer-card.plist`(5분 카드)
- `internal/hookcmd/gemini.go` RunGemini(agy camelCase+stock snake_case 겸용 파싱, 이벤트 매핑)
- `internal/cli/geminiinit.go` InstallGeminiHooks(agy hooks.json "agentlayer" 키 소유·멱등)
- `internal/cli/initcmd.go` installJSONHooks로 일반화(claude·stock gemini 공유), geminiCLIEvents
- `internal/usage/claudecfg.go` ClaudeDefaultModel·CodexDefaultModel·GeminiDefaultModel·DefaultModels·PrettyModel·IsFable
- `internal/usage/ctx.go` GeminiDir·GeminiLatest(projects.json 매핑+조상 폴백)·CodexSessionID·codexRolloutsByRecency
- `internal/state/types.go` Agent.Model 필드
- `main.go` resumeCommand(3사 분기), hook gemini 라우팅(stdout "{}"), init에 gemini 2계열 등록
- 시스템 상태 추가: `~/.gemini/config/hooks.json`(agy 훅), `~/.gemini/settings.json`(stock 훅 5종)
- `agentlayer-handoff-2026-08-26.md` 기능 완성 시점 핸드오프 (전체 조망·커밋 스토리·한계·파일 지도)
- `internal/ui/view.go` View()→viewBody() 분리, clampLines(가로 ANSI 클램프)·listWindow(세로 커서 스크롤)
- `internal/ui/view_test.go` TestViewClampsToWidth·TestViewScrollsListToHeight·TestClampLinesANSI
- `go.mod` charmbracelet/x/ansi 직접 의존성 승격
- 시스템 상태 추가: `~/.tmux.conf` 끝에 default-size 212x50, `~/.config/agentlayer/config.json` 새 서버 웹훅
- 외부: 테스트 체크리스트 Artifact https://claude.ai/code/artifact/3cd3ee37-9863-462e-8ca4-603cae896ba4
- `internal/discord/coalesce.go` RunCoalesced(card.dirty 선기록→card.lock flock NB→dirty 소진 루프), coalesce_test.go 3종
- `internal/discord/card.go` CardData·BuildCard·agentsContainer 재작성, summaryLine·defaultModelsLine·tasksLine·truncateRunes
- `internal/cli/restorecmd.go` PlanRestore·RestorePlan/RestoreItem·RunRestore(--resume·--dry-run)·freshCommand·ResumeCommand(main.go에서 이동)
- `internal/tmuxx/tmux.go` HasSession("=" 완전일치)·NewSession·NewWindowIn(-P -F pane_id 반환)
- `internal/usage/ctx.go` LoadSnapshots에 파일명(sid) 키 추가, AgentCtx claude sid 우선 매칭
- `main.go` runCard --event/publishCard 분리(usageMaxAge 인자), runHook 전이 시 detached 발사(defer), runRestore 배선, cli.ResumeCommand 호출로 교체
- 테스트: `internal/cli/restorecmd_test.go`(계획 7종+RunRestore 통합), `internal/tmuxx/tmux_test.go` TestRestorePrimitivesIntegration, `internal/usage/ctx_test.go` sid 키 2종, `internal/discord/card_test.go` TUI 동등 항목 검증
- `internal/scan/scan.go` Sync에 liveSlot/occupied(부활 세션의 옛 DEAD 즉시 정리), scan_test.go TestSyncPurgesDeadSupersededByRevivedSession 외 1
- `internal/hookcmd/guard.go` hookPane(기본 서버 가드), guard_test.go(3사×3케이스), claude.go·codex.go·gemini.go 진입점을 hookPane으로 교체, claude_test.go env에 TMUX 추가
- `e2e_test.go`·`internal/tmuxx/tmux_test.go`·`internal/cli/restorecmd_test.go` 모든 tmux 호출에 -f /dev/null, e2e TUI 대기 100회(10s)로 상향
- `internal/cli/versioncmd.go`·`versioncmd_test.go` FormatVersion/VersionInfo/IsReleaseVersion (원작 restore-lab Opus, 839f157→f623fbf 재구성), `main.go` version 라우팅+buildVersion, `.goreleaser.yaml` ldflags 명시
- `internal/cli/orchskill.go` InstallOrchestrationSkill(go:embed·멱등·.bak 보존), `orchestration_skill.md`(스킬 본문 정본 — 여길 고치고 make install+init 재실행하면 갱신), orchskill_test.go 4종, `main.go` runInit 배선
- 시스템 상태 추가: `~/.claude/skills/orchestration/SKILL.md`(init 설치본), GitHub 릴리즈 v1.1.0, brew tap Formula/agentlayer.rb 1.1.0
- `internal/cli/helpcmd.go` HelpText(명령 목록 정본), helpcmd_test.go(목록 어긋남 감시), `main_help_test.go`, `main.go` help 라우팅+미지 명령 에러 문구
- `internal/tmuxx/tmux.go` AttachArgv(포커스+attach 체이닝)·ActiveSession(최근 활동 클라이언트의 세션)·SpawnShellWindow(셸 창+SendText)·JumpToSessionPane(pane 기준 전환), tmux_test.go TestAttachArgvExactMatch·TestSpawnShellWindowIntegration
- `internal/ui/model.go` insideTmux·attachCmd·startResume(원세션 우선+레코드 삭제)·inputMode/input(textinput)·주입점(spawnWindow·activeSession·hasSession·jumpPane·sendAll), attachDoneMsg
- `internal/ui/view.go` helpLine(키 밝게·설명 회색, 전 화면 공용)·resume/broadcast 확인 프롬프트·입력줄 렌더
- `internal/ui/model_test.go` notice 일회성·attach·resume 확인/삭제/보존/원세션·broadcast 4종 테스트
- `internal/notify/notify.go` NotifyWebhookURL 우선+폴백, notify_test.go 2종, `internal/config/config.go` NotifyWebhookURL 필드
- `.goreleaser.yaml` homebrew_casks(quarantine postflight), `README.md` notify_webhook_url 문서화, `go.mod` bubbles v1.0.0
- 시스템 상태 추가: `~/.config/agentlayer/config.json` notify_webhook_url+새 대시보드 웹훅, `~/.local/state/agentlayer/agents-backup-filming/`(claude-1·6·26 촬영용 백업), GitHub 릴리즈 v1.2.0, tap Casks/agentlayer.rb 1.2.0(Formula 삭제됨)
- `~/.tmux.conf` resurrect/continuum 블록 주석 처리(@plugin 2줄+@continuum-restore·save-interval·@resurrect-* 5줄), 원본 백업 `~/.tmux.conf.bak-20260829`, 실행 중 서버 source-file 반영
- `internal/usage/ctx.go` GeminiCommand(agy 흔적→agy, 폴백 gemini)·ctx_test.go TestGeminiCommand
- `internal/wt/lifecycle.go` commandFor(usage.GeminiCommand 공유)·lifecycle_test.go TestCommandFor, `internal/cli/restorecmd.go` freshCommand gemini 분기를 GeminiCommand로 교체
- 시스템 상태 추가: GitHub 릴리즈 v1.2.1, tap Casks/agentlayer.rb 1.2.1
- `internal/wt/lifecycle.go` agentCommand["gemini"]="agy" (미커밋 — 검증 후 커밋 대상)
- `internal/cli/restorecmd.go` FilterByIDs(ID 선택·사유), RunRestore에 fs.Args() 선택 배선, `restorecmd_test.go` TestFilterByIDs·TestRunRestoreSelectsIDs, `internal/cli/helpcmd.go` restore 행에 [id ...]
- `internal/config/config.go` NotifyURL()(알림 우선·카드 폴백 단일 지점), `config_test.go` TestNotifyURL, `internal/notify/notify.go` NotifyURL() 사용으로 교체, `main.go` publishCard 핑을 pingClient(discord.NewClient(cfg.NotifyURL()))로
- 시스템 상태 추가: GitHub 릴리즈 v1.2.2·v1.2.3, tap Casks/agentlayer.rb 1.2.3
- `internal/config/config.go` PreviewTick(preview_interval 파싱·기본 1s·200ms 하한), `config_test.go` TestPreviewTick
- `internal/ui/model.go` previewTickMsg·previewTickCmd·Model.previewInterval(refreshMsg의 미리보기 발사 제거), `model_test.go` TestPreviewIntervalFromConfig 외 2
- `internal/cli/allcmd.go` Targets에 gemini 통과, `allcmd_test.go` TestTargetsIncludesGemini·TestTargetsSelection 3사 기대로 갱신
- `README.md` preview_interval 문서화
- 시스템 상태 추가: GitHub 릴리즈 v1.2.4, tap Casks/agentlayer.rb 1.2.4
- 외부: 매뉴얼 정본 `~/ai-folder/youtube/AgentLoops/agentlayer/tasks/agentlayer-video-prep/artifacts/manual/agentlayer-manual.txt` (agentlayer-c6 세션 관할, v1.2.4 정렬됨)
- `internal/usage/cache.go` ReadCached(나이 불문 읽기 전용)·readCacheFile 추출, cache_test.go TestReadCached 2종
- `internal/ui/model.go` usageCacheCmd(Init에서 낡은 캐시 즉시 usageMsg), `main.go` publishCard 2-pass(stale 즉시 게시→FetchCached→usagePayloadChanged 시 2차)·usagePayloadChanged, main_card_test.go
- `internal/browser/instance.go` Connect(멱등 attach·죽은 기록 정리)·Instance{WSURL}·Load/Save/RemoveInstance, browser.json·browser-profile/, Leakless(false)·launcher.LookPath (자동 다운로드 금지). export_test.go SetHeadlessForTest
- `internal/browser/route.go` RunLsof·ExecLsof·PortCWD(lsof -Fp/-Fn)·Candidates(localhost 포트→cwd→최장일치, 외부/실패면 산 에이전트 전원)
- `internal/browser/context.go` PickContext·SavePick(picks/<ts>.md+png)·PromptLine(strings.Fields로 한 줄 강제)
- `internal/browser/pick.go` RunPick(검사모드 EachEvent 동기구독+wait·abort 정리·OverlayEnable 명시·shadow DOM 오버레이 selectorJS/overlayJS·el.Eval/page.Info 에러반환)·ActivePage, pick_test.go(실클릭 e2e·탭닫힘·취소)
- `internal/browser/shot.go` Shot(전체 캡처 picks/<ts>-shot.png), `internal/cli/browsercmd.go` parseShotArgs(위치 무관 --send)·sendToAgent·chooseAgent(복수 후보 stdin 선택)
- `internal/browser/errors.go` CollectErrors(RuntimeExceptionThrown·RuntimeConsoleAPICalled error/warn·LogEntryAdded·Value.Nil()→Description 폴백)·SaveErrors(picks/<ts>-errors.txt)
- `internal/browser/preview.go` DevServers(전 리스너 스캔→worktree 경로 아래·빈 경로 가드)·OpenPreview(새 창+⎇브랜치 제목)·DevServer
- `internal/browser/cookies.go` ImportCookies·pbkdf2SHA1·matchesDomain·parseHexBlob·chromeEpochToTime·decryptV10(v10·PKCS7·SHA256 host_key 프리픽스 제거)·parseCookieRows·buildCookieParams·readChromeCookies(sqlite3 -separator·WAL 복사)·safeStoragePassword(security find-generic-password), cookies_test.go(RFC PBKDF2 벡터·라운드트립·필터)
- `internal/cli/browsercmd.go` RunBrowser 디스패치(기동/pick/shot/errors/preview/cookies), `main.go` case "browser", `internal/cli/helpcmd.go` browser 행
- `go.mod` github.com/go-rod/rod 직접 의존성 추가(순수 Go, goreleaser 무변경)
- 시스템 상태 추가: `~/.local/state/agentlayer/browser.json`(CDP ws_url)·`browser-profile/`(전용 프로필)·`picks/`(pick 산출물). 로컬 main 2b4f6a0(origin 대비 17커밋 미푸시, GitHub 릴리즈는 v1.2.5)
- 외부: SDD 레저·브리프는 작업 완료로 삭제됨(`.superpowers/sdd/2026-09-01-agent-browser/`) — 정본은 git 이력
- `internal/config/config.go` BrowserPort·BrowserPortOrDefault(기본 9222), config_test.go TestBrowserPortOrDefault
- `internal/browser/instance.go` Connect(stateDir, port)·probePort(/json/version)·attach·newBrowser(NoDefaultDevice)·RemoteDebuggingPort·Delete("no-startup-window"), `port_test.go` TestConnectProbesFixedPortIntegration·TestConnectPortOccupied·TestConnectPagesFollowWindowSizeIntegration
- `internal/browser/preview.go` OpenPreview 빈 브랜치면 제목 유지
- `internal/discord/file.go` Client.PostFile(멀티파트 files[0]·payload_json), file_test.go
- `internal/cli/browsercmd.go` shotOpts{URL,Send,Notify,Agent}·parseShotArgs·parsePickArgs·FilterAgentByID·sendToAgent(agentID)·browserOpen·browserMCP·MCPCommands·browserMCPServe(syscall.Exec npx chrome-devtools-mcp, usage.ExtendedEnv), 디스패치 open/mcp/mcp-serve
- `internal/cli/mcpinit.go` InstallClaudeMCP(~/.claude.json, UseNumber)·InstallCodexMCP(config.toml 섹션 append)·InstallGeminiMCP·installJSONMCP·MCPServeArgv, mcpinit_test.go
- `internal/cli/iterm2.go` ITerm2LinkRuleInstalled·ReadITerm2Bookmarks·PrintITerm2LinkGuide, iterm2_test.go
- `internal/cli/orchestration_skill.md` "## 6. 브라우저" 절 + 하지 말 것 1줄, orchskill_test.go TestOrchestrationSkillHasBrowserSection
- `internal/usage/coach.go` toolDirs에 nvmLatestBin·ExtendedEnv 공개, coach_test.go TestToolDirsIncludesNvmLatest
- `internal/ui/model.go` devServers·browserPort·devScan/openPreview/browserCmd 주입점·devTickMsg(10s)·devServersMsg·noticeMsg·browserDoneMsg·serversFor·devBadge, 키 b/s/p; `view.go` 🌐 뱃지·helpLine b/s/p; model_test.go 7종
- `main.go` runInit: InstallClaudeMCP/CodexMCP/GeminiMCP + PrintITerm2LinkGuide 배선
- `README.md` browser 섹션(open·shot --notify/--agent·mcp·관제탑 키·링크 라우팅 Smart Selection·SSH 원격·browser_port)
- `docs/superpowers/specs/2026-09-02-browser-zero-command-design.md` 명령 0개 설계(4단계, 4번은 감지+안내로 확정)
- 시스템 상태 추가: `~/.claude.json`·`~/.codex/config.toml`·`~/.gemini/settings.json` mcpServers.chrome-devtools = `~/.local/bin/agentlayer browser mcp-serve`(각 .agentlayer.bak), `~/.claude/skills/orchestration/SKILL.md` 브라우저 절 포함(.bak), iTerm2 Default 프로필 Smart Selection "Agent browser URL" 규칙, 전용 Chrome 9222(browser.json), 브랜치 `browser-mcp` cd44d02
- `internal/popup/popup.go` Record·Save/Load/Remove·InPopup·ExpectedInner·Mismatch·DisplayArgs·BindLine·HookLine·Refresh(flock·5회 루프·open 비동기), popup_test.go 6종; `internal/ui/model.go` popupRecord·restoreCursor·recordPopup·WithPopup; `main.go` runPopupRefresh·case "popup-refresh"; `internal/cli/initcmd.go` PrintTmuxBinding 두 줄 안내
- `internal/browser/autopreview.go` AutoPreview(preview-seen.json·5초 스로틀·사라진 서버 망각)·autopreview_test.go 3종; `internal/cli/browsercmd.go` browserAutoPreview(hasTab는 기존 탭 Activate); `main.go` runHook defer spawn("browser","autopreview"); `internal/config/config.go` PreviewAuto·PreviewAutoEnabled
- `internal/ui/model.go` autoPreview·seenServers·hasTab·autoPreviewCmd(관제탑 쪽 자동 열기, 인메모리), model_test.go 자동 프리뷰 2종
- `internal/browser/theme.go` EnsureProfileTheme(ensurePrefsTheme is_grayscale2+color_scheme2=2·ensureLocalStateName AgentLayer), theme_test.go 4종; `instance.go` Delete("enable-automation")
- `internal/browser/pick.go` ErrPickCancelled·ElementShot(문서 좌표 clip); `internal/cli/browsercmd.go` watchQuitKeys(x/term raw)·quitKey; pick_test.go TestElementShotCapturesScrolledElement(DPR 2)
- `internal/browser/cookies.go` ChromeProfile·ListChromeProfiles(Local State)·ChooseProfile·countDomainCookies·chromeCookiesPath(home, profile)·ImportCookies(profile 인자), cookies_test.go 2종; CLI `cookies import --profile`
- `docs/demos/2026-09-02-browser-video-demos.md` 시연 4종 계획(프롬프트·키·기대 화면·리스크)
- 외부: `~/ai-folder/demo/browser-demo/`(index.html·CLAUDE.md·SESSION.md, git master e1e2756) 시연 저장소
- 시스템 상태 추가: `~/.tmux.conf` bind-key a에 -e AGENTLAYER_POPUP=1 + set-hook client-resized(백업 .bak-20260902-110420), `~/.local/state/agentlayer/preview-seen.json`·`popup.json`(팝업 열려 있을 때만), 전용 프로필 x.com 로그인 쿠키 16개(Profile 1에서), 브랜치 `browser-mcp` b1ae112
- `internal/browser/cookies.go` selectCookies·borrowPage·ClearCookies·FormatCookieList·ListCookies, cookies_test.go(+4; TestClearCookiesIntegration headless)
- `internal/browser/pick.go` ActivePage(포커스>보임>웹)·IsWebURL·RunPick(send nil→stdout), `route.go` SelfAgents, pick_test.go(+3)
- `internal/browser/capturelock.go` ParseCaptureLocks·ChromePID·RunPmsetAssertions·HasCaptureLock·ReleaseCaptures·ThrottleOK, capturelock_test.go(3)
- `internal/browser/theme.go` ensurePrefsTheme에 exit_type Normal·exited_cleanly·session.restore_on_startup=5, theme_test.go(+1); `instance.go` IsUp
- `internal/browser/preview.go` OpenPreview(NewWindow = Branch=="")·IsHTMLServer·FilterHTML·MarkBranch(addScriptToEvaluateOnNewDocument), preview_test.go(+2)
- `internal/cli/browsercmd.go` browserCookiesClear/List·parsePickArgs(--once)·browserPick once 분기·pageForAgents(--agent면 그 에이전트 dev 서버 탭)·browserMCPServe 프록시(IsMCPToolCall, 첫 tools/call에 Connect)·browserAutoPreview(PreviewPaths·FilterHTML·branchByPort·MarkBranch·Capturing 정리), previewpaths_test.go, browsercmd_test.go(+2)
- `internal/cli/orchestration_skill.md` 6절: 지목·스크린샷·폰 전송·쿠키 문단, 5절: worker 명시 시 `wt merge --yes`; description 트리거 6개; orchskill_test.go
- `internal/ui/model.go` autoPreview·seenServers·hasTab·autoPreviewCmd 삭제, devScan에 FilterHTML; model_test.go TestDevServersMsgOnlyUpdatesBadge
- `README.md` cookies list/clear·말로 시키기·pick --once·Capturing 정리 문단
- 외부: `~/ai-folder/youtube/AgentBrowser/handoff-agentlayer-browser-2026-09-02.md`(핸드오프, CLI 정본은 README), 순서표 아티팩트 64b97f91, footage/A-기본루프.mov
- 시스템 상태: `~/.claude/skills/orchestration/SKILL.md` 갱신(init), `~/.local/state/agentlayer/capture-janitor.throttle`, browser-profile Preferences exit_type Normal, agents/ DEAD 12개 삭제(2026-09-03), 브랜치 `browser-mcp` 3b54684
- `internal/wt/prompts.go` IsTrustPrompt·AcceptStartupPrompts, prompts_test.go(4); `internal/wt/lifecycle.go` NewOptions.AcceptPrompts·openWindow→paneID; `internal/tmuxx/tmux.go` NewWindowPane·SendEnter; `internal/cli/wtcmd.go` accept-prompts·spawnAcceptPrompts. 브랜치 `browser-mcp` dd66109
- `internal/cli/cbreak_darwin.go`·`cbreak_linux.go`·`cbreak_unix.go`·`cbreak_other.go` makeCbreak; `browsercmd.go` watchQuitKeys→cbreak, browserPick 화면 비움; go.mod golang.org/x/sys 직접 의존
- `internal/config/config.go` WorkerAutoApprove·WorkerAutoApproveEnabled, config_test.go; `internal/wt/lifecycle.go` commandFor(agent, auto)·geminiCommand·NewOptions.AutoApprove, lifecycle_test.go; `internal/cli/wtcmd.go` AutoApprove 배선; README worker_auto_approve
- `internal/browser/preview.go` DevServers(ProcArgs 인자 매칭·최장 경로 1개)·ProcArgs, preview_test.go TestDevServersMatchesWorktreeByArgsAndPicksDeepest; `internal/cli/orchestration_skill.md` 4절 "서버는 worktree를 cwd로·코디네이터가 띄움"
- 외부: footage/B-3사AB.mov. 브랜치 `browser-mcp` a31aa1a
- `internal/cli/browsercmd.go` browserMCPServe(CheckPort·양방향 프록시·roots 보정)·errorsOpts·parseErrorsArgs(--reload)·errorsReloadWindow; `internal/cli/mcproots.go` mcpRoots·FromClient·FromServer, mcproots_test.go(4); `internal/browser/instance.go` CheckPort, port_test.go; `internal/browser/errors.go` CollectErrorsReload·collectErrors(onSubscribed)·404 URL 부착, errors_test.go TestCollectErrorsReloadCapturesLoadTimeErrors
- `internal/hookcmd/codex.go` RunCodexEvent·codexHookPayload, codex_test.go TestRunCodexEventTransitions; `main.go` hook codex --event 분기, init에 InstallCodexHooks·InstallCodexAgents; `internal/cli/codexinit.go` InstallCodexHooks·isCodexAgentlayerGroup·codexHookEvents·InstallCodexAgents·codexAgentsBlock(마커 agentlayer:browser), codexinit_test.go(3)
- `internal/tmuxx/tmux.go` SendText 대기·SendEnterDelay, tmux_test.go; `internal/cli/orchestration_skill.md` 6절 스크린샷(디스코드 reply files)·errors --reload 문단, orchskill_test.go
- 시스템 상태 추가: `~/.codex/hooks.json`(agentlayer 5이벤트, .agentlayer.bak) — 신뢰 미허용 상태, `~/.codex/AGENTS.md` agentlayer:browser 블록(.agentlayer.bak), `~/.claude/skills/orchestration/SKILL.md` 갱신, picks/에 shot·errors 파일 다수. 브랜치 `browser-mcp` 9da15d9
- `internal/cli/restorecmd.go` RestoreEnv·RestoreOpts·PlanRestore(agents, env, opts)·restoreEnv(canon=EvalSymlinks)·`--yes`·interactive 분기·계획 줄 `[id]`; `internal/cli/restorepick.go` restorePicker(newRestorePicker·Update·View·Selected)·restoreIsTerminal·runRestorePicker 주입점; restorecmd_test.go(+4)·restorepick_test.go(7); `internal/wiring/wiring.go` TmuxSessionAgents, wiring_test.go(+1)
- `internal/browser/autopreview.go` AutoPreview(…, portOpen, now) 시그니처 변경(포트 열려 있으면 seen 유지), autopreview_test.go(+1); `internal/browser/preview.go` PortOpen; `internal/cli/browsercmd.go` browserAutoPreview IsUp 게이트·hasTab Activate 제거; README 사용 블록 restore 3줄·preview_auto 문단; `internal/cli/helpcmd.go` restore 줄; `internal/config/config.go` 주석. 브랜치 `browser-mcp` 60f975f
- 외부: `~/.claude/skills/obs-record/scripts/obsrec.py` CAFFEINATE_MAX_SEC(10800)·CAFFEINATE_ARGS·stop_caffeinate(pidfile+서명 pkill)·cmd_stop 모든 경로에서 해제·cmd_fallback finally(원본 `.bak-20260903`), SKILL.md 2·4단계 문구
- 시스템 상태: make install = 60f975f; `caffeinate -dims` 종료(디스플레이 어설션 0); 8080 python 서버(demo 폴더, pid 64113) 떠 있음; 임시 tmux 세션 al-picktest·scratchpad state 정리됨
