# 리눅스(WSL2) 포팅 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 같은 main 브랜치에서 `GOOS=linux` 바이너리가 관제탑·hook·에이전트 브라우저까지 동작하고, 리눅스 사용자가 한 줄로 설치할 수 있게 한다.

**Architecture:** macOS 전용 호출(CfT mac zip·ditto, Keychain 쿠키 import, pmset, osascript, iTerm2 문구)마다 `runtime.GOOS` 분기 한 곳을 두고, 리눅스 경로는 대체 수단(linux64 zip·Go unzip, 명시적 미지원 에러, 건너뜀, notify-send, "tmux")을 쓴다. 테스트는 GOOS를 인자로 받는 순수 함수와 기존 주입점으로 OS 무관하게 돈다. 배포는 goreleaser에 linux 타깃을 더하고 `install.sh`로 릴리즈 tar.gz를 설치한다.

**Tech Stack:** Go 1.2x(표준 `archive/zip`), goreleaser v2, bash.

**Spec:** `docs/superpowers/specs/2026-09-07-linux-port-design.md`

## Global Constraints

- 분기는 `runtime.GOOS` 한 곳씩. build tag 분리 없음(macOS 전용 명령은 exec로만 부르므로 어디서나 컴파일된다).
- 리눅스 쿠키 import는 비목표 — 명시적 에러만.
- 테스트는 `go test ./...`가 맥에서도 리눅스에서도 통과해야 한다(플랫폼 의존 테스트는 GOOS 인자화).
- 커밋 메시지 끝: `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` / `Claude-Session: https://claude.ai/code/session_01Cj5gkQnW9HSFwd8QbChYp1`

---

### Task 1: CfT 엔진 — linux64 다운로드·Go unzip

**Files:**
- Modify: `internal/browser/engine.go`
- Test: `internal/browser/engine_test.go`

**Interfaces:**
- Produces: `cftPlatformFor(goos, arch string) string`, `engineRel(goos, arch string) string`(엔진 폴더 기준 실행 파일 상대 경로), `extractZip(zipPath, dir string) error`
- `EngineBin(stateDir)`·`EnsureEngine(stateDir, log)` 시그니처 불변.

- [ ] **Step 1: 실패하는 테스트** — `engine_test.go`의 `TestCFTPlatform`·`TestEngineBinPath`를 교체하고 `TestExtractZip` 추가

```go
func TestCFTPlatform(t *testing.T) {
	cases := []struct{ goos, arch, want string }{
		{"darwin", "arm64", "mac-arm64"}, {"darwin", "amd64", "mac-x64"},
		{"linux", "amd64", "linux64"}, {"linux", "arm64", "linux64"},
	}
	for _, c := range cases {
		if got := cftPlatformFor(c.goos, c.arch); got != c.want {
			t.Errorf("cftPlatformFor(%s,%s) = %s, want %s", c.goos, c.arch, got, c.want)
		}
	}
}

func TestEngineRel(t *testing.T) {
	if got := engineRel("darwin", "arm64"); got != filepath.Join("chrome-mac-arm64", "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing") {
		t.Errorf("darwin rel = %s", got)
	}
	if got := engineRel("linux", "amd64"); got != filepath.Join("chrome-linux64", "chrome") {
		t.Errorf("linux rel = %s", got)
	}
}

func TestEngineBinPath(t *testing.T) {
	got := EngineBin("/st")
	want := filepath.Join("/st", "chrome-for-testing", engineRel(runtime.GOOS, runtime.GOARCH))
	if got != want {
		t.Errorf("EngineBin = %s, want %s", got, want)
	}
}

func TestExtractZip(t *testing.T) {
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	h := &zip.FileHeader{Name: "chrome-linux64/chrome", Method: zip.Deflate}
	h.SetMode(0o755)
	w, _ := zw.CreateHeader(h)
	_, _ = w.Write([]byte("#!/bin/sh\necho cft\n"))
	d, _ := zw.CreateHeader(&zip.FileHeader{Name: "chrome-linux64/locales/", Method: zip.Store})
	_ = d
	_ = zw.Close()
	dir := t.TempDir()
	zp := filepath.Join(dir, "d.zip")
	if err := os.WriteFile(zp, zbuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := extractZip(zp, dir); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, "chrome-linux64", "chrome"))
	if err != nil || st.Mode()&0o111 == 0 {
		t.Fatalf("실행 파일 없거나 권한 없음: %v %v", st, err)
	}
	if st, err := os.Stat(filepath.Join(dir, "chrome-linux64", "locales")); err != nil || !st.IsDir() {
		t.Errorf("디렉터리 항목이 풀려야 한다: %v %v", st, err)
	}
	if err := extractZip(zp, dir); err != nil {
		t.Errorf("같은 zip 재추출은 덮어써야 한다: %v", err)
	}
	// zip slip 방어
	var bad bytes.Buffer
	bw := zip.NewWriter(&bad)
	w2, _ := bw.Create("../evil")
	_, _ = w2.Write([]byte("x"))
	_ = bw.Close()
	bp := filepath.Join(dir, "bad.zip")
	_ = os.WriteFile(bp, bad.Bytes(), 0o644)
	if err := extractZip(bp, filepath.Join(dir, "out")); err == nil {
		t.Error("경로 탈출 항목은 거부해야 한다")
	}
}
```

