# AgentLayer 로컬 Claude Code 핸드오프

- 작성 시각: 2026-08-25 12:55 KST
- 대상: 사용자의 로컬 Mac에서 실행되는 Claude Code 세션
- 작업명(가칭): **AgentLayer**
- 상태: **설계 논의 완료, 구현 미시작**
- 원격 대화 출처: Hermes와 사용자의 `iTerm2 + tmux vs Orca` 설계 논의

---

## 0. 로컬 Claude Code에게 주는 첫 지시

이 문서를 현재 작업의 인수인계 정본으로 사용한다.

1. 사용자의 로컬 환경을 먼저 **읽기 전용으로 검증**한다.
2. 이 문서의 추론을 로컬에서 확인한 사실처럼 표현하지 않는다.
3. 기존 `multi-agent-starter`를 즉시 수정하지 않는다.
4. 먼저 기존 도구와 코드의 재사용 가능성을 조사하고, AgentLayer의 최소 구현 계획을 제시한다.
5. 일반 tmux의 기존 prefix 단축키와 SSH 접근성을 훼손하지 않는다.
6. 구현 전 사용자에게 최종 범위와 단계별 계획을 보여주고 확인받는다.
7. 인증 파일·SSH 키·토큰·`.env`는 문서나 Git에 복사하지 않는다.

---

## 1. 목표

사용자는 현재의 **Mac + iTerm2 + 일반 tmux** 작업환경을 유지하면서, Orca ADE에서 가치가 있다고 판단한 멀티 에이전트 기능만 선택적으로 추가하려 한다.

목표 구조:

```text
iTerm2
└── 일반 tmux
    ├── Claude Code / Codex / Gemini 계열 에이전트
    ├── 에이전트 상태 추적
    ├── 완료·입력 대기 알림
    ├── 선택적 Git worktree 격리
    ├── diff·검토·merge 흐름
    └── 외부 SSH에서 동일하게 사용할 CLI/TUI
```

프로젝트의 가칭은 **AgentLayer**, 폴더명 후보는 `agentlayer`다.

권장 배치(아직 로컬에서 생성·검증하지 않음):

```text
<사용자의 개발 루트>/
├── multi-agent-starter/
└── agentlayer/
```

AgentLayer는 `multi-agent-starter`의 새 버전이 아니라 별도의 실행·관제 계층으로 시작한다.

---

## 2. 사용자가 직접 밝힌 사실

다음은 사용자가 대화에서 직접 밝힌 내용이다.

- 로컬 개발 환경은 Mac의 iTerm2와 일반 tmux 조합이다.
- 기존 tmux prefix 단축키와 조작 경험을 중요하게 여긴다.
- `tmux -CC` Control Mode를 시험했지만 기존 tmux 단축키가 동작하지 않고 화면도 깨져 더 불편하다고 판단했다.
- 외부에서 자신의 PC에 일반 SSH로 접속해 tmux 세션을 이어 쓰는 흐름이 중요하다.
- Claude Code, Codex, Gemini 계열을 멀티 에이전트로 활용한다.
- 기존 하네스는 `https://github.com/netwaif/multi-agent-starter`다.
- 실제 개발은 로컬 Claude Code에서 진행할 예정이다.
- 프로젝트 폴더명 후보로 `agentlayer`를 제안했고, 대화상 장기적인 범위에 적합하다고 판단했다.

---

## 3. 로컬에서 아직 확인되지 않은 사항

다음은 Hermes 서버에서 사용자의 Mac을 직접 확인하지 못했으므로 반드시 로컬에서 검증해야 한다.

- 정확한 macOS 버전
- iTerm2 버전과 프로필 설정
- tmux 버전
- 사용자의 tmux prefix와 `.tmux.conf`
- 현재 tmux 세션·window·pane 구성
- 개발 루트와 저장소의 실제 절대경로
- 로컬 `multi-agent-starter` checkout의 branch, commit, 미커밋 변경
- Claude Code·Codex·Gemini/Antigravity CLI 버전과 hook 지원 상태
- fzf, jq, lazygit, delta, terminal-notifier 등 후보 도구 설치 여부
- SSH 진입 방식, Tailscale 사용 여부, Mac sleep/wake 설정
- Docker, DB, 포트, 캐시 등 병렬 작업 자원의 현재 운영 방식
- `agentlayer`라는 공개 저장소·패키지 이름의 사용 가능성

