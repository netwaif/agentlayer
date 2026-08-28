package main

import (
	"strings"
	"testing"
)

// help·-h·--help 모두 에러 없이(exit 0) 도움말로 라우팅돼야 한다.
func TestRunRoutesHelpAliases(t *testing.T) {
	for _, alias := range []string{"help", "-h", "--help"} {
		if err := run([]string{alias}); err != nil {
			t.Errorf("run(%q) 에러: %v", alias, err)
		}
	}
}

// 알 수 없는 명령 에러는 help로 안내해야 한다.
func TestRunUnknownCommandPointsToHelp(t *testing.T) {
	err := run([]string{"no-such-command"})
	if err == nil {
		t.Fatal("알 수 없는 명령인데 에러가 없다")
	}
	if !strings.Contains(err.Error(), "help") {
		t.Errorf("에러에 help 안내가 없다: %v", err)
	}
}
