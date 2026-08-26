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

v0.8.2, main 브랜치, 전체 테스트 그린. Phase 1~4 완성 + 학원 SSH 실전 테스트에서
25건 이상 수정 완료. 기능 전부 실사용 검증됨: TUI(팝업 C-b a, 미리보기 패널, i 상세,
g lazygit, W/C 전체 기상·마감), status/card/wt/resume/wake-all/close-all/broadcast/info CLI,
Claude hook(유휴 에코 무시)·Codex notify, Discord 카드(LaunchAgent 5분 주기)·단문 알림,
worktree 전 사이클(코멘트 회신 포함). init·팝업 바인딩·notify_discord 활성화까지 완료 상태.

## 다음 단계
<!-- 덮어쓰기. 첫 항목 = 다음 세션이 바로 집어들 일 -->

1. 사용자 실사용 피드백 수집 — 특히 close-all 첫 실전(C 키 y 확인, v0.8.2에서 잘림 수정됨)과
   Discord 카드 안정성(고아 카드 수동 삭제 필요: 최신 1개만 남기기)
2. Codex 재시작 후 turn-complete(DONE) 감지 확인 (notify는 codex 시작 시 로드 — 미검증 항목)
3. macOS 알림 배너 확인 (집 Mac 화면에서만 가능 — 체크리스트 5-a)
4. GitHub 공개 + brew tap 배포 — 사용자 결정 대기 (goreleaser 설정 완료, netwaif/agentlayer 예정)
5. 보류 아이디어: MultiAgent 패널 날짜 필터, 미리보기 원본색(-e), Orca 대비 메모리 측정 스크립트(영상용)

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
- 외부: 테스트 체크리스트 Artifact https://claude.ai/code/artifact/3cd3ee37-9863-462e-8ca4-603cae896ba4