`TestEnsureEngine_Download`는 `t.Skip` 조건을 지우고 `rel := engineRel(runtime.GOOS, runtime.GOARCH)`, `plat := cftPlatformFor(runtime.GOOS, runtime.GOARCH)`로 바꾼다(맥에서는 ditto, 리눅스에서는 extractZip 경로가 각각 돈다).

- [ ] **Step 2: 실패 확인** — `go test ./internal/browser/ -run 'TestCFTPlatform|TestEngineRel|TestEngineBinPath|TestExtractZip|TestEnsureEngine' -v` → 컴파일 실패(인자 수·미정의 함수)

- [ ] **Step 3: 구현** — `engine.go`

```go
import (… "archive/zip" …)

// cftPlatformFor는 OS·아키텍처에 맞는 CfT 플랫폼 이름. 리눅스 CfT는 x64 빌드 하나뿐이다
// (arm64 리눅스는 자동 설치 불가 → EnsureEngine이 시스템 Chrome 대체 안내).
func cftPlatformFor(goos, arch string) string {
	if goos == "linux" {
		return "linux64"
	}
	if arch == "arm64" {
		return "mac-arm64"
	}
	return "mac-x64"
}

// engineRel은 엔진 폴더 기준 실행 파일 상대 경로.
func engineRel(goos, arch string) string {
	plat := cftPlatformFor(goos, arch)
	if goos == "linux" {
		return filepath.Join("chrome-"+plat, "chrome")
	}
	return filepath.Join("chrome-"+plat, "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing")
}

func EngineBin(stateDir string) string {
	return filepath.Join(EngineDir(stateDir), engineRel(runtime.GOOS, runtime.GOARCH))
}
```

`EnsureEngine`: `if runtime.GOOS != "darwin"` 가드를 `if runtime.GOOS != "darwin" && runtime.GOOS != "linux"`로, `platform := cftPlatformFor(runtime.GOOS, runtime.GOARCH)`. 리눅스 arm64는 linux64 zip이 실행 불가이므로 `if runtime.GOOS == "linux" && runtime.GOARCH != "amd64" { return "", fmt.Errorf("Chrome for Testing 리눅스 빌드는 x86_64뿐 — 시스템 Chrome/Chromium을 설치하면 그것을 씁니다") }`. 풀기:

```go
	if runtime.GOOS == "darwin" {
		// ditto(macOS 내장) — 번들 안의 심볼릭 링크·실행 권한·xattr 보존
		if out, err := exec.Command("/usr/bin/ditto", "-x", "-k", zipPath, dir).CombinedOutput(); err != nil {
			return "", fmt.Errorf("엔진 풀기 실패: %v %s", err, strings.TrimSpace(string(out)))
		}
	} else if err := extractZip(zipPath, dir); err != nil {
		return "", fmt.Errorf("엔진 풀기 실패: %w", err)
	}
```

