# AgentLayer Phase 4 — 어댑터 완성·starter 패널·비상 resume·배포 준비 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) 구문.

**Goal:** Codex notify 어댑터, MultiAgent(starter) 활성 작업 패널, 비상 resume, init 확장(웹훅·codex notify 등록), brew 배포 준비물을 완성해 스펙 기능 목록을 닫는다.

**Architecture:** Codex는 config.toml `notify` 프로그램 호출(JSON argv)을 `agentlayer hook codex`로 받는다(자식 프로세스라 TMUX_PANE 상속). starter 패널은 `tasks/*/task.md`의 yaml 헤더 status만 읽는 초경량 파서(mat의 정밀 파싱은 mat 몫). resume은 상태 레코드의 session_id로 `claude --resume`을 새 window에 띄운다. Gemini는 프로세스 감지 기반(문서화)으로 확정.

**Spec:** `docs/superpowers/specs/2026-08-25-agentlayer-design.md` §4.4, §4.5, §6

## Global Constraints
- Phase 1~3 승계. config.toml 수정은 백업+멱등, 섹션 헤더(`[...]`) 앞 최상위에만 삽입.
- starter 정본은 읽기 전용. resume은 새 window만 만든다(기존 pane 불가침).

### Task 1: hookcmd/codex — notify 어댑터 + init 등록
- `RunCodex(st, args []string, env, now)` — 마지막 인자 JSON `{"type":"agent-turn-complete",...}` → DONE_UNREAD, TMUX_PANE으로 레코드 식별
- `InstallCodexNotify(w, configPath, dryRun)` — `notify = ["agentlayer","hook","codex"]`를 첫 섹션 앞에 삽입, 이미 있으면 skip, 백업
- main: `hook codex`, init에 codex 등록 추가

### Task 2: internal/starter — 활성 작업 패널
- `ActiveTasks(root) []Task{Name,Status,UpdatedAt}` — yaml 헤더 status가 in_progress|reviewing|waiting_* 인 것, mtime 역순
- config `starter_root` (기본 ~/VSCodeWorkspace/MultiAgent 존재 시)
- TUI 헤더 아래 "MultiAgent: name(status) ..." 한 줄 (활성 있을 때만)

### Task 3: resume — 비상 복구
- `agentlayer resume` — session_id 있는 레코드 목록(DEAD·ERROR 우선)
- `agentlayer resume <id>` — CWD에 새 window로 `claude --resume <session_id>` (claude만)

### Task 4: 배포 준비물
- `Makefile`(build/test/install), `.goreleaser.yaml`(darwin arm64/amd64, brew tap netwaif/homebrew-tap formula)
- README 설치 절 갱신. GitHub 푸시·tap 반영은 사용자 승인 후(공개 저장소명 미결정 항목)

### Task 5: 종단·병합 — `v0.4.0-phase4`
