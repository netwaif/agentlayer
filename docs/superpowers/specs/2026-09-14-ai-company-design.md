# AI 회사 로컬 운영 — 설계

작성 2026-09-14. 대상: 실사용 + 유튜브(AICheatKey) 콘텐츠. 시청자가 따라 할 수 있어야 하므로
"스킬 한마디 + 결정적 엔진 + 수동은 디스코드 포털뿐" 방식(folder-bot·discord-harness-installer와 동일)을 따른다.

## 배경

호스팅거 Hermes 컨테이너의 AI 회사(`/opt/data/ai-company`)는 총괄=Hermes, 직원=Hermes 프로필 5명 +
외부 CLI 3명(Claude=folder-bot, Codex·agy=codex-discord 브리지)이다. 외부 직원 3명은 이미 사용자의
로컬 스택 그대로이고, Hermes가 담당한 것은 (1) 총괄 역할 (2) Hermes 프로필 직원 (3) `terminal(background,
notify_on_complete)`로 총괄을 깨우는 완료 알림뿐이다. 서버 문서가 미구현으로 남긴 항목(부팅 복구·수신기
재가동·승인창 감지·직원의 자발적 send 의존)은 로컬의 agentlayer 훅·folder-bot LaunchAgent로 대부분 해소된다.

## 결정 (사용자 확정, 2026-09-14)

- 직원층 = **기존 봇을 직원으로 등록**(search-youtube·collab·sendmanual·academy·haendaechacne·codex-live 등). 신설 봇은 총괄 하나.
- 총괄 = **새 폴더 `~/ai-folder/company/` + 새 folder-bot**. 총괄 봇의 폴더가 곧 회사 루트(서버의 총괄=`/opt/data` 루트와 같음).
- 배선 = **agentlayer 기능**(범용 배관) + **ai-company 플러그인**(회사 생성기). 코드 없는 SendMessage 지침 방식은 기각(모델 준수 의존, Codex·agy 제외, 승인 대기 미전달).
- 다이렉트 지시와 회사 경유 지시는 공존한다. 메인 채널 = 다이렉트, 스레드 = 회사 업무.

## 구성 요소

세 층이며 각각 독립 배포물이다.

| 층 | 배포물 | 역할 |
|---|---|---|
| agentlayer v1.5.0 | brew tap · install.sh | 배관: 세션 지정 전송, 업무 등록, 훅 자동 보고, 상주 수신 |
| folder-bot | 기존 플러그인, 변경 없음 | 직원·총괄 봇, 스레드 = 독립 세션 |
| ai-company (신규 `netwaif/ai-company`) | Claude 플러그인: 스킬 `configure-company` + 엔진 `companyctl.py` | 회사 폴더·규칙·명부 생성, 직원 등록, 점검, 제거 |

시청자 설치 경로:

```
brew install netwaif/tap/agentlayer && agentlayer init      # 있으면 brew upgrade 후 init
/plugin marketplace add netwaif/folder-bot  → /plugin install folder-bot
/plugin marketplace add netwaif/ai-company  → /plugin install ai-company
> AI 회사 만들어줘
```

## 1. agentlayer v1.5.0

### 1.1 `agentlayer send <대상> <메시지>`

세션 하나에 지시를 넣는다. `broadcast`(전원)와 `wt send`(worktree 태스크)의 빈자리.

- 대상 표기: `<tmux 세션>` 또는 `<tmux 세션>:<창 이름>`. 창 이름은 folder-bot 스레드 창(`t<스레드ID 끝6자리>`)을 가리키기 위해 쓴다.
  저장소의 Agent 레코드 중 `Tmux.Session`(및 창 이름)이 일치하는 산 pane 하나로 해석한다. 둘 이상이면 오류(명시 요구).
- 상태 게이트: `idle`·`DONE`만 전송. `WORK`는 "현재 턴 뒤에 처리됨" 경고 후 전송하지 않고, `WAIT`(승인창)는 입력이 승인창을 깨뜨리므로 거부. `--force`로 둘 다 강제.
  `dead`는 항상 거부.
- 전송은 `tmuxx.SendText`(텍스트 → 지연 → Enter, codex TUI 대응 그대로).
- 출력: 대상 세션·pane·상태 한 줄. `--json`은 `{session, pane, state, sent}`.
- 메시지는 인자 하나. 여러 줄은 `-`로 stdin에서 받는다(업무 본문 전달용).

### 1.2 `agentlayer task assign|list|done|watch`

업무 ↔ 세션 등록과 자동 보고의 정본. 저장 위치 `~/.local/state/agentlayer/tasks/<agent-id>.json`.

```json
{"task_id":"VIDEO-07-TOPICS","agent_id":"claude-…","session":"search-youtube-bot","window":"t170966",
 "pane":"%16","inbox":"/Users/x/ai-folder/company/runtime/inbox","assigned_at":"2026-09-14T15:00:00+09:00"}
```

