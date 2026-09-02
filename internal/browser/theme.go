package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// 에이전트 브라우저 식별색: 라이트 스킴 + 주황(0xFFE8590C). 실사용 Chrome이
// 다크·기본색이어도 툴바만 보고 구분된다. Chrome Refresh(GM3) Preferences 키.
const (
	agentThemeColor   = -1615604 // SkColor 0xFFE8590C를 int32로
	agentThemeVariant = 3        // vibrant
	agentThemeScheme  = 1        // light
	agentProfileName  = "AgentLayer"
)

// EnsureProfileTheme는 전용 프로필의 Default/Preferences에 식별 테마와 프로필
// 이름을 심는다. Chrome은 기동 시 이 파일을 읽으므로 반드시 Launch 전에,
// 그리고 Chrome이 안 떠 있을 때만 부른다(떠 있으면 되써서 덮인다).
// 사용자가 이미 색을 골랐으면(user_color2 존재) 존중한다.
func EnsureProfileTheme(profileDir string) error {
	path := filepath.Join(profileDir, "Default", "Preferences")
	prefs := map[string]any{}
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &prefs); err != nil {
			return nil // 손상된 Preferences는 Chrome이 스스로 복구 — 우리는 손대지 않는다
		}
	case !os.IsNotExist(err):
		return err
	}
	browser, _ := prefs["browser"].(map[string]any)
	if browser == nil {
		browser = map[string]any{}
	}
	theme, _ := browser["theme"].(map[string]any)
	if theme == nil {
		theme = map[string]any{}
	}
	profile, _ := prefs["profile"].(map[string]any)
	if profile == nil {
		profile = map[string]any{}
	}
	changed := false
	if _, ok := theme["user_color2"]; !ok {
		theme["user_color2"] = agentThemeColor
		theme["color_variant2"] = agentThemeVariant
		theme["color_scheme2"] = agentThemeScheme
		changed = true
	}
	if profile["name"] != agentProfileName && profile["name"] == nil {
		profile["name"] = agentProfileName
		changed = true
	}
	if !changed {
		return nil
	}
	browser["theme"] = theme
	prefs["browser"] = browser
	prefs["profile"] = profile
	out, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}
