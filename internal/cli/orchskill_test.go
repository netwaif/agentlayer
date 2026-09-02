package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallOrchestrationSkillFresh(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := InstallOrchestrationSkill(&buf, dir, false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "orchestration", "SKILL.md"))
	if err != nil {
		t.Fatalf("스킬 파일이 설치돼야 함: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "name: orchestration") || !strings.Contains(s, "wt new") {
		t.Errorf("스킬 내용 불일치:\n%.200s", s)
	}
}

func TestInstallOrchestrationSkillIdempotent(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := InstallOrchestrationSkill(&buf, dir, false); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := InstallOrchestrationSkill(&buf, dir, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "이미 최신") {
		t.Errorf("동일 내용이면 건너뛰어야 함: %s", buf.String())
	}
}

func TestInstallOrchestrationSkillUpgradeKeepsBackup(t *testing.T) {
	// 사용자가 손댔거나 옛 버전이면 .bak을 남기고 갱신한다.
	dir := t.TempDir()
	path := filepath.Join(dir, "orchestration", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("옛 버전"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := InstallOrchestrationSkill(&buf, dir, false); err != nil {
		t.Fatal(err)
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil || string(bak) != "옛 버전" {
		t.Errorf("이전 내용 백업돼야 함: %v %q", err, bak)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "name: orchestration") {
		t.Error("새 내용으로 갱신돼야 함")
	}
}

func TestInstallOrchestrationSkillDryRun(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := InstallOrchestrationSkill(&buf, dir, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "orchestration", "SKILL.md")); err == nil {
		t.Error("dry-run은 파일을 만들면 안 됨")
	}
}

// 브라우저 절: 에이전트가 내장 브라우저 스킬로 새지 않고 chrome-devtools MCP를 자기 탭에서 쓰게 한다.
func TestOrchestrationSkillHasBrowserSection(t *testing.T) {
	for _, want := range []string{"## 6. 브라우저", "chrome-devtools", "자기 탭", "control-in-app-browser", "shot --notify", "cookies import", "cookies list", "cookies clear", "항상 허용", "pick --once", "브라우저에서 지목할게", "스크린샷 확인해봐"} {
		if !strings.Contains(string(orchestrationSkill), want) {
			t.Errorf("스킬 본문에 %q 없음", want)
		}
	}
}