로컬 조사 시 파괴적 명령 없이 다음 범주만 먼저 수집한다.

```bash
sw_vers
printf 'TERM_PROGRAM=%s\nTMUX=%s\nTERM=%s\nLANG=%s\n' \
  "${TERM_PROGRAM-}" "${TMUX-}" "${TERM-}" "${LANG-}"
tmux -V
tmux list-sessions
tmux list-keys | less
```

`.tmux.conf`와 저장소 상태는 실제 경로를 찾은 뒤 읽기 전용으로 확인한다. 기존 키 바인딩과 충돌하지 않는지 확인하기 전에는 `Ctrl-b a` 같은 새 키를 등록하지 않는다.

---

## 4. 합의된 설계 판단

### 4.1 일반 tmux를 유지한다

`tmux -CC`를 사용하지 않는다.

이유:

- Control Mode는 일반 tmux 위에 iTerm2 UI를 추가하는 단순 옵션이 아니다.
- tmux를 백엔드 서버로 두고 window/pane UI를 iTerm2에 맡기는 다른 클라이언트 모델이다.
- 기존 prefix 키와 `.tmux.conf` 중심의 조작 경험이 약해질 수 있다.
- 실험 중 원시 control protocol 출력과 렌더링 문제를 경험했다.
- 사용자의 핵심 요구인 일반 SSH+tmux 재접속성과 기존 muscle memory에 맞지 않는다.

정상 기본 흐름:

```bash
tmux new -s work
tmux attach -t work
```

### 4.2 AgentLayer는 Orca 복제품이 아니다

AgentLayer의 목표는 자체 IDE·터미널 에뮬레이터를 만드는 것이 아니다.

가치가 높은 기능:

- 에이전트별 상태 추적
- 완료·입력 대기·오류 알림
- tmux session/window/pane과 agent task의 매핑
- 병렬 쓰기 작업을 위한 선택적 Git worktree 자동화
- diff·테스트·검토·merge/cleanup 흐름
- 일반 SSH에서 동일하게 사용할 CLI/TUI
- `multi-agent-starter`의 file-as-memory 상태와 연결하는 adapter

가치가 낮거나 비목표인 기능:

- 새로운 터미널 에뮬레이터
- tmux 대체 구현
- Monaco 기반 전체 코드 에디터
- 자체 파일 탐색 GUI
- 자체 브라우저 엔진
- 자체 모바일 앱
- drag-and-drop IDE
- Orca UI의 픽셀 단위 복제

### 4.3 SSH 접근성이 설계 제약이다

일반 tmux는 외부에서 다음처럼 같은 PTY 세션에 재접속할 수 있다.

```bash
ssh <사용자-PC>
tmux attach -t <session>
```

외부 SSH에서 보존해야 할 핵심:

- 에이전트 터미널 화면
- window/pane 구조
- 입력 대기 상태
- 실행 로그
- AgentLayer 대시보드와 제어 명령
- diff와 상태 파일

AgentLayer의 상태 원본을 GUI 프로세스 내부에만 두면 안 된다. 파일, tmux user option 또는 별도 경량 로컬 상태 저장소에 두고 CLI로 읽을 수 있어야 한다.

---

## 5. `Ctrl-b s`와 에이전트 상태의 차이

기존 `Ctrl-b s`는 이미 다음을 잘 보여준다.

- tmux session
- window
- pane 구조
- 사용자가 이동할 작업 위치

그러나 기본 tmux는 다음 의미 상태를 모른다.

- 어떤 agent가 실행 중인가
- 어느 task·branch·worktree인가
- 작업 중인가
- 사용자 입력이나 승인을 기다리는가
- 완료됐지만 아직 확인하지 않았는가
- 오류가 발생했는가

AgentLayer 대시보드의 최소 상태 후보:

```text
IDLE
WORKING
WAITING
DONE
DONE_UNREAD
ERROR
```

예상 표시:

```text
PROJECT     TASK          AGENT    STATE         BRANCH
backend     auth-api      Claude   WORKING       agent/auth-api
frontend    login-ui      Codex    WAITING       agent/login-ui
tests       auth-tests    Claude   DONE_UNREAD   agent/auth-tests
```

