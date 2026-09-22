# 에이전트 브라우저 업그레이드 — 제어권·오버레이·행 감시·호출 효율 설계

2026-09-22. 사용자 요청 3건(가끔 창 전체가 굳어 강제 종료만 통함 / AI 커서가 사용자 마우스에 끌려감·제어권 넘기는 버튼 없음 / ego-lite처럼 조작 중 UI가 화려했으면)과 추가 요청 2건(검색을 시키면 백그라운드로 돌아갔으면 / 가볍고 빨랐으면)을 한 설계로 묶는다. 사용자는 세부 결정을 이 문서의 추천안에 위임했다("추천으로 해줘").

## 0. 실측과 전제

- **페이지 안에서는 합성 입력과 실제 입력을 구분할 수 없다.** CDP가 보낸 `pointermove`와 실제 마우스의 `pointermove` 모두 `isTrusted=true`, `sourceCapabilities=null`(pointer)·객체(mouse)로 동일했다(2026-09-22, 242건). 따라서 커서 분리는 프록시가 아는 "도구 호출 구간"으로만 가능하다.
- 지금 구조: `agentlayer browser mcp-serve`가 chrome-devtools MCP 앞에 프록시로 앉아 stdio 한 줄씩 넘긴다(`internal/cli/browsercmd.go`). `FxTracker`가 tools/call과 응답을 짝지어 `SignalFx`로 모든 웹 탭의 `<html data-agentlayer-fx>`에 on/off를 쓰고, 확장 콘텐츠 스크립트(`internal/browser/fx/content.js`)가 AI 커서·리플·글로우·알약을 그린다. 커서는 `mousemove`를 그대로 따라가서 사용자 마우스에도 반응한다.
- 프록시는 클로드 세션마다 하나씩 뜬다. 여러 에이전트가 브라우저 하나를 나눠 쓴다(현행 규칙 유지).
- 다운 증상은 창 전체(탭 전환·메뉴까지)가 굳고 강제 종료만 통한다. 크래시·행 리포트는 없다(DiagnosticReports·Crashpad 비어 있음). 원인은 미상이므로 이번엔 **감지·진단 채집·자동 복구**까지만 한다.
- 기동 플래그(`internal/browser/instance.go newLauncher`)는 rod 기본값 위에 얹혀 있고 `--disable-hang-monitor`가 포함돼 있다.
- **ego-lite 직접 실측(2026-09-22)**: 셸에서 작업을 시켜도 앞 앱(iTerm2)이 그대로다 — 포커스를 안 뺏는다. 에이전트 작업은 별도 창(task space)에 두고 사용자 창은 건드리지 않는다. 사용자가 제어권을 가져가면 스크립트가 즉시 "hard stop — 재시도하지 말고 사용자가 continue라고 할 때까지 기다려라"로 끝난다. 커서 옆 라벨은 호출마다 `label` 인자로 넘긴다.
- **사용자 불편 추가(2026-09-22)**: 에이전트가 어느 탭에서 작업 중인지 구분이 안 되고, 다른 탭을 띄워도 "AI 조작 중"이 뜬다. 지금 `SignalFx`가 모든 웹 탭에 신호를 쓰기 때문이다.

## 1. 소유권 상태 기계

정본은 `<stateDir>/browser-control.json` 하나. 임시 파일 → rename으로 원자적으로 쓴다(`board.RememberRoot`와 같은 방식). 패키지 `internal/browser/control.go`.

```json
{ "owner": "agent",                       // "agent" | "user" | "idle"
  "agent": "claude-%12",                  // 마지막으로 도구를 부른 에이전트 ID(관제탑 레코드 키)
  "label": "JustWatch 신작 추출",          // 알약에 보일 작업명 = 그 에이전트 레코드의 Task(최근 작업 요약)
  "since": "2026-09-22T14:01:02+09:00",   // 현재 owner가 된 시각
  "last_call": "2026-09-22T14:01:40+09:00",
  "stopped": false }                      // 사용자가 「중단」을 눌렀는지
```

전이(모두 `control.Apply(state, event, now)` 순수 함수로 구현, 테스트는 표로):

| 현재 | 사건 | 다음 |
|---|---|---|
| idle | 도구 호출 통과 | agent (agent·label·since·last_call 갱신) |
| agent | 도구 호출 통과 | agent (agent·label·last_call 갱신 — 다른 에이전트여도 막지 않고 공유) |
| agent | last_call + **20초** 경과 | idle |
| agent·idle | 사용자 「내가 조작하기」 | user |
| user | 사용자 「AI에게 돌려주기」 | agent(stopped=false; 잡혀 있던 호출이 순서대로 나간다) |
| user·agent·idle | 사용자 「중단」 | user(stopped=true) |

