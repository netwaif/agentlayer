새 작업(작음). 브랜치 `restore-bot-picker`, main 대상 PR 하나, 태그/릴리즈/SESSION.md 금지, 한국어. 기존 입력의 결과는 바뀌지 않게(추가만). 먼저 main을 pull. 설계 정본: `docs/superpowers/specs/2026-10-02-restore-bot-picker-design.md` — 먼저 읽고 그 범위만 한다.

문제: 봇이 부팅 때 전부 자동으로 떠서 안 쓰는 봇까지 떠 있다. 사용자는 매일 `agentlayer restore`를 치므로, 그 체크리스트에서 그날 쓸 봇만 골라 띄우게 한다. **엔진을 가리지 않는다 — Claude 폴더 봇, 코덱스·agy 브리지 봇 전부 대상이다(빼지 말 것).** 자동 기동이 꺼진 봇은 두 모양이다: (A) 구동 유닛이 없고 기동 명령이 `~/.config/folder-bot/<세션>.tmux-cmd`에 있는 사이드카형, (B) 구동 유닛은 있되 자동 기동만 꺼진 유닛형(macOS plist `RunAtLoad` false, 리눅스 유닛 disabled; 브리지 봇은 데몬+TUI 유닛 한 쌍).

할 것(`internal/cli/restorecmd.go`·`restorepick.go`·`internal/wiring` 중심):
1. 꺼져 있는 봇 = (A) 사이드카 중 같은 이름 tmux 세션이 없는 것 + (B) tmux 세션을 띄우는 구동 유닛 중 자동 기동이 꺼져 있고 그 세션이 없는 것. 세션↔유닛 대응·브리지 판정은 기존 `internal/wiring`을 쓰고 필요한 만큼만 넓힌다(브리지는 같은 env의 데몬 유닛을 짝으로). 조회는 `RestoreEnv`에 주입점을 더해 테스트에서 바꿔 끼울 수 있게(설정 폴더·유닛 폴더 경로도 주입).
2. 체크리스트(인자 없이 터미널에서 칠 때)에 "꺼져 있는 봇" 묶음(세션 이름·엔진)을 기존 항목 아래에 추가. 기본 체크 = 지난번 선택(`<state dir>/restore-bots.json`, 없으면 전부 해제). 실행 뒤 선택을 저장.
3. 띄우기: (A) `tmux new-session -d -s <세션> <사이드카 내용(끝 개행 제거)>`. (B) macOS `launchctl kickstart gui/<uid>/<라벨>`, 리눅스 `systemctl --user start <유닛>` — 브리지는 데몬 유닛 먼저(이미 돌고 있으면 생략), 그다음 TUI 유닛. 세션이 이미 있으면 건너뛴다. 실패는 봇마다 한 줄로 보고하고 나머지는 계속. 명령 실행은 주입점으로 감싸 테스트한다.
4. 꺼져 있는 봇으로 잡힌 세션의 죽은 레코드(메인 pane·스레드 창)는 구동 유닛 관할과 같은 방식으로 일반 복원에서 뺀다(Skipped 사유 "봇 관할 — 체크리스트의 봇 묶음에서 띄움"). ID를 명시해 강제하는 기존 규칙(`opts.Explicit`)은 구동 유닛과 같게.
5. `--bots <세션,세션>`(그 봇만 띄움·체크리스트 없음, 꺼져 있는 봇 목록에 없는 이름은 오류), `--no-bots`(봇 묶음 숨김). `--yes`는 봇을 띄우지 않는다. `--dry-run`은 띄울 봇 목록과 방법(사이드카/유닛)만 출력.
6. 테스트(패키지별 `go test ./internal/cli -count=1`·`go test ./internal/wiring -count=1`, 전체 한 번에 금지): 사이드카형·유닛형·브리지 쌍 탐지(세션 있음/없음, RunAtLoad true/false), 관할 제외, 기본 체크의 기억·저장, `--bots`·`--no-bots`·`--yes`·`--dry-run`, 기동 명령 인자와 순서(브리지 데몬→TUI). README restore 절과 도움말(helpcmd) 갱신.
7. PR 본문에 "기존 동작 변경 없음(자동 기동이 꺼진 봇이 하나도 없으면 출력·체크리스트가 예전과 같다)"과 맥에서 실기할 항목(자동 기동을 끈 Claude 폴더 봇·코덱스 브리지 봇 각 하나로: 체크리스트 표시 → 선택 기동 → 디스코드 응답, 재실행 때 기본 체크 기억, 봇 관할 레코드가 일반 복원에서 빠지는지)을 적는다.
