package main

import (
	"os"
	"testing"
)

func TestSubcommandHelpDoesNotTouchDisk(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, args := range [][]string{{"task", "watch", "--help"}, {"task", "-h"}, {"remote", "add", "--help"}, {"wt", "--help"}, {"channel", "serve", "-h"}} {
		if err := run(args); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
	if _, err := os.Stat("--help"); err == nil {
		t.Error("'--help' 디렉터리가 생기면 안 됨")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("help는 디스크에 아무것도 만들지 않는다: %v", entries)
	}
}

func TestHelpFlagOnlyForListedSubcommands(t *testing.T) {
	// send/broadcast는 가로채지 않는다 — 여기서는 usage 조회만 확인(실제 send는 tmux가 필요)
	if err := run([]string{"send", "--help"}); err == nil {
		t.Error("send --help는 SubUsage 대상이 아니므로 send 자체 인자 검증 오류가 나야 한다")
	}
}
