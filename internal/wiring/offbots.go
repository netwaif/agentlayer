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
//     브리지 TUI 유닛(tui-up.sh)은 본문에 세션 이름이 없어 <브리지 루트>/<env>의 TUI_PANE에서 읽는다(resolveBridge).
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

// 세션 이름 — tmux new-session의 -s 값. plist 배열(<string>-s</string><string>이름</string>)과 셸 줄(-s 이름, 따옴표 포함) 둘 다.
var (
	plistSessionRe = regexp.MustCompile(`<string>-s</string>\s*<string>([^<]+)</string>`)
	shellSessionRe = regexp.MustCompile(`(?:^|\s)-s(?:\s+|=)(?:"([^"]*)"|'([^']*)'|([^\s"'<>;|&]+))`)
	killSessionRe  = regexp.MustCompile(`kill-session\s+-t\s+(?:"([^"]*)"|'([^']*)'|([^\s"'<>;|&]+))`)
	xmlUnescaper   = strings.NewReplacer("&quot;", `"`, "&apos;", "'", "&lt;", "<", "&gt;", ">", "&amp;", "&")
)

// unitSession — 유닛 본문에서 tmux 세션 이름을 뽑는다. tmux new-session이 있는 유닛만 대상(없으면 "").
// new-session 뒤의 첫 -s만 본다 — 뒤따르는 다른 명령의 -t·-s를 세션으로 잡지 않게. 변수($SESSION)면 모른다("").
func unitSession(text string) string {
	if !strings.Contains(text, "tmux") {
		return ""
	}
	i := strings.Index(text, "new-session")
	if i < 0 {
		return ""
	}
	rest := text[i:]
	name, at := "", -1
	if loc := plistSessionRe.FindStringSubmatchIndex(rest); loc != nil {
		name, at = xmlUnescaper.Replace(rest[loc[2]:loc[3]]), loc[0]
	}
	// 셸 줄은 plist 문자열 안에 있을 수 있어 엔티티(&quot;)를 푼 뒤 본다. 위치 비교는 "어느 쪽이 먼저 나오나"만 필요하다.
	plain := xmlUnescaper.Replace(rest)
	if loc := shellSessionRe.FindStringSubmatchIndex(plain); loc != nil && (at < 0 || loc[0] < at) {
		name = firstGroup(plain, loc)
	}
	return cleanSession(name)
}

// firstGroup — 따옴표 둘·맨 토큰 세 갈래 중 잡힌 것.
func firstGroup(s string, loc []int) string {
	for g := 2; g+1 < len(loc); g += 2 {
		if loc[g] >= 0 {
			return s[loc[g]:loc[g+1]]
		}
	}
	return ""
}

// cleanSession — tmux 정확 일치 접두(=)를 떼고, 변수가 섞였으면 모르는 것으로 본다.
func cleanSession(name string) string {
	name = strings.TrimPrefix(strings.TrimSpace(name), "=")
	if name == "" || strings.Contains(name, "$") {
		return ""
	}
	return name
}

// 브리지 TUI 유닛(codex-discord scripts/tui-up.sh) — 본문에는 스크립트 경로와 env 파일 이름만 있고 세션 이름이 없다.
// 세션은 <브리지 루트>/<env>의 TUI_PANE(<세션>:<창>.<pane>, 기본 codex-live:0.0), 엔진은 ENGINE(agy면 gemini).
var (
	tuiUpRootRe = regexp.MustCompile(`([^\s<>"']+)/scripts/tui-up\.sh`)
	tuiUpEnvRe  = regexp.MustCompile(`tui-up\.sh(?:</string>\s*<string>|[ \t]+)(\.env(?:\.[A-Za-z0-9_-]+)*)`)
	workDirRes  = []*regexp.Regexp{
		regexp.MustCompile(`<key>WorkingDirectory</key>\s*<string>([^<]+)</string>`),
		regexp.MustCompile(`(?m)^WorkingDirectory=(.+)$`),
	}
)

const tuiDefaultSession = "codex-live" // tui-up.sh: PANE="${TUI_PANE:-codex-live:0.0}"

// bridgeUnit은 브리지 TUI 유닛 하나를 푼 결과.
type bridgeUnit struct {
	session string
	engine  string // env의 ENGINE에서 — 못 읽었으면 ""
	env     string // env 파일 이름(.env, .env.<이름>)
	root    string // 브리지 루트(못 찾았으면 "")
}

