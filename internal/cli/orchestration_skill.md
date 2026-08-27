---
name: orchestration
description: AgentLayer로 멀티 에이전트 오케스트레이션. worker N개(claude·codex·gemini)를 각각 git worktree+tmux window에 띄워 태스크를 병렬로 dispatch하고, 완료를 기다렸다가 결과를 취합·비교한다. "worker 2개 만들어서 같은 태스크 시켜줘", "claude랑 codex한테 A/B로 시켜봐", "서로 다른 태스크 나눠서 병렬로", "/orchestration" 등으로 트리거. 머지는 기본적으로 사용자 몫이다.
---

# AgentLayer Orchestration — 코디네이터 지침

너는 코디네이터다. worker 생성·지시·완료 감지·취합을 아래 절차대로 수행한다.
전제: 지금 tmux 세션 안에서 실행 중이고, `agentlayer`가 설치돼 hook 등록(`agentlayer init`)이 끝났고, 대상 폴더는 git 저장소다. 전제가 깨져 있으면 시작 전에 사용자에게 알린다.

## 0. 계획 복창

사용자 요청에서 다음을 정리해 한 문장으로 복창한 뒤 시작한다:
- worker 수와 종류 (claude | codex | gemini)
- 태스크 구성: 같은 태스크 A/B 비교인지, 서로 다른 태스크 분업인지
- 커밋·머지 방침 (기본: worker는 커밋하지 않고, base 브랜치는 아무도 건드리지 않고, 머지는 사용자가 선택)

## 1. worker 생성 — 태스크당 worktree 1개

```
agentlayer wt new <태스크이름> --agent <종류> [--repo <경로>] [--base <브랜치>] [--test '<명령>']
```

- 태스크 이름이 곧 브랜치(`agent/<이름>`)·tmux window 이름이 된다. A/B 비교면 `hero-dark-claude`, `hero-dark-codex`처럼 종류를 접미로 붙여라.
- worktree는 `<repo>/.agentlayer/worktrees/<이름>`에 생기고, window는 지금 tmux 세션에 열리며 CLI가 자동 기동된다.
- **부팅 대기**: 생성 직후 바로 dispatch하지 마라. 8초쯤 기다린 뒤
  `tmux capture-pane -p -t ':<태스크이름>.0' | tail -8`
  로 입력 프롬프트를 확인한다. 안 떠 있으면 5초 간격으로 재확인(최대 60초).
- 새 worktree 경로라 폴더 신뢰(trust)·온보딩 프롬프트가 뜰 수 있다. 화면을 읽고 같은 repo의 worktree임이 확실하면 Enter로 승인하고, 그 외의 프롬프트는 사용자에게 보고한다.

## 2. dispatch — send-keys 2단 규약 (한 번에 보내지 말 것)

```
tmux send-keys -t ':<태스크이름>.0' -l '<지시문>'
sleep 1
tmux send-keys -t ':<태스크이름>.0' Enter
```

- 텍스트와 Enter를 붙여 보내면 CLI가 텍스트를 처리하기 전에 Enter가 도착해 미제출로 남을 수 있다. 반드시 `-l`(리터럴)로 텍스트 → 1초 대기 → Enter 순서.
- 전송 후 capture-pane으로 입력줄에 지시문이 남아 있는지 확인하고, 남아 있으면 Enter만 재전송.
- 지시문에 작은따옴표·개행이 섞이면 이스케이프가 깨지기 쉽다. 그럴 땐 지시문을 파일로 쓰고:
  `tmux load-buffer <파일> && tmux paste-buffer -t ':<태스크이름>.0'` 후 Enter.
- **지시문 끝에 보고 규약을 붙인다**:
  > 작업이 끝나면 worktree 루트에 REPORT.md를 남겨라. 형식: `## Summary`(2~3문장) / `## Changed Files`(파일별 한 줄) / `## How to Verify`(실행·확인 방법) / `## Notes`(기존 파일 연결 필요 등 코디네이터가 알아야 할 것). 커밋은 하지 마라. base 브랜치는 절대 건드리지 마라.

## 3. 완료 대기 — 화면 스크래핑 금지, 상태는 hook이 정본

```
agentlayer status | grep -F 'worktrees/<태스크이름>'
```

- 해당 행이 `[DONE]`이면 그 worker는 완료다. 모든 worker가 DONE이 될 때까지 30초 간격으로 폴링한다(백그라운드 until 루프 권장 — 사이사이 다른 준비 작업을 해도 된다).
- 한 worker가 오래 `[WORK]`에 머물면 capture-pane으로 화면을 확인한다 — 승인 대기 프롬프트면 내용을 사용자에게 보고하고 지시를 기다린다. 임의로 승인하지 않는다.
- `[WAIT]`(응답 필요)로 바뀐 worker도 화면을 확인해 질문이면 답을 보내거나 사용자에게 올린다.

## 4. 취합

각 worktree에서:
```
cat <worktree>/REPORT.md
git -C <worktree> diff --stat        # 필요하면 diff 본문까지
```

- **A/B 비교면**: 변경 규모·접근 방향·구성 차이를 표로 요약하고, 선택 관점(어떤 기준이면 어느 쪽인지)을 한 단락 덧붙인다. 머지는 하지 않는다 — 사용자가 고른다.
- **분업이면**: 각 보고를 요약하고, worker들이 Notes에 남긴 연결 작업(공용 파일 import 추가 등)은 코디네이터인 네가 **각 브랜치의 worktree 안에서** 직접 수정하고 커밋한다. base에서 하지 않는다.
- 미리보기가 필요하면 각 worktree에서 포트를 달리해 dev 서버를 띄워 나란히 비교하게 한다.

## 5. 머지·정리 — 자동 머지는 없다

- 사용자가 브랜치를 고르면: worker 변경 커밋(REPORT.md는 커밋에서 제외) → `agentlayer wt merge <이름>` (검사 요약 후 y 확인을 받아야만 머지된다).
- 정리는 `agentlayer wt clean <이름>` — 미커밋·미병합이 있으면 거부되는 보존 우선 설계다. 거부되면 이유를 사용자에게 보여주고 지시를 기다린다.
- 폐기하는 브랜치도 사용자 확인 전에는 지우지 않는다.

## 하지 말 것

- **별도 tmux 서버(-L/-S) 금지** — 상태 저장소가 공유라 pane ID가 충돌해 다른 세션 레코드를 오염시킨다.
- 화면 파싱으로 완료 판정 금지 — 완료는 `agentlayer status`의 상태로만 판단한다.
- 자동 머지 금지, base 브랜치 직접 수정 금지, worker의 승인 프롬프트 임의 승인 금지.
