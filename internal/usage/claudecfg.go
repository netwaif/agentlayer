package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ClaudeDefaultModel은 Claude Code의 기본 모델 설정(/model로 저장되는 값)을 읽는다.
// settings.local.json이 settings.json보다 우선한다. 못 찾으면 빈 문자열 —
// 기본 모델이 미설정이면 Claude Code가 플랜 기본값(자동)을 쓴다는 뜻이다.
func ClaudeDefaultModel(home string) string {
	for _, name := range []string{"settings.local.json", "settings.json"} {
		b, err := os.ReadFile(filepath.Join(home, ".claude", name))
		if err != nil {
			continue
		}
		var s struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(b, &s) == nil && s.Model != "" {
			return s.Model
		}
	}
	return ""
}

// CodexDefaultModel은 ~/.codex/config.toml 최상위의 model·model_reasoning_effort를
// 읽는다. [profiles.*] 등 섹션 안의 model은 기본값이 아니므로 첫 섹션 헤더에서 멈춘다.
func CodexDefaultModel(home string) (model, effort string) {
	b, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			break // 섹션 시작 — 최상위 끝
		}
		if m := codexTomlRe.FindStringSubmatch(t); m != nil {
			if m[1] == "model" {
				model = m[2]
			} else {
				effort = m[2]
			}
		}
	}
	return model, effort
}

var codexTomlRe = regexp.MustCompile(`^(model|model_reasoning_effort)\s*=\s*"([^"]*)"`)

// GeminiDefaultModel은 ~/.gemini/settings.json의 model 설정을 읽는다.
// 구형(문자열)·신형({"name": ...}) 스키마 모두 허용.
func GeminiDefaultModel(home string) string {
	b, err := os.ReadFile(filepath.Join(home, ".gemini", "settings.json"))
	if err != nil {
		return ""
	}
	var s struct {
		Model json.RawMessage `json:"model"`
	}
	if json.Unmarshal(b, &s) != nil || len(s.Model) == 0 {
		return ""
	}
	var str string
	if json.Unmarshal(s.Model, &str) == nil {
		return str
	}
	var obj struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(s.Model, &obj) == nil {
		return obj.Name
	}
	return ""
}

// DefaultModels는 세 CLI의 기본 모델 설정을 모은다. 빈 값 = 미설정(자동).
// codex는 effort가 있으면 "model effort"로 합친다.
func DefaultModels(home string) map[string]string {
	cx, effort := CodexDefaultModel(home)
	if cx != "" && effort != "" {
		cx += " " + effort
	}
	return map[string]string{
		"claude": ClaudeDefaultModel(home),
		"codex":  cx,
		"gemini": GeminiDefaultModel(home),
	}
}

// modelIDRe: "claude-fable-5", "claude-haiku-4-5-20251001" 류의 모델 ID.
var modelIDRe = regexp.MustCompile(`^claude-([a-z]+)-(\d+(?:-\d+)?)(?:-\d{8})?$`)

// PrettyModel은 모델 ID·별칭을 짧은 표시명으로 바꾼다. 모르는 값은 그대로.
func PrettyModel(raw string) string {
	if raw == "" {
		return ""
	}
	oneM := strings.HasSuffix(raw, "[1m]")
	id := strings.TrimSuffix(raw, "[1m]")
	switch id {
	case "default":
		return "자동"
	case "opusplan":
		return "OpusPlan"
	}
	m := modelIDRe.FindStringSubmatch(id)
	if m == nil {
		return raw
	}
	name := strings.Title(m[1])
	ver := strings.ReplaceAll(m[2], "-", ".")
	out := name + " " + ver
	if oneM {
		out += " (1M)"
	}
	return out
}

// IsFable은 표시명·ID 어느 형태든 Fable 모델인지 판정한다.
// Fable은 최상위 티어라 무심코 기본으로 두면 토큰 소모가 크다 — 관제탑이 경고색으로 띄운다.
func IsFable(model string) bool {
	return strings.Contains(strings.ToLower(model), "fable")
}