```go
// extractZip은 zip을 dir 아래에 푼다(리눅스 경로). 항목 모드를 보존하고 dir 밖으로
// 나가는 경로는 거부한다. 같은 파일이 있으면 덮어쓴다.
func extractZip(zipPath, dir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	root := filepath.Clean(dir)
	for _, f := range r.File {
		p := filepath.Join(root, filepath.FromSlash(f.Name))
		if p != root && !strings.HasPrefix(p, root+string(filepath.Separator)) {
			return fmt.Errorf("zip 항목이 폴더 밖을 가리킴: %s", f.Name)
		}
		mode := f.Mode()
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		if mode&os.ModeSymlink != 0 {
			target, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return err
			}
			_ = os.Remove(p)
			if err := os.Symlink(string(target), p); err != nil {
				return err
			}
			continue
		}
		perm := mode.Perm()
		if perm == 0 {
			perm = 0o644
		}
		w, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
		if err != nil {
			rc.Close()
			return err
		}
		_, cerr := io.Copy(w, rc)
		rc.Close()
		w.Close()
		if cerr != nil {
			return cerr
		}
		_ = os.Chmod(p, perm)
	}
	return nil
}
```

- [ ] **Step 4: 통과 확인** — 같은 명령 → PASS. `GOOS=linux go build ./...` 도 통과.
- [ ] **Step 5: 커밋** — `git commit -m "feat(browser): CfT 엔진 리눅스(linux64) 다운로드·Go unzip 분기"`

---

### Task 2: 쿠키 import 리눅스 명시적 미지원

**Files:**
- Modify: `internal/browser/cookies.go:342-346`
- Test: `internal/browser/cookies_test.go`

**Interfaces:**
- Produces: 패키지 변수 `var cookieImportOS = runtime.GOOS`(테스트 주입점). `ImportCookies` 시그니처 불변.

- [ ] **Step 1: 실패하는 테스트** — `cookies_test.go`에 추가

```go
func TestImportCookiesUnsupportedOS(t *testing.T) {
	orig := cookieImportOS
	cookieImportOS = "linux"
	defer func() { cookieImportOS = orig }()
	var out bytes.Buffer
	err := ImportCookies(nil, t.TempDir(), "", "", []string{"x.com"}, time.Now(), &out)
	if err == nil || !strings.Contains(err.Error(), "macOS") {
		t.Fatalf("리눅스에서는 macOS 전용 에러여야 한다: %v", err)
	}
	if !strings.Contains(err.Error(), "직접 로그인") {
		t.Errorf("대안(직접 로그인) 안내가 있어야 한다: %v", err)
	}
}
```

- [ ] **Step 2: 실패 확인** — `go test ./internal/browser/ -run TestImportCookiesUnsupportedOS` → 컴파일 실패(cookieImportOS 미정의)
- [ ] **Step 3: 구현** — `cookies.go` import에 `"runtime"` 추가, 파일 상단에 `var cookieImportOS = runtime.GOOS`, `ImportCookies` 첫 줄:

```go
	if cookieImportOS != "darwin" {
		return fmt.Errorf("cookies import는 macOS 전용입니다(실사용 Chrome의 Keychain 키가 필요) — 리눅스/WSL2에서는 에이전트 브라우저 창에서 직접 로그인하세요. list·clear·export는 그대로 됩니다")
	}
```

- [ ] **Step 4: 통과 확인** → PASS
- [ ] **Step 5: 커밋** — `git commit -m "fix(browser): cookies import 리눅스에서 명시적 미지원 에러"`

---

### Task 3: 화면 잠자기 잠금 해제 — 리눅스 건너뜀

**Files:**
- Modify: `internal/cli/browsercmd.go:920-928`
- Modify: `internal/browser/capturelock.go`
- Test: `internal/browser/capturelock_test.go`

**Interfaces:**
- Produces: `browser.CaptureJanitorSupported(goos string) bool`

- [ ] **Step 1: 실패하는 테스트** — `capturelock_test.go`에 추가

