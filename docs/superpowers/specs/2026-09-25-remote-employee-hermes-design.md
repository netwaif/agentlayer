# 원격 직원 어댑터 + 편지함 — 첫 구현은 호스팅어 Hermes (설계)

작성 2026-09-25. 대상: 로컬 AI 회사(총괄 = `~/ai-folder/company` 세션)가 다른 실행기(첫 구현: 호스팅어 VPS 컨테이너의
Hermes 전담 프로필)를 직원으로 부린다. 총괄 절차·직원 보고 형식은 기존 로컬 직원과 **완전히 같게** 유지하고,
ssh·칸반은 agentlayer 배관이 숨긴다(명령 0개 원칙). 실행기별 차이는 **어댑터 계약** 하나 뒤에 가둔다 — Hermes는 내장
어댑터, 그 밖(OpenClaw 등)은 명령 4개짜리 `exec` 어댑터로 코드 없이 붙인다. 아울러 직원이 총괄에게 **먼저** 말을 거는
길(편지함 `MESSAGE` 이벤트)을 로컬·원격 공통으로 연다.

## 배경

- 로컬 배관(agentlayer): 총괄 → 직원은 `task assign` + `send`(tmux pane 붙여넣기). 직원 → 총괄은 직원 CLI 훅이
  상태 전이(WORK/WAIT/DONE/ERR)를 `<root>/runtime/inbox/pending/<id>.json`으로 쓰고, 총괄이 Monitor로 띄운
  `task watch`가 한 줄 JSON으로 넘긴다. 총괄 pane에 누가 send-keys를 하는 경로는 없다. 원격·비tmux 직원 개념은 없다
  (`ResolveTarget`·`SendText(paneID)`·`hookPane`·`scan.Sync`의 DEAD 처리 전부 로컬 pane 전제).
- 서버(Hermes v0.20.0, 컨테이너 `hermes-agent-iqxn-hermes-agent-1`, `HERMES_HOME=/opt/data`, 실행 사용자 `hermes`):
  칸반 CLI(`hermes kanban create/dispatch/show --json/block/unblock/complete/archive`), 전담 프로필 5개
  (`business-ops`·`channel-analyst`·`content-pd`·`manual-editor`·`tech-qa`, 모델 gpt-5.6-sol), 보드 `default`,
  `kanban daemon` 없음. 디스코드에 붙은 총괄 `default` 프로필은 이 설계에 관여하지 않는다.
- 실증(2026-09-25): `kanban create --assignee tech-qa` → `dispatch --max 1` → 17초 뒤 `status: done`,
  `result: PONG-OK`, `events[]`에 created/claimed/spawned/heartbeat/completed, `runs[]`에 outcome. 카드 `t_40b3eb2f`.
  scratch 작업 폴더는 완료 시 삭제된다(`tip_scratch_workspace`) → 산출물은 `--workspace dir:<경로>`로 고정해야 한다.
- 네트워크: 아이맥 → 호스팅어는 `ssh hostinger`(키 인증, BatchMode 가능). VPS → 아이맥 방향은 없다(NAT, 컨테이너에
  ssh 키 없음). 따라서 **양방향 모두 아이맥이 주도**한다: 지시는 push(카드 생성), 보고는 pull(카드 상태 폴링).

## 결정 (사용자 확정, 2026-09-25)

- 헤르메스 쪽 직원 = **전담 프로필**(예: `tech-qa`). `dispatch`로 자동 기동. 디스코드 총괄 `default`는 쓰지 않는다
  (kanban 툴셋 비활성 블로커와 무관해진다).
- 방향: 총괄 → Hermes는 ssh로 칸반 카드 생성(사용자 안 1). Hermes → 총괄은 ssh 역방향·tmux 주입이 아니라 로컬
  `task watch`가 카드를 폴링해 **같은 inbox 형식**으로 떨어뜨린다(사용자 안 2를 뒤집음).
- 총괄 지침은 바뀌지 않는다: `task assign <ID> <직원>` → `send <직원> - < 업무요청/<ID>.md` → Monitor 이벤트
  (`WAITING`/`DONE_UNREAD`/`ERROR`) → `send <직원> "<답>"` → `task done <ID>`.
