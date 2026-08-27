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

v1.0.0 위에 기능 3종 추가·커밋 완료(340e7fd, **미push·미릴리즈**): ① Discord 카드 이벤트
구동 갱신(hook 전이 → 2~3초 내 게시) ② 카드 표시 TUI 동등화 ③ `restore` 서브커맨드
(재부팅 후 tmux 배치 재구성, `--resume`이면 대화까지 부활). ctx 오귀속 버그 수정 포함.
make install 반영됨. restore-lab 실험으로 죽이기→부활 전 사이클 검증 완료.
사용자가 workspace=세션 개념으로 폴더별 tmux 세션 분리 정리함(agentlayer dev·agentlayer youtube 등).
사용자 실재부팅 테스트 예정. brew 배포본은 아직 v1.0.0.

## 다음 단계
<!-- 덮어쓰기. 첫 항목 = 다음 세션이 바로 집어들 일 -->

1. 사용자 재부팅 실테스트 지원 — 절차: `restore --dry-run` 확인 → 마감했으면 `restore`+`wake-all`,
   못 했으면 `restore --resume`. 피드백 수렴(예: restore 개별 선택 `restore <id>` 필요성)
2. push + v1.1.0 릴리즈 검토 — 새 기능 3종을 brew 사용자에게 배포 (goreleaser, 사용자 승인 먼저)
3. 영상 세션 후속 질문 응대 — 촬영 중 기술 확인, Orca 실측 요청 오면 사용자 승인 먼저
4. `version` 서브커맨드 추가 검토 — brew 사용자가 `agentlayer version` 시도하면 "알 수 없는 명령"
5. 보류 아이디어: MultiAgent 패널 날짜 필터, 미리보기 원본색(-e), Orca 대비 메모리 측정 스크립트(영상용),
   provider 게이지 막대도 거슬리면 텍스트화

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