// resolveBridge — tui-up.sh를 부르는 유닛이면 세션·엔진·env·루트를 푼다. 아니면 ok=false.
// env를 어디서도 못 읽으면 본문의 kill-session -t(리눅스 유닛 ExecStop)에서 세션을 읽고, 그것도 없으면 세션은 ""(짐작하지 않는다).
func resolveBridge(p Paths, text string) (bridgeUnit, bool) {
	if !strings.Contains(text, "tui-up.sh") {
		return bridgeUnit{}, false
	}
	br := bridgeUnit{env: ".env"}
	if m := tuiUpEnvRe.FindStringSubmatch(text); m != nil {
		br.env = m[1]
	}
	var roots []string
	if m := tuiUpRootRe.FindStringSubmatch(text); m != nil && filepath.IsAbs(m[1]) {
		roots = append(roots, m[1])
	}
	for _, re := range workDirRes {
		if m := re.FindStringSubmatch(text); m != nil {
			roots = append(roots, strings.TrimSpace(m[1]))
		}
	}
	if len(roots) > 0 {
		br.root = roots[0]
	}
	roots = append(roots, p.BridgeRoots...)
	for _, root := range roots {
		b, err := os.ReadFile(filepath.Join(root, br.env))
		if err != nil {
			continue
		}
		vals := envValues(string(b), "TUI_PANE", "ENGINE")
		br.root = root
		br.session = tuiDefaultSession
		if pane := vals["TUI_PANE"]; pane != "" {
			br.session = cleanSession(strings.SplitN(pane, ":", 2)[0])
		}
		br.engine = "codex"
		if e := strings.ToLower(vals["ENGINE"]); e == "agy" || e == "gemini" {
			br.engine = "gemini"
		}
		return br, true
	}
	if loc := killSessionRe.FindStringSubmatchIndex(text); loc != nil {
		br.session = cleanSession(firstGroup(text, loc))
	}
	return br, true
}

// envValues — env 파일에서 지정한 키만 읽는다(export·따옴표 허용). 다른 줄(토큰 등)은 보지 않는다.
func envValues(text string, keys ...string) map[string]string {
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		k, v, ok := strings.Cut(line, "=")
		if !ok || !want[strings.TrimSpace(k)] {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		out[strings.TrimSpace(k)] = v
	}
	return out
}

// envTokenRe — env 파일 이름이 토큰으로 정확히 나오는가(.env가 .env.gemini에, .env.qa가 .env.qa2에 걸리지 않게).
func envTokenRe(env string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[^A-Za-z0-9_.-])` + regexp.QuoteMeta(env) + `($|[^A-Za-z0-9_.-])`)
}

// OffBots는 꺼져 있는 봇을 찾는다. sessionExists는 tmux 세션 존재 조회(nil이면 "없음"). 세션 이름순.
func OffBots(p Paths, sessionExists func(string) bool) []OffBot {
	if sessionExists == nil {
		sessionExists = func(string) bool { return false }
	}
	engines := botEngines(p)
	// 1차: 유닛마다 세션을 푼다. 자동 기동이 켜진 유닛이 하나라도 띄우는 세션은 restore가 건드리지 않는다(읽는 순서와 무관).
	type resolved struct {
		unitText
		session string
		bridge  *bridgeUnit
	}
	var units []resolved
	auto := map[string]bool{}
	for _, u := range unitTexts(p) {
		r := resolved{unitText: u, session: unitSession(u.text)}
		if r.session == "" {
			if br, ok := resolveBridge(p, u.text); ok {
				r.session, r.bridge = br.session, &br
			}
		}
		if r.session != "" && u.autostart {
			auto[r.session] = true
		}
		units = append(units, r)
	}
	var out []OffBot
	seen := map[string]bool{} // 세션 → 유닛형으로 이미 잡힘
	for _, u := range units {
		if u.session == "" || u.autostart || auto[u.session] || seen[u.session] {
			continue
		}
		seen[u.session] = true
		if sessionExists(u.session) {
			continue
		}
		b := OffBot{Session: u.session, Unit: u.label}
		if u.bridge != nil {
			b.Engine = engineOf(engines, u.session, "")
			if b.Engine == "" {
				b.Engine = u.bridge.engine
			}
			// 데몬 짝(브리지 TUI 유닛만): "<라벨>-tui" → "<라벨>", 아니면 같은 루트에서 같은 env 파일을 쓰는 세션 없는 유닛.
			isDaemon := func(d resolved) bool { return d.label != u.label && d.session == "" && d.bridge == nil }
			if base := strings.TrimSuffix(u.label, "-tui"); base != u.label {
				for _, d := range units {
					if d.label == base && isDaemon(d) {
						b.Daemon = d.label
					}
				}
			}
			if b.Daemon == "" {
				tok := envTokenRe(u.bridge.env)
				for _, d := range units {
					if isDaemon(d) && tok.MatchString(d.text) && (u.bridge.root == "" || strings.Contains(d.text, u.bridge.root)) {
						b.Daemon = d.label
						break
					}
				}
			}
		}
		if b.Engine == "" {
			b.Engine = engineOf(engines, u.session, u.label)
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
			if sess == "" || seen[sess] || auto[sess] {
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
			eng := e.Engine
			if eng == "agy" {
				eng = "gemini" // 표기는 claude | codex | gemini
			}
			m[e.Session] = eng
		}
	}
	return m
}

// engineOf — bots.json이 정본, 없으면 유닛 라벨로 짐작(agy/gemini → gemini, codex-discord → codex, folder-bot → claude).
// 브리지 라벨은 com.codex-discord.<이름>이라 agy 봇에도 "codex"가 들어 있다 — agy/gemini를 먼저 본다.
func engineOf(engines map[string]string, session, label string) string {
	if e := engines[session]; e != "" {
		return e
	}
	l := strings.ToLower(label)
	switch {
	case l == "":
		return ""
	case strings.Contains(l, "agy"), strings.Contains(l, "gemini"):
		return "gemini"
	case strings.Contains(l, "codex"):
		return "codex"
	case strings.Contains(l, "folder-bot"), strings.Contains(l, "claude"):
		return "claude"
	}
	return ""
}
