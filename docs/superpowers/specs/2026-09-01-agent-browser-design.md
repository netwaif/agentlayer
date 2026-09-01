# 에이전트 전용 브라우저 (agentlayer browser) 설계

2026-09-01 확정. Orca ADE의 에이전트 브라우저 대응 — "브라우저에서 요소를 찍으면
터미널 에이전트가 코드를 고치는" 역방향 다리를 tmux 관제탑 위에 구현한다.

## 목적·포지션

- **역방향이 핵심**: 기존 도구(claude-in-chrome, 각 CLI의 브라우저 제어)는 전부
  "에이전트→브라우저" 단방향. 이 기능은 "사람이 브라우저에서 가리키고 → 에이전트가
  받는" 다리이며, 현재 어느 터미널 도구에도 없다.
- **관제탑 자산 재사용**: 에이전트 레지스트리(폴더↔pane)와 SendText가 라우팅의
  절반. 받는 쪽이 claude·codex·gemini 무엇이든 동일하게 작동 — 3사 공통 원칙 부합.
- 용도: 실사용(프론트엔드·시각 수정 루프) + 영상 소재(AICheatKey) 둘 다.

## 범위

포함(v1): 전용 브라우저 기동, 요소 찍어 수정 요청(pick), 스크린샷 전달(shot),
콘솔 에러 전달(errors), worktree별 독립 프리뷰(preview).

제외(사유 있는 탈락):
- 원격 스트리밍 렌더링 — 스트리밍 인프라급 난이도, 유스케이스 없음
- 링크 라우팅·홈페이지·검색엔진·줌 — 진짜 Chrome을 쓰므로 Chrome이 이미 함
- 쿠키 가져오기(실브라우저→전용 프로필 복사) — 격리 취지 훼손. 직접 1회 로그인으로 갈음
- 에이전트가 이 브라우저를 조종하는 제어 API — 3사 CLI가 각자 이미 보유, 재구현 안 함
- 주석 그리기(화살표·박스) — pick의 요소 지목이 이미 정밀, 중복
- TUI 통합(상태 표시·단축키) — 필요 확인 후 후속
- 다중 프로필 — v1은 Default 1개

## 구현 접근

Go + **rod** 라이브러리(순수 Go CDP 래퍼). rod 자동 다운로드는 끄고 시스템
Chrome을 쓴다 — 단일 바이너리 goreleaser·brew 배포 유지. 생 CDP(의존성 제로,
개발 2~3배)와 Node/Playwright 헬퍼(배포 원칙 위배)는 탈락.

## 아키텍처

- `internal/browser/` — launch(기동·attach)·pick(검사 모드·오버레이)·route(라우팅)·
  캡처. CLI 로직은 `internal/cli/browsercmd.go`, 명령 라우팅은 main.go.
- 인스턴스 상태: `~/.local/state/agentlayer/browser.json`(pid·CDP 포트).
  모든 서브커맨드는 이 파일로 기존 인스턴스에 attach, 없으면 기동. 멱등.
  죽은 인스턴스의 상태 파일은 attach 실패 시 자동 정리 후 재기동.
- Chrome 탐색: 시스템 Chrome→Chromium 순 자체 탐색(LookupTool 원칙).
- 전용 프로필: `--user-data-dir=~/.local/state/agentlayer/browser-profile` —
  로그인 세션 영구 보존, 실사용 Chrome과 완전 격리.

## 명령

```
agentlayer browser              # 전용 프로필 Chrome 기동 (멱등)
agentlayer browser pick         # 검사 모드 — 클릭→오버레이 입력→에이전트 pane 전송
agentlayer browser shot [url]   # 스크린샷 → 파일 (--send 시 에이전트 전달)
agentlayer browser errors       # 콘솔 에러·JS 예외 수집→덤프 (--send 시 전달)
agentlayer browser preview [port ...]  # wt 워커별 dev 서버 창 나란히 (⎇브랜치 라벨)
```

## pick 파이프라인 (킬러 기능)

1. 활성 탭에 `Overlay.setInspectMode` — 마우스 오버 하이라이트는 Chrome 내장.
2. 클릭 → 노드 확보 → CSS 셀렉터·outerHTML(절단)·요소 영역 스크린샷·페이지 URL.
3. 요소 옆에 **shadow DOM 오버레이 입력창** 주입(페이지 CSS 충돌 방지) →
   지시 타이핑 → Enter.
4. 전송: 맥락(셀렉터·HTML·URL)은 `~/.local/state/agentlayer/picks/<ts>.md`,
   요소 스크린샷은 `<ts>.png`로 저장하고, pane에는 **한 줄만** SendText:
   `브라우저 요소 수정 요청: "<지시>" (맥락: <md 경로>, 스크린샷: <png 경로>)`.
   여러 줄 send-keys의 조기 제출 함정 회피.
5. pick은 반복 가능(연속 지목), Ctrl-C로 종료.

## 라우팅

- 페이지가 localhost:PORT면 lsof로 리스닝 프로세스 cwd 확보 → 에이전트 레코드
  cwd와 **최장 일치** → 그 pane으로 전송. worktree 폴더면 자동으로 그 워커에게.
- 매핑 실패(외부 사이트)·후보 복수면 오버레이에 에이전트 목록(kind·세션명)을
  띄워 선택.

## shot / errors / preview

- **shot**: 인자 없으면 활성 탭, URL 주면 열어서 전체 캡처 → picks/. 기본은
  경로 stdout(스크립트 활용), `--send`로 pick과 같은 라우팅 전달.
- **errors**: 실행 시점부터 `Runtime.exceptionThrown`·`Log.entryAdded` 수집 →
  리로드·재현 후 Enter로 덤프. 온디맨드(상주 감시 아님). 파일+stdout, `--send`.
- **preview**: 인자 없으면 wt 메타 경로 아래 프로세스가 리스닝하는 localhost
  포트를 lsof 역추적 → dev 서버마다 창 열고 제목에 ⎇브랜치. 포트 명시도 지원.
  dev 서버 기동은 워커 몫 — agentlayer는 여는 것만.

## 에러 처리

Chrome 없음·CDP attach 실패·lsof 불가는 명확한 안내 후 종료. 코어 관제는
브라우저 기능 없이 무영향(usage 패키지와 같은 소비 전용 원칙).

## 테스트

- 순수 로직(라우팅 최장일치·프롬프트 조립·상태 파일·포트 역추적 파싱) 단위 테스트.
- CDP 경로는 헤드리스 Chrome 통합 테스트, Chrome 미설치면 skip(tmux 패턴).
  오버레이 주입·셀렉터 생성 JS는 헤드리스 실페이지로 검증.
- e2e: browser 기동→shot→파일 존재.

## 배포

go.mod에 rod 추가만, goreleaser·brew 무변경. README browser 섹션.
매뉴얼은 선반영 금지 원칙대로 릴리즈 후 반영.
