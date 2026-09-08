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

**윈도우 지원 공지 전부 완료**(2026-09-08 마감). v1.4.0 릴리즈·설치기 0.1.18 WSL2 재확인 ✓·디스코드 멤버 공지 3건(agentlayer 1546681035939381289 / 하네스 1546694530210857050 / 확인 목록 1546699591137759282)·유튜브 커뮤니티 통합 공지 1건(수정됨). 설치기 README 리눅스 반영 푸시됨(e38a2f6). 남은 열린 작업 없음 — 다음은 곁가지·후속 후보뿐. VM `ubuntu-agent` 일시정지.

## 다음 단계
<!-- 덮어쓰기. 첫 항목 = 다음 세션이 바로 집어들 일 -->

1. 곁가지: usage-coach는 v0.1.4 태그만 있고 GitHub 릴리즈 페이지 latest는 v0.1.3(설치기는 태그로 받아 무해, 단독 사용자 안내 시 릴리즈 생성 고려). my-videos `videos.json`에 W6C5IuDUFW8(9/6 에이전트 브라우저 편) 미등록 → `sync_videos.py`.
2. 후속 후보: 설치기 remove 끝에 `systemctl --user daemon-reload; reset-failed` 추가(WSL2 재확인 때 유닛 파일 삭제 뒤 런타임 잔상 3건 — codex-discord-tui RemainAfterExit는 stop 필요, rc2 때는 0건) /  `internal/discord/card.go`·`card_test.go` gofmt 미적용(기존, 내용 무관) / agy 권한 프롬프트 대기가 [WORK]로 보임(agy 훅에 승인 대기 이벤트 없음 — 표시 개선 여지) / 에이전트 브라우저 창 식별 강화 / FX iframe / mcp-serve resize 차단 / CfT 갱신 명령 / favicon 404 소음 / autopreview 포트 제외 / 관제탑 restore 체크리스트 키 / codex trusted_hash 자동 기록 / `agentlayer browser`(인자 없음) 재실행 시 붙어 있는 동작·`browser errors` Enter 대기 / 매뉴얼 "윈도우에서 시작하기" 장(멤버가 README를 어려워하면).
3. 디스크: 내장 20GB 여유. 사용자 판단 대기 항목 — agy 대화 기록 6.4GB(`~/.gemini/antigravity-cli/{conversations,brain}`), codex 세션 6GB(`~/.codex/sessions/2026`, `.tmp/marketplaces`), Chrome 캐시 3.3GB(종료 후), VS Code 구버전 확장 0.55GB(`openai.chatgpt-26.825.41651`), Playwright chromium-1187 0.5GB, 영상·백업 7GB는 T7으로 이동. 리포트 `~/Downloads/ClaudeDir/disk_analysis_20260906.md`.
4. 촬영 뒤 정리(이월): 8080 python 서버, `~/.local/state/agentlayer/picks/`, demo 저장소 reset, worktree hero-bold-*. 대시보드 옛 핑 삭제(567c2c9).

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
- 2026-09-04 사용자 "재정박하고 대기해" — v1.3.0 릴리즈 보류(browser-mcp→main FF 머지·빌드·테스트·goreleaser check는 완료). 타워(agentbrowser-1d)에 대본 근거 4회 회신: MCP만 vs agentlayer 경계 O/X(리로드·콘솔·전체 스크린샷·격리는 [M], 공유 브라우저·pick·shot --notify·⎇자동·cookies는 [A]), C 촬영 실측(코덱스는 MCP list_console_messages·take_screenshot fullPage 2560×2656, `errors --reload`·`shot --notify` 0회, 파일 저장은 mcproots 보정 덕), 도식 "기존: 클로드→브라우저→사용자 / 지금: 3사↔에이전트 브라우저↔사용자(맥·폰)" 승인, 기능 17개 목록·강조 순위, 설치 명령 확정(`brew install netwaif/tap/agentlayer`·`agentlayer init`·업그레이드 뒤 init 필수; tap 1.0.0은 로컬 tap 낡음 → `brew update`)
- 2026-09-04 **실크롬이 안 뜨고 에이전트 브라우저만 뜨던 원인** = LaunchServices가 시스템 Chrome 실행 파일로 띄운 에이전트 브라우저를 com.google.Chrome으로 등록 → Dock 클릭이 새 프로세스 대신 그 인스턴스에 창 추가(`open -a` 실측 프로세스 수 불변, `open -n`만 신규). 봇 부팅 때 에이전트 브라우저가 먼저 뜨게 되면서 표면화. 래퍼 앱 3종 폐기(스크립트 exec·심볼릭 링크 → 실행 경로로 재식별 / 복사+ad-hoc 재서명 → LS 식별은 성공하나 렌더러 헬퍼가 샌드박스에 막혀 dlopen 실패). 사용자 결정으로 **Chrome for Testing** 채택(engine.go, 첫 기동 200MB 12초 실측, CfT 떠 있을 때 `open -a "Google Chrome"`이 새 기본 프로필 크롬 기동 확인). 스펙 2026-09-01 정정 절 추가
- 2026-09-04 CfT "자동 테스트 전용입니다" 띠 = infobar_utils.cc, `CommandLineFlagSecurityWarningsEnabled` 정책 IsManaged일 때만 끔(사용자 defaults는 실측 무효). 유일한 예외 IsGpuTest()=`--test-type=gpu` → 채택(실측 제거). 번역 말풍선은 `--disable-features=TranslateUI`가 152에서 무효 → `Translate,TranslateUI`(실측 제거)
- 2026-09-04 **사용자 실크롬 강제 종료 고질 문제 원인**(agentlayer 무관) = macOS iCloud 암호 확장(pejdijmoenmkgeppbflobdenhhabjlaj) 네이티브 헬퍼 PasswordManagerBrowserExtensionHelper가 커널 UE 상태로 멈춤(kill -9 무효, 시작 2분 뒤부터). 크롬은 자식 종료를 기다리므로 창만 닫히고 본체 잔존. 사용자가 확장 제거 → 재실행 3분 뒤 헬퍼 없음 → 종료 메뉴 1초 정상 종료 실측. 메모리 `chrome-force-quit-icloud-passwords.md`
- 2026-09-04 TASK 칸 "Claude needs your permission" 잔존 = Notification 문구를 Task(최근 작업)에 넣고 안 지움(d413c3d 초기 테스트가 "stop 뒤 유지"를 단언, 사유 미기록). Task를 읽는 곳이 5곳(view·notify 본문·discord 카드·info·status)이라 Task 의미는 보존하고 **Ask 필드** 분리: notification이면 채우고 post-tool-use·user-prompt-submit·stop이면 비움. Headline()=Ask 우선. 낡은 레코드 4개 수동 정리

