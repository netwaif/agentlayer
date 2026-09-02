package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readPrefs(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "Default", "Preferences"))
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// 첫 기동 전(프로필 없음)에도 Preferences를 만들어 테마를 심는다.
func TestEnsureProfileThemeCreates(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureProfileTheme(dir); err != nil {
		t.Fatal(err)
	}
	d := readPrefs(t, dir)
	theme := d["browser"].(map[string]any)["theme"].(map[string]any)
	if theme["is_grayscale2"] != true || theme["color_scheme2"] != float64(2) {
		t.Errorf("테마 키 없음: %v", theme)
	}
	if d["profile"].(map[string]any)["name"] != "AgentLayer" {
		t.Errorf("프로필 이름: %v", d["profile"])
	}
}

// 기존 Preferences의 다른 키는 보존하고, 사용자가 이미 색을 골랐으면 건드리지 않는다.
func TestEnsureProfileThemePreservesUserChoice(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "Default"), 0o755)
	os.WriteFile(filepath.Join(dir, "Default", "Preferences"),
		[]byte(`{"session":{"restore_on_startup":1},"browser":{"theme":{"user_color2":-123,"color_scheme2":2}}}`), 0o600)
	if err := EnsureProfileTheme(dir); err != nil {
		t.Fatal(err)
	}
	d := readPrefs(t, dir)
	if d["session"] == nil {
		t.Error("기존 키 유실")
	}
	theme := d["browser"].(map[string]any)["theme"].(map[string]any)
	if theme["user_color2"] != float64(-123) || theme["color_scheme2"] != float64(2) {
		t.Errorf("사용자 선택을 덮어씀: %v", theme)
	}
}

// 프로필 표시 이름은 Local State(profile.info_cache.Default.name)가 정본 —
// Preferences의 profile.name만으로는 "내 Chrome"으로 남는다.
func TestEnsureProfileThemeNamesProfileInLocalState(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Local State"),
		[]byte(`{"profile":{"info_cache":{"Default":{"name":"내 Chrome","using_default_name":true,"avatar_icon":"x"}},"last_used":"Default"},"os_crypt":{"k":1}}`), 0o600)
	if err := EnsureProfileTheme(dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "Local State"))
	var d map[string]any
	json.Unmarshal(raw, &d)
	def := d["profile"].(map[string]any)["info_cache"].(map[string]any)["Default"].(map[string]any)
	if def["name"] != "AgentLayer" || def["using_default_name"] != false || def["avatar_icon"] != "x" {
		t.Errorf("Local State 프로필: %v", def)
	}
	if d["os_crypt"] == nil {
		t.Error("다른 키 유실")
	}
}

// Local State가 아직 없으면(첫 기동 전) 만들지 않는다 — Chrome이 만든 뒤 다음 기동에 적용.
func TestEnsureProfileThemeSkipsMissingLocalState(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureProfileTheme(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Local State")); err == nil {
		t.Error("Local State를 새로 만들면 안 됨")
	}
}

// pkill 뒤 재기동 시 이전 탭 복원 금지 — exit_type Normal·restore_on_startup 5.
func TestEnsureProfileThemeDisablesCrashRestore(t *testing.T) {
	dir := t.TempDir()
	prefPath := filepath.Join(dir, "Default", "Preferences")
	_ = os.MkdirAll(filepath.Dir(prefPath), 0o755)
	_ = os.WriteFile(prefPath, []byte(`{"profile":{"exit_type":"Crashed","name":"AgentLayer"},"browser":{"theme":{"is_grayscale2":true}}}`), 0o600)
	if err := EnsureProfileTheme(dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(prefPath)
	var prefs map[string]any
	_ = json.Unmarshal(raw, &prefs)
	profile := prefs["profile"].(map[string]any)
	if profile["exit_type"] != "Normal" || profile["exited_cleanly"] != true {
		t.Errorf("크래시 복원이 꺼져야 함: %v", profile)
	}
	if session, _ := prefs["session"].(map[string]any); session == nil || session["restore_on_startup"] != float64(5) {
		t.Errorf("restore_on_startup=5 여야 함: %v", prefs["session"])
	}
}