```go
func TestCaptureJanitorSupported(t *testing.T) {
	if !CaptureJanitorSupported("darwin") {
		t.Error("darwin은 pmset이 있어 지원")
	}
	if CaptureJanitorSupported("linux") {
		t.Error("linux는 pmset이 없어 미지원")
	}
}
```

- [ ] **Step 2: 실패 확인** → 컴파일 실패
- [ ] **Step 3: 구현** — `capturelock.go`:

```go
// CaptureJanitorSupported는 잠금 감지 수단(pmset)이 있는 OS인지. 리눅스는 건너뛴다.
func CaptureJanitorSupported(goos string) bool { return goos == "darwin" }
```

`browsercmd.go` 조건 앞에 `browser.CaptureJanitorSupported(runtime.GOOS) &&` 추가(import `"runtime"` 확인).

- [ ] **Step 4: 통과 확인** — `go test ./internal/browser/ ./internal/cli/` → PASS
- [ ] **Step 5: 커밋** — `git commit -m "fix(browser): 화면 잠자기 잠금 해제는 macOS에서만"`

---

### Task 4: 데스크톱 알림 — 리눅스 notify-send

**Files:**
- Modify: `internal/notify/notify.go:16-40`
- Test: `internal/notify/notify_test.go`

**Interfaces:**
- Produces: `desktopNotifier(goos string, lookPath func(string) (string, error)) func(script string) error` — darwin이면 osascript 실행기, linux+notify-send 있으면 notify-send 실행기(title/body는 script 대신 별도 인자로 받아야 하므로 아래처럼 `Sender.RunOSA`를 `Notify(title, body string) error`로 바꾼다).
- `Sender{ Notify func(title, body string) error; PostJSON … }`. 기존 테스트의 `RunOSA` 사용처를 `Notify`로 갱신.

- [ ] **Step 1: 실패하는 테스트** — `notify_test.go`에 추가하고, 기존 테스트의 `RunOSA: func(script string) error {…}`를 `Notify: func(title, body string) error {…}`로 바꾼다(캡처 변수 이름은 그대로).

```go
func TestDesktopNotifierByOS(t *testing.T) {
	has := func(name string) (string, error) { return "/usr/bin/" + name, nil }
	missing := func(name string) (string, error) { return "", errors.New("no") }
	if desktopNotifier("darwin", missing) == nil {
		t.Error("darwin은 osascript(내장)라 항상 있어야 한다")
	}
	if desktopNotifier("linux", has) == nil {
		t.Error("linux에 notify-send가 있으면 알림기가 있어야 한다")
	}
	if desktopNotifier("linux", missing) != nil {
		t.Error("linux에 notify-send가 없으면 nil(조용히 생략)")
	}
}
```

- [ ] **Step 2: 실패 확인** → 컴파일 실패
- [ ] **Step 3: 구현** — `notify.go`

```go
type Sender struct {
	Notify   func(title, body string) error       // 데스크톱 알림(darwin osascript / linux notify-send). nil이면 생략
	PostJSON func(url string, body []byte) error // Discord 웹훅 POST
}

func DefaultSender() Sender {
	return Sender{
		Notify: desktopNotifier(runtime.GOOS, exec.LookPath),
		PostJSON: …(불변),
	}
}

// desktopNotifier는 OS별 데스크톱 알림 실행기. 수단이 없으면 nil.
func desktopNotifier(goos string, lookPath func(string) (string, error)) func(title, body string) error {
	switch goos {
	case "darwin":
		return func(title, body string) error {
			script := fmt.Sprintf("display notification %q with title %q", body, title)
			return exec.Command("osascript", "-e", script).Run()
		}
	case "linux":
		if _, err := lookPath("notify-send"); err != nil {
			return nil
		}
		return func(title, body string) error {
			return exec.Command("notify-send", "--app-name=agentlayer", title, body).Run()
		}
	}
	return nil
}
```

호출부(`cfg.MacOSEnabled() && s.RunOSA != nil` 블록)를 `if cfg.MacOSEnabled() && s.Notify != nil { _ = s.Notify(t, body) }`로. config 키 이름(`notify_macos`)과 접근자 이름은 호환을 위해 그대로 두고 주석만 "데스크톱 알림(macOS osascript / 리눅스 notify-send)"로.