- **편지함(사용자 확정)**: 직원이 배정과 무관하게 총괄에게 먼저 자료·의견을 보낼 수 있어야 한다. 로컬 직원(Claude·Codex·
  Gemini)은 `agentlayer task message`, 원격 직원은 실행기 쪽 편지함(Hermes = 예약 담당자 카드)으로 보내고, 총괄은 둘 다
  같은 `to: "MESSAGE"` 이벤트로 받는다. 이에 따라 총괄 감시(`task watch`)는 **상시** 켠다.
- **어댑터 계약(하드코딩 회피)**: 원격 실행기마다 Go 코드를 짜지 않는다. `remote.Adapter` 인터페이스(지시·조회·답변·회수·
  편지함) 하나를 정의하고, Hermes는 내장 구현, 그 외는 등록 파일에 명령 템플릿을 적는 `exec` 어댑터로 붙인다(Paperclip의
  "heartbeat를 받을 수 있으면 채용" 방식과 같은 발상이되, 계약은 명령 4개 + JSON 응답 규격으로 최소화).

## 구성 요소

| 층 | 변경 | 역할 |
|---|---|---|
| agentlayer v1.9.0 | 신규 패키지 `internal/remote`(어댑터 계약·hermes·exec) + `send`·`task` 분기 + `remote`·`task message` 명령 | 원격 직원 등록, 지시·폴링 → inbox 보고, 산출물 회수, 편지함(로컬·원격) |
| ai-company 플러그인(별도 레포, 후속) | `companyctl employee add --remote <이름>` + 총괄 지침(원격 배정·`MESSAGE` 처리·감시 상시) + 직원 지침(`task message`) | 명부 `mode: "remote"`, `tool: "<kind>"`, `session: "<원격 이름>"` |
| 서버 | 변경 없음 | 프로필·칸반 DB는 그대로. 산출물 폴더 `/opt/data/ai-company/결과물/<업무ID>/`만 생긴다 |

## 1. 원격 직원 등록 (`agentlayer remote`)

```
agentlayer remote add <이름> --kind hermes --ssh <ssh호스트> --profile <Hermes프로필> \
    [--exec "<원격 명령 접두어>"] --workspace-root <원격 절대경로> [--mailbox <예약 담당자>] \
    [--max-runtime 2h] [--poll 5s] [--no-check]
agentlayer remote add <이름> --kind exec --file <어댑터 정의 JSON>      # 그 밖의 실행기(OpenClaw 등)
agentlayer remote list [--json]
agentlayer remote check <이름>
agentlayer remote rm <이름>
```

- 파일: `~/.local/state/agentlayer/remotes/<이름>.json`(0600). 공통 필드: `name`, `kind`(`hermes` | `exec`), `poll`, `added_at`.
  - `hermes`: `ssh`, `exec`(공백으로 나눈 argv, 기본 빈 값 = 호스트에서 `hermes` 직접 실행), `profile`, `board`(선택),
    `workspace_root`(필수, 원격 절대경로; 호스팅어는 `/opt/data/ai-company/결과물`), `mailbox`(예약 담당자, 기본
    `imac-manager`), `max_runtime`.
  - `exec`: `commands{dispatch, poll, reply, pull, mailbox}` — 각각 로컬에서 실행할 argv(셸 미경유). 자리표시자
    `{task_id}` `{title}` `{body_file}` `{handle}` `{text_file}` `{dest_dir}`를 치환한다. 응답 규격은 2절.
- 이름 규칙: `validAgentID`(파일명 한 조각)이고 `:`를 포함하지 않는다(`<세션>:<창>` 파싱과 충돌 금지). 산 tmux 세션 이름과
  같으면 거부한다(대상 해석이 원격을 먼저 보기 때문에 로컬 세션이 가려진다).
- `check`: ssh 왕복 시간, `hermes --version`, `kanban assignees`에 프로필이 `ON DISK yes`인지. `add`는 기본으로 check를
  먼저 돌리고 실패하면 저장하지 않는다(`--no-check`로 생략).
- 호스팅어 등록값(실측): `--ssh hostinger --exec "docker exec -i -u hermes hermes-agent-iqxn-hermes-agent-1"
  --profile tech-qa --workspace-root /opt/data/ai-company/결과물`.

## 2. 어댑터 계약과 원격 실행 계층 (`internal/remote`)

`send`·`task watch`·`task done`은 실행기를 모른다. 아는 것은 이 인터페이스뿐이다.

