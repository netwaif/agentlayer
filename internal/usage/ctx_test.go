package usage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestGeminiCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := GeminiCommand(); got != "gemini" {
		t.Fatalf("agy 흔적 없음 = stock 폴백이어야: got %q", got)
	}
	if err := os.MkdirAll(filepath.Join(home, ".gemini", "antigravity-cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := GeminiCommand(); got != "agy" {
		t.Fatalf("antigravity-cli 흔적 있으면 agy여야: got %q", got)
	}
}

func writeRollout(t *testing.T, root, name, sid, cwd, created string, mod time.Time) {
	t.Helper()
	dir := filepath.Join(root, "2026", "09", "29")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	line := `{"timestamp":"` + created + `","ordinal":0,"type":"session_meta","payload":{"session_id":"` + sid + `","id":"` + sid + `","timestamp":"` + created + `","cwd":"` + cwd + `"}}` + "\n"
	if err := os.WriteFile(p, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(p, mod, mod)
}

// 지금 떠 있는 프로세스보다 나중에 만들어진 세션만 돌려준다 — 옛 세션에 메시지를 넣지 않는다.
func TestCodexSessionSince(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	writeRollout(t, root, "rollout-old.jsonl", "sid-old", "/w/a", "2026-09-29T13:00:00.000Z", base.Add(-time.Hour))
	writeRollout(t, root, "rollout-new.jsonl", "sid-new", "/w/a", "2026-09-29T14:05:00.000Z", base.Add(5*time.Minute))
	writeRollout(t, root, "rollout-b.jsonl", "sid-b", "/w/b", "2026-09-29T13:00:00.000Z", base.Add(10*time.Minute))
	if got := CodexSessionSince(root, "/w/a", base); got != "sid-new" {
		t.Errorf("프로세스 기동 뒤 세션: %q", got)
	}
	if got := CodexSessionSince(root, "/w/a", base.Add(10*time.Minute)); got != "" {
		t.Errorf("가장 최근 세션이 기동보다 오래됐으면 빈 값: %q", got)
	}
	if got := CodexSessionSince(root, "/w/b", base); got != "" {
		t.Errorf("옛 세션만 있으면 빈 값: %q", got)
	}
	if got := CodexSessionSince(root, "/w/none", base); got != "" {
		t.Errorf("기록 없음: %q", got)
	}
}

// 기록 없는 코덱스 세션(데스크톱 앱)은 rollout에서 세션 ID 접두로 전체 ID·작업 폴더를 찾는다.
func TestCodexSessionByPrefix(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	writeRollout(t, root, "rollout-a.jsonl", "0199a1b2-1111-4c1e-9f1a-000000000001", "/w/a", "2026-09-29T13:00:00.000Z", base)
	writeRollout(t, root, "rollout-b.jsonl", "0199a1b2-2222-4c1e-9f1a-000000000002", "/w/b", "2026-09-29T13:10:00.000Z", base.Add(time.Minute))
	writeRollout(t, root, "rollout-c.jsonl", "ffff0000-3333-4c1e-9f1a-000000000003", "/w/c", "2026-09-29T13:20:00.000Z", base.Add(2*time.Minute))
	if id, cwd, err := CodexSessionByPrefix(root, "ffff0000"); err != nil || id != "ffff0000-3333-4c1e-9f1a-000000000003" || cwd != "/w/c" {
		t.Errorf("접두 하나: %q %q %v", id, cwd, err)
	}
	if id, _, err := CodexSessionByPrefix(root, "0199a1b2-2222"); err != nil || id != "0199a1b2-2222-4c1e-9f1a-000000000002" {
		t.Errorf("긴 접두: %q %v", id, err)
	}
	if _, _, err := CodexSessionByPrefix(root, "0199a1b2"); err == nil || !strings.Contains(err.Error(), "둘 이상") || !strings.Contains(err.Error(), "0199a1b2-1111") {
		t.Errorf("모호하면 후보를 담은 오류: %v", err)
	}
	if _, _, err := CodexSessionByPrefix(root, "deadbeef"); !errors.Is(err, ErrCodexSessionNotFound) {
		t.Errorf("없으면 ErrCodexSessionNotFound: %v", err)
	}
	if _, _, err := CodexSessionByPrefix(root, ""); !errors.Is(err, ErrCodexSessionNotFound) {
		t.Errorf("빈 접두: %v", err)
	}
}
