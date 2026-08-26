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

func TestCodexDefaultModel(t *testing.T) {
	home := t.TempDir()
	if m, e := CodexDefaultModel(home); m != "" || e != "" {
		t.Errorf("config 없으면 빈 값: %q %q", m, e)
	}
	// 최상위 키만 읽는다 — [profiles.*] 안의 model은 기본값이 아니다
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), `model = "gpt-5.6-sol"
model_reasoning_effort = "high"

[profiles.lightweight]
model = "gpt-5.4-mini"
`)
	m, e := CodexDefaultModel(home)
	if m != "gpt-5.6-sol" || e != "high" {
		t.Errorf("최상위 model·effort: %q %q", m, e)
	}
}

func TestGeminiDefaultModel(t *testing.T) {
	home := t.TempDir()
	if got := GeminiDefaultModel(home); got != "" {
		t.Errorf("설정 없으면 빈 값: %q", got)
	}
	writeFile(t, filepath.Join(home, ".gemini", "settings.json"), `{"model":"gemini-2.5-pro"}`)
	if got := GeminiDefaultModel(home); got != "gemini-2.5-pro" {
		t.Errorf("문자열 형식: %q", got)
	}
	// 신형 스키마: model이 객체({"name": ...})인 경우
	writeFile(t, filepath.Join(home, ".gemini", "settings.json"), `{"model":{"name":"gemini-3-pro"}}`)
	if got := GeminiDefaultModel(home); got != "gemini-3-pro" {
		t.Errorf("객체 형식: %q", got)
	}
	// Antigravity CLI(agy) 설정이 있으면 우선 — 실사용 CLI가 agy이기 때문
	writeFile(t, filepath.Join(home, ".gemini", "antigravity-cli", "settings.json"),
		`{"model":"Gemini 3.6 Flash (Medium)"}`)
	if got := GeminiDefaultModel(home); got != "Gemini 3.6 Flash (Medium)" {
		t.Errorf("agy settings 우선: %q", got)
	}
}

func TestGeminiLatest(t *testing.T) {
	home := t.TempDir()
	gd := filepath.Join(home, ".gemini")
	if info := GeminiLatest(gd, "/w/proj"); info.Model != "" {
		t.Errorf("아무것도 없으면 빈 CtxInfo: %+v", info)
	}
	writeFile(t, filepath.Join(gd, "projects.json"),
		`{"projects":{"/w/proj":"proj","/w":"w"}}`)
	writeFile(t, filepath.Join(gd, "tmp", "proj", "chats", "session-2026-08-01T00-00-aaaa.jsonl"),
		`{"sessionId":"aaaa","kind":"main"}
{"type":"user","content":"hi"}
{"type":"gemini","model":"gemini-3-flash-preview","tokens":{"input":100,"output":10,"total":110}}
{"type":"gemini","model":"gemini-3-pro","tokens":{"input":200,"output":20,"total":220}}
`)
	info := GeminiLatest(gd, "/w/proj")
	if info.Model != "gemini-3-pro" {
		t.Errorf("마지막 model 레코드: %q", info.Model)
	}
	if info.TS.IsZero() {
		t.Error("TS는 파일 mtime")
	}
	// 하위 폴더로 cd한 경우 — 조상 매핑으로 폴백
	if info := GeminiLatest(gd, "/w/proj/sub/dir"); info.Model != "gemini-3-pro" {
		t.Errorf("조상 프로젝트 매칭: %q", info.Model)
	}
}

func TestDefaultModels(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"model":"claude-opus-5"}`)
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), `model = "gpt-5.6-sol"`)
	got := DefaultModels(home)
	if got["claude"] != "claude-opus-5" || got["codex"] != "gpt-5.6-sol" || got["gemini"] != "" {
		t.Errorf("DefaultModels = %+v", got)
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
