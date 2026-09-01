package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PickContext는 요소 찍기 한 건의 전달 맥락.
type PickContext struct {
	URL, Selector, HTML, Instruction string
	Shot                             []byte // 요소 영역 PNG (없으면 nil)
}

// SavePick은 맥락을 md(+png)로 저장한다. pane에는 경로만 한 줄로 가므로
// 에이전트가 읽을 실질 내용은 전부 여기 담는다.
func SavePick(stateDir string, c PickContext, now time.Time) (string, string, error) {
	dir := filepath.Join(stateDir, "picks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	ts := now.Format("20060102-150405")
	md := filepath.Join(dir, ts+".md")
	body := fmt.Sprintf(`# 브라우저 요소 수정 요청

- 지시: %s
- 페이지: %s
- 셀렉터: `+"`%s`"+`

## 요소 HTML

`+"```html\n%s\n```\n", c.Instruction, c.URL, c.Selector, c.HTML)
	if err := os.WriteFile(md, []byte(body), 0o644); err != nil {
		return "", "", err
	}
	png := ""
	if len(c.Shot) > 0 {
		png = filepath.Join(dir, ts+".png")
		if err := os.WriteFile(png, c.Shot, 0o644); err != nil {
			return "", "", err
		}
	}
	return md, png, nil
}

// PromptLine은 pane에 보낼 한 줄. 여러 줄 send-keys는 CLI 입력창에서
// 조기 제출되므로 반드시 한 줄이어야 한다.
func PromptLine(instruction, mdPath, pngPath string) string {
	one := strings.Join(strings.Fields(instruction), " ")
	s := fmt.Sprintf("브라우저 요소 수정 요청: %q — 맥락 파일을 읽고 반영해줘: %s", one, mdPath)
	if pngPath != "" {
		s += " (요소 스크린샷: " + pngPath + ")"
	}
	return s
}