- 2026-09-05 **에이전트 브라우저 조작 효과(FX)** 설계: 사용자 "AI가 조작할 때 마우스·창 테두리 효과 없어 안 보임". 제어 경로에 agentlayer가 없어(MCP→CDP 직결) 두 사실을 이용 — ① mcp-serve가 tools/call·응답을 다 보는 stdio 프록시 ② 엔진이 CfT라 `--load-extension` 가능(브랜드 Chrome 137+는 무시). 확장(콘텐츠 스크립트)이 그리고, 프록시가 `<html data-agentlayer-fx="on:<tool>:<ts>|off:<ts>">`를 CDP Eval로 써서 신호. 좌표는 CDP 합성 마우스 이벤트를 capture 단계에서 읽음. 테두리는 OS 창 프레임이 아니라 뷰포트 안쪽 inset 글로우(네이티브 오버레이 창은 무거워 기각, 사용자 "추천대로"). 팔레트 테라코타 #d97757·크림 #faf9f5. 첫 실행에서 "AI" 뱃지가 take_snapshot에 StaticText로 노출 → host aria-hidden+inert. config `browser_fx`(기본 true)는 신호만 끔(확장은 항상 로드).
- 2026-09-05 FX 가시성 실측 실패 → 수정: 조작 도구에만 켜고 응답에 즉시 끄면 호출 하나가 수백 ms라 "순간순간 짧게, 거의 안 보임". 모든 tools/call 추적(readOnlyTools·IsActionTool 삭제) + 콘텐츠 스크립트가 off를 `LINGER` 2500ms 뒤 적용(새 on이면 취소) + 글로우 3px·3단, 커서 30px, 상단 "AI 조작 중" 알약. 같은 실측에서 창 크기가 수시로 바뀜 = 에이전트가 `resize_page` 390×844↔1440×900 호출(세션 jsonl로 확인) → 스킬·codex 블록에 "묻지 않고 resize_page·emulate 금지" 명시(프록시 차단은 후속 후보).
- 2026-09-05 **스킬 분리**: 사용자 "브라우저 지목이 왜 orchestration 스킬에?" → 7b3a55a에서 코디네이터용 6절로 시작해 쿠키·pick --once·폰 스크린샷이 관성으로 덧붙은 것. 원칙 "스킬은 사용자가 무엇을 시키는가 단위" → `orchestration`(코디네이터: wt new·2단 dispatch·status 폴링·취합·머지, N=1 포함)과 `agent-browser`(에이전트 누구나: 공통 규칙·보기/찍기·증상 진단·지목 받기·쿠키·dev 서버 보여주기·하지 말 것). orchestration 6절은 "worker 브라우저 일은 agent-browser 규칙"+"dev 서버 URL 한 줄" 2불릿. codex AGENTS.md 블록은 이미 브라우저 전용이라 유지. 설치기 orchskill.go→skills.go `installSkill`+`Skills` 목록, 관심사 분리 테스트(TestSkillsSeparateConcerns) 추가.
- 2026-09-05 유튜브 대본(에이전트브라우저-낭독대본-clean.txt) 시연 C 검수: 콘솔 에러 읽기는 chrome-devtools MCP 능력이라 "에이전트 브라우저 덕분"으로 강조하지 않기로. 장면의 무게는 원격 흐름(폰 한 줄→진단→페이지 스크린샷→고치기→확인)과 "집에 오면 같은 브라우저에 그대로". 사용자 피드백 "낭독이야, 크리티컬만" / "이건 유튜브야, 적당히 설명하고 질문의 여지로" → 메모리 feedback-script-brevity.md. 시연 C 밖은 미수정(diff로 증명).

