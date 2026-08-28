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

**v1.2.0 릴리즈 상태 유지.** 오늘(8-29)은 재부팅 후 "어젯밤 상태로 부활 안 됨" 문제를
추적 — 원인은 tmux-resurrect/continuum(autosave 8/26 14:24부터 고장, 3일 묵은 스냅샷
부활)이라 ~/.tmux.conf에서 제거하고 재부팅 절차를 agentlayer restore 중심으로 확정.
촬영 중 Opus의 gemini→agy 핫픽스는 검증(IneligibleTierError 실측) 후
usage.GeminiCommand 공용화로 재정리·커밋 완료.

## 다음 단계
<!-- 덮어쓰기. 첫 항목 = 다음 세션이 바로 집어들 일 -->

1. 촬영 백업 정리: `~/.local/state/agentlayer/agents-backup-filming/` 레코드 3건
   (claude-1·claude-6·claude-26) 아직 남아 있음 — 복원 불필요 확인되면 폴더째 삭제
2. `restore <id>` 개별 선택 추가 검토 — restore-lab 무승인 부활 사고로 필요성 실증됨
3. 보류 아이디어: Termius용 좁은 폭 컴팩트 모드(60칸 미만 컬럼 축소), MultiAgent 패널
   날짜 필터, 미리보기 원본색(-e), Orca 대비 메모리 측정 스크립트(영상용), provider
   게이지 막대 텍스트화

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
- `internal/wt/lifecycle.go` agentCommand["gemini"]="agy" (미커밋 — 검증 후 커밋 대상)