- `assign <업무ID> <대상> --inbox <폴더>`: 대상 해석은 `send`와 같다. 세션(pane)당 업무 하나 — 이미 있으면 오류(`--replace`로 교체). 업무ID는 `[A-Za-z0-9._-]{1,64}`.
- `list`: 업무ID·세션·창·상태(현재 Agent 상태)·경과. `--json`.
- `done <업무ID>`: 등록 해제(파일 삭제). 보고 파일은 건드리지 않는다.
- `watch <inbox>`: `<inbox>/pending/*.json`을 폴링(200ms)해 한 건마다 JSON 한 줄을 stdout에 쓰고 `received/`로 옮긴다. 종료하지 않는다(총괄이 Monitor로 띄움).
  깨진 파일·심볼릭 링크·16KiB 초과는 `quarantine/`로. `--once`는 한 건 후 종료(테스트·셸용).

### 1.3 훅 자동 보고

`main.go`의 `SetTransitionHook` 콜백에서, 전이한 Agent에 `tasks/<agent-id>.json`이 있고 `to ∈ {DONE, WAIT, ERR}`면
`<inbox>/pending/<uuid>.json`을 임시 파일 + rename으로 쓴다.

```json
{"version":1,"id":"<uuid hex>","task_id":"VIDEO-07-TOPICS","session":"search-youtube-bot","window":"t170966",
 "kind":"claude","from":"WORK","to":"DONE","task":"<Agent.Task 요약>","ask":"<Agent.Ask, WAIT일 때>",
 "cwd":"/Users/x/ai-folder/youtube/search-youtube-contents","at":"2026-09-14T15:07:12+09:00"}
```

- 직원 지침에 보고 명령이 필요 없다. 직원이 무엇을 하든 상태 전이가 보고다.
- `dead`는 훅이 아니라 `scan.Sync`(status·TUI·card 실행 시)에서 전이하므로 v1.5.0 범위 밖. 총괄은 `agentlayer status`로 확인한다.
- 등록 안 된 세션(다이렉트 대화)은 아무것도 쓰지 않는다. 훅은 실패해도 에이전트를 막지 않는다(기존 원칙).
- heartbeat(WORK→WORK)는 무음. WAIT→WORK(승인됨)도 보고하지 않는다 — 총괄이 알아야 하는 것은 "멈췄다"뿐.

### 1.4 표시

`status`·TUI·card의 TASK 열은 지금처럼 Agent.Task(요약)를 쓴다. 업무ID는 `task list`에서 본다. TUI 열 추가는 하지 않는다(YAGNI).

### 1.5 테스트

- `send`: 대상 해석(세션/세션:창/중복/없음), 상태 게이트 표, `--force`, stdin 본문. tmux는 기존 테스트 페이크.
- `task`: assign 중복·replace·ID 검증, list, done, watch(정상·깨진 파일·심볼릭 링크·과대·`--once`).
- 훅 보고: 전이 표(DONE·WAIT·ERR 쓰기, WORK·idle 안 씀, 미등록 안 씀), 원자적 쓰기, 파일 내용.
- 실측: 임시 tmux 세션에 `claude`를 띄워 assign → send → Stop 훅 → pending 파일 → watch 출력까지 한 바퀴.

## 2. ai-company 플러그인

### 2.1 스킬 `configure-company`

트리거 "AI 회사 만들어줘", "회사에 직원 추가해줘", "회사 점검해줘", "회사 제거해줘". 단계:

1. preflight(엔진 `doctor`): agentlayer ≥ 1.5.0, `bot-thread`·`bot-up`(folder-bot), tmux, `~/.config/folder-bot/bots.json`.
2. 질문: 회사 루트(기본 `~/ai-folder/company`), 회사 이름, 부서 목록(기본 = 서버 조직도 9부서, 편집 가능).
3. 직원 등록 — 부서마다 (a) 기존 folder-bot 봇에서 고르기, (b) 새 부서 폴더를 만들고 folder-bot `configure-bot`에 위임해 봇 신설, (c) 비워 두기(호출형: 총괄이 `claude -p`로 필요할 때 실행).
   Codex·agy 봇(codex-discord 브리지)도 (a)로 등록 가능 — 명부의 `tool` 필드로 구분.
4. 엔진 `install`: 폴더·파일 생성(2.3), 마커 블록 추가.
5. 총괄 봇: folder-bot `configure-bot`에 위임(회사 루트를 봇 폴더로). 디스코드 포털·페어링은 사용자 수동(기존 스킬 안내 그대로).
6. 검증: `doctor` + 무해한 사슬 한 번(총괄 → 직원 하나 "OK라고 답해" → 보고 수신) — 스킬이 절차를 안내하고 결과를 사용자에게 보여 준다.

### 2.2 엔진 `companyctl.py`