- [ ] **Step 4: 통과 확인** — `go test ./internal/notify/ ./...` → PASS
- [ ] **Step 5: 커밋** — `git commit -m "feat(notify): 데스크톱 알림 OS 분기 — 리눅스 notify-send, 없으면 생략"`

---

### Task 5: 도움말·init 문구 OS 분기

**Files:**
- Modify: `internal/cli/helpcmd.go:5-6`, `main.go:1`(주석)
- Test: `internal/cli/helpcmd_test.go`

**Interfaces:**
- Produces: `TerminalLabel(goos string) string` — darwin "iTerm2+tmux", 그 외 "tmux". `HelpText()`는 `helpText(runtime.GOOS)`를 감싼다.

- [ ] **Step 1: 실패하는 테스트**

```go
func TestHelpTextTerminalLabelByOS(t *testing.T) {
	if !strings.HasPrefix(helpText("darwin"), "agentlayer — iTerm2+tmux") {
		t.Error("darwin 첫 줄은 iTerm2+tmux")
	}
	if !strings.HasPrefix(helpText("linux"), "agentlayer — tmux") {
		t.Error("linux 첫 줄은 tmux")
	}
	if strings.Contains(helpText("linux"), "iTerm2") {
		t.Error("linux 도움말에 iTerm2가 남으면 안 된다")
	}
}
```

- [ ] **Step 2: 실패 확인** → 컴파일 실패
- [ ] **Step 3: 구현** — `helpcmd.go`

```go
func TerminalLabel(goos string) string {
	if goos == "darwin" {
		return "iTerm2+tmux"
	}
	return "tmux"
}

func HelpText() string { return helpText(runtime.GOOS) }

func helpText(goos string) string {
	return "agentlayer — " + TerminalLabel(goos) + ` 멀티 에이전트 관제탑
…(기존 본문 그대로, 본문 안 iTerm2 언급이 있으면 "터미널"로)`
}
```

main.go의 iTerm2 감지는 `/Applications/iTerm.app` Stat이라 리눅스에서 이미 건너뛴다 — 변경 없음(주석에 그 사실을 한 줄 적는다).

- [ ] **Step 4: 통과 확인** — `go test ./internal/cli/` → PASS(기존 `TestHelpTextListsAllCommands` 포함)
- [ ] **Step 5: 커밋** — `git commit -m "feat(cli): 도움말 첫 줄 OS 분기(리눅스는 tmux)"`

---

### Task 6: 배포 — goreleaser linux 타깃 + install.sh + README

**Files:**
- Modify: `.goreleaser.yaml:5-10`
- Create: `install.sh`
- Modify: `README.md`(설치 절)
- Test: `goreleaser check`, `bash -n install.sh`, `AGENTLAYER_DRY_RUN=1 bash install.sh`

- [ ] **Step 1: goreleaser** — builds의 `goos: [darwin]`을 `goos: [darwin, linux]`로. cask는 darwin 자산만 쓰므로 그대로. `goreleaser check` → validated.

- [ ] **Step 2: install.sh 작성**