- 2026-09-06 **v1.3.0 릴리즈 완료**: origin/main에 사용자가 다른 곳에서 올린 README 배너 2건(47f498b·b5ac566)이 있어 첫 push 거부 → 병합 fe5a8e4, 원격 태그 삭제 후 재태깅, goreleaser 성공(자산 2종+checksums, tap Casks 1.3.0). 릴리즈 노트는 직접 작성(scratchpad). 디스코드 공지·매뉴얼 배포는 보류(영상 링크·매뉴얼 원본 갱신 선행).
- 2026-09-06 **cookies export 신설**(91037b9, bounded 설계 승인 후 TDD): `browser cookies export <도메인> [이름] --to <파일> [--format value|netscape|json]` / `--env <.env> <KEY>`. 값은 0600 파일로만, stdout·로그엔 요약만(list의 "값 비노출" 정책 유지), stdout 출력 옵션은 일부러 없음. 같은 이름 여러 호스트면 정확 호스트→최장 만료. 3사 자연어: agent-browser 스킬 트리거·codex AGENTS 블록·**GEMINI.md 마커 블록 설치기 InstallGeminiAgents 신설**(codexinit.go의 installAgentsBlock 공용화, init이 `~/.gemini` 있으면 설치). 동기: 사용자 "사용량 모니터 설치 때 F12로 sessionKey 찾던 일을 클로드가 못 해줬다".
- 2026-09-06 **구글 세션은 복사 불가 — 원인은 PSIDTS 회전 충돌**(9ca9ea8로 정정): `cookies import google.com` 뒤 첫 구글 요청에서 로그인 쿠키 19개 삭제, export→`notebooklm auth import-cookies`는 형식 통과·호출 "Authentication expired". 처음엔 "기기 바인딩"으로 오판했으나 사용자의 VPS 복사본이 살아 있고 맥 원본이 죽은 것으로 "같은 세션 복사본 둘 중 먼저 회전한 쪽만 생존"이 맞음. 규칙: 맥 storage_state를 VPS로 재복사 금지, VPS용은 `notebooklm -p vps login`으로 별도 세션. x.com 등 회전 없는 사이트는 복사 됨. knot note `auth-migration-mac-to-vps`(1e8c577)에 도구별 격리·폴백 정리.
- 2026-09-06 **cookies import 안착 확인**(37a59b0): StorageSetCookies는 잘못된 쿠키를 에러 없이 버림 → injectCookies가 GetCookies로 대조해 거부된 것을 이름@호스트경로로 보고.
- 2026-09-06 healthcheck 스킬: NotebookLM 항목 추가(판정은 `notebooklm list` 실호출, `auth check`는 파일 만료일만 봐서 죽은 세션도 pass — 자동 알림으로 실증), 갱신 순서 refresh→login, "VPS 같은 세션 의심" 첫 항목. `auth refresh`는 안 넣음(`list`만으로 세션 파일 갱신 실측 15:08→15:38). run.sh EXPIRED 문구 갱신(browser_profile rename 낡은 지시 제거, 로그인 창=흰 크로미움 주의). 사용자가 notebooklm 로그인 창을 에이전트 브라우저로 오인해 닫은 실측 → 후속 후보(창 식별).
- 2026-09-06~07 **리눅스 VM 구축**: 이유 = 윈도우 시청자용 WSL2 검증 환경 + 에이전트 놀이터(부수고 되돌리기). 내장 SSD 14GB→safe 정리+스냅샷 thin으로 20GB, VM은 불가 → T7(Samsung PSSD T7 1TB, 468GB 여유, APFS)에. VMware Fusion 26H1u1(26.0.1, 개인 무료, Broadcom 계정·무역규정 주소 확인 필요, macOS 13+) 에이전트 브라우저로 다운로드(cookies import broadcom.com → 약관 → Screening → 주소 양식 40자 제한). 첫 실행이 quarantine으로 AppTranslocation에서 뜨고 /Applications 번들이 0B가 됨 → ditto 재복사 + xattr 제거로 해결. VM은 vmcli로 생성(VM Create → Disk Create 80GB → vmx 직접 편집: 4코어·8GB·EFI·sata0 디스크·sata0:1 ISO·NAT vmxnet3). Ubuntu 24.04.4 서버(화면 없음, WSL2·VPS와 같은 조건) 설치는 사용자가 콘솔에서 단계별로(LUKS 끔, LVM 38→76.9G 확장, OpenSSH 체크). 설치 후 `bios.bootOrder` 삭제·ISO startConnected FALSE 필수(안 하면 revert/재부팅 때 설치 ISO로 부팅 → "Permission denied (publickey)").
- 2026-09-07 **VM 운용 결정**: 자동 시작 없음(사용자 "매번 켜는 건 아님"). 맥 `~/.local/bin/vm` 명령(여러 VM: `vm <동작> [이름]`, 이름은 폴더 앞부분, T7 마운트 대기, SSH 준비 대기, 게스트 IP 자동 반영 sync_ip → `~/.ssh/config` HostName 갱신, StrictHostKeyChecking accept-new). 전원은 vmrun(`start nogui`·`suspend soft`·`stop soft`·`revertToSnapshot`), 스냅샷 생성·삭제는 vmcli(Delete/Revert는 uid만 받음 → snap_uid). vmcli `Power Suspend`는 `-o` 의미 미문서라 미사용. 실측: suspend 1분30초, 재개 20초. 규칙: 맥 끄기 전 `vm suspend`. 스킬 `linux-vm`(604 words) 서브에이전트 테스트 통과. 스냅샷 이름은 영문(한글은 vmcli가 따옴표 붙임).
- 2026-09-07 VM 안 도구(사용자 요청 "맥에서 즐겨 쓰는 것"): zsh+oh-my-zsh+p10k(맥 `.p10k.zsh` scp)+autosuggestions/syntax-highlighting, yazi·lazygit·zoxide(`cd`)·fzf 0.74(apt 0.44는 `--zsh` 없음→바이너리)·eza·fd·rg·bat·glow·gh·nvim·ranger, Docker 29.8, Node 24, Claude Code 2.1.263(/usr/local/bin 링크로 비대화형 SSH에서도), sudo NOPASSWD. VM에서 GitHub API 무인증 호출은 제한에 걸리므로 릴리즈 URL은 맥에서 조회해 직접 지정.
- 2026-09-06 사용자 질문 정리(답변만, 작업 없음): 리눅스/윈도우 AI 환경 우열(리눅스=GPU·컨테이너·격리 원산지, 맥=로컬 추론·앱 조작, 윈도우는 WSL2면 리눅스급), 멤버 "윈도우 지원" 공지는 검증 뒤(지금은 방향 표명만), Boot Camp(2020 iMac 인텔, Win10까지 공식)에서 WSL2 가능.