화면 문구를 주기적으로 스크래핑해서 상태를 추정하는 방식은 우선순위를 낮춘다. 가능한 경우 각 agent의 공식 hook·notification·process lifecycle을 adapter로 연결한다.

---

## 6. Git worktree 정책

### 6.1 worktree가 필요한 정확한 조건

Claude·Codex·Gemini가 단순히 동시에 실행된다는 이유만으로 worktree가 필요한 것은 아니다.

다음 조건에서 필요하다.

> 같은 Git 저장소를 여러 write-capable agent가 동시에 독립적으로 직접 수정한다.

같은 working directory를 공유하면 다음이 공유된다.

- 실제 파일
- Git index
- HEAD
- 현재 checkout branch
- uncommitted changes

따라서 한 agent의 `git switch`, `git reset`, `git stash`, `git clean`, staging 및 파일 저장이 다른 agent에 영향을 줄 수 있다.

tmux는 화면·프로세스를 분리하지만 파일시스템과 Git 상태를 분리하지 않는다.

### 6.2 worktree가 필요 없는 경우

- agent를 한 번에 하나만 실행
- 분석·리뷰·설계 같은 읽기 전용 작업
- worker가 코드나 diff를 텍스트로 반환하고 단일 Orchestrator가 적용
- 서로 다른 저장소에서 작업
- 별도 clone, container, VM으로 이미 격리
- 사람이 순차적으로 patch를 적용

### 6.3 worktree는 완전한 sandbox가 아니다

별도 관리가 필요한 공유 자원:

- DB
- 개발 서버 포트
- Docker project/container 이름
- 전역 캐시
- 홈 디렉터리
- 외부 API와 서비스
- `.env`와 인증정보
- 절대경로 파일

병렬 실행 모드에서는 포트·DB·Docker namespace 정책도 후속 설계해야 한다.

### 6.4 AgentLayer에서의 권장 정책

worktree를 모든 작업에 강제하지 않는다.

```text
읽기 전용 작업       → 현재 디렉터리 사용
직렬 쓰기 작업       → 현재 디렉터리 사용 가능
병렬 직접 쓰기 작업  → agent별 worktree 기본
이미 외부 격리됨     → worktree 생략 가능
```

---

## 7. `multi-agent-starter` 조사 결과

원격 Hermes 서버에서 공개 GitHub 저장소를 읽기 전용으로 clone해 확인했다.

- 저장소: `https://github.com/netwaif/multi-agent-starter`
- 확인 branch: `main`
- 확인 commit: `e052a90`
- 서버 임시 clone: `/opt/data/cache/repos/multi-agent-starter`
- 이 경로는 사용자의 로컬 경로가 아니며 이 문서 전달 후 정본으로 사용하면 안 된다.

### 7.1 현재 하네스의 실행 모델

현재 `multi-agent-starter`는 file-as-memory 기반 단일 Orchestrator 구조다.

```text
Orchestrator
├── Claude worker → 결과 텍스트 반환
├── Codex main    → task 산출물·diff, 승인 시 외부 repo 직접 쓰기
├── Critic        → 읽기 전용 검토
└── Gemini        → 분석 결과 반환
```

확인한 핵심 규칙:

- Claude `claude-main`은 파일을 직접 쓰지 않고 결과 텍스트를 반환한다.
- Gemini도 직접 쓰지 않고 Orchestrator가 응답을 기록한다.
- `codex-critic`은 read-only다.
- `codex-main`만 조건부로 외부 `target_repo`에 직접 쓸 수 있다.
- 기본 동작은 `tasks/<task>/`에 산출물·diff를 남긴다.
- 외부 직접 쓰기에는 `target_repo`, `write_scope`, worker 승인, log 승인이 모두 필요하다.
- worker별 결과는 `tasks/<task>/workers/<role>/result.md`에 분리 보존한다.

관련 정본 파일:

