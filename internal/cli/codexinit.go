package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const codexNotifyBare = `notify = ["agentlayer", "hook", "codex"]`

func codexNotifyLine(binPath string) string {
	if binPath == "" {
		binPath = "agentlayer"
	}
	return `notify = ["` + binPath + `", "hook", "codex"]`
}

// InstallCodexNotify는 ~/.codex/config.toml에 notify 설정을 추가한다.
// TOML 최상위 키는 첫 섹션 헤더([...]) 앞에 있어야 하므로 그 위치에
// 삽입한다. 남의 notify는 건드리지 않지만, 이전 버전이 넣은 이름-only
// agentlayer 항목은 절대 경로로 마이그레이션한다(PATH 최소 환경 대응).
func InstallCodexNotify(w io.Writer, configPath, binPath string, dryRun bool) error {
	line := codexNotifyLine(binPath)
	raw, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		if dryRun {
			fmt.Fprintf(w, "  %s 생성 예정: %s\n", configPath, line)
			return nil
		}
		return os.WriteFile(configPath, []byte(line+"\n"), 0o600)
	}
	if err != nil {
		return err
	}
	content := string(raw)
	for _, l := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "[") {
			break // 최상위 영역 끝 — notify 없음 확정
		}
		if !strings.HasPrefix(trimmed, "notify") {
			continue
		}
		if trimmed == codexNotifyBare {
			// 이름-only 옛 항목 → 절대 경로 교체
			if trimmed == line {
				fmt.Fprintln(w, "  codex notify: 이미 등록됨 — 건너뜀")
				return nil
			}
			fmt.Fprintln(w, "  codex notify: 절대 경로로 마이그레이션")
			if dryRun {
				fmt.Fprintln(w, "  (dry-run — 파일을 변경하지 않았습니다)")
				return nil
			}
			if err := os.WriteFile(configPath+".agentlayer.bak", raw, 0o600); err != nil {
				return fmt.Errorf("백업 실패 — 설치 중단: %w", err)
			}
			updated := strings.Replace(content, codexNotifyBare, line, 1)
			tmp := configPath + ".agentlayer.tmp"
			if err := os.WriteFile(tmp, []byte(updated), 0o600); err != nil {
				return err
			}
			return os.Rename(tmp, configPath)
		}
		if trimmed == line {
			fmt.Fprintln(w, "  codex notify: 이미 등록됨 — 건너뜀")
			return nil
		}
		fmt.Fprintln(w, "  codex notify: 이미 설정됨 — 건너뜀 (기존 설정 우선)")
		return nil
	}
	fmt.Fprintf(w, "  codex notify: %s\n", line)
	if dryRun {
		fmt.Fprintln(w, "  (dry-run — 파일을 변경하지 않았습니다)")
		return nil
	}
	if err := os.WriteFile(configPath+".agentlayer.bak", raw, 0o600); err != nil {
		return fmt.Errorf("백업 실패 — 설치 중단: %w", err)
	}
	// 첫 섹션 앞(최상위)에 삽입
	idx := len(content)
	for i, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			lines := strings.Split(content, "\n")
			idx = len(strings.Join(lines[:i], "\n"))
			break
		}
	}
	updated := content[:idx] + line + "\n" + content[idx:]
	if idx == len(content) && !strings.HasSuffix(content, "\n") {
		updated = content + "\n" + line + "\n"
	}
	tmp := configPath + ".agentlayer.tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, configPath)
}

// codexHookMark는 hooks.json에서 우리 항목을 알아보는 표식 — 명령 문자열에 이게 있으면 agentlayer 것.
const codexHookMark = " hook codex --event "

// codexHookEvents는 등록할 codex hook 이벤트와 --event 이름.
// notify(agent-turn-complete)는 턴 완료만 알려 codex가 일하는 동안 DONE으로 보였다
// (2026-09-03). PreToolUse는 빼서 승인 흐름에 개입하지 않는다.
var codexHookEvents = [][2]string{
	{"SessionStart", "session-start"},
	{"UserPromptSubmit", "user-prompt-submit"},
	{"PostToolUse", "post-tool-use"},
	{"PermissionRequest", "permission-request"},
	{"Stop", "stop"},
}