- 2026-09-07 **v1.3.1 릴리즈 완료**: push(1cf3144)→태그→goreleaser(GITHUB_TOKEN=$(gh auth token), --release-notes scratchpad)→자산 2종+checksums, tap Casks 1.3.1. 같은 날 매뉴얼 13장을 v1.3.1 실물과 대조해 agentbrowser-loops 세션에 수정 목록 전달(tmux send-keys, 핵심: 엔진이 시스템 Chrome이 아니라 CfT / cookies export 누락 / 구글 예외 / Keychain '항상 허용' / init 설치 목록에 agent-browser 스킬·GEMINI.md / FX 절 신설)
- 2026-09-07 **리눅스 검증 1차(VM 헤드리스)**: codex·gemini·bun·agy 설치(모두 `/usr/local/bin` 링크), `claude plugin marketplace add/install`이 로그인 없이 됨. loadout(0.5.1)·multi-agent-starter(3.6.0) 생성기 전 flavor PASS, discord 플러그인 bun 기동 OK. **디스코드 하네스만 리눅스 불가** — harnessctl.py darwin 체크 + 정본 3레포 launchd 의존(systemd 분기 필요). 상세 `docs/linux-wsl2-verification.md`. VM 스냅샷 `members-tools`(도구+플러그인 설치 후, 로그인 전). 순서 결정: 포팅은 맥 세션이 하고 VM은 ssh로 검증대, VM Claude에게 GitHub 주소 주는 방식은 마지막 시청자 시뮬레이션 단계에서만. 윈도우 버전 별도 없음(같은 코드·linux 자산 1종 추가, brew cask는 mac 전용이라 리눅스 설치 경로 별도 필요)
- 2026-09-07 **agentlayer 리눅스 포팅 완료**(브랜치 linux-port → main FF 34d8fa6): 스펙 `docs/superpowers/specs/2026-09-07-linux-port-design.md`·계획 `docs/superpowers/plans/2026-09-07-linux-port.md`. 분기 원칙 runtime.GOOS 한 곳씩, build tag 없음. 변경: engine.go(linux64+Go extractZip, arm64 리눅스는 시스템 Chrome), cookies.go(CookieImportSupported·ErrCookieImportUnsupported, CLI가 연결 전에 거름), capturelock(CaptureJanitorSupported), notify.go(Sender.RunOSA→Notify, desktopNotifier: darwin osascript/linux notify-send/없으면 nil), helpcmd(TerminalLabel), instance.go(displayAvailable — 리눅스 DISPLAY 없으면 명확한 에러), .goreleaser.yaml goos linux, install.sh(신규, releases/latest 리다이렉트로 태그 조회, AGENTLAYER_VERSION/DRY_RUN/BIN_DIR), README 설치·CfT 절. VM 실측 5차(hook 전이 idle→DONE→dead, CfT linux64 290MB 다운로드 OK, 무화면 에러 문구) `docs/linux-wsl2-verification.md`. 브랜치 정리는 사용자 위임으로 1번(로컬 FF 머지, 푸시 보류) 선택
- 2026-09-07 **v1.4.0-rc1 사전 릴리즈**(b174dd8 직전 34d8fa6 태그): 자산 darwin 2·linux 2·checksums. goreleaser가 rc를 prerelease로 표시하지 않고 tap cask도 1.4.0-rc1로 밀어 수동 복구 — `gh release edit --prerelease`, `gh release edit v1.3.1 --latest`, tap `Casks/agentlayer.rb`를 이전 커밋(3277cf0) 내용으로 PUT(6f987d0). 재발 방지 `.goreleaser.yaml` `release.prerelease: auto`·`homebrew_casks.skip_upload: auto`(b174dd8). 교훈: goreleaser 실행 전 워킹트리 클린 필요(SESSION.md stash). Win10 WSL2 검증 지침을 NAS `/Volumes/private/mac-to-win10/README-agentlayer-wsl2-test.md`에 작성(설치 `AGENTLAYER_VERSION=v1.4.0-rc1 bash <(curl …install.sh)`, 7절 체크리스트, 결과 파일 형식). 배포 전 점검: Drive PDF `agentlayer 한국어 매뉴얼 v1.3.1.pdf`(ID 1c6ud4ALnvfJaOFVtFq4VmFKwJAzQO6eZ) 11:33 로컬 빌드와 크기 일치, latest=v1.3.1, cask 1.3.1
- 2026-09-07 **디스코드 하네스 리눅스 서비스 층 완료**(launchd→systemd 사용자 유닛): 4레포 패치·푸시 — discord-multiagent v0.1.2(오케스트레이터 유닛+.tmux-cmd 사이드카·bot-restart ~/.cache), codex-discord v0.1.7(daemon·gemini simple Restart=always, tui oneshot KillMode=process), usage-coach v0.1.4(service+5분 timer), discord-harness-installer 0.1.15(preflight OS·systemd·WSL2 안내, write_chat_unit, service_file OS별, remove/doctor/verify 경로, 토큰 저장 대체 명령, pins 갱신). 각 스크립트 `HARNESS_OS`+`DRY_RUN` 테스트 계약. VM E2E: 유닛 6개 생성·enable·linger, remove 정리 확인(검증 6차). 사용 환경 결정(사용자 승인): VPS=손 안 댐(enable-linger) / WSL2=우분투 켜져 있는 동안(터미널 열어 두기). **agentlayer codex notify 삽입 버그 곁다리로 발견·수정**(b83aeb1): 최상위 키 다음이 바로 [section]이면 notify가 앞 줄에 붙어 config.toml을 깨뜨림(codex 사망) — 삽입을 헤더 줄 시작으로. **codex TUI 신뢰 프롬프트도 해결**(codex-discord v0.1.8, d7cf793): install.sh가 CODEX_WORKDIR를 config.toml 프로젝트 신뢰로 선등록(섹션 EOF·멱등·.bak), tui-up이 hooks 신뢰를 --dangerously-bypass-hook-trust로 넘김. VM 실측 두 프롬프트 다 제거. 설치기 pins codex-discord v0.1.8·0.1.16.
- 2026-09-07(저녁) **Win10 WSL2 1차 실기 결과 반영 → v1.4.0-rc2**. 결과(NAS `RESULT-wsl2-20260907.md`, 스크린샷 7장): 설치·init·TUI·팝업·브라우저 창(WSLg on Win10)·지목·FX·멤버 플러그인·하네스 systemd 전부 ✓, codex sysctl 불필요. 고친 것 ① npm codex가 영원히 [dead] — pane 전면이 `node` 래퍼. `internal/scan/proc.go` 신설: 래퍼 pane이면 `ps -axo pid=,ppid=,args=`로 pane_pid 자신·자식(깊이 1)의 인자에서 판정(`KindFromArgs`: basename claude/codex/gemini/agy 또는 npm 패키지 경로). 깊이 1로 제한한 이유 = 에이전트가 Bash로 띄운 다른 CLI 오판 방지 ② agy 훅 미등록 — `main.go` 조건을 `~/.gemini/config/` 존재에서 `agyInstalled()`(config/·antigravity-cli/·PATH agy)로. agy 릴리즈 노트로 `/hooks`도 `~/.gemini/config/hooks.json`에 쓰는 것 확인(1차 보고서의 "antigravity-cli/settings.json" 추정은 오류) ③ `displayAvailable`을 `EnsureEngine` 앞으로 ④ 공유 라이브러리 누락 힌트 `launchHint`(리눅스+"error while loading shared libraries"만). 하네스 쪽 ⑤ discord-multiagent `scripts/bot-up.sh` GNU stat/`~/.cache` 분기 → v0.1.3(브랜치 release-snapshot), 설치기 pins→v0.1.3, 0.1.17. **정식 v1.4.0은 rc2 재검증 뒤** — 스캐너 수정은 단위 테스트만이라 실기 확인 필요.
- 2026-09-07 교훈: `GITHUB_TOKEN=$(gh auth token) goreleaser release …` 한 줄은 auto 분류기에 막힘 — `export GITHUB_TOKEN=$(gh auth token)` 뒤 goreleaser를 같은 호출 안에서 따로 쓰면 통과. NAS `private` 공유는 `open smb://Netwaif-Storage.local/private` 뒤 NetAuthAgent 창(키체인 암호 자동 채움)의 「연결」을 System Events로 클릭하면 마운트됨(`smbutil view -N`은 인증 실패).

