# AgentLayer 설계 스펙

- 날짜: 2026-08-25
- 상태: 사용자 승인 완료 ("고", 2026-08-25)
- 전신 문서: `agentlayer-local-handoff-2026-08-25.md` (Hermes 원격 세션 인수인계)

## 1. 배경과 목표

사용자는 Mac + iTerm2 + 일반 tmux 환경에서 Claude Code / Codex / Gemini 멀티
에이전트로 작업한다. Orca ADE 도입을 검토했으나 "인프라 층(세션 유지·SSH·원격)은
tmux가 이미 상위 호환"이라는 결론에 도달했고, Orca에서 가치 있는 관제 기능만
tmux 위에 직접 구현한다. Orca는 재현 불가 기능(Design Mode 등)이 필요할 때만
보조 도구로 접근한다.

목표는 둘이다.

1. **실사용**: 모든 tmux 에이전트의 상태를 터미널·Discord 어디서든 한눈에.
2. **시청자 배포**: 유튜브(AICheatKey) 영상 소재 + `brew install` 한 줄 설치.
   coach(사용량) → mat(starter 관제) → **agentlayer(tmux 관제탑)** 3부작의 완결편.

## 2. 확정된 사용자 환경 (로컬 검증 완료)

- macOS 14.8.5, tmux 3.6a, prefix `C-b`(기본값), `C-b a`는 미사용(충돌 없음)
- iTerm2 + tmux-resurrect/continuum(15분 저장, claude 프로세스 자동 재실행)
- tmux 세션 8개: 폴더 상주 folder-bot 6개(Discord 원격 조종) + 대화형 `ai` 세션
- 원격 접속: 주로 학원 Windows 터미널 SSH, 간간히 폰 Termius
- 아침 "모든 세션 이어서 시작" / 저녁 "모든 세션 마감" 스킬 + `C-b s` 모니터링
- 컨텍스트 관리: loadout "세션 이어가기" 조각(SESSION.md 재정박, 40% 규칙).
  일상에서 `--resume`은 쓰지 않는다.
- 기존 자산:
  - **coach** (`~/VSCodeWorkspace/usage-coach`, Python): codexbar 기반 사용량
    코칭. `--json` 출력 있음. git clone 배포.
  - **mat** (`~/VSCodeWorkspace/mat`, Go + bubbletea + lipgloss):
    multi-agent-starter 작업 관제 TUI. `brew install netwaif/tap/mat` 배포.
    `internal/coach`로 coach 뷰를 Go로 내장한 전례 있음.
  - **discord_dash.py** (usage-coach 안, 개인 인프라): coach 판정 + folder-bot
    생존 확인을 Discord 웹훅 카드 하나로 5분마다 업서트. LaunchAgent 구동.
  - **MultiAgent starter 정본**: `~/VSCodeWorkspace/MultiAgent` (main @ e052a90,
    mat의 MAT_ROOT가 가리키는 실사용 루트)
- 도구: fzf, jq, lazygit, gh, Go 1.25 있음. delta, terminal-notifier 없음.
- Claude Code 2.1.241(hook: Notification/PostToolUse 이미 구성),
  Codex 0.149.0(notify 지원), Gemini 0.49.0(lifecycle 신호 약함)

## 3. 통합 모델 — "수집은 하나, 표면은 셋"

기존 GitHub repo(coach, mat)는 독립 유지한다. agentlayer는 이들의 **데이터를
소비**하는 상위 관제탑이다.

```
데이터 소스: tmux · agent hooks · coach --json · MultiAgent tasks/
        ↓
agentlayer 상태 수집기 (정본: 파일, 데몬 없음)
        ↓
표면 ① 터미널 TUI  ② Discord 카드(discord_dash 승계)  ③ plain CLI/JSON
```

- 세 표면은 같은 상태 파일을 본다. 어디서 보든 정보가 일치한다.
- coach/mat이 없는 환경에서는 해당 패널만 빠지고 tmux 관제 코어는 단독 동작.
- discord_dash.py는 개인 인프라이므로 agentlayer의 Discord 뷰로 흡수한다
  (안정화 전까지 병행 운영, 안정 후 LaunchAgent 교체).

## 4. 기능 범위 (사용자 승인본)

### 4.1 터미널 관제탑 (TUI + CLI)

- 전 tmux 세션의 에이전트를 한 화면에: 상태(WORKING / WAITING / DONE_UNREAD /
  ERROR), 작업 한 줄, 폴더, 상태 경과 시간
- 줄 선택 → Enter → 해당 pane 점프 + 자동 읽음 처리
- `C-b a` tmux popup으로 어디서든 호출 (기존 단축키 무변경)
- 사용량 전용 뷰(키 전환): 현 Discord 카드의 **모든** 정보 — Claude 5h/7d/Fable
  게이지·리셋 시각·코칭 한 줄, Codex 7d, Antigravity 다중 계정별 게이지,
  봇별 모델명·컨텍스트 사용률·마지막 활동 시각
- MultiAgent(starter) 작업이 돌고 있으면 요약 패널 표시
- `agentlayer status`: plain CLI + `--json` — SSH·스크립트용

### 4.2 알림 + Discord

- 에이전트 완료 / 입력·승인 대기 시 macOS 알림 + Discord 알림, 같은 사건 중복 없음
- Discord 상태 카드: discord_dash 승계·업그레이드 — 사용량 + 봇 + 에이전트
  상태 통합, 메시지 하나 업서트, 상태 악화 시 새 메시지로 핑