```text
plugins/multi-agent-starter/skills/configure-multiagent/generator/templates/claude/CLAUDE.md
plugins/multi-agent-starter/skills/configure-multiagent/generator/templates/claude/.claude/agents/claude-main.md
plugins/multi-agent-starter/skills/configure-multiagent/generator/templates/claude/_shared/routing.md
plugins/multi-agent-starter/skills/configure-multiagent/generator/templates/claude/_shared/orchestrator-rules.md
plugins/multi-agent-starter/skills/configure-multiagent/generator/templates/claude/_shared/learnings.md
plugins/multi-agent-starter/skills/configure-multiagent/generator/templates/claude/_shared/backends.json
```

### 7.2 왜 현재 하네스에는 worktree가 핵심이 아니었는가

현재 병렬화 대상은 주로 분석·설계·리뷰·diff 제안이며, 여러 writer가 같은 repo를 동시에 수정하는 구조가 아니다.

또한 Orchestrator는 `tasks/<task>/`를 file-as-memory의 정본으로 사용한다. 공개 `main`의 `_shared/learnings.md`에는 Orchestrator 자체의 worktree 진입을 금지한 기존 결정이 있다.

이유:

- `tasks/` 산출물을 본체에서 읽어야 함
- worktree 내부에 만들면 수동 복사나 merge 사족이 생길 수 있음
- 단순 system file 수정에도 commit·merge가 과함
- background harness의 강제 worktree가 file-as-memory 정본과 충돌한 경험이 있음

따라서 기존 하네스 전체를 worktree 기반으로 바꾸지 않는다.

### 7.3 권장 확장

기존 기본 모드를 유지한다.

```text
기본 orchestrated/advisory 모드
→ worker가 분석·설계·결과·diff 반환
→ Orchestrator가 통합
→ worktree 불필요
```

선택적 병렬 구현 모드만 추가할 수 있다.

```text
parallel-implementation / isolated-write 모드
→ 여러 직접 writer
→ writer별 target_repo worktree+branch
→ diff·test 수집
→ reviewer 검토
→ Orchestrator 선택·merge
```

현재 Gemini는 직접 writer가 아니므로 기본적으로 Gemini worktree는 필요 없다.

---

## 8. 권장 아키텍처

### 8.1 계층 분리

```text
┌──────────────────────────────────────────────┐
│ multi-agent-starter                         │
│ task/brief/result/approval/decision          │
│ = 오케스트레이션 control plane              │
└──────────────────────┬───────────────────────┘
                       │ adapter
┌──────────────────────▼───────────────────────┐
│ AgentLayer                                   │
│ process/tmux/worktree/status/notification    │
│ = 실행·관제 layer                            │
└───────────────┬──────────────────────────────┘
                │
┌───────────────▼──────────────────────────────┐
│ tmux + Git + Claude/Codex/Gemini CLIs        │
│ = execution plane                            │
└──────────────────────────────────────────────┘
```

### 8.2 AgentLayer가 관리할 최소 매핑

```text
project
↔ task
↔ agent type
↔ process/PID
↔ tmux session/window/pane
↔ repository
↔ worktree
↔ branch
↔ state
```

상태 레코드 예시(형식은 구현 전 검토):

```json
{
  "project": "my-project",
  "task": "auth-api",
  "agent": "claude",
  "state": "working",
  "tmux_session": "my-project",
  "tmux_window": "auth-api",
  "tmux_pane": "%3",
  "repository": "/path/to/repo",
  "worktree": "/path/to/worktrees/auth-api",
  "branch": "agent/auth-api"
}
```

상태의 정본 후보:

- 파일: `~/.local/state/agentlayer/` 또는 macOS에 맞는 사용자 상태 경로
- tmux user options: 빠른 표시용 mirror

상태 원본을 tmux option에만 두면 tmux server 종료 시 유실될 수 있으므로 영속 파일과 runtime mirror를 구분한다.

### 8.3 가능한 tmux UI

기존 `Ctrl-b s`는 그대로 유지한다.

추가 dashboard 후보:

```text
Ctrl-b a → AgentLayer popup/TUI
```

단, 실제 `.tmux.conf`에서 키 충돌을 확인한 후 결정한다.

예상 동작:

```text
Enter → 해당 pane으로 이동
d     → diff 보기
g     → Git 도구 실행
r     → 상태 새로고침
m     → merge 흐름 진입
x     → 안전한 cleanup
```

merge와 cleanup은 초기 MVP에서 자동 실행하지 않아도 된다. 먼저 상태·이동·알림을 안정화한다.

