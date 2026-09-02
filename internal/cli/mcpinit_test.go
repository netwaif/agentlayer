package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallClaudeMCPCreatesAndPreserves(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".claude.json")
	// 큰 정수·기존 서버가 있는 실제 모양 — 재인코딩으로 깨지면 안 된다
	os.WriteFile(p, []byte(`{"lastTs":1725000000123456,"mcpServers":{"other":{"command":"x"}},"z":1.5}`), 0o600)
	var out bytes.Buffer
	if err := InstallClaudeMCP(&out, p, "/opt/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	s := string(raw)
	if !strings.Contains(s, "1725000000123456") {
		t.Errorf("큰 정수 보존 실패:\n%s", s)
	}
	var got map[string]any
	json.Unmarshal(raw, &got)
	srv := got["mcpServers"].(map[string]any)
	if srv["other"] == nil {
		t.Error("기존 서버 유실")
	}
	cd := srv["chrome-devtools"].(map[string]any)
	if cd["command"] != "/opt/agentlayer" || cd["type"] != "stdio" {
		t.Errorf("chrome-devtools 항목: %+v", cd)
	}
	args, _ := json.Marshal(cd["args"])
	if string(args) != `["browser","mcp-serve"]` {
		t.Errorf("args: %s", args)
	}
	if _, err := os.Stat(p + ".agentlayer.bak"); err != nil {
		t.Error("백업 없음")
	}
	// 두 번째는 멱등
	out.Reset()
	if err := InstallClaudeMCP(&out, p, "/opt/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "이미 등록됨") {
		t.Errorf("멱등 안내 없음: %s", out.String())
	}
}

func TestInstallClaudeMCPKeepsForeignEntry(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".claude.json")
	os.WriteFile(p, []byte(`{"mcpServers":{"chrome-devtools":{"command":"npx","args":["chrome-devtools-mcp@latest"]}}}`), 0o600)
	var out bytes.Buffer
	if err := InstallClaudeMCP(&out, p, "/opt/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if !strings.Contains(string(raw), `"npx"`) || !strings.Contains(out.String(), "기존 설정 우선") {
		t.Errorf("남의 chrome-devtools를 덮어씀: %s / %s", raw, out.String())
	}
}

func TestInstallClaudeMCPMissingFileCreates(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".claude.json")
	var out bytes.Buffer
	if err := InstallClaudeMCP(&out, p, "/opt/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if !strings.Contains(string(raw), "chrome-devtools") {
		t.Errorf("생성 실패: %s", raw)
	}
}

func TestInstallCodexMCPAppendsSection(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("notify = [\"x\"]\n\n[mcp_servers.other]\ncommand = \"y\"\n"), 0o600)
	var out bytes.Buffer
	if err := InstallCodexMCP(&out, p, "/opt/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	s := string(raw)
	if !strings.Contains(s, "[mcp_servers.chrome-devtools]\ncommand = \"/opt/agentlayer\"\nargs = [\"browser\", \"mcp-serve\"]") {
		t.Errorf("섹션 없음:\n%s", s)
	}
	if !strings.Contains(s, "[mcp_servers.other]") || !strings.HasPrefix(s, "notify") {
		t.Errorf("기존 내용 훼손:\n%s", s)
	}
	out.Reset()
	if err := InstallCodexMCP(&out, p, "/opt/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "이미") || strings.Count(string(mustRead(t, p)), "chrome-devtools") != 1 {
		t.Errorf("멱등 실패: %s", out.String())
	}
}

func TestInstallCodexMCPDryRun(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("a = 1\n"), 0o600)
	var out bytes.Buffer
	if err := InstallCodexMCP(&out, p, "/opt/agentlayer", true); err != nil {
		t.Fatal(err)
	}
	if string(mustRead(t, p)) != "a = 1\n" {
		t.Error("dry-run이 파일을 바꿈")
	}
}

func TestInstallGeminiMCP(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(`{"hooks":{"x":1}}`), 0o600)
	var out bytes.Buffer
	if err := InstallGeminiMCP(&out, p, "/opt/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(mustRead(t, p), &got)
	if got["hooks"] == nil {
		t.Error("hooks 유실")
	}
	cd := got["mcpServers"].(map[string]any)["chrome-devtools"].(map[string]any)
	if cd["command"] != "/opt/agentlayer" {
		t.Errorf("항목: %+v", cd)
	}
}

func TestMCPServeArgv(t *testing.T) {
	argv := MCPServeArgv("/usr/local/bin/npx", 9333)
	want := []string{"/usr/local/bin/npx", "chrome-devtools-mcp@latest", "--browserUrl=http://127.0.0.1:9333"}
	if strings.Join(argv, " ") != strings.Join(want, " ") {
		t.Errorf("argv = %v", argv)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
