package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const existingSettings = `{
  "model": "opus",
  "hooks": {
    "Notification": [
      {"hooks": [{"type": "command", "command": "coach --hook"}]}
    ],
    "PostToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "my-logger"}]}
    ]
  },
  "permissions": {"allow": ["Bash(ls:*)"]}
}`

func writeSettings(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInitAppendsPreservingExisting(t *testing.T) {
	p := writeSettings(t, existingSettings)
	var buf bytes.Buffer
	if err := InstallClaudeHooks(&buf, p, "/abs/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	var s map[string]any
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("설치 후에도 유효 JSON: %v", err)
	}
	// 기존 설정 보존
	if s["model"] != "opus" {
		t.Error("기존 model 보존")
	}
	out := string(b)
	for _, want := range []string{"coach --hook", "my-logger", "Bash(ls:*)"} {
		if !strings.Contains(out, want) {
			t.Errorf("기존 항목 %q 보존돼야 함", want)
		}
	}
	// 4개 이벤트 모두 등록
	for _, ev := range []string{"post-tool-use", "notification", "stop", "session-start", "user-prompt-submit"} {
		if !strings.Contains(out, "/abs/agentlayer hook claude --event "+ev) {
			t.Errorf("%s hook 등록돼야 함", ev)
		}
	}
	// 백업 생성
	if _, err := os.Stat(p + ".agentlayer.bak"); err != nil {
		t.Error("백업 파일 있어야 함")
	}
}

func TestInitIdempotent(t *testing.T) {
	p := writeSettings(t, existingSettings)
	var buf bytes.Buffer
	if err := InstallClaudeHooks(&buf, p, "/abs/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(p)
	if err := InstallClaudeHooks(&buf, p, "/abs/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(p)
	if string(first) != string(second) {
		t.Error("두 번 실행해도 결과 동일(중복 등록 없음)")
	}
}

func TestInitDryRunNoChanges(t *testing.T) {
	p := writeSettings(t, existingSettings)
	var buf bytes.Buffer
	if err := InstallClaudeHooks(&buf, p, "/abs/agentlayer", true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != existingSettings {
		t.Error("dry-run은 파일을 바꾸지 않음")
	}
	if !strings.Contains(buf.String(), "dry-run") {
		t.Error("dry-run 안내 출력")
	}
}

func TestInitMissingSettingsCreates(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	var buf bytes.Buffer
	if err := InstallClaudeHooks(&buf, p, "/abs/agentlayer", false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal("settings.json 새로 생성돼야 함")
	}
	if !strings.Contains(string(b), "/abs/agentlayer hook claude") {
		t.Error("hook 등록")
	}
}

func TestTmuxBindingAdvice(t *testing.T) {
	var buf bytes.Buffer
	PrintTmuxBinding(&buf, "", "/Users/x/.local/bin/agentlayer") // 바인딩 없음 케이스
	out := buf.String()
	if !strings.Contains(out, "bind-key a display-popup") {
		t.Errorf("바인딩 안내 포함: %s", out)
	}
	if !strings.Contains(out, "/Users/x/.local/bin/agentlayer") {
		t.Errorf("절대 경로로 안내해야 함 (tmux 서버 최소 PATH): %s", out)
	}
	// 팝업은 클라이언트가 커져도 안 따라오므로 client-resized 훅으로 재오픈한다
	for _, want := range []string{"-e AGENTLAYER_POPUP=1", "set-hook -g client-resized", "popup-refresh #{client_name}"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q 안내 누락: %s", want, out)
		}
	}
	buf.Reset()
	PrintTmuxBinding(&buf, "bind-key -T prefix a send-prefix", "/x") // 남의 바인딩: 충돌 경고
	if !strings.Contains(buf.String(), "다른 키를 고르세요") || !strings.Contains(buf.String(), "send-prefix") {
		t.Errorf("충돌 경고 + 현재 바인딩 표시: %s", buf.String())
	}
	buf.Reset()
	// 자기 바인딩(init 재실행): 경고가 아니라 "이미 등록됨 — 건너뜀" (WSL2 rc2 검증 피드백)
	own := `bind-key -T prefix a display-popup -E -e AGENTLAYER_POPUP=1 -h "80%" -w "90%" /home/x/.local/bin/agentlayer`
	PrintTmuxBinding(&buf, own, "/home/x/.local/bin/agentlayer")
	if !strings.Contains(buf.String(), "이미 등록됨 — 건너뜀") || strings.Contains(buf.String(), "다른 키") {
		t.Errorf("자기 바인딩은 건너뜀으로: %s", buf.String())
	}
}
