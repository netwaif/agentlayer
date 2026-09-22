package browser

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod"
)

func TestContentScriptWithNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node 없음")
	}
	cmd := exec.Command(node, "fx/content_test.mjs")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("content_test.mjs 실패: %v\n%s", err, out)
	}
}

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
	if _, last := tr.End(resp("1")); !last {
		t.Fatal("응답에 종료 신호")
	}
	if _, last := tr.End(resp("99")); last {
		t.Fatal("추적 안 한 응답에 종료 신호")
	}
	if tool, ok := tr.Start(call("2", "click")); !ok || tool != "click" {
		t.Fatalf("click 시작 신호 기대: %q %v", tool, ok)
	}
	if _, ok := tr.Start(call(`"s3"`, "fill")); !ok {
		t.Fatal("문자열 id도 추적")
	}
	if _, last := tr.End(resp("2")); last {
		t.Fatal("아직 fill이 진행 중 — 종료 신호는 마지막에만")
	}
	if _, last := tr.End(resp(`"s3"`)); !last {
		t.Fatal("마지막 응답에 종료 신호")
	}
	if _, last := tr.End(resp(`"s3"`)); last {
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

func TestFxTrackerKeepsToolName(t *testing.T) {
	var tr FxTracker
	if tool, ok := tr.Start([]byte(`{"id":1,"method":"tools/call","params":{"name":"wait_for"}}`)); !ok || tool != "wait_for" {
		t.Fatal("start")
	}
	if tool, ok := tr.Start([]byte(`{"id":2,"method":"tools/call","params":{"name":"click"}}`)); !ok || tool != "click" {
		t.Fatal("start2")
	}
	tool, last := tr.End([]byte(`{"id":2,"result":{}}`))
	if tool != "click" || last {
		t.Fatalf("End(2) = %q last=%v", tool, last)
	}
	tool, last = tr.End([]byte(`{"id":1,"result":{}}`))
	if tool != "wait_for" || !last {
		t.Fatalf("End(1) = %q last=%v", tool, last)
	}
	if tool, last := tr.End([]byte(`{"id":99,"result":{}}`)); tool != "" || last {
		t.Fatal("모르는 id")
	}
}

func TestIsInputTool(t *testing.T) {
	for _, n := range []string{"click", "hover", "drag", "fill", "fill_form", "type_text", "press_key", "upload_file"} {
		if !IsInputTool(n) {
			t.Errorf("%s는 입력 도구", n)
		}
	}
	for _, n := range []string{"take_snapshot", "wait_for", "navigate_page", "evaluate_script", ""} {
		if IsInputTool(n) {
			t.Errorf("%s는 입력 도구 아님", n)
		}
	}
}

func TestForEachPageRunsInParallelWithinBudget(t *testing.T) {
	// rod.Page 없이 병렬성만 본다 — fn이 각각 200ms 자면 순차면 600ms, 병렬이면 ~200ms
	pages := make([]*rod.Page, 3)
	start := time.Now()
	var n int32
	forEachPage(pages, 300*time.Millisecond, func(*rod.Page) {
		time.Sleep(200 * time.Millisecond)
		atomic.AddInt32(&n, 1)
	})
	if d := time.Since(start); d > 450*time.Millisecond {
		t.Fatalf("병렬이 아님: %v", d)
	}
	if atomic.LoadInt32(&n) != 3 {
		t.Fatalf("3개 다 실행돼야 함: %d", n)
	}
	// 마감을 넘기는 fn은 기다리지 않는다
	start = time.Now()
	forEachPage(pages, 100*time.Millisecond, func(*rod.Page) { time.Sleep(2 * time.Second) })
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Fatalf("마감을 안 지킴: %v", d)
	}
}

func TestLatestRequest(t *testing.T) {
	if _, ok := LatestRequest(nil); ok {
		t.Fatal("없으면 false")
	}
	ev, ok := LatestRequest([]TabRequest{{EvUserTake, 10}, {EvUserStop, 30}, {EvUserReturn, 20}})
	if !ok || ev != EvUserStop {
		t.Fatalf("최신은 stop: %v %v", ev, ok)
	}
}

// TestReqCollectorSnapshotIsRaceFree — SyncTabs가 쓰는 것과 같은 모양(forEachPage로
// 예산을 넘긴 고루틴을 버리고, 그 고루틴들이 반환 뒤에도 add를 계속 부름)을 재현해
// snapshot()이 돌려준 슬라이스가 뒤늦은 add에 물들지 않는지 -race로 검증한다.
func TestReqCollectorSnapshotIsRaceFree(t *testing.T) {
	var col reqCollector
	pages := make([]*rod.Page, 5)
	forEachPage(pages, 50*time.Millisecond, func(*rod.Page) {
		// 예산(50ms)보다 오래 걸려 forEachPage가 기다리지 않고 반환한 뒤에도 도착한다.
		time.Sleep(200 * time.Millisecond)
		col.add(TabRequest{Event: EvCall, At: 1})
	})
	out := col.snapshot()
	_ = out // 이 시점 스냅샷 — 아래 add들이 이 슬라이스의 배킹 배열을 건드리면 안 된다.
	time.Sleep(250 * time.Millisecond)
}