// InstallCodexHooks는 ~/.codex/hooks.json에 agentlayer 훅을 등록한다.
// 파일 구조: {"hooks": {"<Event>": [{"matcher"?, "hooks": [{type,command,timeout}]}]}}.
// 이벤트마다 우리 매처 그룹 하나만 두고(명령에 codexHookMark), 남의 그룹은 안 건드린다.
// binPath가 바뀌면 우리 그룹만 갱신한다(멱등). 새 훅은 codex가 신뢰 확인(/hooks)을
// 요구하므로 설치 뒤 안내를 찍는다.
func InstallCodexHooks(w io.Writer, hooksPath, binPath string, dryRun bool) error {
	if binPath == "" {
		binPath = "agentlayer"
	}
	root := map[string]any{}
	raw, err := os.ReadFile(hooksPath)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &root); err != nil {
			return fmt.Errorf("%s 파싱 실패 — 수동 확인 필요: %w", hooksPath, err)
		}
	case os.IsNotExist(err):
	default:
		return err
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	changed := false
	for _, ev := range codexHookEvents {
		want := map[string]any{"hooks": []any{map[string]any{
			"type": "command", "command": binPath + codexHookMark + ev[1], "timeout": 10,
		}}}
		groups, _ := hooks[ev[0]].([]any)
		var kept []any
		found := false
		for _, g := range groups {
			if isCodexAgentlayerGroup(g) {
				cur, _ := json.Marshal(g)
				exp, _ := json.Marshal(want)
				if bytes.Equal(cur, exp) && !found {
					kept = append(kept, g)
					found = true
					continue
				}
				changed = true // 옛 경로·중복 → 교체
				continue
			}
			kept = append(kept, g)
		}
		if !found {
			kept = append(kept, want)
			changed = true
		}
		hooks[ev[0]] = kept
	}
	if !changed {
		fmt.Fprintln(w, "  agentlayer: 이미 등록됨 — 건너뜀")
		return nil
	}
	root["hooks"] = hooks
	fmt.Fprintf(w, "  agentlayer: SessionStart·UserPromptSubmit·PostToolUse·PermissionRequest·Stop → %s hook codex\n", binPath)
	if dryRun {
		fmt.Fprintln(w, "(dry-run — 파일을 변경하지 않았습니다)")
		return nil
	}
	if raw != nil {
		if err := os.WriteFile(hooksPath+".agentlayer.bak", raw, 0o600); err != nil {
			return fmt.Errorf("백업 실패 — 설치 중단: %w", err)
		}
	}
	out, err := json.MarshalIndent(root, "", "  ")
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
	if err := os.Rename(tmp, hooksPath); err != nil {
		return err
	}
	fmt.Fprintln(w, "  ⚠ codex는 새 훅을 신뢰 확인 전엔 실행하지 않습니다 — codex 안에서 /hooks 로 agentlayer 항목을 한 번 허용하세요")
	return nil
}

// isCodexAgentlayerGroup은 매처 그룹의 훅 명령에 codexHookMark가 있으면 참.
func isCodexAgentlayerGroup(g any) bool {
	m, _ := g.(map[string]any)
	list, _ := m["hooks"].([]any)
	for _, h := range list {
		hm, _ := h.(map[string]any)
		if cmd, _ := hm["command"].(string); strings.Contains(cmd, codexHookMark) {
			return true
		}
	}
	return false
}

const (
	codexAgentsStart = "<!-- agentlayer:browser:start -->"
	codexAgentsEnd   = "<!-- agentlayer:browser:end -->"
)

