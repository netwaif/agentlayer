package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readHooks(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestInstallGeminiHooksFresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.json")
	var out bytes.Buffer
	if err := InstallGeminiHooks(&out, path, "/abs/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	m := readHooks(t, path)
	al, ok := m["agentlayer"].(map[string]any)
	if !ok {
		t.Fatalf("agentlayer 키 생성돼야 함: %+v", m)
	}
	for _, ev := range []string{"PostToolUse", "PreInvocation", "Stop"} {
		if _, ok := al[ev]; !ok {
			t.Errorf("%s 이벤트 등록돼야 함", ev)
		}
	}
	b, _ := json.Marshal(al)
	if !bytes.Contains(b, []byte("/abs/agentlayer hook gemini --event stop")) {
		t.Errorf("절대 경로 명령이어야 함: %s", b)
	}
}

func TestInstallGeminiHooksIdempotentAndPreserves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.json")
	// 사용자 기존 훅은 절대 건드리지 않는다
	seed := `{"my-linter":{"PostToolUse":[{"matcher":"run_command","hooks":[{"command":"./lint.sh"}]}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := InstallGeminiHooks(&out, path, "/abs/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	m := readHooks(t, path)
	if _, ok := m["my-linter"]; !ok {
		t.Error("기존 훅 보존돼야 함")
	}
	// 두 번째 실행은 무변경 (멱등)
	if err := InstallGeminiHooks(&out, path, "/abs/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if !bytes.Equal(first, second) {
		t.Error("재실행 시 파일 무변경이어야 함")
	}
	// binPath가 바뀌면 agentlayer 항목만 갱신 (마이그레이션)
	if err := InstallGeminiHooks(&out, path, "/new/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	third, _ := os.ReadFile(path)
	if !bytes.Contains(third, []byte("/new/agentlayer hook gemini")) {
		t.Error("새 경로로 갱신돼야 함")
	}
	if !bytes.Contains(third, []byte("my-linter")) {
		t.Error("갱신 후에도 기존 훅 보존")
	}
}

func TestInstallGeminiCLIHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	// stock Gemini CLI settings.json의 기존 설정 보존 확인
	seed := `{"security":{"auth":{"selectedType":"oauth-personal"}}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := InstallGeminiCLIHooks(&out, path, "/abs/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	m := readHooks(t, path)
	if _, ok := m["security"]; !ok {
		t.Error("기존 설정 보존돼야 함")
	}
	hooks, _ := m["hooks"].(map[string]any)
	for _, ev := range []string{"SessionStart", "BeforeAgent", "AfterTool", "AfterAgent", "Notification"} {
		if _, ok := hooks[ev]; !ok {
			t.Errorf("%s 이벤트 등록돼야 함", ev)
		}
	}
	b, _ := os.ReadFile(path)
	if !bytes.Contains(b, []byte("/abs/agentlayer hook gemini --event after-agent")) {
		t.Errorf("gemini 이벤트 명령이어야 함: %s", b)
	}
}

func TestInstallGeminiHooksDryRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.json")
	var out bytes.Buffer
	if err := InstallGeminiHooks(&out, path, "/abs/agentlayer", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("dry-run은 파일을 만들지 않는다")
	}
}
