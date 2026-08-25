package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestCodexLatestNoMatch(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "2026", "08", "25", "s1.jsonl"), codexHead)
	info := CodexLatest(root, "/Users/x/다른폴더")
	if info.Model != "" || info.UsedPct != nil {
		t.Errorf("cwd 불일치는 빈 값: %+v", info)
	}
}
