# AgentLayer 핸드오프 — 2026-08-26 (기능 완성 시점)

> 대상: 이 프로젝트를 이어받는 다음 세션, 그리고 유튜브(AICheatKey) 영상 제작 세션.
> 정본 우선순위: `SESSION.md`(세션 재정박) → 이 문서(전체 조망) → `docs/superpowers/specs/2026-08-25-agentlayer-design.md`(원 설계).

## 한 줄 요약

iTerm2+tmux 위에서 도는 **멀티 에이전트 관제탑**. Claude Code·Codex·Gemini(Antigravity CLI 포함) 세션들의
상태(응답 필요/새 완료/작업중)·모델·컨텍스트를 한 화면에 모으고, Discord 카드로도 내보낸다.
Go 단일 바이너리, Orca를 설치하지 않고 그 관제 기능만 tmux 위에 재구현했다.

## 왜 만들었나 (영상 서사용)

- tmux로 에이전트 여러 개를 띄우면 "누가 내 응답을 기다리나"를 창을 돌며 눈으로 확인해야 한다.
- Orca가 이걸 해주지만 무겁고(인프라 층 중복), 우리 환경은 tmux가 이미 인프라다. → 관제 층만 직접 구현.
- 상태 판정은 **화면 스크래핑 없이 hook/notify 이벤트만** 사용 (설계 원칙, 미리보기 패널만 표시 전용 예외).

## 핵심 기능 (전부 실사용 검증됨)

### TUI (`agentlayer`, tmux 팝업 C-b a)
- 세션 목록: 상태(◆WAIT/✔DONE/●WORK/idle/dead) + 세션명 + TASK + 폴더 + 경과.
- **3사 그룹 정렬 + 구분선**: claude → codex → gemini 그룹, `── codex ───` 구분선(브랜드 색 라벨).
- 행 끝 뱃지 `[모델 · ctx% · 나이]`: claude=statusline 스냅샷, codex=rollout, gemini=세션파일/hook.
  gemini ctx는 근사값이라 `ctx ~3%`처럼 `~` 표기.
- 헤더: 상태 집계 + **기본모델 3종**(Claude·Codex·Gemini, 미설정=자동, Claude가 Fable이면 빨강 경고)
  + coach 사용량 요약(느린 것만 늦게 뜸 — ctxCmd/usageCmd 분리).
- 선택 세션 **화면 미리보기**(2초 라이브), i 상세 카드(배선·세션ID·ctx), g lazygit,
  W 전체기상 / C 전체마감(y 확인), u 사용량 화면, enter 점프+읽음, o 읽음.
- Discord 연결 ⌁ 마크(경로 경계 매칭 — 상위/형제 폴더 오탐 수정됨).

### 상태 추적 (hook — 3사 전부)
- **claude**: `~/.claude/settings.json` hook 5종. SessionStart→IDLE(부팅 오탐 방지),
  UserPromptSubmit/PostToolUse→WORK, Notification→WAIT, Stop→DONE_UNREAD.
  유휴 에코("waiting for your input")는 상태 안 덮음 — 단 **WORK에서 오면 놓친 종료로 보고 WAIT 복구**
  (백그라운드 셸 생존 시 Stop 유예·Esc 인터럽트 대응).
- **codex**: `~/.codex/config.toml` notify → agent-turn-complete→DONE_UNREAD.
- **gemini**: 두 계열 모두.
  agy(Antigravity CLI)=`~/.gemini/config/hooks.json`(PostToolUse·PreInvocation→WORK, Stop→DONE),
  stock Gemini CLI=`~/.gemini/settings.json` hooks(SessionStart→IDLE, BeforeAgent·AfterTool→WORK,
  Notification→WAIT, AfterAgent→DONE). gemini hook은 stdout `{}` 규약(main이 출력).
  agy hook payload의 modelName이 Agent.Model로 저장됨(파일 소스 없는 agy의 유일 모델 출처).
- 등록은 전부 `agentlayer init` 멱등 설치 + 절대 경로(최소 PATH LaunchAgent 대응).

### CLI
- `status`(그룹 정렬 표), `card`(Discord 카드 수동 게시), `info <세션>`(배선 상세),
  `wake-all`/`close-all`(세션 이어가기 규율 있는 폴더만)/`broadcast`(전체),
  `wt`(worktree 전 사이클: 생성→리뷰 코멘트 회신→정리), `resume`(비상 복구 — **3사 지원**:
  `claude --resume <sid>` / `codex resume <rollout sid>` / `agy --conversation <cid>`,
  stock gemini CLI는 재개 CLI 없음), `init`, `hook`, `codex-init`.