```go
type Adapter interface {
    Dispatch(ctx, taskID, title, body string, parent Handle) (Handle, error) // 지시 → 실행 시작. Handle = 실행기 쪽 식별자(카드 ID 등)
    Poll(ctx, h Handle) (Status, error)                                       // 상태 조회(폴링)
    Reply(ctx, h Handle, text string) error                                    // WAIT(질문)에 답변 → 실행 재개
    Pull(ctx, h Handle, destDir string) error                                  // 산출물을 로컬 폴더로 회수
    Mailbox(ctx) ([]Letter, error)                                             // 실행기 쪽에서 총괄 앞으로 온 편지(없으면 빈 목록), 반환된 것은 수신 확인 처리
    Check(ctx) (Info, error)                                                   // 연결·프로필 점검(remote check)
}
type Status struct { State state.AgentState; Summary, Ask, Error string; Seen int64 } // Seen = 마지막 관측 이벤트 시각(중복 억제)
type Letter struct { ID, From, Text string; TaskID string; At time.Time }
```

- 내장 어댑터 `hermes`(칸반 CLI 대응, 아래). 범용 어댑터 `exec`: 등록 파일의 `commands`를 실행하고 표준출력의 JSON을 읽는다.
  - `dispatch` → `{"handle": "..."}`; `poll` → `{"status": "idle|working|waiting|done|error", "summary": "...", "ask": "...",
    "error": "...", "seen": 0}`; `reply`·`pull`은 종료 코드만; `mailbox` → `[{"id","from","text","task_id","at"}]`(호출한
    쪽이 수신 확인을 뜻하므로 같은 편지를 두 번 돌려주지 않는 책임은 스크립트에 있다).
  - 본문·답변은 파일(`{body_file}`·`{text_file}`, 임시 파일 0600)로 넘긴다 — 인자 길이·따옴표 문제를 스크립트에 떠넘기지 않는다.
  - OpenClaw 같은 실행기는 이 규격의 스크립트 다섯 개(pull·mailbox는 생략 가능 = 빈 결과)로 붙인다. 이 스펙에서는 규격과
    예시 스크립트 하나(`docs/remote-exec-example/`, Hermes CLI를 exec 규격으로 감싼 셸 예시)까지만 만든다.
- `Runner` 인터페이스 하나: `Run(ctx, stdin io.Reader, args ...string) (stdout []byte, err error)`. 구현은
  `SSHRunner{Host, Exec}` — 로컬에서 `ssh -o BatchMode=yes -o ConnectTimeout=10 -o ControlMaster=auto
  -o ControlPath=<state>/ssh-%C -o ControlPersist=60s <host> <원격 명령 문자열>`을 argv로 실행한다(로컬 셸 없음).
  원격 명령 문자열은 `exec` + `hermes kanban …` 각 인자를 **단일따옴표 이스케이프**로 이어 붙인다(업무 본문의 한글·따옴표·
  줄바꿈 안전). ControlMaster로 폴링 한 번이 수백 ms 안에 끝난다.
- 테스트는 `FakeRunner`(호출 기록 + 정해진 JSON 응답)로 한다. ssh 없이 전 로직을 검증한다.
- 타임아웃: 조회 30초, 생성·기동 60초, 산출물 회수 5분.
- Hermes 칸반 어댑터(`remote.Hermes`): `Create(title, body, workspaceDir, parent) (cardID)`, `Dispatch(cardID) (spawned bool)`,
  `Show(cardID) (Card)`, `Unblock(cardID, reason)`, `Comment(cardID, text)`, `Archive(cardID)`, `Mkdir(dir)`,
  `PullTar(dir) io.Reader`. `Card`는 `show --json`에서 `task.status`·`task.result`·`latest_summary`·`events[]`(kind·payload·
  created_at·run_id)·`runs[]`(outcome·error)만 읽는다.
- 카드 생성 옵션: `--assignee <profile> --idempotency-key agentlayer:<업무ID>[:<n>] --created-by agentlayer
  --workspace dir:<workspace_root>/<업무ID> --max-runtime <max_runtime> [--parent <직전 카드>] [--board <board>] --json`.
  제목은 `<업무ID> <task.md 제목>`(제목 없으면 업무ID). 본문은 `send` 메시지 그대로(64KiB 상한은 기존과 같음).

## 3. 상태 대응

