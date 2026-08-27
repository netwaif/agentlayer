package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSnapshotsLatestWins(t *testing.T) {
	dir := t.TempDir()
	// 같은 폴더의 두 스냅샷 — ts 큰 쪽이 승자
	writeFile(t, filepath.Join(dir, "a.json"),
		`{"cwd":"/Users/x/proj","project_dir":"/Users/x/proj","model":"Opus 5","used":19,"ts":1787581000}`)
	writeFile(t, filepath.Join(dir, "b.json"),
		`{"cwd":"/Users/x/proj/tasks/t1","project_dir":"/Users/x/proj","model":"Fable 5","used":42,"ts":1787582000}`)
	// project_dir 없는 옛 스냅샷은 cwd로 폴백
	writeFile(t, filepath.Join(dir, "c.json"),
		`{"cwd":"/Users/x/other","model":"Sonnet 5","used":7,"ts":1787581500}`)
	// 깨진 파일은 무시
	writeFile(t, filepath.Join(dir, "broken.json"), `{잘림`)

	m := LoadSnapshots(dir)
	p, ok := m["/Users/x/proj"]
	if !ok {
		t.Fatalf("project_dir 키 있어야 함: %v", m)
	}
	if p.Model != "Fable 5" || p.UsedPct == nil || *p.UsedPct != 42 {
		t.Errorf("최신 승자: %+v", p)
	}
	if !p.TS.Equal(time.Unix(1787582000, 0)) {
		t.Errorf("ts: %v", p.TS)
	}
	if o, ok := m["/Users/x/other"]; !ok || o.Model != "Sonnet 5" {
		t.Errorf("cwd 폴백: %+v", m)
	}
}

func TestLoadSnapshotsMissingDir(t *testing.T) {
	if m := LoadSnapshots("/없는/경로"); len(m) != 0 {
		t.Errorf("없는 디렉터리는 빈 맵: %v", m)
	}
}

const codexHead = `{"timestamp":"2026-08-25T01:00:00Z","type":"session_meta","payload":{"id":"x","cwd":"/Users/x/codexproj","originator":"codex_cli"}}
{"timestamp":"2026-08-25T01:00:01Z","type":"turn_context","payload":{"model":"gpt-5.6-sol"}}
{"timestamp":"2026-08-25T01:00:02Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":50000},"last_token_usage":{"total_tokens":30400},"model_context_window":272000}}}
`

func TestCodexLatest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "2026", "08", "25", "s1.jsonl"), codexHead)
	info := CodexLatest(root, "/Users/x/codexproj")
	if info.Model != "gpt-5.6-sol" {
		t.Errorf("model: %+v", info)
	}
	// (30400-12000)/(272000-12000)*100 ≈ 7.08
	if info.UsedPct == nil || *info.UsedPct < 7.0 || *info.UsedPct > 7.2 {
		t.Errorf("used%%: %+v", info.UsedPct)
	}
}

func TestAgentCtxNoCrossKindShadow(t *testing.T) {
	// 같은 폴더의 claude 스냅샷이 gemini 행에 오귀속되면 안 된다
	agents := []*state.Agent{
		{ID: "claude-1", Kind: "claude", CWD: "/w"},
		{ID: "gemini-2", Kind: "gemini", CWD: "/w", Model: "gemini-3.6-flash",
			UpdatedAt: time.Now()},
	}
	snaps := map[string]CtxInfo{"/w": {Model: "Opus 5 (1M context)"}}
	out := AgentCtx(agents, snaps, t.TempDir(), t.TempDir())
	if out["claude-1"].Model != "Opus 5 (1M context)" {
		t.Errorf("claude는 스냅샷 모델: %+v", out["claude-1"])
	}
	if out["gemini-2"].Model != "gemini-3.6-flash" {
		t.Errorf("gemini는 자기 모델: %+v", out["gemini-2"])
	}
}

// 스냅샷 파일명이 곧 Claude session_id — 파일명 키로도 찾을 수 있어야
// 같은 폴더에 세션이 여럿일 때 정확히 귀속된다.
func TestLoadSnapshotsSessionIDKey(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sid-old.json"),
		`{"cwd":"/w","project_dir":"/w","model":"Opus 5","used":3,"ts":100}`)
	writeFile(t, filepath.Join(dir, "sid-new.json"),
		`{"cwd":"/w","project_dir":"/w","model":"Fable 5","used":20,"ts":200}`)
	m := LoadSnapshots(dir)
	if m["sid-old"].Model != "Opus 5" || m["sid-new"].Model != "Fable 5" {
		t.Errorf("session_id 키: %+v", m)
	}
	if m["/w"].Model != "Fable 5" {
		t.Errorf("폴더 키는 최신 승자 유지: %+v", m["/w"])
	}
}

// 같은 폴더의 claude 두 세션은 각자 자기 session_id 스냅샷을 가진다
// (폴더 키만 쓰면 최신 세션 정보가 옛 세션 행에 오귀속 — restore-lab 실사례).
func TestAgentCtxSessionIDBeatsFolder(t *testing.T) {
	agents := []*state.Agent{
		{ID: "claude-1", Kind: "claude", CWD: "/w", SessionID: "sid-old"},
		{ID: "claude-2", Kind: "claude", CWD: "/w", SessionID: "sid-new"},
		{ID: "claude-3", Kind: "claude", CWD: "/w", SessionID: "sid-unknown"},
	}
	snaps := map[string]CtxInfo{
		"/w":      {Model: "Fable 5"},
		"sid-old": {Model: "Opus 5"},
		"sid-new": {Model: "Fable 5"},
	}
	out := AgentCtx(agents, snaps, t.TempDir(), t.TempDir())
	if out["claude-1"].Model != "Opus 5" {
		t.Errorf("옛 세션은 자기 스냅샷: %+v", out["claude-1"])
	}
	if out["claude-2"].Model != "Fable 5" {
		t.Errorf("새 세션도 자기 스냅샷: %+v", out["claude-2"])
	}
	if out["claude-3"].Model != "Fable 5" {
		t.Errorf("스냅샷 없는 세션은 폴더 키 폴백: %+v", out["claude-3"])
	}
}

func TestCodexSessionID(t *testing.T) {
	root := t.TempDir()
	head := `{"timestamp":"2026-08-26T00:00:00Z","type":"session_meta","payload":{"session_id":"01a03b74-0823-7450","id":"01a03b74-0823-7450","cwd":"/Users/x/codexproj"}}
`
	writeFile(t, filepath.Join(root, "2026", "08", "26", "s1.jsonl"), head)
	if got := CodexSessionID(root, "/Users/x/codexproj"); got != "01a03b74-0823-7450" {
		t.Errorf("session_id 추출: %q", got)
	}
	if got := CodexSessionID(root, "/다른/폴더"); got != "" {
		t.Errorf("cwd 불일치는 빈 값: %q", got)
	}
}

func TestCodexLatestNoMatch(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "2026", "08", "25", "s1.jsonl"), codexHead)
	info := CodexLatest(root, "/Users/x/다른폴더")
	if info.Model != "" || info.UsedPct != nil {
		t.Errorf("cwd 불일치는 빈 값: %+v", info)
	}
}
