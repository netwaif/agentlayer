package browser

// 에이전트 브라우저 엔진 = Chrome for Testing (구글이 자동화용으로 배포하는 정식 Chrome).
//
// 시스템 Chrome을 전용 프로필로 띄우면 macOS LaunchServices가 그 프로세스를
// "Google Chrome"으로 등록해, 사용자가 Dock·Spotlight에서 Chrome을 열어도 새 프로세스가
// 뜨지 않고 에이전트 브라우저에 창만 추가된다(2026-09-04 실측). 번들 ID가 다른
// Chrome for Testing(com.google.chrome.for.testing)은 별개 앱이라 섞이지 않는다.
// 래퍼 앱(스크립트·심볼릭 링크·실행 파일 복사+재서명)은 전부 Chrome 샌드박스·서명에 막혔다.
//
// 첫 기동 때 한 번 내려받아 stateDir/chrome-for-testing/에 둔다(약 200MB). 코덱은
// 실Chrome과 같고 Widevine DRM만 없다. 내려받기에 실패하면 시스템 Chrome으로 대체한다.

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// cftVersionsURL은 구글의 "마지막 정상 버전" 목록. Stable 채널의 mac zip을 고른다.
var cftVersionsURL = "https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json"

// cftGet은 HTTP GET 주입점 (테스트 대체용).
var cftGet = http.Get

// cftPlatformFor는 OS·아키텍처에 맞는 CfT 플랫폼 이름. 리눅스 CfT는 x64 빌드 하나뿐이다
// (arm64 리눅스는 자동 설치 불가 → EnsureEngine이 시스템 Chrome 대체를 안내).
func cftPlatformFor(goos, arch string) string {
	if goos == "linux" {
		return "linux64"
	}
	if arch == "arm64" {
		return "mac-arm64"
	}
	return "mac-x64"
}

// engineRel은 엔진 폴더 기준 실행 파일 상대 경로. 맥은 .app 번들 안, 리눅스는 평면 폴더.
func engineRel(goos, arch string) string {
	plat := cftPlatformFor(goos, arch)
	if goos == "linux" {
		return filepath.Join("chrome-"+plat, "chrome")
	}
	return filepath.Join("chrome-"+plat, "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing")
}

// EngineDir은 엔진이 풀리는 폴더.
func EngineDir(stateDir string) string { return filepath.Join(stateDir, "chrome-for-testing") }

// EngineBin은 이 머신 아키텍처의 Chrome for Testing 실행 파일 경로.
func EngineBin(stateDir string) string {
	return filepath.Join(EngineDir(stateDir), engineRel(runtime.GOOS, runtime.GOARCH))
}

// pickCFTDownload는 버전 JSON에서 Stable 채널의 (버전, 플랫폼 zip URL)을 고른다.
func pickCFTDownload(js []byte, platform string) (version, url string, err error) {
	var doc struct {
		Channels map[string]struct {
			Version   string `json:"version"`
			Downloads struct {
				Chrome []struct {
					Platform string `json:"platform"`
					URL      string `json:"url"`
				} `json:"chrome"`
			} `json:"downloads"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(js, &doc); err != nil {
		return "", "", fmt.Errorf("버전 목록 파싱 실패: %w", err)
	}
	st, ok := doc.Channels["Stable"]
	if !ok || st.Version == "" {
		return "", "", fmt.Errorf("버전 목록에 Stable 채널이 없습니다")
	}
	for _, d := range st.Downloads.Chrome {
		if d.Platform == platform {
			return st.Version, d.URL, nil
		}
	}
	return "", "", fmt.Errorf("버전 목록에 %s 빌드가 없습니다", platform)
}

// EnsureEngine은 엔진 실행 파일 경로를 돌려준다. 없으면 내려받아 푼다(진행 상황은 log로).
func EnsureEngine(stateDir string, log io.Writer) (string, error) {
	bin := EngineBin(stateDir)
	if st, err := os.Stat(bin); err == nil && st.Mode()&0o111 != 0 {
		return bin, nil
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return "", fmt.Errorf("Chrome for Testing 자동 설치는 macOS·리눅스 전용")
	}
	if runtime.GOOS == "linux" && runtime.GOARCH != "amd64" {
		return "", fmt.Errorf("Chrome for Testing 리눅스 빌드는 x86_64뿐 — 시스템 Chrome/Chromium을 설치하면 그것을 씁니다")
	}
	platform := cftPlatformFor(runtime.GOOS, runtime.GOARCH)
	resp, err := cftGet(cftVersionsURL)
	if err != nil {
		return "", fmt.Errorf("버전 목록 요청 실패: %w", err)
	}
	js, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("버전 목록 읽기 실패: %w", err)
	}
	version, url, err := pickCFTDownload(js, platform)
	if err != nil {
		return "", err
	}

	dir := EngineDir(stateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	zipPath := filepath.Join(dir, "download.zip")
	defer os.Remove(zipPath)
	fmt.Fprintf(log, "에이전트 브라우저 엔진(Chrome for Testing %s, %s) 내려받는 중… 약 200MB, 한 번만\n", version, platform)
	if err := download(url, zipPath); err != nil {
		return "", fmt.Errorf("엔진 다운로드 실패: %w", err)
	}
	if runtime.GOOS == "darwin" {
		// 풀기는 ditto(macOS 내장) — 번들 안의 심볼릭 링크·실행 권한·xattr를 그대로 보존한다.
		if out, err := exec.Command("/usr/bin/ditto", "-x", "-k", zipPath, dir).CombinedOutput(); err != nil {
			return "", fmt.Errorf("엔진 풀기 실패: %v %s", err, strings.TrimSpace(string(out)))
		}
	} else if err := extractZip(zipPath, dir); err != nil {
		return "", fmt.Errorf("엔진 풀기 실패: %w", err)
	}
	if st, err := os.Stat(bin); err != nil || st.Mode()&0o111 == 0 {
		return "", fmt.Errorf("엔진을 풀었지만 실행 파일이 없습니다: %s", bin)
	}
	_ = os.WriteFile(filepath.Join(dir, "VERSION"), []byte(version+"\n"), 0o644)
	fmt.Fprintf(log, "에이전트 브라우저 엔진 준비 완료: %s\n", filepath.Dir(bin))
	return bin, nil
}

func download(url, dst string) error {
	resp, err := cftGet(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

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