// codexAgentsBlock은 ~/.codex/AGENTS.md에 심는 브라우저 지침. codex는 브라우저 요청을
// ChatGPT 앱 내장 "인앱 브라우저 제어" 스킬로 처리하려다 "No browser is available"로
// 실패한다(2026-09-02·09-03 실측). 사용자가 "chrome-devtools 이용해"라고 하면 되는데,
// 그 한마디를 기본값으로 만든다.
const codexAgentsBlock = codexAgentsStart + `
## 에이전트 브라우저 (AgentLayer)

- 웹 페이지를 열고·보고·클릭하고·스크린샷을 찍는 일은 **chrome-devtools MCP**(` + "`list_pages`·`new_page`·`navigate_page`·`take_screenshot`·`click`·`fill`·`evaluate_script`" + `)로 한다. 이 MCP가 AgentLayer 전용 Chrome(에이전트 브라우저)에 붙어 있다.
- ChatGPT 앱 내장 브라우저 스킬(인앱 브라우저 제어, ` + "`agent.browsers.list()`" + ` 류 node_repl 브라우저 API)은 쓰지 않는다 — 그건 이 브라우저를 모르고 "No browser is available"로 끝난다.
- 브라우저가 안 떠 있어도 MCP 서버가 띄우므로 기동 명령은 없다. 자기 탭(` + "`new_page`" + `의 pageId)에서만 작업하고 남의 탭은 이동·닫지 않는다.
- 사람이 같이 보는 창이다 — 묻지 않고 ` + "`resize_page`·`emulate`" + `로 창 크기를 바꾸지 않는다.
- "뭔가 잘못된 것 같다"처럼 증상만 오면 사람에게 로그를 요구하지 말고 ` + "`list_console_messages`" + `(JS 예외·console.error)와 ` + "`list_network_requests`" + `(404·CORS)를 직접 읽는다. 보이는 증상 하나에 에러가 여럿인 경우가 흔하니 전부 짚고 원인별로 고친 뒤 같은 탭을 리로드해 확인한다.
- 로그인 쿠키는 셸 명령으로 다룬다(값은 안 찍힘): "에이전트 브라우저에 뭐 들어 있어?" → ` + "`~/.local/bin/agentlayer browser cookies list`" + `(호스트별 개수) / ` + "`cookies list <도메인>`" + `(이름·만료), "OO 로그인 쿠키 가져와줘" → ` + "`cookies import <도메인>`" + `(macOS Keychain 팝업이 뜨니 "항상 허용"을 누르라고 먼저 말한다), "OO 쿠키 지워줘" → ` + "`cookies clear <도메인>`" + `. 실사용 Chrome은 절대 바꾸지 않는다.
- 스크린샷 파일은 ` + "`take_screenshot`" + `의 ` + "`filePath`" + `로 **현재 작업 폴더 안**에 저장한다(그 밖은 MCP가 거부한다).
` + codexAgentsEnd + "\n"

// InstallCodexAgents는 ~/.codex/AGENTS.md에 브라우저 지침 블록을 심는다.
// 마커 사이만 소유한다 — 있으면 내용 비교 후 교체, 없으면 끝에 덧붙인다. 다른 내용은 안 건드린다.
func InstallCodexAgents(w io.Writer, agentsPath string, dryRun bool) error {
	raw, err := os.ReadFile(agentsPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(raw)
	var updated string
	if i := strings.Index(content, codexAgentsStart); i >= 0 {
		j := strings.Index(content[i:], codexAgentsEnd)
		if j < 0 {
			return fmt.Errorf("%s: 시작 마커만 있고 끝 마커가 없음 — 수동 확인 필요", agentsPath)
		}
		end := i + j + len(codexAgentsEnd)
		if end < len(content) && content[end] == '\n' {
			end++
		}
		if content[i:end] == codexAgentsBlock {
			fmt.Fprintln(w, "  브라우저 지침: 이미 설치됨 — 건너뜀")
			return nil
		}
		updated = content[:i] + codexAgentsBlock + content[end:]
		fmt.Fprintln(w, "  브라우저 지침: 갱신")
	} else {
		sep := ""
		if content != "" && !strings.HasSuffix(content, "\n") {
			sep = "\n"
		}
		if content != "" {
			sep += "\n"
		}
		updated = content + sep + codexAgentsBlock
		fmt.Fprintln(w, "  브라우저 지침: 추가 (chrome-devtools MCP 사용·인앱 브라우저 스킬 금지)")
	}
	if dryRun {
		fmt.Fprintln(w, "(dry-run — 파일을 변경하지 않았습니다)")
		return nil
	}
	if raw != nil {
		if err := os.WriteFile(agentsPath+".agentlayer.bak", raw, 0o600); err != nil {
			return fmt.Errorf("백업 실패 — 설치 중단: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(agentsPath), 0o755); err != nil {
		return err
	}
	tmp := agentsPath + ".agentlayer.tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, agentsPath)
}