---

## 9. 권장 MVP

### MVP 1 — 관찰만

파괴적 side effect 없이 다음을 제공한다.

- tmux session/window/pane inventory
- pane별 current command/path
- agent process 감지
- task·agent 수동 등록
- CLI `status`
- TUI/popup에서 pane 이동

### MVP 2 — 상태와 알림

- agent adapter
- `WORKING/WAITING/DONE/ERROR` 상태 업데이트
- 완료·입력 대기 macOS 알림
- 선택적 Discord/Hermes 알림
- `DONE_UNREAD` 확인 처리

### MVP 3 — 선택적 worktree lifecycle

- Git repo 사전검사
- 안전한 branch 이름
- agent별 worktree 생성
- tmux window/pane 생성과 cwd 매핑
- diff와 test 결과 수집
- 수동 review/merge
- 보존 우선 cleanup

### MVP 4 — `multi-agent-starter` adapter

- `tasks/<task>/task.md`, `context.md`, `log.md`, worker status 읽기
- 기존 file-as-memory 정본을 변경하지 않고 AgentLayer view로 표시
- 선택적 `parallel-implementation` 실행 모드 연결
- 기존 승인 게이트를 우회하지 않음

---

## 10. 안전 원칙

- 사용자의 기존 tmux 세션·window·pane을 자동으로 kill하지 않는다.
- 기존 prefix와 키 바인딩을 변경하기 전에 명시적으로 비교한다.
- agent pane이 종료됐다고 worktree를 바로 삭제하지 않는다.
- 미커밋 변경, untracked 파일, 미병합 commit이 있으면 cleanup을 중단한다.
- 자동 merge는 초기 범위에서 제외한다.
- worktree 생성 전 target repo와 base branch를 명시적으로 기록한다.
- 서로 다른 agent가 같은 branch를 사용하지 않게 한다.
- 병렬 writer가 같은 DB·포트·Docker namespace를 쓰는지 별도 검사한다.
- 화면 스크래핑에만 의존해 `WAITING`이나 `DONE`을 판정하지 않는다.
- 인증정보를 상태 파일·로그·Discord 알림에 포함하지 않는다.
- 로컬 Mac이 sleep/reboot하면 tmux와 agent process도 유지되지 않을 수 있음을 문서화한다.

---

## 11. 기존 도구 우선 조사

직접 구현하기 전에 다음 범주를 조사한다.

- tmux session/window/pane selector
- fzf 기반 tmux popup
- Git worktree manager
- Claude Code hook/notification
- Codex hook/notification
- Gemini/Antigravity lifecycle 신호
- macOS notification 도구
- lazygit/delta 기반 diff 검토
- tmux-resurrect/continuum과의 역할 중복
- 이미 존재하는 tmux 기반 coding-agent manager

목표는 모든 것을 새로 만드는 것이 아니라:

```text
기존 도구
+
AgentLayer의 공통 상태 모델과 adapter
+
사용자의 tmux/SSH 워크플로에 맞는 glue
```

다.

---

## 12. 로컬 Claude Code가 먼저 산출할 문서

코드를 작성하기 전에 다음을 사용자에게 보여준다.

1. 로컬 환경 조사 결과
2. 기존 도구 재사용 후보와 채택/배제 이유
3. AgentLayer의 명확한 비목표
4. MVP 범위
5. 파일·모듈 구조
6. 상태 데이터 모델
7. tmux 매핑 방식
8. agent adapter 방식
9. worktree lifecycle과 안전장치
10. `multi-agent-starter` integration 경계
11. 테스트 전략
12. 단계별 구현 순서

사용자 확인 전 기존 `multi-agent-starter`를 수정하거나 자동 merge/cleanup 기능을 구현하지 않는다.

---

## 13. 첫 로컬 세션 권장 진행 순서

### 단계 A — 로컬 환경 확인

- 실제 개발 루트 확인
- `multi-agent-starter` 실제 경로와 Git 상태 확인
- `.tmux.conf` 읽기
- 현재 tmux session/key 확인
- 사용 중인 Claude/Codex/Gemini 실행 방식 확인

### 단계 B — 기존 도구 조사

- 후보 도구를 기능별로 분류
- 사용자의 일반 tmux/SSH 요구와 충돌 여부 확인
- 유지보수 상태와 macOS 지원 확인

