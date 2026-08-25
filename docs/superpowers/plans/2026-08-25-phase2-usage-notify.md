# AgentLayer Phase 2 — 사용량·알림·Discord Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** TUI에 coach 사용량 전용 뷰와 봇별 모델·컨텍스트%·활동시각을 얹고, 상태 전이(DONE_UNREAD/WAITING) 시 macOS+Discord 알림을 보내고, discord_dash를 승계하는 Discord 상태 카드(`agentlayer card`)를 만든다.

**Architecture:** 외부 데이터는 전부 읽기 전용 소비 — coach는 `coach --json` 서브프로세스, 봇 컨텍스트는 statusline 스냅샷(`~/.config/usage-coach/sessions/*.json`)과 codex rollout(`~/.codex/sessions/**/*.jsonl`) 파싱(discord_dash의 `_codex_latest` 로직 Go 이식). 알림은 hookcmd의 상태 전이 지점에서만 발화(중복 없음 보장). Discord 카드는 Components V2 웹훅 메시지 하나를 업서트(현 discord_dash와 같은 프로토콜, 별도 메시지·별도 상태 파일이라 병행 운영 안전).

**Tech Stack:** Go 표준 라이브러리(os/exec, net/http, encoding/json). 새 외부 의존성 없음.

**Spec:** `docs/superpowers/specs/2026-08-25-agentlayer-design.md` §4.1(사용량 뷰), §4.2(알림+Discord)

## Global Constraints

- Phase 1의 Global Constraints 전부 승계.
- coach·스냅샷·codex 세션 파일이 없으면 해당 패널/필드만 조용히 비운다 — 코어는 단독 동작.
- 웹훅 URL 값을 로그·상태 파일·에러 메시지에 노출하지 않는다.
- 기존 discord_dash의 상태 파일(`~/.config/usage-coach/discord-state.json`)과 메시지를 건드리지 않는다 — agentlayer는 자기 카드만 만든다.
- 알림은 상태가 실제로 바뀐 순간에만 1회 — heartbeat(post-tool-use 반복)는 알림을 만들지 않는다.

## 파일 구조 (Phase 2 추가분)

```
internal/
├── usage/
│   ├── coach.go        # coach --json 실행·파싱 → Payload{Providers}
│   ├── coach_test.go
│   ├── ctx.go          # statusline 스냅샷 + codex rollout → cwd별 CtxInfo{Model,UsedPct,TS}
│   └── ctx_test.go
├── config/
│   ├── config.go       # ~/.config/agentlayer/config.json 로드 (웹훅, 알림 on/off)
│   └── config_test.go
├── notify/
│   ├── notify.go       # macOS osascript + Discord 웹훅 단문 알림
│   └── notify_test.go
├── discord/
│   ├── card.go         # Components V2 카드 조립 (usage + agents)
│   ├── card_test.go
│   ├── webhook.go      # 업서트(POST/PATCH) + 상태 파일(message_id)
│   └── webhook_test.go
└── ui/  (수정)
    ├── model.go        # u 키 뷰 전환, ctx 데이터 로드
    └── view.go         # 행에 모델·ctx%·age, 사용량 뷰 렌더
```

### Task 1: usage/coach — coach --json 소비

- `type Window struct { LeftPct *float64; ResetMin *float64 }` (null 허용)
- `type Provider struct { OK bool; Plan, Email, Level, Action, Reason string; Windows map[string]Window }`
- `type Payload struct { TS string; Providers map[string]Provider }`
- `func Fetch(runner func() ([]byte, error)) (*Payload, error)` — 실행 주입, coach 부재 시 (nil, nil)
- `func Gauge(pct *float64, width int) string` — "█████░░░" 유니코드 바
- 테스트: 실측 JSON fixture 파싱, null 윈도우, coach 부재
- Commit `feat(usage): coach --json 파싱`

### Task 2: usage/ctx — 봇별 모델·컨텍스트%

