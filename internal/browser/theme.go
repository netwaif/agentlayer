package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// 에이전트 브라우저 식별색 = AICheatKey 하우스 스타일(웜다크 배경 + 테라코타
// 오렌지 #d97757, AgentLoops 산출물 팔레트). 다크 스킴 + neutral 변형이면 툴바가
// 웜다크 회색(#443935 근처)이 되고 액센트만 테라코타로 든다. 라이트·tonal spot·
// vibrant는 실물 비교 후 탈락(2026-09-02). Chrome Refresh(GM3) Preferences 키.
const (
	agentThemeColor   = 0xFFD97757 - 1<<32 // SkColor(ARGB)를 int32로
	agentThemeVariant = 2                  // neutral
	agentThemeScheme  = 2                  // dark
	agentProfileName  = "AgentLayer"
)

// EnsureProfileTheme는 전용 프로필에 식별 테마(Default/Preferences)와 프로필
// 이름(Local State — 표시 이름의 정본)을 심는다. Chrome은 기동 시 이 파일들을
// 읽으므로 반드시 Launch 전에, Chrome이 안 떠 있을 때만 부른다(떠 있으면 덮인다).
// 사용자가 이미 색을 골랐으면(user_color2 존재) 존중한다.
func EnsureProfileTheme(profileDir string) error {
	if err := ensurePrefsTheme(profileDir); err != nil {
		return err
	}
	return ensureLocalStateName(profileDir)
}

// ensureLocalStateName은 Local State의 profile.info_cache.Default.name을 바꾼다.
// 파일이 없으면(첫 기동 전) 건드리지 않는다 — Chrome이 만든 뒤 다음 기동에 적용.
func ensureLocalStateName(profileDir string) error {
	path := filepath.Join(profileDir, "Local State")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var ls map[string]any
	if err := json.Unmarshal(raw, &ls); err != nil {
		return nil
	}
	profile, _ := ls["profile"].(map[string]any)
	cache, _ := profile["info_cache"].(map[string]any)
	def, _ := cache["Default"].(map[string]any)
	if def == nil || def["name"] == agentProfileName {
		return nil
	}
	if nm, _ := def["name"].(string); nm != "" && def["using_default_name"] == false {
		return nil // 사용자가 직접 지은 이름은 존중
	}
	def["name"] = agentProfileName
	def["using_default_name"] = false
	out, err := json.Marshal(ls)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}

func ensurePrefsTheme(profileDir string) error {
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
