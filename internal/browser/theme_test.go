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
	if theme["user_color2"] == nil || theme["color_scheme2"] != float64(1) {
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
