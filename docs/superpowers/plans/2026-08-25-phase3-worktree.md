# AgentLayer Phase 3 — worktree 병렬 모드 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `agentlayer wt` 명령군으로 태스크별 worktree+브랜치+tmux window+에이전트 실행을 한 번에 만들고, diff 보기·코멘트 회신·테스트 수집·merge 안내·보존 우선 정리를 제공한다.

**Architecture:** `internal/wt` 패키지가 git worktree lifecycle을 소유한다. 메타데이터(태스크→repo/base/branch/worktree 경로/테스트 명령/테스트 결과)는 상태 디렉터리의 `worktrees/<task>.json`에 기록. 에이전트 실행은 tmux `new-window -c <worktree>`로 하고, 이후 상태 추적은 Phase 1 스캐너·hook이 그대로 담당(별도 기제 없음). 코멘트 회신은 리뷰 파일(diff + 코멘트 마커) 편집 → 마커 추출 → 해당 pane에 `send-keys`.

**Tech Stack:** Go 표준 라이브러리 + 기존 내부 패키지. git·tmux는 CLI 호출.

**Spec:** `docs/superpowers/specs/2026-08-25-agentlayer-design.md` §4.3, §7

## Global Constraints

- Phase 1·2 Global Constraints 전부 승계.
- cleanup은 미커밋·untracked·미병합 커밋이 하나라도 있으면 **중단** — force 자동화 없음.
- worktree 생성 전 target repo와 base branch를 메타데이터에 기록.
- 브랜치명은 `agent/<task>` — 태스크가 유일키이므로 서로 다른 에이전트가 같은 브랜치를 쓸 수 없다.
- 자동 merge 없음: merge는 명령 안내 + 명시적 y 확인 후에만 실행.
- 테스트는 임시 git repo fixture + 임시 tmux 소켓으로만. 사용자 repo·세션 불가침.

## 파일 구조 (Phase 3 추가분)

```
internal/wt/
├── meta.go        # Task 메타(repo/base/branch/path/test) 저장·조회
├── meta_test.go
├── git.go         # worktree add/remove, dirty·unmerged 검사, diff
├── git_test.go
├── lifecycle.go   # New(생성+window+에이전트), Clean(보존 우선), Merge 안내
├── lifecycle_test.go
├── review.go      # 리뷰 파일 생성·코멘트 추출·pane 전송
├── review_test.go
└── test.go        # 테스트 명령 실행·결과 기록  (파일명 runtest.go)
```

### Task 1: wt/meta — 태스크 메타데이터
- `type Meta struct { Task, Repo, Base, Branch, Path, Agent, TestCmd string; TestPass *bool; TestAt time.Time; CreatedAt time.Time }`
- `SaveMeta/LoadMeta/ListMetas(stateDir)` — agents 저장소와 같은 원자적 쓰기
- Commit `feat(wt): 태스크 메타데이터`

### Task 2: wt/git — git 검사·worktree 조작
- `Add(repo, base, branch, path)` — `git worktree add -b branch path base`
- `Dirty(path) ([]string, error)` — status --porcelain -uall
- `Unmerged(repo, base, branch) (int, error)` — `git rev-list --count base..branch`
- `Diff(repo, base, path) (string, error)` — `git -C path diff base` + untracked 표시
- `Remove(repo, path)` — worktree remove (검사는 호출자)
- 임시 repo fixture로 전부 실측 테스트
- Commit `feat(wt): git worktree 조작·검사`

### Task 3: wt/lifecycle — 생성·정리·merge 안내
- `New(opts)` — repo·base 검증 → 메타 기록 → worktree add → tmux new-window(-c path,
  이름 task) → 에이전트 명령 send-keys → 성공 요약 반환. 중간 실패 시 만든 것 롤백.
- `Clean(task)` — Dirty 또는 Unmerged>0 이면 목록과 함께 **거부**. 깨끗하면
  worktree remove + branch -d + 메타 삭제.
- `MergeGuide(task)` — 검사 요약(dirty·unmerged·테스트 결과) + 실행할 명령 목록 출력,
  `--yes`/stdin y 확인 시에만 `git merge --no-ff` 실행. 충돌 시 그대로 보고(자동 해소 없음).
- Commit `feat(wt): 생성·보존 우선 정리·merge 안내`

### Task 4: wt/runtest + review — 테스트 수집·코멘트 회신
- `RunTest(task, cmd)` — worktree에서 실행, pass/fail·시각을 메타에 기록
- `WriteReviewFile(task) path` — diff에 `#> ` 코멘트 안내 헤더를 붙인 파일 생성
- `ExtractComments(file) []Comment{Context, Text}` — `#> ` 줄 + 직전 diff 컨텍스트
- `SendComments(task, comments)` — 태스크 pane에 수정 지시 문단 send-keys
- Commit `feat(wt): 테스트 수집 + diff 코멘트 회신`

### Task 5: CLI 연결 + TUI 표시
- `agentlayer wt new <task> [--agent claude] [--repo .] [--base main] [--test 'cmd']`
- `wt list` / `wt diff <task>` / `wt test <task>` / `wt review <task>` / `wt send <task>`
- `wt merge <task>` / `wt clean <task>`
- TUI 행: worktree 태스크 에이전트에 `⎇ branch` 표시(메타 조인)
- Commit `feat(cli,ui): wt 명령군 + TUI 브랜치 표시`

### Task 6: e2e — 임시 repo+tmux 종단
- new → 에이전트 pane 생성 확인 → 파일 수정·커밋 → diff → test → clean 거부(dirty)
  → 커밋 후 merge 안내 → clean 성공
- 병합, 태그 `v0.3.0-phase3`
