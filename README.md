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
brew install netwaif/tap/agentlayer   # (Phase 4에서 제공 예정)
# 또는 소스 빌드 (Go 1.22+)
git clone https://github.com/netwaif/agentlayer.git && cd agentlayer && go build .
```

설정은 한 번:

```bash
agentlayer init            # Claude hook 등록 (기존 hook 보존, 백업 생성)
agentlayer init --dry-run  # 뭘 바꾸는지 먼저 확인
```

tmux 팝업(`C-b a`)을 쓰려면 init이 안내하는 한 줄을 `.tmux.conf`에 추가한다.
agentlayer는 tmux 설정을 자동으로 수정하지 않는다.

## 사용

```bash
agentlayer            # TUI 관제탑 (j/k 이동, enter 점프+읽음, o 읽음, r 새로고침, q 종료)
agentlayer status     # plain 표 — SSH·스크립트용
agentlayer status --json
```

## 안전 원칙

- 기존 tmux 세션·window·pane을 절대 kill하지 않는다
- 기존 prefix·키 바인딩을 변경하지 않는다 (`C-b a`는 옵트인)
- settings.json 수정 전 백업(`settings.json.agentlayer.bak`)을 만든다
- 상태 파일(`~/.local/state/agentlayer/`)에 인증정보를 쓰지 않는다
- Mac이 sleep/재부팅하면 tmux와 에이전트 프로세스는 유지되지 않는다
  (tmux-resurrect 등과 병용 권장)

## 로드맵

- **Phase 1 (현재)**: 상태 관제 코어 — TUI·status CLI·Claude hook
- **Phase 2**: 사용량 뷰(coach 통합)·macOS/Discord 알림·Discord 상태 카드
- **Phase 3**: worktree 병렬 모드 — 생성·diff 코멘트 회신·테스트 수집·보존 우선 정리
- **Phase 4**: Codex/Gemini 어댑터 완성·MultiAgent 패널·비상 resume·brew 배포
