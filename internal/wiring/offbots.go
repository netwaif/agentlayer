package wiring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// OffBot은 "자동 기동이 꺼져 있고 지금 안 떠 있는 봇" 하나 — `agentlayer restore` 체크리스트의 봇 묶음 한 줄.
// 엔진을 가리지 않는다(Claude 폴더 봇, 코덱스·agy 브리지 봇). 두 모양이 있다:
//   - 사이드카형(Sidecar != ""): 구동 유닛이 없고 기동 명령이 <SidecarDir>/<세션>.tmux-cmd에 있다 → tmux new-session -d -s <세션> <명령>.
//   - 유닛형(Unit != ""): 구동 유닛은 있되 자동 기동만 꺼졌다(plist RunAtLoad false·systemd disabled) → 유닛 기동.
//     브리지 봇은 같은 env의 데몬 유닛(Daemon)을 짝으로 — 데몬 먼저, 그다음 TUI 유닛.
type OffBot struct {
	Session string
	Engine  string // claude | codex | gemini | "" (모름)
	Sidecar string // 사이드카형: 기동 명령(끝 개행 제거)
	Unit    string // 유닛형: 세션을 띄우는 유닛 라벨
	Daemon  string // 유닛형 브리지: 짝 데몬 유닛 라벨(없으면 빈 값)
}

// Method는 띄우는 방법 표시("사이드카"/"유닛"/"유닛(데몬 …)").
func (b OffBot) Method() string {
	switch {
	case b.Sidecar != "":
		return "사이드카"
	case b.Daemon != "":
		return "유닛(" + b.Daemon + " → " + b.Unit + ")"
	default:
		return "유닛(" + b.Unit + ")"
	}
}

// 세션 이름: plist(<string>-s</string><string>이름</string>)·셸(-s 이름)·ExecStop(kill-session -t 이름) 어느 쪽이든.
var (
	sessionFlagRe = regexp.MustCompile(`(?:^|[\s>"'])-[st](?:</string>\s*<string>|[\s=]+)([A-Za-z0-9_.-]+)`)
	envFileRe     = regexp.MustCompile(`\.env(?:\.[A-Za-z0-9_-]+)?\b`)
)

// unitSession — 유닛 본문에서 tmux 세션 이름을 뽑는다. tmux new-session이 있는 유닛만 대상(없으면 "").
func unitSession(text string) string {
	if !strings.Contains(text, "tmux") || !strings.Contains(text, "new-session") {
		return ""
	}
	// new-session 뒤의 -s가 가장 믿을 만하다. 없으면(브리지 TUI 유닛: 사이드카가 없다) kill-session -t.
	if i := strings.Index(text, "new-session"); i >= 0 {
		if m := sessionFlagRe.FindStringSubmatch(text[i:]); m != nil {
			return m[1]
		}
	}
	if m := sessionFlagRe.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

// OffBots는 꺼져 있는 봇을 찾는다. sessionExists는 tmux 세션 존재 조회(nil이면 "없음"). 세션 이름순.
func OffBots(p Paths, sessionExists func(string) bool) []OffBot {
	if sessionExists == nil {
		sessionExists = func(string) bool { return false }
	}
	engines := botEngines(p)
	units := unitTexts(p)
	byLabel := map[string]unitText{}
	for _, u := range units {
		byLabel[u.label] = u
	}
	var out []OffBot
	seen := map[string]bool{} // 세션 → 유닛형으로 이미 잡힘(또는 자동 기동이 켜진 유닛이 있음)
	for _, u := range units {
		sess := unitSession(u.text)
		if sess == "" {
			continue
		}
		if u.autostart {
			seen[sess] = true // 자동 기동 봇 — restore가 건드리지 않는다(사이드카가 같이 있어도 유닛이 정본)
			continue
		}
		if seen[sess] {
			continue
		}
		seen[sess] = true
		if sessionExists(sess) {
			continue
		}
		b := OffBot{Session: sess, Unit: u.label, Engine: engineOf(engines, sess, u.label)}
		// 브리지 짝: "<라벨>-tui" → "<라벨>", 아니면 같은 .env 파일을 쓰는 tmux 없는 유닛.
		if strings.HasSuffix(u.label, "-tui") {
			if d, ok := byLabel[strings.TrimSuffix(u.label, "-tui")]; ok && unitSession(d.text) == "" {
				b.Daemon = d.label
			}
		}
		if b.Daemon == "" {
			if env := envFileRe.FindString(u.text); env != "" {
				for _, d := range units {
					if d.label != u.label && unitSession(d.text) == "" && strings.Contains(d.text, env) {
						b.Daemon = d.label
						break
					}
				}
			}
		}
		out = append(out, b)
	}
	// 사이드카형 — 유닛이 전혀 없는 세션만.
	sideDir := p.SidecarDir
	if sideDir == "" && p.BotsJSON != "" {
		sideDir = filepath.Dir(p.BotsJSON)
	}
	if sideDir != "" {
		files, _ := filepath.Glob(filepath.Join(sideDir, "*.tmux-cmd"))
		for _, f := range files {
			sess := strings.TrimSuffix(filepath.Base(f), ".tmux-cmd")
			if sess == "" || seen[sess] {
				continue
			}
			seen[sess] = true
			if sessionExists(sess) {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			cmd := strings.TrimRight(string(b), "\r\n")
			if strings.TrimSpace(cmd) == "" {
				continue
			}
			out = append(out, OffBot{Session: sess, Sidecar: cmd, Engine: engineOf(engines, sess, "")})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Session < out[j].Session })
	return out
}

// botEngines — bots.json의 세션 → 엔진.
func botEngines(p Paths) map[string]string {
	m := map[string]string{}
	b, err := os.ReadFile(p.BotsJSON)
	if err != nil {
		return m
	}
	var bots map[string]botEntry
	if json.Unmarshal(b, &bots) != nil {
		return m
	}
	for _, e := range bots {
		if e.Session != "" && e.Engine != "" {
			m[e.Session] = e.Engine
		}
	}
	return m
}

// engineOf — bots.json이 정본, 없으면 유닛 라벨로 짐작(codex-discord → codex, agy/gemini → gemini, folder-bot → claude).
func engineOf(engines map[string]string, session, label string) string {
	if e := engines[session]; e != "" {
		return e
	}
	l := strings.ToLower(label)
	switch {
	case strings.Contains(l, "codex"):
		return "codex"
	case strings.Contains(l, "agy"), strings.Contains(l, "gemini"):
		return "gemini"
	case strings.Contains(l, "folder-bot"), strings.Contains(l, "claude"):
		return "claude"
	}
	return ""
}