사용자 입력이 파일에 닿는 경로: 콘텐츠 스크립트는 파일을 못 쓴다. 버튼 클릭은 `<html data-agentlayer-request="user:<ms>|agent:<ms>|stop:<ms>">`에 남기고, 프록시가 탭들을 훑어 가장 최신 요청을 파일에 반영한 뒤 모든 탭에 현재 상태를 미러(`data-agentlayer-owner="<owner>:<agent>:<label>:<since_ms>:<last_ms>:<stopped>:<waiting>:<target>:<ack_ms>:<title>"`)로 쓴다. 훑는 시점은 (a) 도구 호출마다, (b) 대기 중(owner=user)엔 0.5초마다.

요청 속성은 **읽으면서 지우지 않는다**(2026-09-22 최종 리뷰). 지우면, 탭 응답이 300ms 예산을 넘겨 버려졌을 때 클릭이 DOM에서만 사라지고 파일에는 반영되지 않는다. 대신 파일에 `last_request_ms`(ack)를 두고 미러로 같이 내려보내, 페이지는 ack보다 새 요청만 돌려주고 ack된 요청만 지운다(`internal/browser/fx/sync.js`).

사용자 소유(owner=user)일 때는 작업 탭 여부(`target`)와 무관하게 **모든 탭**에 알약과 「AI에게 돌려주기」를 띄운다 — PageMap이 비었거나 작업 탭이 닫혀 모든 탭이 `target=0`이 되면 돌려줄 버튼이 사라져 영구 잠금이 되기 때문이다. 버튼조차 못 쓰는 상황(확장이 안 붙은 브라우저 등)의 비상구로 `agentlayer browser control reset`이 있다.

프록시가 하나도 없을 때(에이전트가 전부 꺼짐)는 아무도 파일을 안 갱신하므로 오버레이가 미러의 `last_ms`로 20초 만료를 스스로 계산해 내려간다. 사용자가 갇히는 일은 없다.

## 2. 프록시 게이트 · 배경 동작 · 오버헤드 예산

`internal/cli/browsercmd.go`의 mcp-serve 루프에 `controlGate`를 끼운다. 클라이언트→서버 방향에서 tools/call 한 줄마다:

1. `browser-control.json`을 읽고 탭의 요청 속성을 반영한다(1절).
2. owner=user & stopped=false → 그 줄을 **잡고** 0.5초마다 다시 본다. 최대 **120초**. 풀리면 그대로 전달. 넘기면 JSON-RPC 결과로 `isError:true` 텍스트 "사용자가 브라우저를 직접 조작 중입니다 — 돌려줄 때까지 기다린 뒤 다시 시도하세요"를 클라이언트에 직접 돌려준다(서버로 안 보냄).
3. owner=user & stopped=true → 즉시 `isError:true` "사용자가 브라우저 조작을 중단시켰습니다 — 사용자 지시를 기다리세요".
4. 통과하면 `agent`로 전이하고 파일·미러를 갱신한 뒤 전달한다. 에이전트 ID는 프록시가 물려받은 `TMUX_PANE`으로 상태 저장소(`state.Store.List`)에서 pane이 같은 레코드를 찾아 얻는다(없으면 `"agent"`·label "브라우저 작업").

잡고 있는 동안 MCP 클라이언트 타임아웃보다 먼저 답해야 하므로 120초 상한은 설정 `browser_control_wait_seconds`로 조절 가능(기본 120). 타임아웃·중단 응답 문구는 ego-lite처럼 "재시도하지 말고 사용자 지시를 기다려라"를 명시한다.

