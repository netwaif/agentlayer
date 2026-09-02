package wt

import (
	"errors"
	"testing"
	"time"
)

var errTest = errors.New("pane gone")

func TestIsTrustPromptMatchesThreeCLIs(t *testing.T) {
	for _, s := range []string{
		"Do you trust the files in this folder?\n\n> 1. Yes, continue\n  2. No, quit",                        // codex
		"Do you trust the files in this folder?\n/Users/x/repo\n❯ Yes, proceed\n  No, exit",                  // claude
		"Do you trust this folder?\nTrusting a folder allows Gemini CLI to execute commands\n● Trust folder", // gemini
		"이 폴더를 신뢰하시겠습니까?\n[확인]",                                                                              // 한국어 UI
	} {
		if !IsTrustPrompt(s) {
			t.Errorf("신뢰 질문으로 인식해야 함:\n%s", s)
		}
	}
	for _, s := range []string{"› 무엇을 도와드릴까요?", "Reading files in folder src/", "git status: trust me it's fine"} {
		if IsTrustPrompt(s) {
			t.Errorf("오인식:\n%s", s)
		}
	}
}

func TestAcceptStartupPromptsPressesEnterOnceThenStops(t *testing.T) {
	screens := []string{"booting…", "Do you trust the files in this folder?\n> 1. Yes", "› ready"}
	i, pressed := 0, 0
	capture := func(string, int) (string, error) {
		s := screens[min(i, len(screens)-1)]
		i++
		return s, nil
	}
	enter := func(string) error { pressed++; return nil }
	n := AcceptStartupPrompts(capture, enter, "%1", 5*time.Second, time.Millisecond, func(time.Duration) {})
	if n != 1 || pressed != 1 {
		t.Errorf("한 번 누르고 질문이 사라지면 끝나야 함: n=%d pressed=%d", n, pressed)
	}
}

func TestAcceptStartupPromptsGivesUpAfterThree(t *testing.T) {
	capture := func(string, int) (string, error) { return "Do you trust this folder?", nil }
	pressed := 0
	enter := func(string) error { pressed++; return nil }
	n := AcceptStartupPrompts(capture, enter, "%1", 5*time.Second, time.Millisecond, func(time.Duration) {})
	if n != 3 || pressed != 3 {
		t.Errorf("모르는 형태면 3번에서 포기: %d", n)
	}
}

func TestAcceptStartupPromptsStopsWhenPaneGone(t *testing.T) {
	capture := func(string, int) (string, error) { return "", errTest }
	n := AcceptStartupPrompts(capture, func(string) error { return nil }, "%1", time.Second, time.Millisecond, func(time.Duration) {})
	if n != 0 {
		t.Error("pane이 없으면 즉시 0")
	}
}
