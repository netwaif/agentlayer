package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// InstallGeminiHooks는 Antigravity CLI(agy) 전역 훅 파일
// (~/.gemini/config/hooks.json)에 agentlayer 항목을 등록한다.
// 파일 최상위 키가 훅 이름이므로 "agentlayer" 키만 소유하고, 다른 이름의
// 훅은 절대 건드리지 않는다. binPath가 바뀌면 우리 키만 갱신한다(멱등).
// 이벤트 선택 이유:
//   - PostToolUse·PreInvocation → WORK (출력 규약이 빈 객체라 동작 무간섭)
//   - Stop → DONE (decision을 안 내면 종료를 막지 않는다)
//   - PreToolUse는 등록하지 않는다 — decision 출력이 필수라 승인 흐름에 개입하게 된다.
func InstallGeminiHooks(w io.Writer, hooksPath, binPath string, dryRun bool) error {
	if binPath == "" {
		binPath = "agentlayer"
	}
	cmd := func(event string) map[string]any {
		return map[string]any{
			"type":    "command",
			"command": binPath + " hook gemini --event " + event,
			"timeout": 10,
		}
	}
	desired := map[string]any{
		"PostToolUse": []any{map[string]any{
			"matcher": "*",
			"hooks":   []any{cmd("post-tool-use")},
		}},
		"PreInvocation": []any{cmd("pre-invocation")},
		"Stop":          []any{cmd("stop")},
	}

	hooks := map[string]any{}
	raw, err := os.ReadFile(hooksPath)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return fmt.Errorf("%s 파싱 실패 — 수동 확인 필요: %w", hooksPath, err)
		}
	case os.IsNotExist(err):
		// 새로 만든다
	default:
		return err
	}

	// 멱등 비교: JSON 직렬화 결과가 같으면 무변경
	cur, _ := json.Marshal(hooks["agentlayer"])
	want, _ := json.Marshal(desired)
	if bytes.Equal(cur, want) {
		fmt.Fprintln(w, "  agentlayer: 이미 등록됨 — 건너뜀")
		return nil
	}
	hooks["agentlayer"] = desired
	fmt.Fprintf(w, "  agentlayer: PostToolUse·PreInvocation·Stop → %s hook gemini\n", binPath)

	if dryRun {
		fmt.Fprintln(w, "(dry-run — 파일을 변경하지 않았습니다)")
		return nil
	}
	if raw != nil {
		if err := os.WriteFile(hooksPath+".agentlayer.bak", raw, 0o600); err != nil {
			return fmt.Errorf("백업 실패 — 설치 중단: %w", err)
		}
	}
	out, err := json.MarshalIndent(hooks, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o755); err != nil {
		return err
	}
	tmp := hooksPath + ".agentlayer.tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, hooksPath)
}
