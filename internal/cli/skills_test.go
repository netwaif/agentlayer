package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSkillsFresh(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := InstallSkills(&buf, dir, false); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, marker string }{
		{"orchestration", "wt new"},
		{"agent-browser", "pick --once"},
	} {
		b, err := os.ReadFile(filepath.Join(dir, c.name, "SKILL.md"))
		if err != nil {
			t.Fatalf("%s 스킬이 설치돼야 함: %v", c.name, err)
		}
		s := string(b)
		if !strings.Contains(s, "name: "+c.name) || !strings.Contains(s, c.marker) {
			t.Errorf("%s 스킬 내용 불일치:\n%.200s", c.name, s)
		}
	}
}

func TestInstallSkillsIdempotent(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := InstallSkills(&buf, dir, false); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := InstallSkills(&buf, dir, false); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "이미 최신") != len(Skills) {
		t.Errorf("동일 내용이면 전부 건너뛰어야 함: %s", buf.String())
	}
}

func TestInstallSkillUpgradeKeepsBackup(t *testing.T) {
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
	if err := InstallSkills(&buf, dir, false); err != nil {
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

func TestInstallSkillsDryRun(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := InstallSkills(&buf, dir, true); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Error("dry-run은 파일을 만들면 안 됨")
	}
}

// 관심사 분리: 브라우저 지침은 agent-browser에만, 코디네이터 절차는 orchestration에만.
func TestSkillsSeparateConcerns(t *testing.T) {
	orch := string(orchestrationSkill)
	br := string(agentBrowserSkill)
	for _, want := range []string{"chrome-devtools", "자기 탭", "control-in-app-browser", "shot <url>`", "`files`", "--notify",
		"browser errors --reload", "list_console_messages", "cookies import", "cookies list", "cookies clear", "cookies export", "--env", "항상 허용",
		"pick --once", "브라우저에서 지목할게", "스크린샷 확인해봐", "폰으로 보내줘",
		"type_text", "Google Chrome for Testing", "did not become interactive"} {
		if !strings.Contains(br, want) {
			t.Errorf("agent-browser에 %q 없음", want)
		}
	}
	for _, gone := range []string{"pick --once", "cookies import", "errors --reload", "브라우저에서 지목할게", "control-in-app-browser"} {
		if strings.Contains(orch, gone) {
			t.Errorf("orchestration에 브라우저 세부 %q가 남아 있음", gone)
		}
	}
	for _, want := range []string{"wt new", "send-keys", "agentlayer status", "wt merge <이름> --yes", "wt clean", "agent-browser"} {
		if !strings.Contains(orch, want) {
			t.Errorf("orchestration에 %q 없음", want)
		}
	}
	for _, gone := range []string{"wt new", "send-keys", "REPORT.md"} {
		if strings.Contains(br, gone) {
			t.Errorf("agent-browser에 오케스트레이션 세부 %q가 섞여 있음", gone)
		}
	}
}
