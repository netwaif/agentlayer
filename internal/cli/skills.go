package cli

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// 바이너리에 동봉되는 스킬 본문. 스킬은 wt·status·browser 등 이 바이너리의 명령을
// 그대로 지시하는 지침서라, 별도 채널(플러그인 등)로 배포하면 바이너리와 버전이
// 어긋난다 — 같이 싣고 init이 설치해 버전을 잠근다.
//
// 스킬은 "사용자가 무엇을 시키는가" 단위로 하나씩:
//   - orchestration: 코디네이터가 worker N개를 worktree에 띄워 지시·대기·취합·머지
//   - agent-browser: 에이전트 누구나(worker 포함) 전용 브라우저로 보고·찍고·진단하고·지목 받고·로그인
//
//go:embed orchestration_skill.md
var orchestrationSkill []byte

//go:embed agent_browser_skill.md
var agentBrowserSkill []byte

// Skills는 init이 설치하는 스킬 목록 (폴더 이름 → 본문).
var Skills = []struct {
	Name string
	Body []byte
}{
	{"orchestration", orchestrationSkill},
	{"agent-browser", agentBrowserSkill},
}

// InstallSkills는 skillsDir(보통 ~/.claude/skills) 아래에 동봉 스킬을 전부 설치한다.
func InstallSkills(w io.Writer, skillsDir string, dryRun bool) error {
	for _, s := range Skills {
		if err := installSkill(w, skillsDir, s.Name, s.Body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// installSkill은 <skillsDir>/<name>/SKILL.md를 설치한다. 내용이 같으면 건너뛰고(멱등),
// 다르면 기존 파일을 .bak으로 남기고 갱신한다 — 사용자가 손댔어도 유실되지 않는다.
func installSkill(w io.Writer, skillsDir, name string, body []byte, dryRun bool) error {
	path := filepath.Join(skillsDir, name, "SKILL.md")
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == string(body) {
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
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	fmt.Fprintf(w, "  %s: 설치됨 (/%s으로 호출)\n", path, name)
	return nil
}
