새 작업(작음). 브랜치 `send-from`, main 대상 PR 하나, 태그/릴리즈/SESSION.md 금지, 한국어. 기존 입력의 결과는 바뀌지 않게(추가만). 먼저 main을 pull(v1.12.0, `internal/cli/inboxcmd.go`에 inbox open/close·--remote가 있음).

문제: tmux 밖(코덱스 데스크톱 앱, 앱 Claude 세션 등)에서 `agentlayer send <주소> "…"`를 실행하면 `senderName`이 발신 세션을 못 찾아 편지의 from이 "user"로 찍힌다(2026-09-30 실기: 코덱스 앱 회신이 `from: user`).

할 것:
1. `agentlayer send --from <이름>`: 발신자 이름을 명시한다. 값은 주소록 이름 형식과 같은 제한(경로 문자·숨김 접두 금지, 128자). 지정하면 모든 경로(채널·큐·tmux·inbox·원격 직송)에서 from에 그 값을 쓴다. `--from=값`도 받는다.
2. `--from` 없이 tmux 밖이면 지금처럼 "user"이되, 이 프로세스의 부모 사슬에서 코덱스(`scan.KindFromArgs`=="codex")를 찾으면 "codex", claude를 찾으면 그 세션이 `inbox open`으로 등록한 별칭(주소록에서 pid로 역조회)이 있으면 그 별칭, 없으면 "claude"로 한다. tmux 안 세션은 기존 그대로(세션명).
3. `inbox wait` 출력의 `from:` 줄은 그 값을 그대로 쓴다(코드 변경 없을 수도 있음 — 확인만).
4. 테스트(패키지별 `go test ./internal/cli -count=1`, 전체 한 번에 금지), README send 절에 `--from` 한 줄, 도움말(helpcmd) 갱신. PR 본문에 "기존 동작 변경 없음(from이 'user'였던 경우만 더 정확한 이름으로)"과 맥에서 실기할 항목(코덱스 앱에서 `agentlayer send al-… "…"` 시 from 값) 명시.