### 4.3 Worktree 병렬 모드 (선택적)

- 명령 하나로: worktree + 브랜치 + tmux window 생성 + 에이전트 실행.
  에이전트 지정 자유(`--agent claude|codex|gemini` 혼합 가능)
- worktree별 diff 보기
- **diff 코멘트 회신 루프**: diff에 코멘트를 모아 해당 에이전트 pane에 수정
  지시로 재주입 (Orca Annotate AI Diff의 터미널 재현)
- **테스트 결과 수집**: worktree별 테스트 명령 실행, 통과/실패를 대시보드 표시
- **merge 흐름 안내**: 자동 실행 없음. 명령 안내 + 사용자 확인
- 보존 우선 정리: 미커밋·미머지·untracked가 있으면 절대 자동 삭제하지 않음
- 협업 병렬(역할 분담·승인 게이트)은 기존 MultiAgent가 하네스.
  agentlayer는 실행·관제층으로서 starter의 writer에게 worktree를 제공할 뿐,
  file-as-memory 정본과 승인 게이트를 우회·변경하지 않는다.

### 4.4 비상 복구

- 에이전트별 세션 ID를 자동 기록 (기록 비용 0, 워크플로우 무변경)
- 마감 의식 없이 죽은 대화만 `agentlayer resume`으로 구조
- 일상 재시작은 SESSION.md 방식 그대로 (resume은 비상용 + 시청자용)

### 4.5 설치 (시청자 배포)

- `brew install netwaif/tap/agentlayer`
- `agentlayer init` 한 번: hook 등록, Discord 웹훅 설정, coach/mat 자동 감지

## 5. 제외 확정 (사용자 승인)

- Quick Commands (프롬프트 팔레트) — 스킬 체계가 이미 담당
- worktree 체크포인트 — git + Claude Code `/rewind`와 삼중 중복
- 하이버네이션 — Mac 로컬에서 불필요, SESSION.md 철학과 충돌, 봇은 상시 대기 필요
- 자동 merge — 검토 게이트 훼손. 추후 옵트인 재검토 가능
- 자체 터미널/에디터/브라우저, 모바일 앱, Orca UI 복제
- 기존 tmux prefix·단축키 변경, 기존 세션 자동 kill

## 6. 기술 방침 (구현 재량 사항)

- **Go + bubbletea + lipgloss** — mat과 동일 스택, 단일 바이너리, brew tap 배포
- **데몬 없음**: 상태 정본은 `~/.local/state/agentlayer/` 파일.
  에이전트 1개 = 파일 1개, temp→rename 원자적 쓰기로 무잠금 동시 기록.
  tmux user option은 빠른 표시용 mirror로만 사용 가능(정본 아님)
- **상태 판정은 hook + 프로세스 신호만**. 화면 스크래핑으로 WAITING/DONE을
  판정하지 않는다 (우선순위 명시적 강등)
  - Claude: hooks (PostToolUse=WORKING heartbeat, Notification=WAITING,
    Stop=DONE_UNREAD) — 기존 settings.json hook과 공존
  - Codex: notify 설정
  - Gemini: 프로세스/pane 감지 기반 최선 노력
- 오래된 WORKING은 STALE 표시 (hook 유실 대비)
- DONE_UNREAD → IDLE 전환은 반드시 사용자 행동(점프·읽음 키)으로만
- coach 데이터는 `coach --json` 호출로 소비, starter 데이터는 tasks/ 읽기 전용
  파싱(mat 파서 로직 참조·이식)
- 상태 파일·로그·Discord에 인증정보 미포함

## 7. 안전 원칙 (핸드오프 §10 승계)

- 기존 tmux 세션·window·pane을 자동 kill하지 않는다
- worktree cleanup은 미커밋·미머지·untracked 발견 시 중단
- worktree 생성 전 target repo와 base branch를 기록
- 서로 다른 에이전트가 같은 branch를 쓰지 않게 한다
- 병렬 writer의 DB·포트·Docker 충돌은 초기 범위 밖 (문서화만)
- Mac sleep/reboot 시 tmux·에이전트 프로세스 소실 가능성을 README에 명시

## 8. 테스트 전략

- Go 표준 테스트, TDD. 파서·상태 전이·정리 안전장치는 단위 테스트 필수
- tmux 의존 로직은 실제 tmux 서버를 띄우는 통합 테스트(임시 socket `-L`)로
  기존 세션과 격리
- worktree lifecycle은 임시 git repo fixture로 검증 (미커밋 보존 시나리오 포함)
- Discord는 웹훅 payload 조립까지 단위 테스트, 실전송은 수동 검증

## 9. 완료 기준 (핸드오프 §16 승계 + 확장)

- 기존 tmux 단축키가 그대로 작동한다
- 로컬 iTerm2와 외부 SSH에서 같은 상태를 확인할 수 있다
- 여러 에이전트의 상태를 한 화면에서 보고 pane으로 점프할 수 있다
- 완료·입력 대기 알림이 중복 없이 macOS·Discord로 전달된다
- 사용량 뷰가 현 discord_dash 카드의 정보를 전부 표시한다
- worktree가 안전하게 생성되고, 미커밋 worktree는 자동 삭제되지 않는다
- MultiAgent의 file-as-memory 정본과 승인 게이트를 훼손하지 않는다
- GUI 전용 상태 없이 CLI만으로 장애 원인 확인·복구가 가능하다
- 시청자가 brew + init 두 명령으로 설치를 끝낼 수 있다