**작업 탭 특정(어느 탭에서 일하는지)** — 신호·오버레이를 모든 탭이 아니라 **작업 탭에만** 쓴다.
- chrome-devtools MCP의 `pageId`는 서버 내부 번호다. 프록시는 `list_pages`·`new_page`·`select_page`·`close_page`·`navigate_page` 응답에 실리는 `## Pages` 목록(`N: 제목 (url) [selected]`)을 파싱해 `pageId → url` 표를 세션 동안 유지한다(`internal/browser/pagemap.go`).
- tools/call의 `params.arguments.pageId`로 url을 찾고, rod `Pages()`에서 그 url인 탭(들)에만 `SignalFx`·미러를 쓴다. 표에 없거나 pageId가 없는 호출(예: `list_pages`)은 `[selected]` 탭으로 본다. 같은 url 탭이 여럿이면 전부 작업 탭으로 취급(오탐보다 누락이 나쁘다).
- 나머지 웹 탭에는 미러 값에 `target=0`을 붙여 쓴다 → 오버레이는 방패·디밍·커서·알약 없이 **상단 얇은 띠**("AI가 다른 탭에서 작업 중 · <작업 탭 제목>")만 보인다. 그 탭에서 사용자는 자유롭게 움직인다.
- 방패는 작업 탭에만 걸린다. 사용자가 다른 탭을 보는 동안 작업 탭은 가려진 상태가 되지만 기동 플래그(`disable-backgrounding-occluded-windows`·`disable-renderer-backgrounding`)로 CDP 조작은 계속된다.
- 별도 창(ego-lite의 task space 창)은 `new_page` 응답 합성이 필요해 이번 범위 밖(7절).

**배경 동작(포커스 안 뺏기)** — macOS 한정, 리눅스는 건너뜀.
- 기동: `Connect`가 Chrome을 띄우기 직전 앞에 있던 앱 이름을 `osascript`(System Events, frontmost process)로 기억하고, 창이 생긴 뒤 그 앱을 다시 활성화한다(`open -a` 대신 System Events `set frontmost`). 기억 실패·복원 실패는 삼킨다.
- 새 탭: `new_page` 호출의 `background`가 비어 있고 **에이전트 브라우저가 앞에 있지 않으면** 프록시가 `background:true`를 채워 보낸다. 앞에 있으면(사용자가 보는 중) 그대로 전면 탭. 판정은 그 호출 때 한 번 osascript(≈50ms).
- 이미 있는 `preview_auto` 자동 열기는 건드리지 않는다.

**오버헤드 예산** — 호출당 프록시 추가 지연 **50ms 이하** 목표.
- `SignalFx`·미러 쓰기를 탭마다 고루틴으로 병렬 실행하고 전체 마감 300ms 하나만 둔다(지금은 탭마다 400ms 순차).
- 게이트의 파일 읽기는 수십 µs. 탭 요청 속성 훑기는 미러 쓰기와 같은 Eval에 묶어 왕복 1회로 한다(`internal/browser/fx/sync.js`).
- 탭 목록은 `Target.getTargets` 한 번으로 얻는다(예전엔 `Pages()` + 탭마다 `Info()`). 이 왕복도 `b.Timeout(300ms)`로 묶어 굳은 브라우저가 게이트를 붙잡지 못하게 한다. 페이지 핸들은 원본 브라우저로 만든다 — 타임아웃 클론으로 만들면 rod가 그 컨텍스트에서 파생된 페이지를 캐시에 넣어 300ms 뒤 영구히 죽는다.
- 도구 호출 하나에 드는 Eval을 탭당 1회로: fx "on" 신호는 게이트의 미러 왕복에 실어 보내고(`sync.js`의 `fxValue` 인자), 통과 확정 뒤의 미러 재기록은 내용이 그대로면(`last_call`만 다름) 건너뛴다. "off"는 응답 줄에서 별도 `SignalFx`로 남는다.

## 3. 오버레이(콘텐츠 스크립트)

ego-lite 캡처(2026-09-22)를 기준으로 하되 색은 하우스 팔레트(테라코타 `#d97757`·크림 `#faf9f5`·잉크 `#1f1e1d`). `internal/browser/fx/content.js`를 확장하고 `manifest.json`은 그대로.

