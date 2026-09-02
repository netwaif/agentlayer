package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestITerm2LinkRuleInstalled(t *testing.T) {
	if !ITerm2LinkRuleInstalled(`"Smart Selection Rules" = ( { parameter = "~/.local/bin/agentlayer browser open \"\\0\""; } )`) {
		t.Error("규칙이 있으면 true")
	}
	if ITerm2LinkRuleInstalled(`"Smart Selection Rules" = ( { regex = "\\S+"; } )`) {
		t.Error("규칙이 없으면 false")
	}
}

func TestPrintITerm2LinkGuide(t *testing.T) {
	var out bytes.Buffer
	PrintITerm2LinkGuide(&out, "/opt/agentlayer", false)
	s := out.String()
	for _, want := range []string{"Smart Selection", "Very High", "Run Command", `/opt/agentlayer browser open "\0"`, `https?://`} {
		if !strings.Contains(s, want) {
			t.Errorf("안내에 %q 없음:\n%s", want, s)
		}
	}
	out.Reset()
	PrintITerm2LinkGuide(&out, "/opt/agentlayer", true)
	if !strings.Contains(out.String(), "이미 설정됨") {
		t.Errorf("설치된 경우 안내: %s", out.String())
	}
}
