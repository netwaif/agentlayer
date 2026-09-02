// chrome-devtools MCP 등록: 세 CLI 설정에 `agentlayer browser mcp-serve`를
// 서버로 심는다. 에이전트가 MCP 도구를 부르면 mcp-serve가 Chrome을 띄우고
// chrome-devtools-mcp로 넘기므로, 사용자는 브라우저 기동 명령을 칠 일이 없다.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const mcpServerName = "chrome-devtools"

func mcpServeArgs() []any { return []any{"browser", "mcp-serve"} }

// MCPServeArgv는 mcp-serve가 exec할 chrome-devtools-mcp 명령줄.
func MCPServeArgv(npx string, port int) []string {
	return []string{npx, "chrome-devtools-mcp@latest", fmt.Sprintf("--browserUrl=http://127.0.0.1:%d", port)}
}

// InstallClaudeMCP는 ~/.claude.json(user scope)의 mcpServers에 등록한다.
func InstallClaudeMCP(w io.Writer, path, binPath string, dryRun bool) error {
	return installJSONMCP(w, path, "claude", map[string]any{
		"type": "stdio", "command": binPath, "args": mcpServeArgs(),
	}, dryRun)
}

// InstallGeminiMCP는 ~/.gemini/settings.json의 mcpServers에 등록한다.
func InstallGeminiMCP(w io.Writer, path, binPath string, dryRun bool) error {
	return installJSONMCP(w, path, "gemini", map[string]any{
		"command": binPath, "args": mcpServeArgs(),
	}, dryRun)
}

// installJSONMCP는 JSON 설정의 mcpServers[chrome-devtools]를 심는다.
// ~/.claude.json은 크고 큰 정수(타임스탬프)를 품으므로 UseNumber로 읽어
// 재인코딩 손상을 막는다. 같은 이름의 남의 항목은 덮지 않는다(기존 설정 우선).
func installJSONMCP(w io.Writer, path, label string, entry map[string]any, dryRun bool) error {
	settings := map[string]any{}
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&settings); err != nil {
			return fmt.Errorf("%s 파싱 실패 — 수동 확인 필요: %w", path, err)
		}
	case os.IsNotExist(err):
	default:
		return err
	}
	servers, _ := settings["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	if cur, ok := servers[mcpServerName]; ok {
		if sameMCPEntry(cur, entry) {
			fmt.Fprintf(w, "  %s MCP %s: 이미 등록됨 — 건너뜀\n", label, mcpServerName)
		} else {
			fmt.Fprintf(w, "  %s MCP %s: 이미 설정됨 — 건너뜀 (기존 설정 우선)\n", label, mcpServerName)
		}
		return nil
	}
	fmt.Fprintf(w, "  %s MCP %s: %s browser mcp-serve\n", label, mcpServerName, entry["command"])
	if dryRun {
		fmt.Fprintln(w, "  (dry-run — 파일을 변경하지 않았습니다)")
		return nil
	}
	servers[mcpServerName] = entry
	settings["mcpServers"] = servers
	if raw != nil {
		if err := os.WriteFile(path+".agentlayer.bak", raw, 0o600); err != nil {
			return fmt.Errorf("백업 실패 — 설치 중단: %w", err)
		}
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".agentlayer.tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func sameMCPEntry(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// InstallCodexMCP는 ~/.codex/config.toml에 [mcp_servers.chrome-devtools] 섹션을
// 덧붙인다. 같은 헤더가 이미 있으면 내용과 무관하게 건너뛴다(기존 설정 우선).
func InstallCodexMCP(w io.Writer, configPath, binPath string, dryRun bool) error {
	header := "[mcp_servers." + mcpServerName + "]"
	section := fmt.Sprintf("%s\ncommand = %q\nargs = [\"browser\", \"mcp-serve\"]\n", header, binPath)
	raw, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(raw)
	for _, l := range strings.Split(content, "\n") {
		if strings.TrimSpace(l) == header {
			fmt.Fprintf(w, "  codex MCP %s: 이미 설정됨 — 건너뜀 (기존 설정 우선)\n", mcpServerName)
			return nil
		}
	}
	fmt.Fprintf(w, "  codex MCP %s: %s browser mcp-serve\n", mcpServerName, binPath)
	if dryRun {
		fmt.Fprintln(w, "  (dry-run — 파일을 변경하지 않았습니다)")
		return nil
	}
	if raw != nil {
		if err := os.WriteFile(configPath+".agentlayer.bak", raw, 0o600); err != nil {
			return fmt.Errorf("백업 실패 — 설치 중단: %w", err)
		}
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if content != "" {
		content += "\n"
	}
	content += section
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	tmp := configPath + ".agentlayer.tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, configPath)
}
