package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const codexNotifyBare = `notify = ["agentlayer", "hook", "codex"]`

func codexNotifyLine(binPath string) string {
	if binPath == "" {
		binPath = "agentlayer"
	}
	return `notify = ["` + binPath + `", "hook", "codex"]`
}

// InstallCodexNotify는 ~/.codex/config.toml에 notify 설정을 추가한다.
// TOML 최상위 키는 첫 섹션 헤더([...]) 앞에 있어야 하므로 그 위치에
// 삽입한다. 남의 notify는 건드리지 않지만, 이전 버전이 넣은 이름-only
// agentlayer 항목은 절대 경로로 마이그레이션한다(PATH 최소 환경 대응).
func InstallCodexNotify(w io.Writer, configPath, binPath string, dryRun bool) error {
	line := codexNotifyLine(binPath)
	raw, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		if dryRun {
			fmt.Fprintf(w, "  %s 생성 예정: %s\n", configPath, line)
			return nil
		}
		return os.WriteFile(configPath, []byte(line+"\n"), 0o600)
	}
	if err != nil {
		return err
	}
	content := string(raw)
	for _, l := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "[") {
			break // 최상위 영역 끝 — notify 없음 확정
		}
		if !strings.HasPrefix(trimmed, "notify") {
			continue
		}
		if trimmed == codexNotifyBare {
			// 이름-only 옛 항목 → 절대 경로 교체
			if trimmed == line {
				fmt.Fprintln(w, "  codex notify: 이미 등록됨 — 건너뜀")
				return nil
			}
			fmt.Fprintln(w, "  codex notify: 절대 경로로 마이그레이션")
			if dryRun {
				fmt.Fprintln(w, "  (dry-run — 파일을 변경하지 않았습니다)")
				return nil
			}
			if err := os.WriteFile(configPath+".agentlayer.bak", raw, 0o600); err != nil {
				return fmt.Errorf("백업 실패 — 설치 중단: %w", err)
			}
			updated := strings.Replace(content, codexNotifyBare, line, 1)
			tmp := configPath + ".agentlayer.tmp"
			if err := os.WriteFile(tmp, []byte(updated), 0o600); err != nil {
				return err
			}
			return os.Rename(tmp, configPath)
		}
		if trimmed == line {
			fmt.Fprintln(w, "  codex notify: 이미 등록됨 — 건너뜀")
			return nil
		}
		fmt.Fprintln(w, "  codex notify: 이미 설정됨 — 건너뜀 (기존 설정 우선)")
		return nil
	}
	fmt.Fprintf(w, "  codex notify: %s\n", line)
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
	updated := content[:idx] + line + "\n" + content[idx:]
	if idx == len(content) && !strings.HasSuffix(content, "\n") {
		updated = content + "\n" + line + "\n"
	}
	tmp := configPath + ".agentlayer.tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, configPath)
}