### 단계 C — spike

임시 디렉터리에서 다음만 검증한다.

- tmux pane 목록과 이동
- pane별 상태 metadata 설정/읽기
- 작은 popup/TUI
- 정상 종료와 입력 대기 hook 한 종류

기존 프로젝트에는 아직 연결하지 않는다.

### 단계 D — 계획 확정

spike 결과를 바탕으로 MVP 계획을 갱신하고 사용자 승인을 받는다.

### 단계 E — 구현

새 `agentlayer` 저장소에서 테스트 우선으로 구현한다.

---

## 14. 결정 요약

### 확정

- 일반 tmux를 유지한다.
- `tmux -CC`는 사용하지 않는다.
- 프로젝트 가칭은 AgentLayer, 폴더명 후보는 `agentlayer`다.
- AgentLayer는 `multi-agent-starter`와 별도 프로젝트로 시작한다.
- core UI는 일반 SSH에서 접근 가능한 CLI/TUI여야 한다.
- worktree는 병렬 직접 쓰기 작업에서만 기본 사용한다.
- `multi-agent-starter`의 기존 file-as-memory 기본 모드는 유지한다.
- 기존 도구를 우선 조사하고 부족한 glue만 구현한다.

### 미결정

- 구현 언어
- 최종 CLI 명령과 package 이름
- 상태 파일 경로와 schema
- tmux key binding
- hook 방식과 지원 agent 범위
- 첫 MVP에서 worktree까지 포함할지 여부
- Discord/Hermes 알림의 첫 버전 포함 여부
- 공개 저장소 이름의 사용 가능성
- `multi-agent-starter` adapter의 정확한 API

---

## 15. 참고 자료

### 사용자 하네스

- https://github.com/netwaif/multi-agent-starter

### Orca 공식 문서

- Ways to run Orca: https://www.onorca.dev/docs/ways-to-run
- SSH worktrees: https://www.onorca.dev/docs/ssh
- Remote Orca Servers: https://www.onorca.dev/docs/remote-servers
- Mobile companion: https://www.onorca.dev/docs/mobile
- Session restore: https://www.onorca.dev/docs/model/session-restore
- Terminal: https://www.onorca.dev/docs/terminal

### 핵심 비교

```text
tmux
→ 화면·프로세스·PTY·세션 수명 분리
→ 일반 SSH attach 가능

Git worktree
→ working directory·index·HEAD·branch 분리
→ 병렬 writer 충돌 완화

AgentLayer
→ tmux와 worktree를 task·agent·state·notification으로 연결

multi-agent-starter
→ brief·approval·result·decision을 file-as-memory로 관리
```

---

## 16. 완료 기준 초안

최소한 다음을 만족하면 AgentLayer의 초기 목표가 달성된 것으로 본다.

- 기존 일반 tmux 단축키가 그대로 작동한다.
- 로컬 iTerm2와 외부 일반 SSH에서 같은 AgentLayer 상태를 확인할 수 있다.
- 여러 agent의 `WORKING/WAITING/DONE/ERROR` 상태를 한 화면에서 볼 수 있다.
- 완료 또는 입력 대기 알림이 중복 없이 전달된다.
- 선택적 병렬 쓰기 모드에서 agent별 worktree가 안전하게 생성된다.
- 미커밋 작업이 있는 worktree는 자동 삭제되지 않는다.
- `multi-agent-starter`의 file-as-memory 정본과 승인 게이트를 훼손하지 않는다.
- GUI 전용 상태 없이 CLI에서 장애 원인을 확인하고 복구할 수 있다.

---

## 17. 인수인계 확인 질문

로컬 Claude Code는 이 문서를 읽은 뒤 먼저 다음을 사용자에게 짧게 확인한다.

1. 실제 `agentlayer` 생성 경로가 어디인가?
2. 현재 로컬 `multi-agent-starter` 경로와 branch는 무엇인가?
3. 첫 MVP를 상태 대시보드부터 시작할지, worktree launcher까지 포함할지?
4. 첫 지원 agent를 Claude Code 하나로 제한할지, Codex까지 동시에 포함할지?

그 후 읽기 전용 환경 조사와 계획 수립을 진행한다.
