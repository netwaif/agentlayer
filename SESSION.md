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
- 외부: 테스트 체크리스트 Artifact https://claude.ai/code/artifact/3cd3ee37-9863-462e-8ca4-603cae896ba4
