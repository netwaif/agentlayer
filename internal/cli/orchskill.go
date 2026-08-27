package cli

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// orchestrationSkill은 바이너리에 동봉되는 /orchestration 스킬 본문.
// 스킬은 wt·status 등 이 바이너리의 명령을 그대로 지시하는 지침서라,
// 별도 채널(플러그인 등)로 배포하면 바이너리와 버전이 어긋난다 —
// 같이 싣고 init이 설치해 버전을 잠근다.
//
//go:embed orchestration_skill.md
var orchestrationSkill []byte

// InstallOrchestrationSkill은 skillsDir(보통 ~/.claude/skills) 아래에
// orchestration/SKILL.md를 설치한다. 내용이 같으면 건너뛰고(멱등),
// 다르면 기존 파일을 .bak으로 남기고 갱신한다 — 사용자가 손댔어도
// 유실되지 않는다.
func InstallOrchestrationSkill(w io.Writer, skillsDir string, dryRun bool) error {
	path := filepath.Join(skillsDir, "orchestration", "SKILL.md")
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == string(orchestrationSkill) {
		fmt.Fprintf(w, "  %s: 이미 최신 — 건너뜀\n", path)
		return nil
	}
	if dryRun {
		fmt.Fprintf(w, "  %s: 설치 예정 (dry-run — 파일을 변경하지 않았습니다)\n", path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if existing != nil {
		if err := os.WriteFile(path+".bak", existing, 0o644); err != nil {
			return fmt.Errorf("백업 실패 — 설치 중단: %w", err)
		}
		fmt.Fprintf(w, "  기존 파일을 %s.bak으로 백업\n", path)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, orchestrationSkill, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	fmt.Fprintf(w, "  %s: 설치됨 (/orchestration으로 호출)\n", path)
	return nil
}