| 카드 status | agentlayer 상태 | 비고 |
|---|---|---|
| todo · ready · triage | IDLE | 생성 직후. `send`가 곧 dispatch |
| running · scheduled | WORKING | |
| blocked | WAITING | `ask` = 마지막 `blocked` 이벤트 payload의 reason(없으면 "BLOCKED:"로 시작하는 마지막 코멘트, 그것도 없으면 "입력 대기") |
| done | DONE_UNREAD | `task` = `latest_summary`(없으면 `result`) 첫 줄 120자 |
| crashed · timed_out · gave_up(runs[].outcome 또는 status) | ERROR | `task` = `runs[].error` 첫 줄 |
| archived | IDLE | 보고 없음 |

보고 여부는 기존 `ShouldReport`(같은 상태 재관측 무음, DONE/WAIT/ERR만)가 결정한다.

## 4. 총괄 → 원격 (`task assign` · `send`)

- 대상 해석: `send`·`task assign`은 대상 문자열을 먼저 `remotes/<이름>.json`에서 찾고, 있으면 원격 경로. 없으면 기존
  `ResolveTarget`. `scan.Sync`·에이전트 저장소(`agents/`)에는 원격 레코드를 **넣지 않는다**(DEAD 처리 회피, 대시보드 무관).
- `task assign <ID> <원격이름> --inbox … [--root …] [--replace]`: 기존 `Assignment`에 `AgentID: "remote-<이름>"`,
  `Session: <이름>`, `Pane: "remote"`를 쓰고 새 필드 `remote: {name, card_id:"", last_status:"", last_event_at:0,
  workspace:""}`를 더한다. task.md `in_progress` + `[ASSIGN] <이름> (hermes:<profile>@<ssh>)`. 카드는 아직 안 만든다.
- `send <원격이름> [- | "<메시지>"]`: 등록이 없으면 거부("먼저 task assign"). 있으면 카드 상태를 실시간 조회해
  기존 `SendGate`를 그대로 적용한다.
  - 카드 없음(IDLE): 원격 `mkdir -p <workspace>` → `create` → `dispatch --max 1`. `spawned`에 그 카드가 없으면 카드는
    남기고 "기동 실패(다음 send로 재시도)"로 실패 반환. 성공 시 등록 파일에 `card_id`·`workspace` 저장.
  - blocked(WAITING): `--force` 없이는 기존 문구로 거부하지 않고 **허용**한다 — 원격에서는 "승인창을 깨뜨릴" 입력창이 없고
    `unblock --reason <메시지>`가 곧 답변이다(로컬 WAIT와 의미가 같은 유일한 응답 경로). 이어서 `dispatch --max 1`.
  - running(WORKING): 기존처럼 거부. `--force`면 `comment`만 남기고 "작업자가 읽는다는 보장 없음" 경고.
  - done(DONE_UNREAD): 같은 업무의 후속 지시 → 새 카드(`--parent <직전 카드>`, 같은 workspace, idempotency-key에 `:<n>`)
    + dispatch. `card_id` 교체.
  - crashed/timed_out(ERROR): 기존처럼 거부(`--force`로도 안 됨). 재시도는 `task assign --replace` 뒤 `send`.
  - 성공 시 log.md `[SEND]`·보드 갱신은 기존 코드 그대로(등록 조건 `as.Session == 이름 && as.Pane == "remote"`).

## 5. 원격 → 총괄 (`task watch`의 폴링)

- `task watch <inbox>`가 상주하는 동안, 등록 목록 중 `remote != nil && card_id != ""`인 업무마다 `poll` 간격(기본 5초)으로
  `show --json`을 부른다. 등록 목록은 폴링 주기마다 다시 읽는다(assign·done을 재시작 없이 반영).
- 상태가 `last_status`와 다르면 가짜 에이전트 `state.Agent{ID: "remote-<이름>", Kind: "hermes", Task, Ask,
  Tmux: {Session: <이름>, PaneID: "remote"}, CWD: <로컬 산출물 경로>}`를 만들어 **기존** `task.ApplyTransition`(task.md·log.md)
  과 `task.ReportFor`→`WriteReport`(inbox)를 그대로 태운다. 훅 코드 변경 없음. 그 뒤 등록 파일의 `last_status`·`last_event_at`
  갱신.
- done 전이 때 산출물 회수: `tar -C <workspace> -cf - .`를 ssh 표준출력으로 받아 `<root>/결과물/<업무ID>/remote/`에 푼다
  (상한 50MiB, 넘으면 중단하고 `[ERROR] 산출물 50MiB 초과`). 같은 폴더에 `RESULT.md`(result·latest_summary·카드 ID·runs)를 쓴다.
  보고 JSON의 `cwd`는 이 로컬 폴더 — 총괄이 기존처럼 "산출물을 읽어" 검증한다.
