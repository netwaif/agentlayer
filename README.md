![agentlayer — The best of every tool. 도구의 장점을 흡수하는 마스코트](docs/assets/agentlayer-banner.png)

# agentlayer — iTerm2+tmux 멀티 에이전트 관제탑

tmux 안에서 돌아가는 Claude Code / Codex / Gemini 에이전트들이
**누가 일하는 중이고, 누가 입력을 기다리고, 누가 끝났는데 아직 안 봤는지**를
한 화면에서 보여주는 터미널 도구.

Orca ADE의 관제 기능을 일반 tmux 위에 재현한다 — 자체 터미널도, GUI도,
데몬도 없다. tmux가 이미 잘하는 것(세션 유지, SSH 재접속)은 tmux에 맡기고,
tmux가 모르는 것(에이전트의 의미 상태)만 채운다.

[coach](https://github.com/netwaif/usage-coach)(사용량 코칭) ·
[mat](https://github.com/netwaif/mat)(MultiAgent 작업 관제)의 자매 도구.

## 상태 모델

```
● WORK   일하는 중 (hook heartbeat)
◆ WAIT   사용자 입력·승인 대기  ← 가장 위에 정렬
✔ DONE   끝났는데 아직 안 봄 (읽음 처리 전까지 유지)
✖ ERR    비정상 종료
· idle   대기
  dead   pane 소실 (24시간 뒤 자동 정리)
```

상태는 **화면 스크래핑 없이** 에이전트 공식 hook과 tmux 메타데이터로만
판정한다. `DONE → idle` 전환은 반드시 사용자 행동(점프·읽음 키)으로만
일어난다 — "끝났는데 안 본 것"을 놓치지 않는 게 이 도구의 존재 이유다.

## 설치

```bash
brew install netwaif/tap/agentlayer                 # macOS
# 리눅스 / Windows WSL2 (brew 없이) — 맥도 가능. 릴리즈 tar.gz를 ~/.local/bin 에 놓는다
curl -fsSL https://raw.githubusercontent.com/netwaif/agentlayer/main/install.sh | bash
# 또는 소스 빌드 (Go 1.22+)
git clone https://github.com/netwaif/agentlayer.git && cd agentlayer
make install   # ~/.local/bin/agentlayer
```

리눅스는 Ubuntu 24.04(VM)에서 관제탑·hook·worktree·Discord를 검증했고, 창이 필요한 에이전트
브라우저는 WSL2(WSLg) 기준이다. 리눅스에서 다른 점: iTerm2 링크 라우팅 없음(터미널 링크는
`agentlayer browser open <url>`로), 데스크톱 알림은 `notify-send`가 있을 때만, `cookies import`는
macOS 전용(실사용 Chrome이 윈도우 쪽이라 에이전트 브라우저 창에서 직접 로그인). 깨끗한
Ubuntu에는 Chrome for Testing이 쓰는 공유 라이브러리와 한글 폰트가 없으니 먼저 깐다:

```bash
sudo apt install -y libnss3 libnspr4 libatk-bridge2.0-0 libgtk-3-0 libgbm1 libasound2t64 fonts-noto-cjk
# Ubuntu 24.04 이전은 libasound2t64 대신 libasound2
```

Win10도 스토어판 WSL(2.x)이면 WSLg가 있어 브라우저 창이 뜬다. 상세는
`docs/linux-wsl2-verification.md`.

설정은 한 번:

```bash
agentlayer init            # Claude hook 등록 (기존 hook 보존, 백업 생성)
agentlayer init --dry-run  # 뭘 바꾸는지 먼저 확인
```

tmux 팝업(`C-b a`)을 쓰려면 init이 안내하는 두 줄(bind-key + client-resized 훅)을 `.tmux.conf`에 추가한다. 훅은 터미널을 쪼갰다 합칠 때 팝업이 옛 크기로 남는 tmux 동작을 같은 자리에 다시 여는 것으로 보정한다(커서 유지).
agentlayer는 tmux 설정을 자동으로 수정하지 않는다.

## 사용

```bash
agentlayer            # TUI 관제탑 (j/k 이동, enter 점프+읽음, o 읽음, b 지목, s 캡처, p 프리뷰, t 업무 보드, u 사용량 뷰, r 새로고침, q 종료)
agentlayer status     # plain 표 — SSH·스크립트용
agentlayer status --json
agentlayer card       # Discord 상태 카드 업서트 (주기 실행용) / --out은 JSON만
agentlayer resume     # 죽은 claude 대화 목록 / resume <id>로 구조
agentlayer restore    # 재부팅 뒤 죽은 세션 배치 복원 — 체크리스트에서 골라 enter (space 토글, a 전체, q 취소)
agentlayer restore --resume       # 대화까지 이어서 복원 (같은 체크리스트) / --yes는 체크 없이 전부 / --dry-run은 계획만
agentlayer restore <id> ...       # ID 지정 복원. LaunchAgent(리눅스는 systemd 유닛) 봇·이미 pane 있는 자리는 자동 제외 (봇은 ID 지정 시 강제)
agentlayer wake-all   # 모든 claude·codex 세션에 "세션 이어서하자" 일괄 전송
agentlayer close-all  # "세션 마감하자" 전송 → 전원 완료(DONE)까지 감시 → 요약
agentlayer broadcast "<메시지>"   # 임의 메시지 일괄 전송 (--except로 제외, --yes로 무확인)
agentlayer info <세션>            # 배선 상세 카드: 폴더·엔진·Discord 채널·구동 주체·resume 경로
agentlayer board [--out 경로] [--json] [--no-open] [--refresh]   # 회사 업무 보드 HTML을 전용 브라우저로(--refresh: 열린 보드 파일만 조용히 갱신)
agentlayer wt ...     # worktree 병렬 모드 (아래 참고)
agentlayer browser ...            # 에이전트 전용 브라우저 (아래 참고)
```

## 에이전트별 상태 신호

| 에이전트 | 신호 | 상태 |
|---|---|---|
| Claude Code | hooks (PostToolUse/Notification/Stop) | WORK/WAIT/DONE 전부 |
| Codex | config.toml notify (turn-complete) | DONE (나머지는 스캐너) |
| Gemini | 프로세스·pane 감지 | 존재·소실만 (lifecycle 신호 없음) |

`agentlayer init` 한 번으로 Claude hook과 Codex notify가 함께 등록된다
(기존 설정 보존·백업·멱등).

## 사용량·컨텍스트 (선택적 통합)

[usage-coach](https://github.com/netwaif/usage-coach)가 설치돼 있으면:

- TUI 헤더에 provider별 요약 한 줄, `u` 키로 전용 뷰(게이지·리셋·코칭)
- 각 에이전트 행에 모델·컨텍스트%·마지막 활동 (statusline 스냅샷 + codex rollout)
- coach 콜드 실행이 느려도 5분 파일 캐시 + 중복 실행 방지로 TUI는 블로킹되지 않는다

## 알림·Discord

`~/.config/agentlayer/config.json`:

```json
{
  "discord_webhook_url": "https://discord.com/api/webhooks/...",
  "notify_webhook_url": "https://discord.com/api/webhooks/...",
  "notify_macos": true,
  "notify_discord": false
}
```

- 에이전트가 **완료(DONE)** 되거나 **입력 대기(WAIT)** 로 바뀐 순간에만 알림 1회
  (heartbeat는 무음) — macOS 알림 + (켜면) Discord 단문
- `notify_webhook_url`(선택): 단문 알림을 별도 알림 채널로 분리. 비우면 카드
  웹훅으로 감. 분리하면 대시보드 채널이 카드 한 장짜리로 유지돼 스크롤이 없다
- `agentlayer card`는 사용량 + 에이전트 상태를 Discord 메시지 하나로 계속
  업서트하고, provider level이 악화되면 새 메시지로 핑한다.
  LaunchAgent 등으로 5분 주기 실행을 권장
- `preview_interval`(선택): TUI 미리보기 갱신 주기. Go duration 문자열
  (`"500ms"`, `"2s"`). 기본 `1s`, 하한 200ms(그 아래는 200ms로 보정).
  목록 폴링(2초)과는 별개로 미리보기만 조절된다. TUI 재시작 시 적용
- `worker_auto_approve`(선택, 기본 true): `wt new`로 띄우는 gemini worker를 승인 없이
  돌린다(agy `--dangerously-skip-permissions`, stock gemini `--yolo`). claude·codex는 각자
  설정(auto 모드·trusted)을 따른다. false면 gemini worker가 도구마다 승인을 묻는다
- `preview_auto`(선택): 에이전트 폴더 아래에 새 dev 서버가 뜨면 전용 브라우저에
  자동으로 연다. 기본 `true`. 에이전트 hook(상태 전이)마다 백그라운드로 스캔하므로
  관제탑이 닫혀 있어도 동작한다(스캔 5초 스로틀). 같은 서버는 한 번만, 이미 탭이
  있으면 안 열고(앞으로 끌어오지도 않음), 서버를 내렸다 다시 띄우면 다시 연다.
  전용 브라우저가 떠 있을 때만 동작하며 닫아 둔 브라우저를 띄우지 않는다.
  `false`면 뱃지만 붙고 `p`로 연다
- `browser_port`(선택): 전용 브라우저 CDP 디버깅 포트. 기본 `9222`.
  MCP 설정이 이 주소를 고정으로 보므로 바꾸면 `agentlayer browser mcp`를 다시 등록
- `browser_fx`(선택): 에이전트가 브라우저를 조작할 때 AI 커서·테두리 글로우를
  그린다. 기본 `true`. `false`면 효과만 꺼진다(아래 "조작 효과")
- `browser_control_wait_seconds`(선택): 「내가 조작하기」를 누른 뒤 에이전트의 다음
  도구 호출이 잡혀서 기다리는 최대 시간(초). 기본 `120`. 넘기면 도구 호출에
  타임아웃 안내가 돌아간다(아래 "제어권·작업 탭")
- `browser_trim_snapshots`(선택): `wait_for`·`navigate_page` 응답에 자동으로 붙는
  페이지 스냅샷을 잘라 토큰을 아낀다. 기본 `true`. 구조가 필요하면 에이전트가
  `take_snapshot`을 따로 부른다
- `browser_hangwatch`(선택): 굳은 에이전트 브라우저를 감지해 강제 종료·재기동한다.
  기본 `true`. `false`면 훅에서 감시 자체를 건너뛴다(프로세스 조회도 안 하고, 어떤
  경우에도 브라우저를 죽이지 않는다 — 아래 "행 감시")
- `company_root`(선택): AI 회사 루트(`tasks/`·`runtime/inbox/`가 있는 폴더). 비면 등록된 업무의 `<root>/runtime/inbox` 경로에서 유추한다.
- `board_stale_ready`(선택, 기본 `30m`, 하한 `1m`): 업무 보드에서 ready·blocked 카드가 이 시간 넘게 방치되면 ⚠(Go duration 문자열).

## 세션 지시·업무 보고 (AI 회사 배관)

총괄 세션이 직원 세션에 일을 주고, 직원이 멈추면(끝남·승인 대기·에러) 자동으로 보고받는 최소 배관이다.
직원 지침에 보고 명령이 필요 없다 — 상태 전이가 곧 보고다.

```bash
agentlayer send collab-bot "협업 제안 3건 요약해줘"          # idle·DONE 세션에만 들어간다
agentlayer send search-youtube-bot:t170966 - < 업무요청.md   # 스레드 창(t+6자리)에 여러 줄 본문
agentlayer task assign VIDEO-07 search-youtube-bot:t170966 --inbox ~/ai-folder/company/runtime/inbox
agentlayer task list
agentlayer task watch ~/ai-folder/company/runtime/inbox     # 총괄이 Monitor로 띄워 두는 상주 수신
agentlayer task done VIDEO-07
```

- `send`는 `WORK`(작업 중)·`WAIT`(승인창)에는 넣지 않는다. `--force`로 강제. `dead`는 거부.
- 등록된 세션의 `DONE`·`WAIT`·`ERR` 전이를 hook이 `<inbox>/pending/<id>.json`으로 쓴다
  (`task_id`·세션·창·이전/현재 상태·요약·승인 문구·cwd·시각). heartbeat·승인됨·읽음은 무음.
- `task watch`는 정상 건을 한 줄 JSON으로 출력하고 `received/`로 옮긴다. 깨진 파일·심볼릭 링크·16KiB 초과는 `quarantine/`.
- 세션 소실(dead)은 hook이 아니라 `status`·TUI 실행 때 판정되므로 보고되지 않는다 — `agentlayer status`로 본다.
- 회사 폴더·총괄 절차·직원 등록은 별도 플러그인 `ai-company`가 만든다(이 바이너리는 배관만).
- 보고 JSON의 상태 값은 원문 그대로다: `WORKING`·`WAITING`·`DONE_UNREAD`·`IDLE`·`ERROR`·`DEAD` (`from`/`to`).
- `ERR`(비정상 종료)·`dead`는 `--force`로도 보내지 않는다.
- 막 띄운 세션은 첫 hook이 오기 전까지 idle로 보인다 — 첫 지시는 TUI가 뜬 것을 확인한 뒤 보낸다.
- inbox는 로컬 경로여야 한다(NAS·SMB 마운트 금지) — hook은 2초 안에 못 쓰면 보고를 포기하고 에이전트를 막지 않는다.
- 여러 줄 본문(`-`)은 `\r`·제어문자를 제거하고 64KiB까지만 보낸다. 여러 줄은 tmux 붙여넣기(브래킷)로 들어가 Claude Code·codex·gemini 입력창에서 줄바꿈이 그대로 살아 있다(한 줄은 예전대로 키 입력).

### 원격 직원 (호스팅어 Hermes 등)

다른 머신의 실행기를 직원으로 붙인다. 등록 뒤에는 총괄 절차가 로컬 직원과 같다 — `task assign` → `send` → Monitor 이벤트 → `task done`.

```bash
agentlayer remote add hermes-qa --kind hermes --ssh hostinger --profile tech-qa \
    --exec "docker exec -i -u hermes hermes-agent-iqxn-hermes-agent-1" --workspace-root /opt/data/ai-company/결과물
agentlayer remote check hermes-qa            # ssh 왕복·버전·프로필 확인
agentlayer task assign PING-2 hermes-qa --inbox ~/ai-folder/company/runtime/inbox
agentlayer send hermes-qa - < 업무요청/PING-2.md   # 첫 send가 칸반 카드를 만들고 dispatch로 띄운다
agentlayer send hermes-qa "답: a.txt로"            # 카드가 blocked(질문)면 unblock --reason = 답변
agentlayer task done PING-2                        # 서버 카드 archive까지
```

- 보고는 훅이 아니라 **`task watch`의 폴링**(기본 5초)이 만든다 — 감시가 켜져 있어야 온다. 카드 상태 대응: running→`WORKING`, blocked→`WAITING`(ask = block 사유), done→`DONE_UNREAD`, crashed/timed_out→`ERROR`.
- 완료 시 원격 작업 폴더를 `결과물/<업무ID>/remote/`로 회수하고 `RESULT.md`(result·summary·카드 ID)를 쓴다(50MiB 상한). 보고 JSON의 `cwd`가 그 폴더다.
- 작업 중(`WORKING`)인 원격에는 `--force`로도 보내지 않는다. 기동 실패(프로필 동시 실행 상한 등)는 `send`가 사유를 그대로 보여 주고, 다음 `send`가 같은 카드로 재시도한다.
- 원격 직원은 `agentlayer status`·대시보드에 나오지 않는다(`task list`·`remote list`로 본다).
- ssh는 아이맥이 연다(원격→로컬 방향 없음). 연결이 5분 넘게 끊기면 `ERROR` 한 번 보고.

**편지함 — 직원이 먼저 말 걸기.** 배정과 무관하게 직원이 총괄에게 자료·의견을 보낸다. 총괄은 `to: "MESSAGE"` 이벤트(`from`·본문·있으면 `task_id`)로 받는다. 편지는 언제 올지 모르므로 총괄 감시는 상시로 둔다.

```bash
agentlayer task message "정리본을 결과물/VIDEO-07/에 두었습니다"      # 로컬 직원 pane에서 (Claude·Codex·Gemini 공통)
agentlayer task message --task VIDEO-07 - < 정리본.md                 # 업무ID를 붙이면 log.md에 [MESSAGE]
hermes kanban create "[VIDEO-07] 정리본" --assignee imac-manager --body "…"   # 원격 Hermes 쪽(예약 담당자 = 편지함)
```

**다른 실행기 붙이기(exec 어댑터).** Hermes가 아니라도 명령 몇 개와 JSON 응답 규격만 맞추면 직원이 된다. `docs/remote-exec-example/`에 Hermes CLI를 이 규격으로 감싼 예시가 있다.

```bash
agentlayer remote add oc --kind exec --file oc-adapter.json
```

`oc-adapter.json`의 `commands`: `dispatch`(→`{"handle":"…"}`), `poll`(→`{"status":"idle|working|waiting|done|error","summary","ask","error","seen"}`), 선택 `reply`·`pull`·`mailbox`(→`[{"id","from","text","task_id","at"}]`)·`finish`·`check`. 자리표시자 `{task_id}` `{title}` `{body_file}` `{parent}` `{handle}` `{text_file}` `{dest_dir}`. 본문·답변은 파일로 넘어온다.

### 업무 보드 (칸반 라이트)

회사 루트(`tasks/<업무ID>/task.md`·`log.md`)를 agentlayer가 자동으로 갱신하고 보여 준다. 상태값은 mat과 같은
`pending / in_progress / waiting_<세션> / reviewing / done`이고, 보드 6열은 여기서 파생한다
(`ready` = pending이면서 `parents:`가 전부 done).

- `task assign` → `in_progress` + `[ASSIGN]`, 훅 WAIT → `waiting_<세션>` + `[ASK]`, WORK 복귀 → `in_progress`, DONE → `reviewing` + `[REPORT]`, ERR → `[ERROR]`(status 유지).
- `agentlayer send`로 등록 세션에 보낸 지시는 `[SEND]`로 남는다 — `[ASK]` 뒤의 `[SEND]`가 Q&A 한 쌍.
- `task done <ID>` → `done` + `[COMPLETE]`, 그 결과 부모가 전부 끝난 자식마다 수신함에 `to: READY` 이벤트. 이미 done인 업무에 다시 실행하면 상태·로그는 그대로 두고 자식만 재평가한다 — 부모를 먼저 닫고 나중에 붙인 자식의 READY가 이때 나간다(수신함에 이미 있는 자식은 중복 발송 없음).
- `[REPORT] DONE:` 뒤의 요약은 직원의 마지막 답변 첫 줄(Claude·Codex Stop 훅의 `last_assistant_message`, codex notify의 `last-assistant-message`, 120자 말줄임) — status·보드 카드의 "최근 작업"도 같은 값.
- ready·blocked가 30분(`board_stale_ready`) 넘게 방치되면 ⚠.
- 디스코드 카드에 "업무 보드" 절, `agentlayer board`는 6열 HTML을 전용 브라우저로 연다(`--json`·`--out`).
  보드는 열마다 색점·개수·설명 줄, 방치 카드는 붉은 테두리와 상단 `!!!` 경고 줄, Done 열은 최근 12장만 펼친다.
  카드를 누르면 오른쪽 상세 패널 — 상태·담당 세션·부모/자식 링크·`task.md` 본문(목표·완료 기준 체크)·`log.md` 전체 기록(최신이 위).
  검색(`/` 키, ID·제목·담당·기록 전문)·담당 세션 필터·"방치만"·"Done 표시"·집계 칩으로 열 접기 — 필터 상태는 브라우저에 남아 다시 열어도 유지된다(필터가 카드를 전부 숨기면 도구 줄에 붉은 "N장 숨김" 경고 — 초기화로 되돌림).
  열어 두면 알아서 최신이다 — 훅이 상태 전이마다, `task assign`·`task done`·`send`가 기록을 남길 때마다 `board.html`을 다시 쓰고, 페이지는 30초마다(또는 "새로고침" 버튼·`R` 키) 파일을 다시 읽는다. 머리에 "생성 N초 전". 데몬 없음.
- 회사 루트는 `company_root` 설정이 없으면 등록된 업무의 `<root>/runtime/inbox`에서 유추한다.
  마지막으로 등록한 루트를 기억한다(`~/.local/state/agentlayer/company.json`) — 등록이 모두 사라져도(예: 마지막 업무를 `task done`으로 닫음) 보드는 계속 열린다.
- 등록 없이 회사 루트를 바로 지정하려면 `task assign … --root <회사루트>`, 닫을 때는 `task done <ID> --root <회사루트>`(등록이 이미 해제됐을 때 필수).

task.md는 `status:`·`updated:` 줄만 agentlayer가 건드린다. 총괄은 status를 손으로 고치지 말 것(훅과 충돌).

## Worktree 병렬 모드

같은 저장소에서 여러 에이전트(claude/codex/gemini 혼합 자유)가 서로 파일을
밟지 않고 병렬 작업하게 한다. 명령 하나가 worktree + `agent/<task>` 브랜치 +
tmux window + 에이전트 실행까지 만든다.

```bash
agentlayer wt new auth-api --agent claude --test 'go test ./...'
agentlayer wt new login-ui --agent codex        # 같은 repo, 충돌 없음
agentlayer wt list                              # dirty·미병합·테스트 상태 한눈에
agentlayer wt diff auth-api                     # base 대비 변경
agentlayer wt test auth-api                     # 테스트 실행·기록
agentlayer wt review auth-api                   # 리뷰 파일 생성 → "#> 코멘트" 작성
agentlayer wt send auth-api                     # 코멘트를 에이전트에 수정 지시로 전송
agentlayer wt merge auth-api                    # 검사 요약 + 명령 안내 + y 확인 후 병합
agentlayer wt clean auth-api                    # 보존 우선 정리
```

- **자동 merge 없음** — merge는 항상 안내 + 명시적 확인
- **보존 우선 정리** — 미커밋·untracked·미병합 커밋이 하나라도 있으면 clean 거부
- worktree는 `<repo>/.agentlayer/worktrees/<task>`에, 메타는 상태 디렉터리에 기록

- **말로 시키는 오케스트레이션**: 코디네이터 세션에 "claude랑 codex한테 A/B로
  시켜봐"라고 하면 `orchestration` 스킬(`agentlayer init`이 설치)이 `wt new` →
  2단 dispatch → `status` 폴링 → 취합 절차를 대신 밟는다. 머지는 사용자가 고른다.
  worker의 브라우저 일은 `agent-browser` 스킬 규칙(자기 탭·chrome-devtools MCP)을 따른다

## 에이전트 전용 브라우저

에이전트가 만든 웹 화면을 사람이 직접 보고, 본 것(요소 지목·스크린샷·콘솔
에러)을 다시 담당 에이전트 pane으로 돌려주는 전용 Chrome 관제.

```bash
agentlayer browser                 # 전용 브라우저 기동 (떠 있으면 기존 인스턴스에 attach)
agentlayer browser pick [--agent <id>]   # 활성 탭에서 요소 클릭 → 오버레이에 지시 입력 → 담당 에이전트 pane으로 한 줄 전송 (연속 지목, Ctrl-C 종료)
agentlayer browser pick --once           # 한 번 지목하고 요청 한 줄을 stdout으로 — 에이전트가 "브라우저에서 지목할게"를 받아 직접 실행하는 경로
agentlayer browser open <url>      # 전용 브라우저에 탭 열기 (터미널 링크 클릭을 여기로 보내는 진입점)
agentlayer browser shot [url] [--send] [--notify] [--agent <id>]   # 전체 페이지 스크린샷 — 경로 출력, --send면 담당 에이전트 pane, --notify면 알림 웹훅(폰 Discord)으로 이미지 전송
agentlayer browser errors [--send]       # 콘솔 에러·JS 예외를 Enter까지 수집해 덤프(stdout+파일), --send면 pane 전송
agentlayer browser preview [포트...]     # worktree dev 서버(또는 지정 포트)를 브랜치 라벨(⎇) 창으로 열기
agentlayer browser cookies import <도메인...> [--profile <이름>]  # 실사용 크롬의 지정 도메인 쿠키만 골라 전용 프로필로 가져오기 (macOS, 프로필은 쿠키 많은 쪽 자동)
agentlayer browser cookies list [도메인...]     # 전용 프로필에 있는 쿠키 현황 — 호스트별 개수, 도메인 지정 시 이름·만료 (값은 안 보임)
agentlayer browser cookies clear <도메인...>    # 전용 프로필에서 지정 도메인 쿠키만 지우기 (로그아웃, 실사용 크롬은 무관)
agentlayer browser cookies export <도메인> [이름] --to <파일> [--format value|netscape|json]  # 쿠키를 파일로만 내보내기 (0600, 화면엔 값 안 찍힘) — 이름 있으면 값 한 줄, 없으면 yt-dlp·curl용 cookies.txt
agentlayer browser cookies export <도메인> <이름> --env <.env파일> <KEY>  # .env의 KEY= 줄만 그 쿠키 값으로 갱신 (봇·스크립트 설정 주입)
agentlayer browser mcp             # claude·codex·gemini에 chrome-devtools-mcp를 이 브라우저로 붙이는 설치 명령 출력
```

- **말로 시켜도 된다**: 지금 대화 중인 세션이 대상이면 관제탑을 열 필요 없이
  "브라우저에서 지목할게"·"스크린샷 확인해봐"·"콘솔 에러 확인해봐"·"x.com 쿠키
  가져와줘"라고 하면 된다. `agent-browser` 스킬(`agentlayer init`이 `~/.claude/skills`에
  설치)이 `pick --once`·`shot`·`errors --reload`·`cookies`를 대신 실행하고 결과를
  읽는다. 관제탑 키는 여러 에이전트 중 대상을 고를 때 쓴다
- **관제탑에서 키 하나로**: 에이전트 행을 고르고 `b`(요소 지목)·`s`(활성 탭
  캡처)를 누르면 그 에이전트 pane으로 바로 간다(후보 선택 없음). 에이전트
  폴더 아래에서 dev 서버가 listen 중이면 행 끝에 `🌐:3000` 뱃지가 붙는다. 처음
  보는 서버는 관제탑이 닫혀 있어도 hook이 자동으로 전용 브라우저 창(⎇브랜치
  제목)에 연다(`preview_auto`). `p`는 그 서버를 다시 연다. 위 명령들은 이 키들이
  뒤에서 부르는 배관이다
- **엔진은 Chrome for Testing** — 구글이 자동화용으로 배포하는 정식 Chrome을
  첫 기동 때 한 번 `~/.local/state/agentlayer/chrome-for-testing/`에 내려받는다
  (약 200MB, 십수 초). 시스템 Chrome을 전용 프로필로 띄우면 macOS가 그 프로세스를
  "Google Chrome"으로 등록해, Dock·Spotlight에서 Chrome을 열어도 실사용 크롬이
  뜨지 않고 에이전트 브라우저에 창만 추가된다. 번들 ID가 다른 Chrome for Testing은
  별개 앱이라 섞이지 않는다. 코덱은 실Chrome과 같고 Widevine DRM만 없다.
  내려받기에 실패하면 시스템 Chrome으로 대체한다(위 Dock 문제는 남음). 엔진을
  새 버전으로 바꾸려면 그 폴더를 지우면 다음 기동 때 다시 받는다. 리눅스는
  `linux64` 빌드(x86_64만)를 받아 `chrome-linux64/chrome`으로 띄운다 — arm64 리눅스는
  시스템 Chrome/Chromium을 쓴다
- **조작 효과(FX)** — 에이전트가 MCP로 클릭·입력·드래그·스크립트를 실행하는 동안
  뷰포트 안쪽 테두리에 테라코타 글로우가 켜지고, "AI" 뱃지가 달린 커서가 CDP
  마우스 이벤트를 따라 움직이며 클릭 지점에 리플, 입력 중인 요소에 하이라이트가
  뜨고, 상단에 "AI 조작 중" 표식이 붙는다. 사람이 "지금 AI가 손대고 있다"를 보게
  하는 장치다. 도구 호출이 이어지는 동안 켜져 있고 마지막 응답 뒤 2.5초 유지된다
  (호출 하나는 수백 ms라 즉시 끄면 보이지 않는다). 구현은 `mcp-serve` 프록시가 도구 호출의
  시작·끝을 CDP로 페이지에 알리고, 기동 시 프로필에 붙인 작은 확장
  (`~/.local/state/agentlayer/browser-fx/`, 콘텐츠 스크립트)이 그린다.
  Chrome for Testing 전용(브랜드 Chrome 137+는 확장 로드 플래그를 무시 → 효과만
  없음). 이미 떠 있는 브라우저에는 다음 기동부터 붙는다. `browser_fx: false`로 끔
- **제어권·작업 탭(v1.7.0+)** — 도구를 부른 탭이 **작업 탭**이 되어 어둡게 덮이고
  AI 커서(동작 라벨)·하단 알약이 뜬다. 다른 탭에는 얇은 띠만("AI가 다른 탭에서
  작업 중 · <제목>") 보이고 사용자가 자유롭게 쓸 수 있다. 20초 동안 호출이
  없으면 저절로 내려간다. 알약의 「내가 조작하기」를 누르면 에이전트의 다음
  도구 호출이 잡혀서 기다리고(기본 120초, `browser_control_wait_seconds`), 「AI에게
  돌려주기」를 누르면 이어서 실행된다. 「중단」을 누르면 에이전트에게 "재시도하지
  말고 사용자 지시를 기다리세요"가 돌아가고, 대기 시간을 넘겨도 같은 취지의
  안내가 대신 돌아간다. 알약·버튼은 기동 시 프로필에 붙는 확장이 그리므로
  **이미 떠 있는 브라우저에는 다음 기동부터** 보인다. 버튼을 누를 수 없는 상태에서
  제어권이 사용자로 잠겨 에이전트 호출이 계속 막히면
  `agentlayer browser control reset`으로 제어권을 idle로 되돌린다(정본 파일은
  재시작해도 남으므로 이 명령이 비상구다).
- **배경 동작(macOS만)** — 에이전트 브라우저를 기동하면 그 순간 앞에 있던 앱으로
  포커스가 곧바로 돌아간다. 새 탭(`new_page`)은 **앞 탭**으로 열린다(v1.8.3+) —
  사람이 브라우저 창을 보고 있으면 작업이 그대로 보여야 하고, 숨은 탭은 그리기가
  멈춰 `click`이 "did not become interactive"로 줄줄이 실패했기 때문이다(2026-09-24
  실측). 예전처럼 숨은 탭으로 열려면 `browser_background_tabs: true`. 리눅스는 해당 없음.
- **타임아웃 힌트(v1.8.3+)** — screenshot 타임아웃 응답에 프록시가 설명을 붙이되
  에이전트에게 재시작을 시키지 않는다(클릭 타임아웃은 손대지 않는다). 굳음의 판정과
  재시작은 행 감시만 한다 — 에이전트가 힌트대로 촬영 중 브라우저를 8번 재시작한
  사고의 재발 방지.
- **행 감시** — 훅이 도는 정리 작업 맨 앞에서 UI가 3초 안에 응답하는지, 그리고
  보이는 탭 하나가 3초 안에 화면 프레임(8×8 캡처)을 내놓는지 검사한다(프레임 검사는
  창 전체가 그림으로 굳는 증상 — GPU의 vsync 시계가 죽어 CDP는 살아 있는데 아무것도
  안 그려지는 상태 — 를 잡기 위한 것. 가려진 창·숨은 탭은 원래 프레임이 없으므로
  제외). 10초 이상 간격으로 연속 3회(20초 이상) 실패면 macOS `sample`로 진단 파일
  (`~/.local/state/agentlayer/hang/<시각>.txt`)을 남기고 강제 종료·재기동한 뒤
  알림 웹훅으로 "에이전트 브라우저가 멈춰 재시작했습니다 · 진단: …"을 보낸다
  (재기동에 실패하면 "…강제 종료했습니다 · 재기동 실패: …"). 로그인(프로필)은
  그대로 유지된다. `--disable-hang-monitor` 플래그를 빼 뒀으므로 렌더러(페이지)
  자체가 멈추면 Chrome 본연의 "페이지 응답 없음" 안내가 먼저 뜬다.
  `browser_hangwatch: false`면 감시 자체를 끈다. 사이에 브라우저가 죽고 새로 뜨면
  (pid가 바뀌면) 실패 횟수는 0부터 다시 센다. 감시를 기다리지 않고 바로 되살리려면
  `agentlayer browser restart`(강제 종료 → 같은 프로필로 재기동). 재발 원인 추적용으로
  Chrome 로그를 켜 둔다(`~/.local/state/agentlayer/browser-profile/chrome_debug.log`,
  vsync 모듈 verbose).
- **스냅샷 잘라내기** — `wait_for`·`navigate_page` 응답에 자동으로 붙는 페이지
  스냅샷을 잘라 토큰을 아낀다(`browser_trim_snapshots`, 기본 `true`). 구조가
  필요하면 에이전트가 `take_snapshot`을 따로 부른다.
- **오버레이 변화** — 위 "조작 효과"의 상시 테두리 글로우는 이제 작업 탭 안에서
  어두운 덮개·AI 커서·하단 알약으로 나타난다. 다른 탭은 얇은 띠만 보여 사용자가
  동시에 다른 작업을 계속할 수 있다.
- **"AI 뷰파인더"(v1.8.0+)** — 작업 탭의 오버레이를 다시 디자인했다. 가장자리를
  도는 웜 그라디언트 테두리·빛줄기·모서리 조준 브래킷, 가운데는 선명하고 가장자리만
  어두운 비네트, 읽기 구간의 스캔 빔, 입력 구간의 혜성 꼬리 커서·클릭 버스트·포커스
  락온, 진입 부트 스윕·이탈 접힘, 빔이 흐르는 알약, 다른 탭의 캡슐 배너.
  `prefers-reduced-motion`이면 반복 애니메이션을 멈춘다. 오버레이는 닫힌 shadow
  root 안에 있어 페이지 CSS와 서로 새지 않는다. 브라우저 없이 여섯 상태를 재생해
  보려면 `internal/browser/fx/preview.html`(숫자 1~6, 0은 자동 순회).
- 전용 프로필 `~/.local/state/agentlayer/browser-profile`로 뜬다 —
  로그인 세션은 이 프로필에 보존되고, 실사용 Chrome과 격리된다.
  기동 시 프로필에 다크·중성 회색 테마(Claude Desktop 다크 톤)와 프로필 이름
  (AgentLayer, 우상단 프로필 메뉴)을 심는다. Customize Chrome에서 테마를 바꾸면
  그 선택을 존중한다. 실사용 Chrome과의 구분은 북마크바 등 사용자 몫
- CDP 디버깅 포트는 고정(`browser_port`, 기본 9222)이라 재부팅 뒤에도
  기록 없이 같은 브라우저를 찾아 attach한다. 포트를 다른 프로세스가
  물고 있으면 명시적으로 실패한다 — config에서 포트를 바꾸면 된다
- **에이전트가 직접 조작·검사(클릭·입력·콘솔·네트워크)하는 건 MCP** —
  agentlayer는 제어 기능을 자체 구현하지 않고 chrome-devtools-mcp에 위임한다.
  `agentlayer init`이 claude(`~/.claude.json`)·codex(`~/.codex/config.toml`)·
  gemini(`~/.gemini/settings.json`)에 MCP 서버 `chrome-devtools`를 등록한다
  (백업 `.agentlayer.bak`, 같은 이름의 기존 항목은 건드리지 않음). 서버 명령은
  `agentlayer browser mcp-serve`라 에이전트가 브라우저 도구를 부르는 순간
  Chrome이 없으면 띄우고 chrome-devtools-mcp로 넘긴다 — 기동 명령이 따로 없다.
  세 에이전트는 같은 로그인 브라우저를 공유하되 각자 자기 탭에서 작업한다.
  수동 등록이 필요하면 `agentlayer browser mcp`가 명령을 출력한다. Playwright
  MCP를 쓰려면 `npx @playwright/mcp --cdp-endpoint http://127.0.0.1:9222`
- `cookies import`는 2FA 재로그인이 번거로운 사이트용 — 실사용 크롬에서
  지정 도메인 쿠키만 복호화해 전용 프로필에 심는다(전체 프로필 복사가 아님).
  실행 시 macOS Keychain 접근 팝업이 뜨면 '항상 허용'을 눌러야 한다.
  실사용 Chrome에 프로필(계정)이 여럿이면 그 도메인 쿠키가 가장 많은 프로필을
  고르고 어느 것인지 출력한다 — 다른 프로필은 `--profile "Profile 1"`처럼 지정
  예: `agentlayer browser cookies import youtube.com google.com`
  무엇이 들어 있는지는 `cookies list`(호스트별 개수)·`cookies list x.com`(이름·만료)으로
  보고, 다 쓴 로그인은 `cookies clear x.com`처럼 그 도메인만 지운다 — 다른 사이트
  로그인과 실사용 Chrome은 그대로다
- `cookies export`는 "브라우저에선 되는데 자동화만 하면 로그인에서 막히는" 도구
  (사용량 모니터·yt-dlp·notebooklm CLI·감시 봇)에 세션을 넘겨주는 통로 — 값은
  파일(0600)로만 가고 터미널·로그·Discord에는 요약("… → 파일 기록, 만료 날짜")만
  남는다. 에이전트에게 "claude.ai 세션 키 꺼내서 ~/bot/.env에 넣어줘"라고 하면
  `cookies export claude.ai sessionKey --env ~/bot/.env CLAUDE_SESSION_KEY`를 치고
  값은 에이전트도 보지 않는다. 만료되면 같은 말을 다시 하면 된다(healthcheck 뒤 갱신)
- **구글 계정은 예외** — 구글은 세션 쿠키(`__Secure-1PSIDTS`)를 주기적으로 회전시켜서
  같은 세션의 복사본이 두 곳에서 쓰이면 한쪽이 회전한 순간 다른 쪽이 죽는다.
  `cookies import google.com`은 형식상 들어가지만 실사용 Chrome이 계속 회전시키므로
  복사본은 첫 요청에서 무효화되고 로그인 쿠키 19개가 지워진다(2026-09-06 실측).
  에이전트 브라우저에서 구글이 필요하면 그 창에서 직접 로그인해 **별도 세션**을 만들고,
  notebooklm CLI 같은 구글 도구도 그 도구의 `login`으로 자기 세션을 갖게 한다.
  import는 안착 확인을 하므로 Chrome이 조용히 거부한 쿠키가 있으면 이름을 보고한다
- MCP 스크린샷은 Chrome에 화면 잠자기 방지 잠금("Capturing")을 남길 때가 있어
  모니터가 안 꺼진다 — hook이 에이전트 브라우저에 그런 잠금을 보면 탭마다 1×1
  캡처를 완료시켜 풀어 준다(자동, 30초 간격). 에이전트 브라우저는 재기동 때 이전
  탭을 복원하지 않는다(매번 깨끗하게 시작)
- pick 산출물(요소 컨텍스트 `.md` + 스크린샷 `.png`)과 shot·errors 덤프는
  `~/.local/state/agentlayer/picks/`에 저장된다
- **터미널 링크를 전용 브라우저로**: 기본은 ⌘-클릭이 실사용 브라우저로 간다.
  iTerm2 → Settings → Profiles → (쓰는 프로필) → Advanced → Smart Selection
  "Edit…" → `+`로 규칙 추가: Regex `https?://[^\s"'<>)]+`, Precision Very High,
  Actions에 "Run Command" / Parameter `~/.local/bin/agentlayer browser open "\0"`.
  Smart Selection 규칙에 액션이 있으면 ⌘-클릭이 첫 액션을 실행하므로 링크가
  전용 브라우저에 열린다(에이전트가 보는 화면 = 사람이 클릭한 화면). 시스템
  브라우저로 열고 싶을 땐 우클릭 → Open URL. (Semantic History는 파일 경로
  전용이라 URL에는 안 걸린다)
- **SSH 원격에서 쓸 때**: 에이전트(MCP)는 Mac의 전용 브라우저를 그대로
  조작한다(Mac이 GUI 로그인 상태면 됨). 사람이 화면을 보는 방법은 두 가지 —
  ① `agentlayer browser shot --notify`로 스크린샷을 알림 웹훅(폰 Discord)으로
  받기, ② 노트북이면 `ssh -L 9222:127.0.0.1:9222 <mac>` 뒤 노트북 Chrome의
  `chrome://inspect` → Configure에 `localhost:9222` 등록 → inspect로 전용
  브라우저 탭을 실시간으로 보고 클릭까지 할 수 있다
- 전송 대상 라우팅: 페이지가 localhost면 포트를 `lsof`로 역추적해
  dev 서버의 작업 폴더와 에이전트 폴더를 최장일치로 대조한다.
  좁혀지지 않으면 산 에이전트 전원이 후보가 된다

## 안전 원칙

- 기존 tmux 세션·window·pane을 절대 kill하지 않는다
- 기존 prefix·키 바인딩을 변경하지 않는다 (`C-b a`는 옵트인)
- settings.json 수정 전 백업(`settings.json.agentlayer.bak`)을 만든다
- 상태 파일(`~/.local/state/agentlayer/`)에 인증정보를 쓰지 않는다
- Mac이 sleep/재부팅하면 tmux와 에이전트 프로세스는 유지되지 않는다
  (tmux-resurrect 등과 병용 권장)

## 로드맵

- **Phase 1 ✔**: 상태 관제 코어 — TUI·status CLI·Claude hook
- **Phase 2 ✔**: 사용량 뷰(coach 통합)·macOS/Discord 알림·Discord 상태 카드
- **Phase 3 ✔**: worktree 병렬 모드 — 생성·diff 코멘트 회신·테스트 수집·보존 우선 정리
- **Phase 4 ✔**: Codex 어댑터·MultiAgent 패널·비상 resume·배포 준비
- **v1.0 ✔**: Gemini(Antigravity CLI·stock CLI) 완전 편입 — hook 상태추적·모델·ctx(근사)·resume,
  3사 그룹 정렬·구분선, 기본모델 헤더(Fable 경고), GitHub 공개 + brew tap
