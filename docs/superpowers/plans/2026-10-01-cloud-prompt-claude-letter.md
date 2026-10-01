새 작업(작음). 브랜치 `claude-letter`, main 대상 PR 하나, 태그/릴리즈/SESSION.md 금지, 한국어. 기존 입력의 결과는 바뀌지 않게(추가만). 먼저 main을 pull(v1.12.1). 설계 정본: `docs/superpowers/specs/2026-10-01-claude-letter-design.md` — 먼저 읽고 그 범위만 한다.

문제: 헤르메스가 먼저 보내는 편지는 서버의 `company-letter`(담당자=회사 총괄 편지함) 하나뿐이라, 디스코드→헤르메스→Claude 앱 세션(baton) 방향이 없다. 편지함만 다른 원격을 하나 더 등록해도 `task.PollRemotesOnce`가 등록된 모든 원격의 편지함을 걷어 가고, 준비물 설치가 `company-letter`를 덮어쓴다.

할 것:
1. `internal/remote/hermesside/`에 `claude-letter.sh`·`claude-letter/SKILL.md`(이름은 맞춰도 됨) 추가. `company-letter.sh`와 같은 구조(카드 생성 + 지금 대화 notify-subscribe)이되 담당자는 상수 `claude-app`(예: `remote.AppMailbox`), 문구는 "Claude에게". 스킬 description은 "Claude/클로드/Claude Code/데스크톱 앱 세션에게 전달·요청·질문, 그 답"에 걸리게. `Hermes.Setup`이 회사용 둘 + Claude용 둘을 모두 깐다(멱등, 돌려주는 경로 목록에 포함). 기존 `company-letter` 렌더 결과는 바이트 단위로 그대로.
2. `agentlayer inbox wait --remote <이름> --app-mailbox`: 그 원격에서 담당자 `claude-app` 앞 todo·ready 카드를 편지로 받는다(claim 방식은 `Hermes.Mailbox`와 동일). 기존 `--mailbox`의 동작·도움말은 그대로. `--remote`에 카드도 `--mailbox`도 `--app-mailbox`도 없으면 지금처럼 오류.
3. `--app-mailbox`로 받은 편지의 출력: `from: <원격>/<보낸이>` 다음 줄에 `letter: <원격>:<카드ID>`, 빈 줄, 본문. 다른 경로(로컬 편지·카드 결과·`--mailbox`) 출력은 그대로.
4. `agentlayer inbox reply <원격>:<카드ID> [--file <경로>]... <답|->`: 어댑터 `Answer(ctx, 카드ID, 답, 파일)` 호출. 회사 수신함(`task reply`)을 거치지 않는다. 성공하면 한 줄 확인 출력, 원격이 없거나 형식이 틀리면 오류.
5. 테스트(패키지별 `go test ./internal/remote -count=1`, `go test ./internal/cli -count=1`, 전체 한 번에 금지): Setup이 네 파일을 깔고 회사용 내용이 안 바뀜 / `--app-mailbox`가 `claude-app` 카드만 집고 `company-manager` 카드는 건드리지 않음 / 출력의 `letter:` 줄 / `inbox reply` 파싱·Answer 호출·첨부. README(inbox 절)와 도움말(helpcmd) 갱신.
6. PR 본문에 "기존 동작 변경 없음"과 맥에서 실기할 항목 명시: `agentlayer remote setup <이름>`으로 서버에 `claude-letter`가 깔리는지, 디스코드에서 헤르메스에게 "Claude에게 … 전해" → 앱 세션 `inbox wait --remote <이름> --app-mailbox` 수신(`letter:` 줄) → `inbox reply` → 디스코드 알림.