```bash
#!/usr/bin/env bash
# agentlayer 설치 — 최신(또는 AGENTLAYER_VERSION) 릴리즈 tar.gz를 받아 ~/.local/bin/agentlayer 에 놓는다.
# 리눅스/WSL2 기본 설치 경로. 맥에서도 brew 없이 쓸 수 있다.
#   curl -fsSL https://raw.githubusercontent.com/netwaif/agentlayer/main/install.sh | bash
set -euo pipefail
REPO="netwaif/agentlayer"
BIN_DIR="${AGENTLAYER_BIN_DIR:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')   # darwin | linux
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "지원하지 않는 아키텍처: $arch" >&2; exit 1 ;;
esac
case "$os" in darwin|linux) ;; *) echo "지원하지 않는 OS: $os (WSL2 안의 리눅스에서 실행하세요)" >&2; exit 1 ;; esac

ver="${AGENTLAYER_VERSION:-}"
if [ -z "$ver" ]; then
  # API 대신 releases/latest 리다이렉트로 태그를 알아낸다(무인증 API 제한 회피)
  ver=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" | sed -E 's#.*/tag/##')
fi
ver="${ver#v}"
url="https://github.com/$REPO/releases/download/v$ver/agentlayer_${ver}_${os}_${arch}.tar.gz"
echo "agentlayer v$ver ($os/$arch) → $BIN_DIR/agentlayer"
if [ "${AGENTLAYER_DRY_RUN:-}" = "1" ]; then echo "dry-run: $url"; exit 0; fi

tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$url" -o "$tmp/a.tgz"
tar -xzf "$tmp/a.tgz" -C "$tmp" agentlayer
mkdir -p "$BIN_DIR"
install -m 0755 "$tmp/agentlayer" "$BIN_DIR/agentlayer"
echo "설치 완료: $("$BIN_DIR/agentlayer" version 2>/dev/null | head -1 || echo "$BIN_DIR/agentlayer")"
case ":$PATH:" in *":$BIN_DIR:"*) ;; *) echo "PATH에 $BIN_DIR 을 추가하세요: export PATH=\"$BIN_DIR:\$PATH\"" ;; esac
echo "다음: agentlayer init"
```

- [ ] **Step 3: 검증** — `bash -n install.sh && AGENTLAYER_DRY_RUN=1 bash install.sh` → "agentlayer v1.3.1 (darwin/arm64) …" 와 dry-run URL 출력. `AGENTLAYER_VERSION=v1.3.1 AGENTLAYER_DRY_RUN=1 bash install.sh`도 같은 URL. `chmod +x install.sh`.

- [ ] **Step 4: README** — 설치 절에 리눅스/WSL2 줄 추가:

```
# 리눅스 / Windows WSL2 (brew 없이) — 맥도 가능
curl -fsSL https://raw.githubusercontent.com/netwaif/agentlayer/main/install.sh | bash
agentlayer init
```
그리고 에이전트 브라우저 절에 한 줄: "리눅스(WSL2)에서는 Chrome for Testing linux64를 받는다(창은 WSLg로 뜬다). `cookies import`는 macOS 전용(실사용 Chrome이 윈도우 쪽) — 에이전트 브라우저 창에서 직접 로그인. 데스크톱 알림은 `notify-send`가 있을 때만."

- [ ] **Step 5: 커밋** — `git commit -m "feat(release): 리눅스 amd64·arm64 빌드 + install.sh 한 줄 설치, README 리눅스/WSL2 절"`

---

### Task 7: VM 검증(수동, 릴리즈 전)

**Files:** 없음(기록은 `docs/linux-wsl2-verification.md`에 추가)

- [ ] **Step 1: 크로스빌드·전송** — `GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.version=dev-linux" -o /tmp/agentlayer-linux . && scp /tmp/agentlayer-linux ubuntu-agent:~/.local/bin/agentlayer`
- [ ] **Step 2: init·status** — `ssh ubuntu-agent 'agentlayer version; agentlayer init; agentlayer status'` → 배선(claude·codex·gemini) 설치 로그, iTerm2 절 없음, help 첫 줄 "tmux".
- [ ] **Step 3: hook 전이** — VM 안 tmux 세션에서 `claude -p "echo ok 해줘"`가 아니라 대화형 세션이 필요하므로 사용자 pane에서 `tmux new -s t1` → `claude` 실행 → 맥에서 `ssh ubuntu-agent 'agentlayer status'`에 그 세션이 잡히는지.
- [ ] **Step 4: 브라우저(무화면 한계)** — `ssh ubuntu-agent 'agentlayer browser'` → 엔진 다운로드·풀기 성공 로그, 그 다음 기동 실패 메시지(디스플레이 없음)가 명확한지. `agentlayer browser cookies import x.com` → macOS 전용 에러 문구.
- [ ] **Step 5: 기록** — 결과를 `docs/linux-wsl2-verification.md` "5차: agentlayer 리눅스 빌드"로 추가, 커밋.
