package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallFxWritesExtension(t *testing.T) {
	dir, err := InstallFx(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Version int `json:"manifest_version"`
		Scripts []struct {
			JS    []string `json:"js"`
			RunAt string   `json:"run_at"`
		} `json:"content_scripts"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("manifest.json 파싱: %v", err)
	}
	if m.Version != 3 || len(m.Scripts) != 1 || m.Scripts[0].RunAt != "document_start" {
		t.Fatalf("manifest 내용 이상: %+v", m)
	}
	js, err := os.ReadFile(filepath.Join(dir, m.Scripts[0].JS[0]))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), fxAttr) {
		t.Fatalf("content.js가 신호 속성 %q를 안 본다", fxAttr)
	}
	// 멱등 — 두 번 불러도 같은 경로, 에러 없음
	if again, err := InstallFx(filepath.Dir(dir)); err != nil || again != dir {
		t.Fatalf("재설치: %v %q", err, again)
	}
}

func TestLaunchArgsLoadsFxExtension(t *testing.T) {
	args := strings.Join(launchArgs("/bin/chrome", "/p", 9222, "/fx"), "\n")
	if !strings.Contains(args, "--load-extension=/fx") {
		t.Fatalf("확장 로드 플래그 없음:\n%s", args)
	}
	if strings.Contains(strings.Join(launchArgs("/bin/chrome", "/p", 9222, ""), "\n"), "load-extension") {
		t.Fatal("fxDir 비면 플래그도 없어야 한다")
	}
}

func TestFxTracker(t *testing.T) {
	var tr FxTracker
	call := func(id, name string) []byte {
		return []byte(`{"jsonrpc":"2.0","id":` + id + `,"method":"tools/call","params":{"name":"` + name + `","arguments":{}}}` + "\n")
	}
	resp := func(id string) []byte {
		return []byte(`{"jsonrpc":"2.0","id":` + id + `,"result":{"content":[]}}` + "\n")
	}
	if tool, ok := tr.Start(call("1", "take_snapshot")); !ok || tool != "take_snapshot" {
		t.Fatalf("읽기 도구도 추적(깜빡임 방지): %q %v", tool, ok)
	}
	if !tr.End(resp("1")) {
		t.Fatal("응답에 종료 신호")
	}
	if tr.End(resp("99")) {
		t.Fatal("추적 안 한 응답에 종료 신호")
	}
	if tool, ok := tr.Start(call("2", "click")); !ok || tool != "click" {
		t.Fatalf("click 시작 신호 기대: %q %v", tool, ok)
	}
	if _, ok := tr.Start(call(`"s3"`, "fill")); !ok {
		t.Fatal("문자열 id도 추적")
	}
	if tr.End(resp("2")) {
		t.Fatal("아직 fill이 진행 중 — 종료 신호는 마지막에만")
	}
	if !tr.End(resp(`"s3"`)) {
		t.Fatal("마지막 응답에 종료 신호")
	}
	if tr.End(resp(`"s3"`)) {
		t.Fatal("같은 응답 두 번은 무시")
	}
	if _, ok := tr.Start([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")); ok {
		t.Fatal("tools/call 아니면 무시")
	}
}

func TestFxValue(t *testing.T) {
	if v := fxValue("click", true); !strings.HasPrefix(v, "on:click:") {
		t.Fatalf("on 값: %q", v)
	}
	if v := fxValue("", false); !strings.HasPrefix(v, "off:") {
		t.Fatalf("off 값: %q", v)
	}
}
