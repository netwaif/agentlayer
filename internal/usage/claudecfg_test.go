package usage

import (
	"path/filepath"
	"testing"
)

func TestClaudeDefaultModel(t *testing.T) {
	home := t.TempDir()
	if got := ClaudeDefaultModel(home); got != "" {
		t.Errorf("설정 파일 없으면 빈 문자열: %q", got)
	}

	writeFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"model":"claude-fable-5","theme":"dark"}`)
	if got := ClaudeDefaultModel(home); got != "claude-fable-5" {
		t.Errorf("settings.json model 읽어야 함: %q", got)
	}

	// settings.local.json이 있으면 우선
	writeFile(t, filepath.Join(home, ".claude", "settings.local.json"),
		`{"model":"claude-opus-5"}`)
	if got := ClaudeDefaultModel(home); got != "claude-opus-5" {
		t.Errorf("settings.local.json 우선: %q", got)
	}

	// model 키 없는 local은 건너뛰고 settings.json으로
	writeFile(t, filepath.Join(home, ".claude", "settings.local.json"), `{"theme":"dark"}`)
	if got := ClaudeDefaultModel(home); got != "claude-fable-5" {
		t.Errorf("model 없는 local은 폴백: %q", got)
	}
}

func TestPrettyModel(t *testing.T) {
	cases := map[string]string{
		"claude-fable-5":            "Fable 5",
		"claude-fable-5[1m]":        "Fable 5 (1M)",
		"claude-opus-5":             "Opus 5",
		"claude-opus-5[1m]":         "Opus 5 (1M)",
		"claude-sonnet-5":           "Sonnet 5",
		"claude-haiku-4-5-20251001": "Haiku 4.5",
		"default":                   "자동",
		"opusplan":                  "OpusPlan",
		"":                          "",
		"gpt-5.6-sol":               "gpt-5.6-sol", // 모르는 값은 그대로
	}
	for in, want := range cases {
		if got := PrettyModel(in); got != want {
			t.Errorf("PrettyModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsFable(t *testing.T) {
	for _, s := range []string{"claude-fable-5", "Fable 5", "Fable 5 (1M context)", "claude-fable-5[1m]"} {
		if !IsFable(s) {
			t.Errorf("IsFable(%q) = false", s)
		}
	}
	for _, s := range []string{"Opus 5 (1M context)", "claude-sonnet-5", "", "gpt-5.6-sol"} {
		if IsFable(s) {
			t.Errorf("IsFable(%q) = true", s)
		}
	}
}
