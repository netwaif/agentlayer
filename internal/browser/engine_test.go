package browser

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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
	_, _ = zw.CreateHeader(&zip.FileHeader{Name: "chrome-linux64/locales/", Method: zip.Store})
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

const sampleVersions = `{"channels":{"Stable":{"version":"152.0.7977.82","downloads":{"chrome":[
{"platform":"linux64","url":"https://x/linux64.zip"},
{"platform":"mac-arm64","url":"https://x/mac-arm64.zip"},
{"platform":"mac-x64","url":"https://x/mac-x64.zip"}]}}}}`

func TestPickCFTDownload(t *testing.T) {
	ver, url, err := pickCFTDownload([]byte(sampleVersions), "mac-arm64")
	if err != nil {
		t.Fatal(err)
	}
	if ver != "152.0.7977.82" || url != "https://x/mac-arm64.zip" {
		t.Errorf("got %s %s", ver, url)
	}
	if _, _, err := pickCFTDownload([]byte(sampleVersions), "win64"); err == nil {
		t.Error("모르는 플랫폼은 에러여야 한다")
	}
	if _, _, err := pickCFTDownload([]byte("{}"), "mac-x64"); err == nil {
		t.Error("Stable 채널 없으면 에러여야 한다")
	}
}

// 엔진이 이미 있으면 네트워크를 건드리지 않는다.
func TestEnsureEngine_Existing(t *testing.T) {
	dir := t.TempDir()
	bin := EngineBin(dir)
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := cftGet
	cftGet = func(string) (*http.Response, error) { t.Fatal("다운로드하면 안 된다"); return nil, nil }
	defer func() { cftGet = orig }()
	got, err := EnsureEngine(dir, io.Discard)
	if err != nil || got != bin {
		t.Fatalf("got %s, %v", got, err)
	}
}

// 없으면 버전 JSON → zip 다운로드 → 풀기 → 실행 파일 경로. VERSION 기록.
func TestEnsureEngine_Download(t *testing.T) {
	plat := cftPlatformFor(runtime.GOOS, runtime.GOARCH)
	rel := engineRel(runtime.GOOS, runtime.GOARCH)
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	h := &zip.FileHeader{Name: filepath.ToSlash(rel), Method: zip.Deflate}
	h.SetMode(0o755)
	w, _ := zw.CreateHeader(h)
	_, _ = w.Write([]byte("#!/bin/sh\necho cft\n"))
	_ = zw.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zip") {
			_, _ = rw.Write(zbuf.Bytes())
			return
		}
		js := strings.ReplaceAll(sampleVersions, "https://x/"+plat+".zip", "http://"+r.Host+"/cft.zip")
		_, _ = rw.Write([]byte(js))
	}))
	defer srv.Close()
	orig, origURL := cftGet, cftVersionsURL
	cftGet = http.Get
	cftVersionsURL = srv.URL + "/versions.json"
	defer func() { cftGet, cftVersionsURL = orig, origURL }()

	dir := t.TempDir()
	var log bytes.Buffer
	got, err := EnsureEngine(dir, &log)
	if err != nil {
		t.Fatal(err)
	}
	if got != EngineBin(dir) {
		t.Errorf("bin = %s", got)
	}
	st, err := os.Stat(got)
	if err != nil || st.Mode()&0o111 == 0 {
		t.Errorf("실행 파일이 없거나 실행 권한 없음: %v %v", st, err)
	}
	if v, _ := os.ReadFile(filepath.Join(EngineDir(dir), "VERSION")); strings.TrimSpace(string(v)) != "152.0.7977.82" {
		t.Errorf("VERSION = %q", v)
	}
	if _, err := os.Stat(filepath.Join(EngineDir(dir), "download.zip")); err == nil {
		t.Error("zip은 풀고 나서 지워야 한다")
	}
	if !strings.Contains(log.String(), "152.0.7977.82") {
		t.Errorf("진행 로그에 버전이 있어야 한다: %q", log.String())
	}
}

// 기동 플래그: 경고 띠를 띄우는 site-isolation 계열은 빼고, 사람이 같이 쓰는 창에 맞게.
func TestLaunchArgs(t *testing.T) {
	args := strings.Join(launchArgs("/bin/chrome", "/p", 9222, ""), "\n")
	for _, bad := range []string{"disable-site-isolation-trials", "site-per-process", "enable-automation", "no-startup-window"} {
		if strings.Contains(args, bad) {
			t.Errorf("플래그에 %s가 있으면 안 된다", bad)
		}
	}
	for _, need := range []string{"--user-data-dir=/p", "--remote-debugging-port=9222", "--test-type=gpu", "--disable-features=Translate,TranslateUI"} {
		if !strings.Contains(args, need) {
			t.Errorf("플래그에 %s가 있어야 한다", need)
		}
	}
}