### Discord
- 카드: LaunchAgent 5분 주기 업서트(선게시 후삭제 — 카드 소실 방지), 상태·ctx 게이지·악화 핑.
- 단문 알림: 상태 전이 시 notify (config.json `notify_discord=true`).

## 오늘(08-26) 커밋 스토리 — 영상 소재로 좋은 순서

1. `ee9c453` 부팅하면 모든 봇이 WORK로 보이던 오탐 — SessionStart를 IDLE로 (세션이 떴다고 일하는 게 아니다).
2. `7417961` 기본모델 3사 표시 — Fable이 기본이면 빨강 경고 (토큰 새는 사고 방지).
3. `f999a25` **Gemini 완전 편입** — "gemini엔 hook이 없다"는 내 주장을 gemini 세션에 직접 물어 반증,
   agy·stock 두 계열 hook 발견/등록, 실전 검증(프롬프트 → DONE·conversationId·model 기록).
4. `068ab87` gemini ctx% 근사 + 3사 그룹 정렬 + **오귀속 버그**(같은 폴더의 claude 스냅샷이
   gemini 행에 붙던 것 — ctx 맵을 CWD 키에서 에이전트 ID 키로).
5. `f84fe14` ctx ~0% 보정(고정 오버헤드 100KB 실측 가산) + Discord 연결 오표시(경로 경계) + 구분선.
6. `0841009` 빠른 정보를 coach 대기에서 분리 — TUI 열자마자 뱃지가 뜬다.
7. `b0a682e` WORK 고착 복구 — 유휴 에코를 놓친 종료 신호로.

교훈 서사: "LLM(나)의 단정을 실측과 당사자(gemini 세션) 인터뷰로 반증" — 3·5번이 좋은 예시.

## 아키텍처 (파일 지도)

```
main.go                      서브커맨드 라우팅 전부 (hook 라우팅·init·resume 분기 포함)
internal/state/              Agent 레코드(Model 필드 포함)·Store(원자적 파일 저장, KindRank 그룹 정렬)
internal/scan/               tmux pane 발견·kind 판정(claude 버전형 명령·agy→gemini)·DEAD 정리
internal/hookcmd/            claude.go·codex.go·gemini.go — 이벤트→상태 전이 (알림 발화 지점)
internal/tmuxx/              tmux 호출 (자체 탐색·UTF-8 강제 — 최소 PATH 대응)
internal/ui/                 bubbletea TUI (model.go 갱신 루프·view.go 렌더)
internal/usage/              coach 사용량·ctx 수집(AgentCtx가 3사 규칙 단일 정본)·기본모델 리더
internal/discord/            카드 빌드·웹훅 업서트
internal/wiring/             폴더↔Discord/LaunchAgent 배선 수집 (경계 매칭)
internal/cli/                status·init(installJSONHooks 공용)·geminiinit·codexinit·all·info·wt
internal/wt/                 worktree 수명주기·리뷰 회신
```

시스템 상태: `~/.local/bin/agentlayer`(설치본), `~/.local/state/agentlayer/`(agents·캐시·카드 상태),
hook 등록 4곳(`~/.claude/settings.json`, `~/.codex/config.toml`, `~/.gemini/config/hooks.json`,
`~/.gemini/settings.json`), `~/.tmux.conf:75`(C-b a), `~/Library/LaunchAgents/com.netwaif.agentlayer-card.plist`.

## 알려진 한계 (정직하게 — 영상에서도 이대로 말하면 됨)

- gemini ctx%는 **근사값**(`~` 표기): stock=마지막 턴 tokens.total/1M, agy=transcript 크기/4+오버헤드 100KB.
  창 크기·정확 토큰이 디스크에 없어서다.
- stock Gemini CLI는 대화 재개 CLI 플래그가 없어 `resume` 불가(agy는 가능).
- coach 사용량은 콜드 시작 시 최대 2분(5분 캐시로 완화, 빠른 정보와 분리돼 화면은 안 막음).
- 상태는 hook 기반이라 hook 미등록 세션(예: 도구 갱신 전에 뜬 세션)은 재시작해야 잡힌다.

## 빌드·테스트·배포

- `make test`(전체 그린), `make install`(~/.local/bin). e2e 1건(TestTUILaunchesInTmux)은 가끔 플레이키 — 재실행.
- **brew 배포 대기**: goreleaser 설정 완료, `netwaif/agentlayer` 공개 + tap 생성만 남음 (사용자 결정 대기).

## 다음 세션이 집어들 만한 것

1. GitHub 공개 + brew tap (배포 결정 나면 바로).
2. 실사용 피드백 계속 수렴 (Discord 카드 안정성, Codex DONE 감지 재확인).
3. 보류 아이디어: MultiAgent 패널 날짜 필터, 미리보기 원본색(-e), Orca 대비 메모리 측정 스크립트(영상용).