- `type CtxInfo struct { Model string; UsedPct *float64; TS time.Time }`
- `func LoadSnapshots(dir string) map[string]CtxInfo` — statusline 스냅샷 JSON들
  {cwd, project_dir, model, used, ts} → key는 ~축약 project_dir(없으면 cwd), 최신 ts 승자.
  파일 삭제 등 부수효과 없음(청소는 discord_dash 몫).
- `func CodexLatest(root, workdir string) CtxInfo` — rollout jsonl: 첫 줄 cwd 매칭,
  tail 128KB에서 마지막 token_count의 (total-12000)/(window-12000), model 정규식(전체 폴백)
- 테스트: fixture 스냅샷 2개 최신 승자, codex jsonl fixture 파싱, 빈 디렉터리
- Commit `feat(usage): 봇별 모델·컨텍스트% 수집`

### Task 3: TUI 통합 — 행 확장 + 사용량 뷰

- 메인 뷰 행에 `[모델 · ctx% · age]` 추가 (CtxInfo를 CWD로 조인, 40%↑ 노랑·80%↑ 빨강)
- 헤더에 사용량 요약 한 줄: `Claude 5h 77% · 7d 16% | Codex 80%` (worst level 색)
- `u` 키: 사용량 전용 뷰 전환 — provider별 블록(level 이모지+action, 게이지 바+리셋 시각,
  Antigravity 계정별 행, reason 한 줄) — 캡처된 Discord 카드와 동일 정보
- refreshCmd에서 coach는 15초 캐시(2초 폴링마다 subprocess 방지)
- 테스트: 조인 로직, 사용량 뷰 렌더 토큰, u 토글
- Commit `feat(ui): 봇 컨텍스트 표시 + 사용량 전용 뷰`

### Task 4: config — agentlayer 설정

- `~/.config/agentlayer/config.json`: `{"discord_webhook_url":"", "notify_macos":true, "notify_discord":false}`
- `func Load() (*Config, error)` — 없으면 기본값, `AGENTLAYER_CONFIG` env 오버라이드
- 테스트: 기본값, 파일 로드, env 오버라이드
- Commit `feat(config): 설정 파일`

### Task 5: notify — 상태 전이 알림

- `func Notify(cfg *config.Config, a *state.Agent, prev state.AgentState)` —
  DONE_UNREAD·WAITING 진입 시에만: macOS `osascript -e 'display notification ...'`,
  Discord 웹훅 단문(에이전트·세션·작업 한 줄)
- hookcmd.RunClaude에 전이 전 상태를 캡처해 실제 변경시에만 호출 (heartbeat 무음)
- 실행기 주입으로 테스트(osascript/HTTP 목), 웹훅 URL 미노출 확인
- Commit `feat(notify): 완료·대기 전이 알림`

### Task 6: discord — 상태 카드 업서트

- `card.go`: coach Payload + agents + CtxInfo → Components V2 JSON
  (provider 컨테이너: discord_dash와 동일 구성 / 봇 섹션: 상태 뱃지 + ctx 게이지 + age)
- `webhook.go`: `?with_components=true` POST(wait=true)/PATCH 업서트,
  message_id는 `~/.local/state/agentlayer/discord-card.json`
- `agentlayer card` 서브커맨드: 1회 실행(LaunchAgent 주기 실행용), `--out` JSON만 출력
- 테스트: 카드 payload 골든 조립, 업서트 로직(HTTP 목: 404 시 새 POST)
- 실전송은 수동 검증 1회(사용자 웹훅으로, 기존 카드와 별개 메시지)
- Commit `feat(discord): 상태 카드 업서트 + card 서브커맨드`

### Task 7: 통합 검증·병합

- `go test ./...` + e2e에 알림 무음 확인(알림 off 기본) 추가
- 실환경 `agentlayer status`·TUI·`card --out` 스모크
- main 병합, 태그 `v0.2.0-phase2`