- 2026-09-08 **WSL2 rc2 재검증 ✓ → v1.4.0 정식 릴리즈**(57a407f). 결과(NAS `RESULT-wsl2-rc2-20260908.md`): agy 훅 등록·codex idle→WORK→DONE→dead·agy idle→WORK→DONE·브라우저 디스플레이 선검사·FX 배지 한글·하네스 chat-claude 세션 기동 전부 ✓. 정식에 추가로 넣은 것 1건: init 재실행 때 자기 팝업 바인딩에도 "다른 키를 고르세요"가 나오던 것 → `PrintTmuxBinding(w, existing, binPath)`(list-keys 출력을 넘겨 "agentlayer" 포함이면 `이미 등록됨 — 건너뜀`, 남의 것이면 경고+현재 줄). goreleaser 12초, prerelease auto·cask 1.4.0 정상. rc 태그는 그대로 둠(prerelease 표시).
- 2026-09-08 **설치기 0.1.18 — chat-claude MCP 신뢰 프롬프트 원인은 폴더 신뢰**. 보고서 진단("설치기가 chat/.claude에 폴더만 만듦")은 remove 뒤 관찰이라 틀림 — 설치기는 chat settings.local.json도 쓴다(테스트 `test_overlay_writes_bot_settings_with_merge`). 맥 재현(scratchpad `mcprepro-*`, tmux로 `claude --permission-mode auto`를 chat/에서 기동): git 아닌 폴더는 루트·chat 양쪽 `enableAllProjectMcpServers`여도 프롬프트(E1·E2·E3·E4), git 폴더는 신뢰 수락 뒤 프롬프트 없음, `~/.claude.json projects["<chat 절대경로>"].hasTrustDialogAccepted=true` 선등록이면 바로 입력창(E5), 루트만 선등록은 불충분(E6). 근거: 신뢰 전 폴더에선 프로젝트 settings의 MCP 승인이 무시됨(Claude Code 2.1.196+, 문서 mcp.md) + 프로젝트 경계=git 루트(없으면 cwd). 수정: `write_project_trust`(오버레이 단계, `<work>`·`<work>/chat` 둘 다, 이미 true면 손 안 댐, `state.trust_added`), remove가 false로 되돌림. `--strict-mcp-config`는 chat이 codex MCP를 못 쓰게 되니 배제. WSL2 실기 재확인은 Win10 세션 몫.
- 2026-09-08 교훈: 실기 보고서의 "원인 진단"은 관찰 시점(설치 직후 vs remove 뒤)을 먼저 따질 것. 재현은 맥 tmux+scratchpad로 1분이면 됨 — `~/.claude.json`에 scratch 경로 항목이 남으니 실험 뒤 지울 것(이번엔 지움, 백업 scratchpad `claude.json.bak-*`).

- 2026-09-08 **v1.4.0 멤버 공지 게시**(채널 1522490241859059784, id 1546681035939381289, 게시 뒤 edit_message로 설치 절 교체). 사용자 피드백으로 잡힌 공지 규칙: ① 매뉴얼처럼 기술적으로 쓰지 말 것 — 명령어 대신 "클로드 코드에 이렇게 부탁하세요" 붙여넣기 문장 ② 이번 건 리눅스보다 **윈도우** 강조 ③ 실기 검증은 Win10 스토어판 WSL+Ubuntu VM뿐 — Win11은 "검증했다" 쓰지 말 것(조건이 기본 충족이라 같은 방식으로 된다 정도) ④ WSL 설치 안내를 빼먹었었음 — Win10 검증도 윈도우 쪽 클로드 세션이 WSL·우분투·클로드 코드 설치를 다 했으니(사람은 재부팅만) 멤버도 "윈도우 클로드에 부탁" 한 문장으로 충분, 재부팅 뒤 같은 문장 재입력만 안내 ⑤ 단락마다 빈 줄 ⑥ 하네스 공지 예고 문장 포함. 매뉴얼 리눅스 절은 개정 안 함 — README 설치 절에 이미 다 있음(사용자 확인).
- 2026-09-08 **고정댓글 갱신 규칙**: agentlayer 계열 최신 영상은 W6C5IuDUFW8(9/6, 고정댓글 UgyV5G5YSvYz2y9JkER4AaABAg, 🔄 업데이트 블록 보유). 이전 COcgg7Q_r8U 고정댓글은 이미 리다이렉트 문구라 손대지 않음. 갱신은 comments.update로 기존 본문 끝에 줄 추가(scratchpad `update_comment.py`, 백업 `~/.claude/skills/youtube-pinned-redirect/backups/`). **유튜브 댓글은 공개라 "디스코드 공지 참고" 같은 멤버 전용 언급 금지**(사용자 지적).
- 2026-09-08 공지 순서 결정(사용자 승인): agentlayer 본체 공지 먼저 → Win10에서 설치기 0.1.18 재확인 → 하네스 윈도우 공지 따로. 합치지 않는 이유: 하네스 미확인 상태로 같이 올리면 되돌리기 어렵고 본체 공지가 밀림.