folder-bot `botctl.py`와 같은 결정적·멱등 엔진. 명령: `init --root --name`, `dept add|remove`, `employee add --dept --bot <bots.json 이름> | --folder <새폴더> | --on-demand`,
`install`, `doctor`, `remove`, `list`. 정본은 `<루트>/직원명부.json`. 마커 블록(`<!-- store:ai-company:start/end -->`)은 추가만 하고 제거 시 원문 복원(folder-bot 규칙).
비밀값(토큰)은 절대 다루지 않는다.

### 2.3 회사 루트 구조

```
company/
├─ CLAUDE.md          공동 규칙 + 총괄 절차(마커 블록; folder-bot 블록은 configure-bot이 따로 추가)
├─ SESSION.md         loadout 세션 이어가기(총괄 세션용)
├─ 직원명부.json       {dept, name, tool(claude|codex|gemini), session, folder, mode(bot|on-demand)}
├─ 업무요청/  참고자료/  결과물/  docs/
├─ tasks/<업무ID>/     task.md·log.md (discord-multiagent 형식 → agentlayer MultiAgent 패널에 표시)
├─ runtime/inbox/{pending,received,quarantine}/
└─ _templates/        task.md·업무요청.md·보고.md
```

직원 폴더에는 CLAUDE.md 마커 한 블록만 추가한다: 소속 부서, 총괄 업무는 스레드로 온다는 점, 업무ID를 산출물 파일명·요약에 남기라는 요청.
보고 명령은 없다.

### 2.4 총괄 절차 (CLAUDE.md 마커 블록의 내용)

1. 시작·재정박: `SESSION.md` → `직원명부.json` → `agentlayer task list` → Monitor로 `agentlayer task watch runtime/inbox` 상주(persistent).
2. 요청 접수: 사슬이 없는 일(폴더 하나로 끝남)은 회사 경유를 권하지 않고 그 봇 채널에 직접 지시하라고 안내한다.
3. 업무 등록: `tasks/<업무ID>/task.md`(목표·담당·입력·허용 범위·완료 기준·산출물 경로), `업무요청/<업무ID>.md`.
4. 배정: 직원이 Claude 봇이면 `bot-thread open <봇> <채널ID> <업무ID>` → 스레드 창 `t<끝6자리>` → `agentlayer task assign <업무ID> <세션>:<창> --inbox …` →
   `agentlayer send <세션>:<창> -` 로 업무 본문 전달(파일 경로 대신 본문. 직원 폴더 밖 읽기 권한 프롬프트 회피).
   Codex·agy 직원은 스레드 없이 `<세션>`에 직접(브리지 스레드는 후속).
5. 수신: Monitor 이벤트 → `to=WAIT`면 `ask`를 사용자에게 전달(승인 대행 금지) / `to=DONE`이면 산출물을 직접 읽어 완료 기준 대조 → 다음 직원 배정 또는 사용자 보고 / `ERR`은 보고.
6. 마감: `task done`, `tasks/<업무ID>/log.md` 갱신, 결과물을 `결과물/<업무ID>/`에 복사·링크.
7. 하지 말 것: 메인 채널 대화에 개입, 승인 대행, 봇끼리 디스코드 멘션, 등록 안 된 세션에 전송.

### 2.5 테스트

`tests/test_companyctl.py`(pytest, folder-bot과 같은 형식): init 멱등, employee add 3모드, install 마커 추가·제거 원문 복원, doctor 판정(버전 미달·명령 없음), 명부 스키마.

## 3. 단계와 완료 기준

| 단계 | 산출물 | 완료 기준 |
|---|---|---|
| 1 | agentlayer v1.5.0 릴리즈 | 1.5 테스트 통과 + 실측 한 바퀴 + GitHub latest·tap 갱신 |
| 2 | `netwaif/ai-company` 공개 | 2.5 테스트 통과, README 설치 경로 3줄 |
| 3 | 사용자 본인 회사 설치 | 기존 봇 등록, 총괄 봇 페어링, doctor 통과 |
| 4 | 무해한 2단 사슬 시연 | 총괄 → 직원 A → 직원 B → 총괄 취합, 사용자 개입 0회(승인 제외) |
| 5 | 실사용 사슬 선택 | 사용자 결정 |

각 단계 끝에 멈추고 사용자 확인을 받는다.

## 범위 밖 (v1)

A2A·HTTP 엔드포인트, dead 자동 보고, TUI 업무 열, 자동 승인, 봇끼리 디스코드 대화, Hermes 연동, 브리지(Codex·agy) 스레드 배정, 매뉴얼·영상 제작(5단계 뒤).

## 위험

- 같은 Max 계정을 직원 전원이 나눠 쓴다 — 사용량은 agentlayer 헤더로 감시, 호출형 직원은 Codex·agy로 분산 고려.
- 맥이 꺼지면 회사도 멈춘다(지금의 봇 7개와 같은 조건).
- 스레드 세션의 폴더 밖 쓰기는 권한 프롬프트(WAIT)로 총괄에 보고되며 승인은 사용자 몫이다. 산출물은 직원 폴더 안에 두는 것을 기본으로 한다.
