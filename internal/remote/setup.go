package remote

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"strings"
)

// Hermes 쪽 준비물 — 직원(Hermes)이 총괄에게 먼저 편지를 보내는 명령과, 그 명령을 쓰게 하는 스킬.
// 정본은 이 폴더(hermesside/)이고 agentlayer가 원격에 깔아 준다. 담당자 이름과 첨부 폴더는 등록값으로 채운다.
var (
	//go:embed hermesside/company-letter.sh
	letterScript string
	//go:embed hermesside/SKILL.md
	letterSkill string
	// Claude 앱 세션용 한 벌 — 담당자는 고정값 AppMailbox. 회사용과 나란히 깔린다(claude-letter 설계, 2026-10-01).
	//go:embed hermesside/claude-letter.sh
	appLetterScript string
	//go:embed hermesside/claude-letter/SKILL.md
	appLetterSkill string
)

// LetterCommand는 원격에 깔리는 편지 명령 이름(회사 총괄 앞).
const LetterCommand = "company-letter"

// AppLetterCommand는 Claude 앱 세션 앞 편지 명령 이름. AppMailbox는 그 담당자(고정값) — 회사 편지함과 분리돼
// 총괄의 원격 폴링(task.PollRemotesOnce)과 경합하지 않는다.
const (
	AppLetterCommand = "claude-letter"
	AppMailbox       = "claude-app"
)

// Installer는 원격에 준비물을 까는 어댑터(지금은 Hermes). 돌려주는 값은 깐 경로 목록.
type Installer interface {
	Setup(ctx context.Context) ([]string, error)
}

// 담당자 이름은 셸 스크립트에 그대로 들어간다 — 안전한 글자만 받는다.
var assigneeRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func renderSide(tmpl, mailbox, attachRoot string) string {
	return strings.NewReplacer("{{MAILBOX}}", mailbox, "{{ATTACH_ROOT}}", attachRoot).Replace(tmpl)
}

// Setup은 편지 명령을 ~/.local/bin에, 스킬을 $HERMES_HOME(없으면 ~/.hermes)/skills 아래에 쓴다. 멱등(덮어쓰기).
// 경로는 원격 셸이 정한다 — 컨테이너마다 HOME·HERMES_HOME이 다르다(호스팅어 이미지는 둘 다 /opt/data).
func (h *Hermes) Setup(ctx context.Context) ([]string, error) {
	mailbox := h.MailboxAssignee
	if mailbox == "" {
		mailbox = DefaultMailbox
	}
	if !assigneeRe.MatchString(mailbox) {
		return nil, fmt.Errorf("편지함 담당자 이름 형식 오류: %q", mailbox)
	}
	attach := h.AttachRoot
	if attach == "" {
		attach = h.WorkspaceRoot + "/" + AttachDirName
	}
	steps := []struct{ cmd, body string }{
		{`d="$HOME/.local/bin"; mkdir -p "$d" && cat > "$d/` + LetterCommand + `" && chmod 755 "$d/` + LetterCommand + `" && echo "$d/` + LetterCommand + `"`,
			renderSide(letterScript, mailbox, attach)},
		{`d="${HERMES_HOME:-$HOME/.hermes}/skills/autonomous-ai-agents/` + LetterCommand + `"; mkdir -p "$d" && cat > "$d/SKILL.md" && echo "$d/SKILL.md"`,
			renderSide(letterSkill, mailbox, attach)},
		// Claude 앱 세션용 — 회사용 두 파일 뒤에 같은 방식으로. 회사용 렌더 결과는 그대로다.
		{`d="$HOME/.local/bin"; mkdir -p "$d" && cat > "$d/` + AppLetterCommand + `" && chmod 755 "$d/` + AppLetterCommand + `" && echo "$d/` + AppLetterCommand + `"`,
			renderSide(appLetterScript, AppMailbox, attach)},
		{`d="${HERMES_HOME:-$HOME/.hermes}/skills/autonomous-ai-agents/` + AppLetterCommand + `"; mkdir -p "$d" && cat > "$d/SKILL.md" && echo "$d/SKILL.md"`,
			renderSide(appLetterSkill, AppMailbox, attach)},
	}
	var paths []string
	for _, s := range steps {
		c, cancel := context.WithTimeout(ctx, TimeoutQuery)
		out, err := h.R.Run(c, strings.NewReader(s.body), "sh", "-c", s.cmd)
		cancel()
		if err != nil {
			return paths, fmt.Errorf("준비물 설치 실패: %w", err)
		}
		if p := strings.TrimSpace(string(out)); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}