- **작업 탭만**: 아래 방패·디밍·커서·알약은 미러 `target=1`인 탭에만 그린다. `target=0` 탭은 상단 띠 하나(높이 28px, 테라코타 바탕·크림 글씨, "AI가 다른 탭에서 작업 중 · <제목>")만.
- **방패**: owner=agent일 때 전체 화면 투명 층(`pointer-events:auto`)이 실제 마우스를 삼킨다. 프록시가 입력 도구(click·hover·drag·fill·fill_form·type_text·press_key·upload_file)를 보내는 구간(`data-agentlayer-fx` on~off)에는 `pointer-events:none`으로 내려 CDP 합성 입력이 DOM에 닿게 한다. owner=idle·user에서는 방패 없음. 키보드는 막지 않는다.
- **디밍**: owner=agent일 때 페이지 위에 어두운 점묘 층(rgba 0,0,0,.35 + radial 점 패턴)과 가장자리 글로우(테라코타, 지금 글로우를 넓힘). 사용자가 내용을 읽을 수 있을 만큼만 어둡게.
- **AI 커서**: 입력 도구 구간의 `mousemove`·`mousedown`만 따라간다(구간 밖 이벤트 무시 — 사용자 마우스에 안 끌려감). 커서 옆 라벨은 도구명을 우리말로: click→"클릭", hover→"가리키는 중", fill·fill_form·type_text→"입력 중", drag→"끌기", navigate_page·new_page→"이동 중", take_snapshot·take_screenshot→"읽는 중", wait_for→"기다리는 중", evaluate_script→"확인 중", 그 외 "작업 중". 클릭 리플·포커스 하이라이트는 유지.
- **알약(하단 중앙, ego-lite 자리)**: 왼쪽에 회전 점(작업 중) 또는 일시정지 아이콘, 가운데 두 줄(작업명 / 상태), 오른쪽 버튼.
  - owner=agent: "AI가 조작 중 · <에이전트 이름>" · 버튼 「내가 조작하기」「중단」
  - owner=user(stopped=false): "내가 조작 중 · 대기 중인 호출 N" · 버튼 「AI에게 돌려주기」
  - owner=user(stopped=true): "중단됨" · 버튼 「AI에게 돌려주기」
  - owner=idle: 알약·디밍·방패 전부 내려감(기존 LINGER 2.5초 뒤).
  - 호스트는 지금처럼 `aria-hidden`을 유지해 `take_snapshot`에 버튼이 섞이지 않게 한다. 대신 `inert`는 뗀다(inert면 버튼 클릭이 안 먹는다). 호스트 자체는 `pointer-events:none`, 방패와 버튼만 `pointer-events:auto`.
- 대기 중인 호출 수 N은 미러 값에 프록시가 넣는다(각 프록시가 자기 대기열 길이를 파일 `waiting` 필드에 합산 — 단순화를 위해 "마지막으로 쓴 프록시의 값"으로 두고, 0/1 이상만 구분해 표시).
- 미러 값에 `target`(1/0)과 작업 탭 제목을 더한다.
- 미러 파싱: `data-agentlayer-owner` 값을 `:`로 나눈다. label에 `:`가 있으면 깨지므로 프록시가 label의 `:`를 `∶`로 바꿔 쓴다.

## 4. 행 감시 · 자동 재시작

훅마다 도는 정리 작업(`capture-janitor`, `internal/browser/capturelock.go` 경로)에 `hangwatch`를 붙인다(`internal/browser/hangwatch.go`).

- 판정: CDP 포트 pid가 있고(`ChromePID`), UI 스레드를 타는 호출 `Browser.getWindowForTarget`(첫 웹 탭)이 **3초** 안에 답이 없으면 1회 실패. `<stateDir>/hangwatch.json`에 실패 횟수를 남겨 **연속 3회**면 행으로 확정(두 판정 사이 최소 10초 → 20초 이상 무응답). 2회에서 3회로 올린 이유(2026-09-22 리뷰): 네이티브 파일 대화상자가 열려 있는 동안 Chrome UI 스레드가 중첩 런루프에 들어가 CDP가 멈추므로, 사용 중인 브라우저를 죽이는 오탐을 줄인다.
- 실행 위치: 훅 정리 작업(`browser autopreview`)의 **맨 앞** — 프리뷰 조기 반환(`preview_auto`·경로 없음·`IsUp`)이나 rod 무제한 호출보다 앞. `IsUp`은 굳은 브라우저에 false라 그 뒤에 두면 영영 도달하지 못한다. 기록된 WS 연결이 빠르게 거부되면(낡은 instance.json) 포트 프로브로 폴백해 건강한 브라우저를 오판하지 않는다. 재기동 실패는 삼키지 않고 알림에 "재기동 실패: <err>"로 적는다. `sample`은 20초 상한.
- 확정 시: macOS면 `sample <pid> 2 -file <stateDir>/hang/<ts>.txt`로 스택을 남기고(리눅스는 `/proc/<pid>/stack`류는 권한 문제라 생략), 프로세스 그룹을 `kill -9`, `browser.json` 제거, `Connect`로 재기동(프로필 보존이라 로그인 유지). 알림 웹훅(`notify`)으로 "에이전트 브라우저가 멈춰 재시작했습니다 · 진단: <경로>" 한 줄.
- 스로틀: `ThrottleOK(dir, "hangwatch", 10s)`. 설정 `browser_hangwatch`(기본 true)가 false면 `ChromePID`(lsof)조차 부르지 않고 즉시 빠진다. 실패 횟수는 pid별로 센다 — 그 사이 브라우저가 죽고 새로 떴으면(pid 변경) 0부터 다시. 프로세스 그룹 `kill -9`는 그 pid가 그룹 리더일 때만(`Getpgid(pid)==pid`).
- `--disable-hang-monitor`를 기동 플래그에서 뺀다(`Delete`). 렌더러가 멈추면 Chrome이 "페이지 응답 없음" 안내를 띄워 탭만 죽일 수 있게 된다.

