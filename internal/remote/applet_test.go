package remote

import (
	"context"
	"strings"
	"testing"
)

// Setup은 회사용 둘 + Claude용 둘을 깐다. 회사용 렌더 결과는 예전과 바이트 단위로 같다.
func TestHermesSetupInstallsClaudeLetterToo(t *testing.T) {
	f := &FakeRunner{Reply: func(args []string) ([]byte, error) {
		cmd := args[2]
		switch {
		case strings.Contains(cmd, "claude-letter") && strings.Contains(cmd, "SKILL.md"):
			return []byte("/opt/data/skills/autonomous-ai-agents/claude-letter/SKILL.md\n"), nil
		case strings.Contains(cmd, "claude-letter"):
			return []byte("/opt/data/.local/bin/claude-letter\n"), nil
		case strings.Contains(cmd, "SKILL.md"):
			return []byte("/opt/data/skills/autonomous-ai-agents/company-letter/SKILL.md\n"), nil
		}
		return []byte("/opt/data/.local/bin/company-letter\n"), nil
	}}
	h := newHermes(f)
	h.MailboxAssignee = "company-manager"
	h.AttachRoot = "/opt/data/ai-company/참고자료/from-company"
	paths, err := h.Setup(context.Background())
	if err != nil || len(paths) != 4 {
		t.Fatalf("paths=%v err=%v", paths, err)
	}
	want := []string{"/company-letter", "company-letter/SKILL.md", "/claude-letter", "claude-letter/SKILL.md"}
	for i, w := range want {
		if !strings.HasSuffix(paths[i], w) {
			t.Errorf("paths[%d]=%q want suffix %q", i, paths[i], w)
		}
	}
	// 회사용 둘은 예전 렌더 그대로(바이트 단위)
	if f.Stdins[0] != renderSide(letterScript, "company-manager", h.AttachRoot) || f.Stdins[1] != renderSide(letterSkill, "company-manager", h.AttachRoot) {
		t.Error("회사용 준비물 내용이 바뀌면 안 된다")
	}
	script, skill := f.Stdins[2], f.Stdins[3]
	if !strings.Contains(script, "--assignee claude-app") || strings.Contains(script, "{{") || strings.Contains(script, "company-manager") || !strings.Contains(script, "Claude 앞") {
		t.Errorf("claude-letter 스크립트:\n%s", script)
	}
	if !strings.Contains(skill, "name: claude-letter") || !strings.Contains(skill, "`claude-app`") || !strings.Contains(skill, "클로드") ||
		!strings.Contains(skill, "참고자료/from-company/<카드ID>") || strings.Contains(skill, "{{") {
		t.Errorf("claude-letter 스킬:\n%s", skill)
	}
	if !strings.Contains(f.Calls[2][2], `"$d/claude-letter"`) || !strings.Contains(f.Calls[2][2], "chmod 755") ||
		!strings.Contains(f.Calls[3][2], "skills/autonomous-ai-agents/claude-letter") {
		t.Errorf("설치 명령: %v %v", f.Calls[2], f.Calls[3])
	}
}

// AppMailbox는 담당자 claude-app 카드만 집고(claim만), 회사 편지함 카드는 건드리지 않는다. Mailbox는 그 반대.
func TestHermesAppMailboxPicksOnlyClaudeApp(t *testing.T) {
	list := `[{"id":"t_c1","title":"썸네일 문구","body":"3개 뽑아 줘","assignee":"claude-app","status":"ready","created_by":"default","created_at":1790320000},
	{"id":"t_m1","title":"회사 편지","body":"x","assignee":"imac-manager","status":"ready","created_by":"default","created_at":1790320001},
	{"id":"t_c2","title":"이미 답한 편지","body":"y","assignee":"claude-app","status":"done","created_by":"default","created_at":1790320002}]`
	f := &FakeRunner{Reply: replyTable(t, map[string]string{"hermes kanban list --json": list, "hermes kanban claim": "/w"})}
	letters, err := newHermes(f).AppMailbox(context.Background())
	if err != nil || len(letters) != 1 || letters[0].ID != "t_c1" || letters[0].From != "default" || letters[0].Text != "3개 뽑아 줘" {
		t.Fatalf("letters=%+v err=%v", letters, err)
	}
	if got := strings.Join(f.Calls[1], " "); got != "hermes kanban claim t_c1 --ttl 86400" || len(f.Calls) != 2 {
		t.Errorf("claim만: %v", f.Calls)
	}
	f2 := &FakeRunner{Reply: replyTable(t, map[string]string{"hermes kanban list --json": list, "hermes kanban claim": "/w"})}
	letters, _ = newHermes(f2).Mailbox(context.Background())
	if len(letters) != 1 || letters[0].ID != "t_m1" {
		t.Errorf("회사 편지함은 예전 그대로 imac-manager만: %+v", letters)
	}
	var _ AppMailboxer = (*Hermes)(nil)
}
