package remote

import (
	"context"
	"strings"
	"testing"
)

// 준비물은 등록된 담당자 이름·첨부 폴더로 채워져 원격에 쓰인다.
func TestHermesSetupInstallsLetterFiles(t *testing.T) {
	f := &FakeRunner{Reply: func(args []string) ([]byte, error) {
		if strings.Contains(args[2], "SKILL.md") {
			return []byte("/opt/data/skills/autonomous-ai-agents/company-letter/SKILL.md\n"), nil
		}
		return []byte("/opt/data/.local/bin/company-letter\n"), nil
	}}
	h := newHermes(f)
	h.MailboxAssignee = "company-manager"
	h.AttachRoot = "/opt/data/ai-company/참고자료/from-company"
	paths, err := h.Setup(context.Background())
	// 1.12.2+: 회사용 둘 뒤에 Claude 앱 세션용 claude-letter 둘이 더 깔린다(applet_test.go). 회사용 내용·순서는 그대로.
	if err != nil || len(paths) != 4 || !strings.HasSuffix(paths[0], "/company-letter") {
		t.Fatalf("paths=%v err=%v", paths, err)
	}
	if len(f.Calls) != 4 || f.Calls[0][0] != "sh" || f.Calls[0][1] != "-c" {
		t.Fatalf("calls: %v", f.Calls)
	}
	script, skill := f.Stdins[0], f.Stdins[1]
	if !strings.Contains(script, "--assignee company-manager") || strings.Contains(script, "{{") || strings.Contains(script, "imac") {
		t.Errorf("스크립트:\n%s", script)
	}
	if !strings.Contains(skill, "`company-manager`") || !strings.Contains(skill, "참고자료/from-company/<카드ID>") || strings.Contains(skill, "{{") {
		t.Errorf("스킬:\n%s", skill)
	}
	if !strings.Contains(f.Calls[0][2], "chmod 755") || !strings.Contains(f.Calls[1][2], "${HERMES_HOME:-$HOME/.hermes}/skills") {
		t.Errorf("설치 명령: %v", f.Calls)
	}
}

// 옛 등록(담당자 imac-manager)도 그 이름 그대로 깔린다 — 이름을 바꾸기 전까지 편지가 끊기지 않게.
func TestHermesSetupKeepsRegisteredMailbox(t *testing.T) {
	f := &FakeRunner{}
	h := newHermes(f) // MailboxAssignee: imac-manager
	if _, err := h.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.Stdins[0], "--assignee imac-manager") {
		t.Errorf("등록된 담당자를 써야 한다:\n%s", f.Stdins[0])
	}
}

func TestHermesSetupRejectsUnsafeMailbox(t *testing.T) {
	f := &FakeRunner{}
	h := newHermes(f)
	h.MailboxAssignee = "x; rm -rf /"
	if _, err := h.Setup(context.Background()); err == nil || len(f.Calls) != 0 {
		t.Fatalf("위험한 이름은 거부: err=%v calls=%v", err, f.Calls)
	}
}
