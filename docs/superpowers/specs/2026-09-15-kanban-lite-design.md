# 칸반 라이트 — 설계

작성 2026-09-15. 대상: AI 회사(`2026-09-14-ai-company-design.md`)의 업무 흐름을 **보이게** 만들고, 사람이 손으로 하던
상태 갱신·기록을 agentlayer 배관이 대신하게 한다. 산출물은 agentlayer v1.6.0 + ai-company v0.2.

## 배경

Hermes 컨테이너의 칸반(`/opt/data/ai-company/docs/KANBAN_WAKE_REVIEW.md` 조사본, 2026-09-15 수신)에서 가져올 것은 넷이다:
보드 화면, Q&A가 카드에 남는 것(block→코멘트→unblock), 상태 자동 전이, 선후 관계와 ready 방치 경고.
가져오지 않는 것: 자동 디스패처(총괄=LLM이 배정한다), gateway 5초 폴링 wake(우리는 훅이 즉시 보고한다).

지금 우리에게 있는 것: `tasks/<업무ID>/task.md`(```yaml 블록의 `status:`), `log.md`(append-only), `agentlayer task assign/done`,
훅이 DONE·WAIT·ERR 전이를 `runtime/inbox/pending/`에 보고, 디스코드 카드의 "MultiAgent: …" 한 줄(`starter.ActiveTasks`, 고정 경로).
빠진 것: task.md의 status를 사람이 고친다 / WAIT의 질문과 총괄의 답이 어디에도 안 남는다 / 업무 간 선후 관계가 없다 /
회사 루트의 tasks/는 어디에도 표시되지 않는다.

## 결정 (사용자 승인 2026-09-15, 상태값 결정은 이 스펙)

- 범위 4항목: **① 보드 화면 ② Q&A 기록 ③ 상태 자동 전이 ④ 선후 관계·방치 경고**. 자동 디스패처는 만들지 않는다.
- **상태값은 새로 만들지 않는다.** mat·starter·ai-company가 이미 쓰는 `pending / in_progress / waiting_<누구> / reviewing / done`을 그대로 쓰고,
  Hermes의 6열은 여기에 **매핑**한다. `ready`는 저장하지 않고 파생한다(pending이면서 parents가 전부 done).
  이유: starter.active()·mat·`companyctl doctor`·기존 task.md 전부 무수정으로 호환되고, 열 이름은 표시층에서만 붙이면 된다.
- 정본은 여전히 파일(`task.md`·`log.md`)이다. 데몬·HTTP 서버 없음. 보드 HTML은 파일로 생성해 전용 브라우저로 연다.
- 회사 루트는 설정 `company_root`로 지정하되, 비어 있으면 **등록된 업무의 inbox에서 유추**한다(`<root>/runtime/inbox` 규약). 설정 0개로 동작.

## 열 매핑

| 보드 열 | task.md `status:` | 누가 언제 쓰나 |
|---|---|---|
| todo | `pending` (parents 미완) | 총괄이 task.md 생성(템플릿 기본값) |
| ready | `pending` (parents 전부 done, 또는 parents 없음) | 파생 — 저장하지 않음 |
| running | `in_progress` | `task assign` 시 / 등록 세션이 WORK로 전이할 때(승인됨 포함) |
| blocked | `waiting_<세션>` (tmux 세션 이름, 창 제외) | 등록 세션이 WAIT로 전이할 때 |
| review | `reviewing` | 등록 세션이 DONE으로 전이할 때 |
| done | `done` | `task done <업무ID>` |

ERR 전이는 status를 바꾸지 않는다(어느 열에 둘지 사람이 정해야 한다 — 카드·보드에 `ERR` 배지만 붙인다).
등록되지 않은 업무(assign 안 함)는 총괄이 만든 상태 그대로 보인다.

## 1. agentlayer v1.6.0

### 1.1 패키지 `internal/board` (신규)

회사 루트의 `tasks/*/task.md`·`log.md`를 읽고 쓰는 유일한 곳. `starter`는 손대지 않는다(다른 루트·다른 목적).

```go
type Card struct {
    ID, Title, Status string   // Title = task.md 첫 "# " 제목(없으면 ID)
    Parents  []string          // yaml parents: [A, B]
    Column   string            // todo|ready|running|blocked|review|done (파생)
    Session  string            // task 등록이 있으면 세션[:창], 없으면 ""
    State    string            // 등록 세션의 현재 Agent 상태(idle·WORK·WAIT·DONE·ERR·dead), 없으면 ""
    Updated  time.Time         // task.md mtime
    Ready    time.Time         // ready 열에 들어온 시각(= 마지막 부모의 done 시각 또는 task.md mtime)
    LastLog  string            // log.md 마지막 줄(카드 부제)
}
func Root(cfg, assignments) string             // company_root → inbox 유추(`runtime/inbox` 접미) → ""
func Load(root string, assigns []task.Assignment, agents []*state.Agent, now) ([]Card, error)
func SetStatus(root, id, status string) error   // ```yaml 블록의 status: 줄만 교체(+ updated: 오늘), 원자적 쓰기
func AppendLog(root, id, tag, text string) error // "[YYYY-MM-DD HH:MM] [TAG] text" 한 줄 append
func Children(cards []Card, parentID string) []Card
func StaleReady(c Card, now, limit time.Duration) bool
func HTML(cards []Card, now time.Time) []byte   // 정적 페이지
```

- `parents:` 파싱은 yaml 블록 안의 `parents: [A, B]` 한 줄 형태만 받는다(readStatus와 같은 줄 단위 방식, 외부 yaml 의존 없음). 여러 줄 리스트(`- A`)도 받는다.
- 알 수 없는 status 값은 `todo` 열에 `?` 배지로 둔다. 관제탑을 멈추지 않는다(starter와 같은 원칙).
- yaml 블록이 없는 task.md는 status를 못 쓴다 → SetStatus는 오류를 돌려주고 호출자(훅)는 stderr 한 줄만 남긴다.

### 1.2 등록에 업무 폴더 연결 — `task assign`

`task.Assignment`에 `TaskDir string`(json `task_dir`)을 더한다. `assign`은 `--root <회사루트>`를 받되 생략하면 `--inbox`가
`<root>/runtime/inbox` 꼴일 때 root를 유추한다. `<root>/tasks/<업무ID>/task.md`가 있으면 TaskDir를 채우고 status를 `in_progress`로
바꾸며 log에 `[ASSIGN] <세션[:창]>`을 남긴다. 없으면 TaskDir는 비고 경고 한 줄("보드에 표시되지 않음") — 등록 자체는 성공한다.

### 1.3 훅 자동 전이·기록 (③·②)

`main.go` runHook의 전이 콜백에서 `task.ReportFor` 뒤에, 등록이 있고 `TaskDir != ""`이면:

| 전이 `to` | task.md status | log.md |
|---|---|---|
| WAIT | `waiting_<세션>` | `[ASK] <Agent.Ask>` |
| WORK (prev가 WAIT·idle·DONE) | `in_progress` | (없음 — heartbeat 소음 금지) |
| DONE | `reviewing` | `[REPORT] DONE: <Agent.Headline()>` |
| ERR | (변경 없음) | `[ERROR] <Headline>` |

- 쓰기는 보고와 같은 2초 상한 고루틴 안에서 한다. 실패해도 훅은 exit 0.
- inbox 보고(1.5.0)는 그대로. 보고 JSON에 `task_dir`를 더해 총괄이 경로를 다시 찾지 않게 한다.

### 1.4 총괄 답변 기록 — `send` (②)

`agentlayer send <대상> <메시지>`의 대상이 등록된 세션이고 TaskDir가 있으면 전송 성공 뒤 log에 `[SEND] <메시지 첫 줄> (<n>자)`를 남긴다.
메시지가 1000자 이하면 전문을 한 줄로(개행은 `⏎`), 넘으면 첫 1000자 + `…`. WAIT 중 `--force` 전송도 같은 규칙으로 남는다.
`[ASK]` 다음의 `[SEND]`가 곧 Q&A 한 쌍이다 — 별도 코멘트 구조를 만들지 않는다.

### 1.5 `task done` — 부모 완료 → 자식 ready (④)

`task done <업무ID>`는 지금처럼 등록을 해제하고, 추가로 TaskDir가 있으면 status `done` + log `[COMPLETE]`.
그 뒤 같은 루트의 카드 중 `parents`에 이 ID가 있고 parents가 **전부** done이 된 자식마다 inbox에 이벤트를 하나 쓴다:

```json
{"version":1,"id":"<32hex>","task_id":"<자식ID>","kind":"board","from":"pending","to":"READY",
 "task":"<자식 제목>","task_dir":"<root>/tasks/<자식ID>","at":"…"}
```

기존 `task watch`는 무수정으로 이 건을 흘려보낸다(version·id·task_id·to 검사만 하므로). 총괄 절차는 `to=READY`면 배정하라는 뜻.
등록이 이미 해제된 업무(`gone`)를 done 하려면 `--root`가 필요하다(등록에서 루트를 못 얻으므로).

### 1.6 방치 경고 (④)

ready 열 카드의 `Ready` 시각이 `board_stale_ready`(설정, Go duration, 기본 `30m`)를 넘으면 **⚠**를 붙인다.
알림은 따로 쏘지 않는다 — 카드가 훅 전이마다 갱신되고, 총괄이 READY 이벤트를 이미 받았다. 데몬 없이 얻을 수 있는 최대치다.
`blocked`도 같은 기준으로 ⚠(승인이 사람 몫이라 잊히기 쉬움).

### 1.7 보드 화면 (①)

**디스코드 카드**: `agentsContainer` 아래에 컨테이너 하나 `### 업무 보드 — <회사 이름>`(이름은 `<root>/직원명부.json`의 name, 없으면 폴더명).

```
ready 2 · running 3 · blocked 1 · review 1 · todo 4 · done 12
🟡 VIDEO-07-TOPICS  search-youtube-bot:t170966  WAIT 12분  ⚠
   -# [ASK] 참고자료 폴더 밖을 읽어도 될까요?
🟢 VIDEO-07-SCRIPT  collab-bot  WORK 3분
⚪ VIDEO-07-THUMB  (ready 41분 ⚠)  ← VIDEO-07-TOPICS
```

- 행은 blocked → review → running → ready 순, todo·done은 집계 줄에만. 최대 8행, 넘으면 `외 n`.
- `Root()`가 빈 문자열이면 컨테이너 생략(회사가 없는 시청자에게는 아무것도 안 보임). 기존 "MultiAgent:" 줄은 그대로.

**`agentlayer board`**: `<state>/board.html`을 생성하고 전용 브라우저로 연다(`browser open` 경로 재사용). `--out <경로>`는 파일만,
`--json`은 카드 배열만(stdout). 6열, 카드에 ID·제목·세션·상태·경과·parents·마지막 log 줄, ⚠. 자바스크립트 없음, 새로고침은 재실행
(훅이 매 전이마다 `board`를 재생성하지는 않는다 — 열려 있는 탭은 `browser` 탭 재로드로 갱신. 후속에서 필요하면 `card --event`에 얹는다).
TUI에는 열을 추가하지 않는다(1.4 원칙 유지).

### 1.8 설정

`internal/config/config.go`에 `CompanyRoot string`(`company_root`)와 `BoardStaleReady string`(`board_stale_ready`, 기본 30m, 하한 1m) 추가.

### 1.9 테스트

- `board`: Load(열 파생 표·parents 파싱 두 형태·알 수 없는 status·yaml 없음), SetStatus(줄만 교체·다른 내용 보존·원자성), AppendLog 형식, Children, StaleReady, HTML(열 6개·⚠·XSS 이스케이프), Root 유추.
- `task`: assign이 TaskDir·status·log를 채움 / task.md 없으면 경고만 / done이 자식 READY 이벤트를 쓰는 표(부모 둘 중 하나만 done이면 안 씀) / `--root` 폴백.
- 훅: 전이 표 4행(WAIT·WORK·DONE·ERR) + 미등록·TaskDir 없음은 무동작.
- `send`: 등록 세션이면 `[SEND]` 기록(1000자 절단·개행 치환), 미등록이면 안 남김.
- 카드: 보드 컨테이너 유무(루트 없음→생략)·정렬·8행 상한·⚠.
- 실측: `~/ai-folder/company`에 LAB-2(parents: [LAB-1]) 두 건 → assign → 직원 턴 → 카드·`board --json`에서 열 이동 확인 → `task done LAB-1` → READY 이벤트 수신.

## 2. ai-company v0.2

- `assets/task.md`: yaml에 `parents: []` 줄 추가, 주석에 열 매핑 한 줄. status 기본값 `pending` 유지.
- `assets/company-block.md` 총괄 절차 개정: 2단계 "status: in_progress"를 삭제(assign이 한다) / 3단계 `task assign … --root {ROOT}` /
  4단계에 `to=READY`(자식 배정 가능) 추가, `[ASK]`에 답할 때는 `agentlayer send`로(기록이 남는다) / 5단계 "status를 done으로" 삭제(`task done`이 한다) /
  보드 확인은 `agentlayer board`. 파일 직접 수정 금지 목록에 `tasks/*/task.md의 status:` 추가.
- `companyctl doctor`: agentlayer ≥ 1.6.0, `tasks/*/task.md`의 parents가 존재하는 ID인지 검사(끊긴 참조 WARN).
- `SKILL.md` 7단계 검증 사슬을 LAB-1 → LAB-2(parents) 두 건으로.
- 매니페스트 0.2.0. 테스트: task.md 템플릿 parents 존재, doctor의 끊긴 parents WARN.

## 3. 단계와 완료 기준

| 단계 | 산출물 | 완료 기준 |
|---|---|---|
| 1 | agentlayer v1.6.0 | 1.9 테스트 통과 + 실측 한 바퀴 + GitHub latest·tap 갱신 |
| 2 | ai-company v0.2.0 | 2 테스트 통과, 사용자 회사(`~/ai-folder/company`)에 `install` 재실행으로 블록 갱신 |
| 3 | 사용자 확인 | 디스코드 카드에 업무 보드 절이 보이고 `agentlayer board`가 열린다 |

## 범위 밖

자동 디스패처, 보드 자동 새로고침(HTTP 서버·웹소켓), TUI 보드 열, 방치 알림 푸시, 카드 코멘트 구조(log.md가 코멘트다),
MultiAgent(`starter`) 루트를 보드로 합치기, 여러 회사 루트, 우선순위 정렬(`priority:`는 표시만).

## 위험

- 훅이 task.md를 쓰므로 총괄과 훅이 같은 파일을 동시에 쓸 수 있다 — 둘 다 temp→rename이고 훅은 `status:` 한 줄만 바꾼다. 총괄이 편집기로 열어 둔 채 저장하면 훅의 전이가 덮일 수 있다(문서에 "status는 손대지 말 것"으로 막는다).
- `waiting_<세션>`은 tmux 세션 이름만 쓴다(`waiting_collab-bot`, 창 `t170966`은 뺀다). mat은 접두사 `waiting_`만 보므로 어느 쪽이든 무해하고, 창은 카드의 Session 열에 따로 보인다.
- `Root()` 유추는 등록이 하나도 없을 때 실패한다(회사는 있는데 배정 전, 또는 총괄이 마지막 업무를 `task done`으로 닫아 등록이 전부 사라진 직후). 후자는 필드 테스트에서 실제로 관측됐다 —
  `agentlayer board`가 "회사 루트를 찾지 못했습니다"로 실패하고 디스코드 카드의 업무 보드 절이 사라졌다.
  `task assign`·`task done`이 루트를 찾을 때마다 `<stateDir>/company.json`에 기억해 두는 것으로 고쳤다(`board.RememberRoot`/`RememberedRoot`, 우선순위는 `company_root` 설정 → inbox 유추 → 기억된 값). 배정 전(회사는 있는데 아직 한 번도 등록이 없음)은 여전히 실패한다 — `company_root`를 `companyctl install`이 안내한다(자동 기록은 안 함: agentlayer 설정 파일은 사용자 소유).
