package cli

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// ITerm2LinkRuleInstalled은 `defaults read com.googlecode.iterm2 "New Bookmarks"`
// 출력에 agentlayer 링크 라우팅(Smart Selection 액션)이 있는지 본다.
func ITerm2LinkRuleInstalled(defaultsOut string) bool {
	return strings.Contains(defaultsOut, "agentlayer browser open")
}

// ReadITerm2Bookmarks는 iTerm2 프로필 설정 덤프. iTerm2가 없거나 실패하면 "".
func ReadITerm2Bookmarks() string {
	out, err := exec.Command("defaults", "read", "com.googlecode.iterm2", "New Bookmarks").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// PrintITerm2LinkGuide는 터미널 링크를 전용 브라우저로 보내는 iTerm2 설정 절차를
// 출력한다. iTerm2는 실행 중이면 plist를 되써서 자동 주입이 덮이므로 안내만 한다.
func PrintITerm2LinkGuide(w io.Writer, binPath string, installed bool) {
	fmt.Fprintln(w, "iTerm2 링크 라우팅 (⌘-클릭 → 에이전트 전용 브라우저):")
	if installed {
		fmt.Fprintln(w, "  이미 설정됨 — 건너뜀")
		return
	}
	fmt.Fprintln(w, "  아직 설정 안 됨. iTerm2 → Settings → Profiles → (쓰는 프로필) → Advanced → Smart Selection [Edit]")
	fmt.Fprintln(w, "    1. + 로 규칙 추가 — Name: Agent browser URL")
	fmt.Fprintln(w, `    2. Regular expression: https?://[^\s"'<>)]+`)
	fmt.Fprintln(w, "    3. Precision: Very High")
	fmt.Fprintf(w, "    4. Actions… → + → Title: Agent browser / Action: Run Command / Command: %s browser open \"\\0\"\n", binPath)
	fmt.Fprintln(w, "  (\"Use interpolated strings\"는 꺼 둔다. 규칙에 액션이 있으면 ⌘-클릭이 첫 액션을 실행한다)")
}