## 5. 호출 효율

- 프록시가 `wait_for`·`navigate_page` 응답의 자동 스냅샷을 잘라낸다: 결과 텍스트에서 `## Latest page snapshot` 이후를 지우고 "(스냅샷 생략 — 필요하면 take_snapshot)" 한 줄로 바꾼다. 설정 `browser_trim_snapshots`(기본 true)로 끌 수 있다. 판정은 tools/call 요청의 도구명을 `FxTracker`가 이미 id별로 아니 응답 짝짓기에 도구명을 함께 들고 간다.
- 스킬 `agent-browser/SKILL.md`에 규칙 추가: "여러 단계로 데이터를 뽑을 땐 `evaluate_script` 하나 안에서 기다렸다가(폴링) 추출한다. `wait_for`는 눈으로 확인할 때만." 그리고 제어권·배경 동작 안내(「내가 조작하기」가 눌리면 도구가 기다린다는 것, 중단 응답을 받으면 사용자에게 묻고 멈출 것).
- 후속 후보(이번 범위 밖): ego처럼 한 스크립트로 이동·대기·추출을 묶는 `agentlayer browser run <js>`.

## 6. 테스트

- `internal/browser/control_test.go`: 전이 표 전부, 파일 원자 쓰기·읽기, 만료 20초, `:` 치환.
- `internal/browser/pagemap_test.go`: `## Pages` 파싱(selected·닫힘·재번호), pageId→url, 없는 id 폴백.
- `internal/cli/browsercmd_test.go`(또는 `controlgate_test.go`): 게이트 — user면 잡았다가 풀림(가짜 시계), 120초 초과 응답, stopped 즉시 응답, 통과 시 파일 갱신·에이전트 ID 조회, `new_page` background 채우기(앞 앱 판정은 주입 함수), 응답 스냅샷 잘라내기.
- `internal/browser/fx_test.go`: 병렬 SignalFx가 300ms 마감을 지키는지(느린 페이지 흉내), 입력 도구 목록.
- `internal/browser/hangwatch_test.go`: 연속 3회·10초 간격 판정, 재시작 순서(주입 함수로 kill·sample·notify 호출 기록).
- 콘텐츠 스크립트: node로 DOM 흉내(jsdom 없이 최소 스텁)해서 미러 파싱·상태별 표시·방패 pointer-events 전환을 검사하는 `internal/browser/fx/content_test.mjs`(go test에서 `node`가 있으면 실행, 없으면 skip).
- 실측 체크리스트(릴리즈 전 수동): ⓪ 에이전트가 탭 A에서 작업할 때 탭 B에는 띠만 뜨고 마우스가 자유로움 ① 에이전트가 클릭하는 동안 사용자 마우스를 흔들어도 AI 커서가 안 따라옴 ② 「내가 조작하기」 뒤 에이전트 호출이 멈추고, 「돌려주기」 뒤 이어짐 ③ 「중단」 뒤 에이전트가 오류 문구를 받음 ④ 에이전트가 전부 꺼진 뒤 20초 안에 알약이 내려감 ⑤ 다른 앱을 앞에 두고 검색을 시켜도 브라우저가 앞으로 안 튀어나옴 ⑥ `kill -STOP`으로 굳힌 브라우저가 20초 안에 재시작되고 알림이 옴 ⑦ `wait_for` 응답에 스냅샷이 없음.

## 7. 범위 밖

- 에이전트 작업을 별도 창으로 분리하는 것(ego-lite의 task space 창) — 작업 탭 특정으로 시작한다.
- 다운 근본 원인 수정 — 진단 파일이 모이면 다음에.
- `agentlayer browser run` 스크립트 실행기.
- 리눅스의 배경 동작·`sample` 채집.
