package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const codexConfigFixture = `model = "gpt-5.6"
approval_policy = "on-request"

[mcp_servers.context7]
command = "npx"
`

func TestInstallCodexNotifyInsertsBeforeSection(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte(codexConfigFixture), 0o600)
	var buf bytes.Buffer
	if err := InstallCodexNotify(&buf, p, "/abs/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	content := string(b)
	notifyIdx := strings.Index(content, "notify =")
	sectionIdx := strings.Index(content, "[mcp_servers")
	if notifyIdx < 0 || sectionIdx < 0 || notifyIdx > sectionIdx {
		t.Errorf("notify는 섹션 앞 최상위에:\n%s", content)
	}
	if !strings.Contains(content, `"/abs/agentlayer", "hook", "codex"`) {
		t.Errorf("명령 배열:\n%s", content)
	}
	if !strings.Contains(content, `model = "gpt-5.6"`) {
		t.Error("기존 설정 보존")
	}
	if _, err := os.Stat(p + ".agentlayer.bak"); err != nil {
		t.Error("백업 생성")
	}
}

func TestInstallCodexNotifyExistingSkipped(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	orig := "notify = [\"my-notifier\"]\n" + codexConfigFixture
	os.WriteFile(p, []byte(orig), 0o600)
	var buf bytes.Buffer
	if err := InstallCodexNotify(&buf, p, "/abs/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != orig {
		t.Error("기존 notify가 있으면 무변경 (사용자 설정 우선)")
	}
	if !strings.Contains(buf.String(), "건너뜀") {
		t.Error("건너뜀 안내")
	}
}

func TestInstallCodexNotifyDryRun(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte(codexConfigFixture), 0o600)
	var buf bytes.Buffer
	if err := InstallCodexNotify(&buf, p, "/abs/agentlayer", true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != codexConfigFixture {
		t.Error("dry-run 무변경")
	}
}

func TestInstallCodexNotifyNoFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	var buf bytes.Buffer
	if err := InstallCodexNotify(&buf, p, "/abs/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "notify =") {
		t.Error("파일 없으면 새로 생성")
	}
}

func TestInstallCodexNotifyNoSections(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("model = \"x\"\n"), 0o600)
	var buf bytes.Buffer
	if err := InstallCodexNotify(&buf, p, "/abs/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "notify =") || !strings.Contains(string(b), "model") {
		t.Errorf("섹션 없는 파일 끝에 추가:\n%s", b)
	}
}

func TestInstallCodexHooksMergesAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.json")
	other := `{"hooks":{"PostToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"python -m py_compile \"$FILEPATH\""}]}],"Stop":[{"hooks":[{"type":"command","command":"/old/agentlayer hook codex --event stop","timeout":10}]}]}}`
	if err := os.WriteFile(path, []byte(other), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := InstallCodexHooks(&buf, path, "/bin/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	var root struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	// 남의 py_compile 그룹 유지 + 우리 그룹 추가
	if got := root.Hooks["PostToolUse"]; len(got) != 2 || got[0].Matcher != "Edit" || got[1].Hooks[0].Command != "/bin/agentlayer hook codex --event post-tool-use" {
		t.Errorf("PostToolUse 병합: %+v", got)
	}
	// 옛 경로의 우리 Stop 그룹은 교체(중복 없음)
	if got := root.Hooks["Stop"]; len(got) != 1 || got[0].Hooks[0].Command != "/bin/agentlayer hook codex --event stop" {
		t.Errorf("Stop 교체: %+v", got)
	}
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PermissionRequest"} {
		if len(root.Hooks[ev]) != 1 {
			t.Errorf("%s 등록 안 됨", ev)
		}
	}
	if !strings.Contains(buf.String(), "/hooks") {
		t.Error("신뢰 확인 안내가 있어야 함")
	}
	if _, err := os.Stat(path + ".agentlayer.bak"); err != nil {
		t.Error("백업 없음")
	}
	buf.Reset()
	if err := InstallCodexHooks(&buf, path, "/bin/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "이미 등록됨") {
		t.Errorf("멱등이어야 함: %s", buf.String())
	}
}

func TestInstallCodexHooksNoFileDryRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	var buf bytes.Buffer
	if err := InstallCodexHooks(&buf, path, "/bin/agentlayer", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("dry-run은 파일을 만들면 안 됨")
	}
}

func TestInstallCodexAgentsAppendReplaceIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(path, []byte("# mine\n- rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := InstallCodexAgents(&buf, path, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	s := string(raw)
	if !strings.HasPrefix(s, "# mine\n- rule\n\n"+codexAgentsStart) || !strings.HasSuffix(s, codexAgentsEnd+"\n") {
		t.Errorf("덧붙이기: %q", s)
	}
	if !strings.Contains(s, "chrome-devtools") || !strings.Contains(s, "인앱 브라우저") {
		t.Error("지침 내용 누락")
	}
	buf.Reset()
	if err := InstallCodexAgents(&buf, path, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "이미 설치됨") {
		t.Errorf("멱등: %s", buf.String())
	}
	// 옛 블록 → 교체, 뒤 내용 보존
	old := "# mine\n" + codexAgentsStart + "\nold\n" + codexAgentsEnd + "\n# after\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InstallCodexAgents(&buf, path, false); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	if s := string(raw); strings.Contains(s, "\nold\n") || !strings.HasSuffix(s, "# after\n") || strings.Count(s, codexAgentsStart) != 1 {
		t.Errorf("교체: %q", s)
	}
}