- ssh 실패: 보고하지 않고 stderr 한 줄. 연속 실패 5분 이상이면 한 번만 `ERROR` 보고(`task` = "원격 연결 실패 N분")하고
  복구되면 다음 실제 전이부터 정상. `task list`는 `remote:unreachable`을 보인다.
- 같은 주기에 등록된 원격마다 `Adapter.Mailbox()`를 한 번 불러 편지를 `MESSAGE` 이벤트로 옮긴다(6절).
- 훅과 달리 폴링은 총괄이 감시를 켜 둔 동안만 돈다. 편지함 때문에 감시 규칙은 **상시**로 바뀐다(6절). 감시가 꺼져 있던 동안의
  전이는 다음 폴링 첫 회에 한꺼번에 따라잡는다(`last_status` 비교라 누락 없음, 중간 상태는 생략).

## 6. 편지함 — 직원이 먼저 말 걸기 (`MESSAGE` 이벤트)

- 이벤트: 기존 `Report` 형식에 `Kind: "message"`, `From: "<보낸 세션·원격 이름>"`, `To: "MESSAGE"`, `Task: <본문>`(최대 8KiB,
  넘으면 절단 표시), `TaskID`(직원이 지정했으면 그것, 아니면 `"-"` — watch 검증이 빈 값을 거부하므로), `Session`, `At`. 기존
  watch 검증(version·id·task_id·to)을 그대로 통과한다.
- 로컬 직원: `agentlayer task message [--task <업무ID>] "<본문>" | -`. 훅과 같은 `hookPane` 규칙으로 보낸 pane을 알아내
  (`TMUX_PANE` 필수, tmux 밖이면 거부) 세션 이름을 `From`에 넣는다. inbox 경로는 `--task`가 있고 그 업무가 이 세션에
  등록돼 있으면 등록의 inbox, 아니면 `company_root` 설정 → 기억된 루트(`company.json`) 순으로 `<root>/runtime/inbox`.
  `--task`가 있으면 `tasks/<ID>/log.md`에 `[MESSAGE] <발췌>`도 남긴다. 명부에 없는 세션도 막지 않는다(검증은 총괄 몫 —
  이벤트에 `from`이 있다).
- Hermes 편지함: 직원(디스코드의 `default`든 전담 프로필이든)이 `hermes kanban create --assignee <mailbox> --body "<본문>"
  [--parent <원래 카드>]`로 카드를 만든다(kanban 도구 또는 terminal 도구로 CLI). `Mailbox()`는 `kanban list --json`에서
  `assignee == mailbox && status in (todo, ready)`인 카드를 골라 `Letter{ID: 카드ID, From: created_by, Text: body,
  TaskID: 제목이 `[<업무ID>]`로 시작하면 그것}`로 돌려주고 `claim` → `complete --result "received by agentlayer <시각>"`로
  수신 확인한다(다음 폴링에 다시 안 보인다). 실패하면 그 편지는 돌려주지 않는다(다음 주기에 재시도, 중복 없음).
- 총괄 지침(ai-company 후속): "`to`가 `MESSAGE`면 `from`과 본문을 읽고 업무 등록·대표 전달·무시를 판단한다. 직원 지침:
  배정 없이 총괄에게 전할 것이 있으면 `agentlayer task message`(원격 Hermes는 `<mailbox>` 앞 카드)".
- 감시 상시: 총괄 절차 1·3·5의 "활성 업무가 있을 때만 감시" 조건을 "재정박 때 켜고 만료 알림마다 다시 켠다"로 바꾼다. 30분마다
  빈 재기동 한 번이 비용의 전부다. 등록된 원격이 없고 활성 업무도 없으면 폴링은 inbox 파일 확인만 한다(ssh 없음).

## 7. 마감 (`task done`)

- 기존 동작(task.md done, `[COMPLETE]`, 자식 READY, 등록 해제)에 더해 원격 등록이면 `kanban archive <card_id>`를 시도한다
  (실패는 경고만). 원격 workspace 폴더는 지우지 않는다.

## 8. `task list`

- 원격 행: 세션 열에 `<이름> (hermes)`, 상태는 `last_status` 대응값, 카드 ID 열 추가(`--json`에는 `remote` 객체 그대로).
  `stale`·`gone` 판정은 원격에 적용하지 않는다(pane이 없다).