- 2026-09-08 **설치기 0.1.18 WSL2 실기 재확인 통과**(Win10 세션, 결과 NAS `mac-to-win10/RESULT-wsl2-followup-0.1.18-20260908.md`): install `폴더 신뢰 선등록 …/chat`·chat-claude "New MCP server found" 없이 입력창·`~/.claude.json` 양쪽 True·remove `폴더 신뢰 선등록 회수`(chat False 복원, 사용자 settings.local.json 50B 원복). Win10 쪽 자동 모드 분류기가 tmux send-keys를 막아 harnessctl.py를 직접 실행했음(검증 대상 동일). 참고: wsl.exe 직접 셸에서 `/run/user/1000` 위 WSLg tmpfs 이중 마운트로 `systemctl --user` 버스 실패 → `sudo systemctl restart user@1000`(설치기 무관). NAS는 이번엔 `/Volumes/private`로 정상 마운트(-1 아님).
- 2026-09-08 **하네스 윈도우 공지 게시**(채널 1522490241859059784, id 1546694530210857050). 사용자 수정 1건: "지난 공지에서 예고드린 대로" → "바로 앞 agentlayer 공지에 이어서 올립니다"(연이어 올리는 공지라 '지난'이 어색). 공지 전 발견: 설치기 README 요구 사항 절이 "macOS 전용·리눅스·윈도우 미지원"으로 낡아 있어 리눅스(VPS)·WSL2 지원 + 터미널 열어 둔 동안만 동작 + 실기 검증 환경으로 교체(e38a2f6). 매뉴얼 v3.0은 개정 안 함.

- 2026-09-08 **유튜브 커뮤니티 공지(일반 시청자용)**: 멤버는 디스코드, 비멤버는 커뮤니티 — 둘 다 올려야 함(사용자). 두 공지를 하나로 합쳐 기술 설명 없이·윈도우 강조·마무리 인사 없음·문단 개행. 전날 커뮤니티에 "윈도우 검증 시작합니다"(네 가지: 실행환경·loadout·멀티에이전트·디스코드) 글이 있어 "어제 예고한 검증 중 두 가지 완료"로 이어 씀. 실사용 크롬(claude-in-chrome)으로 작성창에 넣어 두고 사용자가 게시 버튼만 누르기로 했는데, 마지막 줄 교체를 위해 본문을 클릭한 직후 글이 게시돼 버림(제 클릭이 게시 버튼에 닿았는지 사용자가 눌렀는지 불명). 사용자 승인 후 ⋮→수정으로 마지막 줄을 "CLAUDE.md 구성 백화점(loadout)과 멀티 에이전트도 윈도우 10에서 확인했습니다"로 교체(원래 "확인되는 대로 이어서"는 사실과 어긋남 — 둘 다 Win10 7차에서 이미 PASS). 교훈: 사용자가 누르게 남겨 둔 작성창은 다시 건드리지 말고, 수정이 필요하면 전체 재입력 전에 스크린샷으로 좌표 재확인.
- 2026-09-08 리눅스·Win10 지원 범위 정리(사용자 질문): agentlayer·loadout·multi-agent-starter·codex 워커·usage-coach·codex-discord·discord-multiagent 전부 VM+Win10 WSL2 실측 ✓. 단서: usage-coach 단독 install.sh 경로는 하네스 설치기 경유로만 실기 확인 / graph-run 미검증 / Win11 실기 없음.

