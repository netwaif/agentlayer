package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const codexNotifyLine = `notify = ["agentlayer", "hook", "codex"]`

// InstallCodexNotify는 ~/.codex/config.toml에 notify 설정을 추가한다.
// TOML 최상위 키는 첫 섹션 헤더([...]) 앞에 있어야 하므로 그 위치에
// 삽입한다. 이미 notify 키가 있으면 건드리지 않는다(사용자 설정 우선).
func InstallCodexNotify(w io.Writer, configPath string, dryRun bool) error {
	raw, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		if dryRun {
			fmt.Fprintf(w, "  %s 생성 예정: %s\n", configPath, codexNotifyLine)
			return nil
		}
		return os.WriteFile(configPath, []byte(codexNotifyLine+"\n"), 0o600)
	}
	if err != nil {
		return err
	}
	content := string(raw)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			break // 최상위 영역 끝 — notify 없음 확정
		}
		if strings.HasPrefix(trimmed, "notify") {
			fmt.Fprintln(w, "  codex notify: 이미 설정됨 — 건너뜀 (기존 설정 우선)")
			return nil
		}
	}
	fmt.Fprintf(w, "  codex notify: %s\n", codexNotifyLine)
	if dryRun {
		fmt.Fprintln(w, "  (dry-run — 파일을 변경하지 않았습니다)")
		return nil
	}
	if err := os.WriteFile(configPath+".agentlayer.bak", raw, 0o600); err != nil {
		return fmt.Errorf("백업 실패 — 설치 중단: %w", err)
	}
	// 첫 섹션 앞(최상위)에 삽입
	idx := len(content)
	for i, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			lines := strings.Split(content, "\n")
			idx = len(strings.Join(lines[:i], "\n"))
			break
		}
	}
	updated := content[:idx] + codexNotifyLine + "\n" + content[idx:]
	if idx == len(content) && !strings.HasSuffix(content, "\n") {
		updated = content + "\n" + codexNotifyLine + "\n"
	}
	tmp := configPath + ".agentlayer.tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, configPath)
}