## 9. 오류 처리·경계

- 원격 텍스트는 전부 argv 인자로만 전달(로컬 셸 미경유). 원격 셸 문자열은 단일따옴표 이스케이프 함수 하나로만 만든다
  (테스트로 `'`·`$`·백틱·개행 포함 본문 검증).
- `show --json` 파싱 실패·필드 누락은 그 주기만 건너뛴다(보고·등록 파일 변경 없음).
- `dispatch`가 `skipped_nonspawnable`·`skipped_per_profile_capped`로 카드를 안 띄우면 `send`는 실패 반환하고 사유를 그대로
  출력한다(프로필 없음·동시 실행 상한 등).
- 같은 프로필에 여러 업무를 동시에 배정할 수 있다(카드가 다르므로). Hermes의 프로필당 동시 실행 상한에 걸리면 위 사유로
  드러난다.
- 서버의 디스코드 총괄(`default`)이 깨어나는 일은 없다(카드에 `session_id`가 없다 — 실증에서 `null` 확인).

## 10. 테스트

- 단위: 단일따옴표 이스케이프, 상태 대응표, `Card` 파싱(실증 JSON 고정본), `send` 분기(카드 없음/blocked/running/done/error)
  를 `FakeRunner`로, watch 폴링 전이→`ApplyTransition`·`WriteReport` 호출과 `last_status` 갱신, tar 회수 상한.
- 실측(호스팅어, 한 바퀴): `remote add` → `task assign PING-2 hermes-qa` → `send hermes-qa "결과에 PONG-2를 적고 complete"`
  → Monitor에 `DONE_UNREAD` → `결과물/PING-2/remote/RESULT.md` → `task done PING-2` → 서버 카드 archived.
  두 바퀴째는 `block --kind needs_input`을 시키는 지시로 `WAITING`→`send "<답>"`→`DONE_UNREAD` 경로.
- 기존 로컬 직원 경로 회귀: `internal/cli`·`internal/task` 패키지 테스트(패키지별 실행, 전체 `./...` 금지 — 메모리).
- 편지함: `task message`가 `TMUX_PANE` 없이는 거부, 있으면 `MESSAGE` 이벤트가 inbox에 쓰이고 `--task`면 `[MESSAGE]` 로그;
  `FakeRunner`로 Hermes `Mailbox()`가 mailbox 카드만 고르고 claim→complete 순으로 부르는지, 실패 시 편지를 돌려주지 않는지.
- `exec` 어댑터: 자리표시자 치환, 본문 파일 전달, JSON 응답 파싱(불량 JSON은 오류), 예시 스크립트로 Hermes 한 바퀴.
- 실측 셋째 바퀴: 서버에서 `hermes kanban create --assignee imac-manager --body "테스트 편지"` → Monitor에 `MESSAGE`,
  로컬 Codex 직원 pane에서 `agentlayer task message "테스트 편지"` → Monitor에 `MESSAGE`.

## 11. 문서·릴리즈

- README "세션 지시·업무 보고" 절에 "원격 직원(Hermes)" 소절: 등록 3줄, 상태 대응표 요약, 산출물 위치, 감시가 켜져 있어야
  보고가 온다는 점.
- `agentlayer help`에 `remote`·`task message` 명령. README에 "편지함" 소절과 "다른 실행기 붙이기(exec 어댑터 규격)" 소절,
  `docs/remote-exec-example/`에 예시 스크립트.
- ai-company 플러그인(후속, 별도 레포): `companyctl employee add --remote <이름>`(명부 `mode: remote`, `tool: hermes`,
  `session: <이름>`), `company-block.md` 배정 항목에 "원격 직원(Hermes): 스레드 없이 `task assign`·`send`, 산출물은
  `결과물/<업무ID>/remote/`" 한 줄, doctor에 `agentlayer remote check` 호출.
- 릴리즈 v1.9.0(brew tap·install.sh). 시험 카드 `t_40b3eb2f`는 실측 뒤 archive.

## 범위 밖

- Hermes 외 실행기의 **내장** 어댑터(OpenClaw 등은 `exec` 규격으로 사용자가 붙인다), 원격 직원의 대시보드(`agentlayer status`)
  표시, 서버에 `kanban daemon` 상주, 첨부(`kanban attach`) 회수, 산출물의 git worktree 모드, 원격 → 로컬 push 알림,
  편지에 대한 자동 답장(총괄이 판단).