- 2026-09-08 **윈도우 확인 목록 공지**(멤버 채널, id 1546699591137759282): 지원 범위 표를 ✅ 목록으로(디스코드는 표 미지원), 미확인(graph-run·Win11 실기)과 VM 전용 단서(codex sysctl)는 제외. 마무리 인사 없음·항목마다 개행(사용자).

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
- 2026-09-04 `internal/browser/engine.go`(신규) cftVersionsURL·cftGet 주입점·cftPlatformFor·EngineDir·EngineBin·pickCFTDownload·EnsureEngine(ditto 풀기, VERSION)·download; `engine_test.go`(신규 6개: TestCFTPlatform·TestEngineBinPath·TestPickCFTDownload·TestEnsureEngine_Existing·TestEnsureEngine_Download·TestLaunchArgs); `internal/browser/instance.go` Connect→EnsureEngine(실패 시 launcher.LookPath 폴백)·newLauncher(Delete disable-site-isolation-trials, Set disable-features=Translate,TranslateUI, Set test-type=gpu)·launchArgs; README 브라우저 절 "엔진은 Chrome for Testing" 불릿; `docs/superpowers/specs/2026-09-01-agent-browser-design.md` 2026-09-04 정정 절
- 2026-09-04 `internal/state/types.go` Agent.Ask·Headline(); `internal/hookcmd/claude.go` notification→Ask, 그 외 이벤트→Ask 비움(Task 불변); `claude_test.go` TestNotificationMessageBecomesTask(Ask, stop 뒤 비움)·TestPendingAskClearedWhenWorkResumes(신규); `internal/ui/view.go`·`internal/notify/notify.go`·`internal/discord/card.go`·`internal/cli/status.go` Headline() 사용; `internal/cli/infocmd.go` "묻는 중"/"최근 작업" 분리
- 시스템 상태: make install = ee1fa6f; `~/.local/state/agentlayer/chrome-for-testing/chrome-mac-x64/`(CfT 152.0.7977.82, VERSION) — 에이전트 브라우저 pid는 재기동마다 바뀜; `~/.local/state/agentlayer/agents/*.json` 4개 task 필드 수동 삭제; `defaults com.google.chrome.for.testing` 정책 키는 실험 뒤 삭제함; 실크롬 iCloud 암호 확장 제거됨(사용자); 8080 python 서버 여전히 떠 있음(demo); 메모리 `~/.claude/projects/-Users-soonho-ai-folder-dev-agentlayer/memory/chrome-force-quit-icloud-passwords.md`
- 2026-09-05 `internal/browser/fx.go`(신규) fxFS embed·fxAttr·InstallFx(<state>/browser-fx)·FxTracker{Start,End}·fxValue·SignalFx; `internal/browser/fx/manifest.json`·`fx/content.js`(신규, LINGER·glow·cursor·ripple·highlight·pill, MutationObserver on data-agentlayer-fx); `fx_test.go`(신규); `instance.go` Connect가 InstallFx→newLauncher(bin,profile,port,fxDir) `--load-extension`; `engine_test.go` launchArgs 4인자; `internal/config/config.go` BrowserFx·BrowserFxEnabled; `internal/cli/browsercmd.go` fxSignaler{OnClientLine,OnServerLine,signal} mcp-serve 루프 배선; README "조작 효과(FX)" 문단·browser_fx
- 2026-09-05 `internal/cli/skills.go`(orchskill.go 개명) Skills·InstallSkills·installSkill; `skills_test.go`(개명, TestSkillsSeparateConcerns); `agent_browser_skill.md`(신규); `orchestration_skill.md` description·6절 축소·하지 말 것; `codexinit.go` codexAgentsBlock에 resize 금지 줄; `main.go` runInit → InstallSkills; README 스킬 문단 2곳
- 시스템 상태: make install = 8afde8f; init 재실행(스킬 2개 `~/.claude/skills/{orchestration,agent-browser}/SKILL.md`, .bak 있음; `~/.codex/AGENTS.md` 블록 갱신); 에이전트 브라우저 CfT 재기동됨(확장 `~/.local/state/agentlayer/browser-fx/` 로드), 테스트 탭(127.0.0.1:8765, 서버는 죽음) 남아 있을 수 있음; 사용자 Claude 세션의 mcp-serve는 구버전 프로세스 → 새 세션 필요
- 외부: `~/ai-folder/youtube/AgentLoops/agentbrowser/대본/에이전트브라우저-낭독대본-clean.txt` 시연 C(243~311줄) 재작성(백업 `.bak-sceneC-*`), 시연 E 검수 의견만(미수정: 342 중복어, 349 "프로필" 중의, 358 키체인 팝업 대조, 346/353 기준 통일); 메모리 `feedback-script-brevity.md` 추가
- 2026-09-06 `internal/browser/cookies_export.go`(신규: ExportOptions·ExportCookies·exportableCookies·pickCookie·FormatNetscape·FormatCookieJSON·writeSecretFile 0600·updateEnvFile·expandHome)·`cookies_export_test.go`(순수 함수+실브라우저 통합, 값 누출 검사); `cookies.go` injectCookies·missingAfterSet·cookieKey(ImportCookies가 거부 목록 출력); `internal/cli/browsercmd.go` parseCookiesExport·browserCookiesExport·case "export"; `codexinit.go` agentsCommonHead/Tail·codexAgentsBlock·geminiAgentsBlock·InstallGeminiAgents·installAgentsBlock; `main.go` `~/.gemini/GEMINI.md` 설치 배선; `agent_browser_skill.md` export 절; `helpcmd.go`·`README.md`(export 2줄+설명+구글 예외 문단); 테스트 `browsercmd_test.go` TestParseCookiesExport, `codexinit_test.go` TestCodexAgentsBlockMentionsCookiesExport·TestInstallGeminiAgentsAppendIdempotent, `skills_test.go` export 단언.
- 시스템 상태: make install = 37a59b0(export·안착 확인 포함); init 재실행으로 `~/.gemini/GEMINI.md` 마커 블록 신설, `~/.codex/AGENTS.md`·`~/.claude/skills/agent-browser/SKILL.md` 갱신. 에이전트 브라우저에 `cookies import` 실행: google.com(103개, 구글 요청 시 무효화됨 — 지워도 무방), broadcom.com(26개). GitHub 릴리즈 노트 원본은 scratchpad(세션 종료 시 소실, GitHub 릴리즈 페이지에 있음).
- 외부(에이전트 밖): `~/.claude/skills/healthcheck/SKILL.md` NotebookLM 절·4단계 양식, `run.sh` 145행 EXPIRED 문구; `~/VSCodeWorkspace/knot/wiki/auth-migration-mac-to-vps.md`(note, 1e8c577)+`notebooklm-py.md`·`agentlayer.md` 역링크·index·log; `~/.notebooklm/profiles/kshxxthm/`(사용자가 `notebooklm login` 재로그인, browser_profile 생김→Headless Reauth ready); Broadcom 지원 포털 계정(kshxxthm@gmail.com); `/Applications/VMware Fusion.app`(26.0.1, quarantine 제거); `~/Downloads/VMware-Fusion-26H1u1-25689522_universal.dmg`(521MB, 지워도 됨); `~/Downloads/ClaudeDir/disk_analysis_20260906.md`·`disk_report.json`.
- VM: `/Volumes/VIDEO_WORK/VMs/ubuntu-agent/{ubuntu-agent.vmx,ubuntu-agent.vmdk(80GB 성장형),ubuntu-agent-Snapshot1.vmem(8GB)…}`, `VMs/iso/ubuntu-24.04.4-live-server-amd64.iso`(+SHA256SUMS.server); 스냅샷 `clean-base`(uid1, 도구 전, 메모리 포함)·`tools`(uid3, 현재 기준, ISO 해제 설정 포함); VM IP DHCP(.128/.129 오감), MAC 00:0c:29:89:05:b5; 맥 `~/.local/bin/vm`(bash, 동작 start|stop|suspend|status|ssh|snap|revert|snapdel|snaps|console|list), `~/.ssh/config` Host ubuntu-agent(HostName 자동 갱신), `~/.claude/skills/linux-vm/SKILL.md`. VM 안: `/etc/sudoers.d/soonho`, `~/.zshrc`·`~/.p10k.zsh`·`~/.oh-my-zsh`, `/usr/local/bin/{yazi,ya,lazygit,glow,fzf,fd,bat,node,npm,npx,claude}`, apt 저장소 docker·gierens(eza)·github-cli. 맥 사용자 pane "ubunt"는 `ssh ubuntu-agent` 상태.
- 2026-09-07: `docs/linux-wsl2-verification.md`(신규). VM 안: `/usr/local/bin/{codex,gemini,bun,agy}`, `~/.bun`, `~/.local/bin/agy`, `~/.claude/plugins/`(loadout·multi-agent-starter·discord·harness-installer), `~/lab/{loadout,multi-agent-starter,t-*}`, `~/.local/share/discord-harness/repos`, `~/.config/discord-harness/state.json`; 스냅샷 `members-tools`. scratchpad `manual-ch13-review.md`·`release-notes-v1.3.1.md`·`vm-verify1.{sh,log}`
- 2026-09-07(리눅스 포팅): `internal/browser/{engine.go,engine_test.go,cookies.go,cookies_test.go,capturelock.go,capturelock_test.go,instance.go,instance_test.go}`, `internal/cli/{browsercmd.go,helpcmd.go,helpcmd_test.go}`, `internal/notify/{notify.go,notify_test.go}`, `internal/config/config.go`(주석), `main.go`(주석), `.goreleaser.yaml`, `install.sh`(신규), `README.md`, `docs/superpowers/specs/2026-09-07-linux-port-design.md`, `docs/superpowers/plans/2026-09-07-linux-port.md`, `docs/linux-wsl2-verification.md`. VM 안: `~/.local/bin/agentlayer`(dev-linux 빌드), `~/.local/state/agentlayer/{chrome-for-testing/chrome-linux64,agents,browser-profile}`, init 배선(`~/.claude.json`·`~/.codex/{config.toml,hooks.json,AGENTS.md}`·`~/.gemini/{GEMINI.md,settings.json,config/hooks.json}`), `~/.claude/skills/{orchestration,agent-browser}`
- 2026-09-07(rc1): `.goreleaser.yaml`(prerelease·skip_upload auto), GitHub 릴리즈 v1.4.0-rc1(prerelease), NAS `/Volumes/private/mac-to-win10/README-agentlayer-wsl2-test.md`, scratchpad `release-notes-v1.4.0-rc1.md`
- 2026-09-07(하네스 리눅스): (agentlayer) `internal/cli/codexinit.go`+`codexinit_test.go`, `docs/linux-wsl2-verification.md`(6차). (외부 레포) `~/ai-folder/dev/discord-multiagent/scripts/{install-autostart.sh,bot-restart.sh}`+`test/autostart-linux.test.sh`, `~/ai-folder/dev/codex-discord/scripts/{install.sh,uninstall.sh}`+`test/install-linux.test.sh`, `~/VSCodeWorkspace/usage-coach/scripts/{install,uninstall}.sh`+`tests/test_install_scripts.py`, `~/VSCodeWorkspace/discord-harness-installer/`(harnessctl.py·SKILL.md·pins.json·plugin.json·marketplace.json·tests·docs/superpowers). VM 안: 하네스 잔재 전부 정리(유닛 0, linger off)
- 2026-09-07(rc2): `internal/scan/proc.go`(신규, ProcTable·KindFromArgs·DescendantKind), `internal/scan/scan.go`(Sync 래퍼 2차 판정), `internal/scan/scan_test.go`(+3 테스트), `internal/browser/instance.go`(displayAvailable 선행·launchHint), `internal/browser/instance_test.go`(TestLaunchHint), `main.go`(agyInstalled), `README.md`(리눅스 apt·fonts-noto-cjk), `docs/linux-wsl2-verification.md`(7차). 태그 v1.4.0-rc2(f634c17). 스크래치 `release-notes-v1.4.0-rc2.md`, `RESULT-wsl2-20260907.md` 사본. NAS `README-agentlayer-wsl2-retest-rc2.md`(신규). 다른 레포: `~/ai-folder/dev/discord-multiagent/scripts/bot-up.sh`(e3577d3, v0.1.3, `_shared/learnings.md`는 사용자 미커밋분 그대로 둠), `~/VSCodeWorkspace/discord-harness-installer`(ee7ed96: pins.json·plugin.json·marketplace.json 0.1.17).
- 2026-09-08(v1.4.0): `main.go`(existing 바인딩 줄 전달), `internal/cli/initcmd.go`(PrintTmuxBinding 시그니처 변경, strings), `internal/cli/initcmd_test.go`(자기/남의 바인딩 케이스), `docs/linux-wsl2-verification.md`(8차). 태그 v1.4.0(57a407f), tap Casks/agentlayer.rb 1.4.0. 스크래치 `release-notes-v1.4.0.md`·`discord-announce-v1.4.0.md`·`RESULT-wsl2-rc2-20260908.md` 사본·`mcprepro-*`(재현 폴더)·`claude.json.bak-*`. NAS `README-agentlayer-wsl2-followup-0.1.18.md`(신규). 다른 레포: `~/VSCodeWorkspace/discord-harness-installer`(28fa323: harnessctl.py `write_project_trust`+remove 회수, tests 2건, SKILL.md 한 줄, plugin.json·marketplace.json 0.1.18).
- 2026-09-08(공지): 코드 변경 없음. scratchpad `discord-announce-v1.4.0.md`(최종본은 디스코드 메시지 1546681035939381289가 정본), `get_comment.py`·`update_comment.py`(YouTube comments.list/update, youtube-reply auth 재사용). 고정댓글 백업 `~/.claude/skills/youtube-pinned-redirect/backups/W6C5IuDUFW8_UgyV5G5YSvYz2y9JkER4AaABAg_20260908-094616.txt`.
- 2026-09-08(하네스 공지): 코드 변경 없음. scratchpad `discord-announce-harness-windows.md`(정본은 디스코드 메시지 1546694530210857050). 다른 레포: `~/VSCodeWorkspace/discord-harness-installer/README.md`(e38a2f6, 요구 사항 절). NAS 읽기만: `/Volumes/private/mac-to-win10/RESULT-wsl2-followup-0.1.18-20260908.md`.
- 2026-09-08(커뮤니티 공지): scratchpad `youtube-community-windows.md`(정본은 유튜브 커뮤니티 게시물). 코드 변경 없음.
