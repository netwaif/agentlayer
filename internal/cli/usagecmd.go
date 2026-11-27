package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/usage"
)

// usageWinLabel·usageWinOrder — TUI 사용량 화면과 같은 창 이름·순서(internal/ui/view.go winLabel·windowOrder).
var usageWinLabel = map[string]string{"5h": "5h", "7d": "7d", "daily": "1d", "fable_7d": "Fable"}

func usageWinOrder(ws map[string]usage.Window) []string {
	var std, rest []string
	for _, k := range []string{"5h", "daily", "7d", "fable_7d"} {
		if _, ok := ws[k]; ok {
			std = append(std, k)
		}
	}
	for k := range ws {
		if usageWinLabel[k] == "" {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(std, rest...)
}

// UsageText는 사용량(coach 페이로드)을 색 없는 글자로 — 메뉴 막대 알림창·셸 출력용(TUI의 u).
// nil이면 안내문. 공급자 순서는 TUI와 같다(claude·codex·antigravity).
func UsageText(p *usage.Payload, now time.Time) string {
	if p == nil {
		return "사용량을 아직 못 읽었습니다. 공식 값을 다시 읽으려면 agentlayer usage read --refresh를 실행하세요."
	}
	var b strings.Builder
	if ts, err := time.Parse(time.RFC3339, p.TS); err == nil {
		if age := now.Sub(ts); age > time.Minute {
			fmt.Fprintf(&b, "(%d분 전 측정)\n", int(age.Minutes()))
		}
	}
	for _, key := range []string{"claude", "codex", "antigravity"} {
		pr, ok := p.Providers[key]
		if !ok || !pr.OK {
			continue
		}
		fmt.Fprintf(&b, "%s — %s\n", strings.ToUpper(key[:1])+key[1:], pr.Action)
		if pr.Email != "" {
			fmt.Fprintf(&b, "  %s\n", pr.Email)
		}
		if pr.Source != "" {
			fmt.Fprintf(&b, "  %s · %s\n", pr.Source, pr.MeasuredAt)
		}
		for _, wk := range usageWinOrder(pr.Windows) {
			w := pr.Windows[wk]
			label := usageWinLabel[wk]
			if label == "" {
				label = w.Label
				if label == "" {
					label = wk
				}
			}
			line := fmt.Sprintf("  %-8s %s", label, usage.Gauge(w.LeftPct, 14))
			if w.LeftPct != nil && w.ResetPassed {
				line += fmt.Sprintf(" (%d%%) 재설정됨 — 새 값 기다림", int(*w.LeftPct))
			} else if w.LeftPct != nil {
				line += fmt.Sprintf(" %3d%%", int(*w.LeftPct))
				if r := usage.ResetLabel(w.ResetMin); r != "" {
					line += " · 리셋 " + r
				}
			} else {
				line += "   —%"
			}
			b.WriteString(line + "\n")
		}
		if pr.Reason != "" {
			fmt.Fprintf(&b, "  %s\n", pr.Reason)
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "읽을 수 있는 공급자가 없습니다."
	}
	return strings.TrimRight(b.String(), "\n")
}

// RunUsage는 공식 사용량과 선택한 CodexBar 경로를 같은 형식으로 출력한다.
func RunUsage(w io.Writer, stateDir string, runner func() ([]byte, error), now time.Time, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "read":
			return runUsageRead(w, stateDir, now, args[1:])
		case "setup":
			return runUsageSetup(w, stateDir, args[1:], now)
		case "source":
			return runUsageSource(w, stateDir, args[1:])
		}
	}
	var p *usage.Payload
	if hasArg(args, "--cached") {
		p = usage.CurrentCached(stateDir, now)
	} else {
		p = usage.Current(stateDir, 5*time.Minute, runner, now)
	}
	return writeUsage(w, p, now, hasArg(args, "--json"))
}

func writeUsage(w io.Writer, p *usage.Payload, now time.Time, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(w).Encode(p)
	}
	_, err := fmt.Fprintln(w, UsageText(p, now))
	return err
}

func runUsageRead(w io.Writer, stateDir string, now time.Time, args []string) error {
	for _, arg := range args {
		if arg != "--json" && arg != "--refresh" {
			return fmt.Errorf("쓰는 법: agentlayer usage read [--json] [--refresh]")
		}
	}
	var refreshErr error
	if hasArg(args, "--refresh") {
		refreshErr = usage.RefreshOfficial(stateDir, now)
		now = time.Now()
	}
	home, _ := os.UserHomeDir()
	if err := writeUsage(w, usage.ReadNative(stateDir, home, now), now, hasArg(args, "--json")); err != nil {
		return err
	}
	return refreshErr
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func runUsageSetup(w io.Writer, stateDir string, args []string, now time.Time) error {
	if len(args) == 0 || args[0] != "claude" || len(args) > 2 || (len(args) == 2 && args[1] != "--undo") {
		return fmt.Errorf("쓰는 법: agentlayer usage setup claude [--undo]")
	}
	home, _ := os.UserHomeDir()
	dir, _ := usage.ClaudeConfigDir(home)
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	undo := hasArg(args, "--undo")
	res, err := usage.SetupClaude(dir, bin, undo, now)
	if err != nil {
		return err
	}
	defer fmt.Fprintf(w, "  화면 경로: %s\n", usage.SourceOf(stateDir))
	if !res.Changed {
		fmt.Fprintf(w, "바꿀 것 없음: %s\n  statusLine.command = %q\n", res.Settings, res.Before)
		return nil
	}
	fmt.Fprintf(w, "고침: %s\n", res.Settings)
	if res.Backup != "" {
		fmt.Fprintf(w, "  사본: %s\n", res.Backup)
	}
	fmt.Fprintf(w, "  전: %q\n  후: %q\n", res.Before, res.After)
	return nil
}

func runUsageSource(w io.Writer, stateDir string, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("쓰는 법: agentlayer usage source [native|codexbar]")
	}
	if len(args) > 0 {
		if err := usage.SetSource(stateDir, args[0]); err != nil {
			return err
		}
	}
	fmt.Fprintln(w, usage.SourceOf(stateDir))
	return nil
}
