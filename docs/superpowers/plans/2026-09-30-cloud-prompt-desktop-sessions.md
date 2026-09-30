최종 지시(앞선 지시 두 개를 대체한다). 브랜치 `desktop-sessions`·PR 하나·태그/릴리즈/SESSION.md 금지는 그대로. 한국어로 작업·보고.

## 원칙
기존 입력의 결과는 하나도 바뀌지 않아야 한다. 상태 저장소(state)·훅(hookcmd)·status/UI 코드는 수정하지 않는다. 새 기능은 새 파일 위주로 넣고, 기존 함수에는 "기존 경로가 실패한 뒤에만 타는 분기"만 덧붙인다. 전송 규칙(채널·큐 정본, tmux 폴백)은 그대로.

## 만들 것
A. `agentlayer send <UUID>` — 상태 기록이 없는 UUID(앞자리 접두 허용)면 `codex queue --thread <ID>`로 직송. cwd는 `--cwd` 또는 rollout 폴더(`usage.CodexSessionsRoot`)에서 찾음. via=queue, 실패 시 오류(폴백 없음). (이미 진행 중이면 유지.)

E. `agentlayer inbox wait [--name <이름>] [--timeout <기간, 기본 30m>]` — "메시지 받을 준비" 명령. Claude 세션이 Bash로 백그라운드 실행한다.
  1) 자기 등록: 부모 프로세스 사슬을 올라가 claude 프로세스(인자에 claude가 있는 첫 조상, `scan.LoadProcTable` 활용)를 찾아 그 PID를 세션 PID로 삼는다. 주소록 파일 `~/.local/state/agentlayer/addresses/<이름>.json`에 {name, pid, cwd, inbox, registered_at}을 쓴다. `--name` 없으면 `<폴더명>` (겹치면 `<폴더명>-<pid 끝 4자리>`). 상태 저장소에는 넣지 않는다.
  2) 수신함: `~/.local/state/agentlayer/inboxes/a<pid>/`. 기존 편지 구조(`task.Report`, `task.Watch`, `DirectiveEvent`="SEND")를 재사용한다. 시작 시 pending에 이미 있는 편지는 `purgeStale`처럼 quarantine으로 치운다(옛 세션 앞으로 온 것).
  3) 편지 한 통이 오면 stdout에 `from: <보낸이>` 한 줄 + 빈 줄 + 본문을 찍고 exit 0. `--timeout` 넘으면 "답 없음(<기간>)"을 stderr에 찍고 exit 2. 종료 시(둘 다) 주소록 항목을 지운다.
  4) SIGINT/SIGTERM에도 주소록을 지우고 끝난다.

F. `agentlayer send <이름>` — `ResolveTarget`이 실패했을 때만 주소록(addresses/)을 본다. 항목이 있고 그 PID가 살아 있으면 `SendViaChannel`과 같은 방식으로 그 수신함에 편지를 넣고 via=inbox로 보고(JSON 포함). PID가 죽었으면 항목을 지우고 오류. tmux 세션명이 같으면 tmux가 우선(기존 동작 유지).

G. `agentlayer send <원격이름> [--file <경로>]...` — 원격(`remotes/<이름>.json`)인데 업무(task) 등록이 없으면 지금은 오류다. 그 경우에만 새 경로: 어댑터의 기존 함수(Dispatch, 첨부 업로드는 Answer/upload에 있는 것)만 써서 본문과 파일을 보낸다. 어댑터(`internal/remote`) 자체는 수정하지 않는다. 업무가 등록돼 있으면 기존 경로 그대로. `--file`은 이 새 경로에서만 유효(기존 경로에 주면 오류 메시지).

## 테스트·문서
각 항목마다 테스트(패키지별 `go test ./internal/<pkg> -count=1`, 전체 한 번에 돌리지 말 것). README에 "앱 세션에서 받기(`inbox wait`)·이름으로 보내기·코덱스 세션 ID 직송·원격 직송" 절 하나. 설계 메모 `docs/superpowers/plans/2026-09-30-desktop-session-address.md`에 결정과 확인 못 한 것. PR 본문에 "기존 동작이 바뀐 입력은 없음(오류였던 두 입력이 동작으로 바뀜: UUID 직송, 업무 없는 원격 전송)"을 명시. 마지막 보고: PR 링크, 바뀐 파일, 맥에서 실기로 확인할 항목.
