# agentlayer 리눅스(WSL2) 포팅 설계

날짜: 2026-09-07. 배경: `docs/linux-wsl2-verification.md`. 목적지는 Windows WSL2(Ubuntu). 1차 실험대는 Ubuntu 24.04 VM(화면 없음), 창이 필요한 검증은 WSL2(WSLg)에서.

## 목표
같은 코드베이스·같은 main 브랜치에서 `GOOS=linux` 바이너리가 관제탑·hook·worktree·Discord·에이전트 브라우저까지 동작한다. 윈도우 네이티브(.exe)는 비목표 — tmux가 없다.

## 비목표
- 리눅스에서 실사용 브라우저 쿠키 import(WSL2의 실사용 브라우저는 윈도우 쪽이라 성립 안 함).
- systemd 자동 기동, Linuxbrew formula.

## 변경 지점(실측 목록)
| 영역 | 현재(macOS) | 리눅스 동작 | 파일 |
|---|---|---|---|
| CfT 엔진 | mac-arm64/mac-x64 zip, `ditto`로 풀기, `.app` 경로 | `linux64` zip, `unzip`(없으면 Go archive/zip)으로 풀기, `chrome-linux64/chrome` | `internal/browser/engine.go` |
| 쿠키 import | Keychain+Chrome DB | 명시적 에러 "리눅스 미지원 — 에이전트 브라우저에서 직접 로그인" (list·clear·export는 전용 프로필 DB라 그대로) | `internal/browser/cookies.go`(darwin), `cookies_other.go`(신규) |
| 잠자기 잠금 해제 | `pmset -g assertions` | 리눅스에서 건너뜀 | `internal/cli/browsercmd.go` autopreview |
| 알림 | `osascript` | `notify-send` 있으면 사용, 없으면 무시 | `internal/notify/notify.go` |
| iTerm2 링크 안내 | `/Applications/iTerm.app` 감지 후 안내 | 건너뜀 | `main.go`, `internal/cli/iterm2.go` |
| 도움말 첫 줄 | "iTerm2+tmux" | "tmux"(리눅스) | `internal/cli/helpcmd.go` |
| LaunchAgents 봇 감지 | `~/Library/LaunchAgents` | 폴더 없음 → 빈 결과(이미 그러함, 변경 없음) | — |
| 릴리즈 | darwin만 | linux amd64·arm64 추가 | `.goreleaser.yaml` |
| 설치 | brew cask(mac 전용) | `install.sh`: 최신 릴리즈 tar.gz → `~/.local/bin/agentlayer` (mac도 brew 없이 가능) | `install.sh`(신규), README |

분기 원칙: `runtime.GOOS` 한 곳씩. macOS 전용 패키지·명령에 묶인 파일(cookies)만 build tag로 분리. 테스트는 주입점(기존 패턴)으로 OS 무관하게 돌린다.

## 검증
1. `GOOS=linux go build`·`go test ./...`(맥) 통과.
2. 바이너리를 VM에 scp → `agentlayer init`(claude·codex·gemini 배선), `status`, VM 안 tmux에서 claude 세션 띄워 hook 전이가 관제탑에 잡히는지.
3. `agentlayer browser`: VM(무화면)은 엔진 다운로드·풀기까지 성공하고 기동 실패 메시지가 명확한지. 창은 WSL2에서.
4. `install.sh`를 VM에서 실행해 최신 릴리즈가 `~/.local/bin`에 놓이는지(릴리즈 뒤).
