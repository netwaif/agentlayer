// Package wiring은 에이전트 하나의 "배선"을 읽기 전용으로 수집한다:
// folder-bot 등록 정보, 담당 Discord 채널·정책, 구동 LaunchAgent.
// 어떤 파일도 수정하지 않는다 — 관리(등록·pairing)는 각 도구의 영역이다.
package wiring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// Channel은 봇이 물려 있는 Discord 채널/그룹 하나.
type Channel struct {
	ID             string
	Label          string // config channel_labels에서, 없으면 빈 값
	RequireMention bool
	AllowCount     int // 허용된 사용자 수
}

// Discord는 봇의 Discord 연결 요약.
type Discord struct {
	DMPolicy string
	Channels []Channel
}

// Bridge는 codex/gemini Discord 브리지 연결 (CODEX_WORKDIR 매칭).
type Bridge struct {
	Dir      string   // 브리지 루트
	Alive    bool     // daemon.pid 생존
	Channels []string // .env의 *CHANNEL* 키에서 모은 채널 ID들
}

// Info는 에이전트 하나의 배선 전체.
type Info struct {
	BotName      string // folder-bot 등록 이름 (미등록이면 빈 값)
	Engine       string
	Discord      *Discord // .discord-state 없으면 nil
	Bridge       *Bridge  // codex 브리지로 연결된 경우
	LaunchAgents []string // 이 세션·폴더를 언급하는 plist 라벨들
}

// DiscordConnected는 어떤 형태로든 Discord로 조종되는지 (⌁ 마크 기준).
func (i Info) DiscordConnected() bool {
	if i.Discord != nil || i.Bridge != nil {
		return true
	}
	for _, la := range i.LaunchAgents {
		if strings.Contains(la, "discord") {
			return true
		}
	}
	return false
}

// Paths는 수집 소스 위치. 테스트에서 교체한다.
type Paths struct {
	BotsJSON        string   // ~/.config/folder-bot/bots.json
	LaunchAgentsDir string   // ~/Library/LaunchAgents
	BridgeRoots     []string // codex-discord 브리지 루트 후보
}

func DefaultPaths() Paths {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}
	}
	return Paths{
		BotsJSON:        filepath.Join(home, ".config", "folder-bot", "bots.json"),
		LaunchAgentsDir: filepath.Join(home, "Library", "LaunchAgents"),
		BridgeRoots: []string{
			filepath.Join(home, "ai-folder", "dev", "codex-discord"),
			filepath.Join(home, "codex-discord"),
		},
	}
}

type botEntry struct {
	Engine  string `json:"engine"`
	Folder  string `json:"folder"`
	Session string `json:"session"`
}

type accessFile struct {
	DMPolicy  string   `json:"dmPolicy"`
	AllowFrom []string `json:"allowFrom"`
	Groups    map[string]struct {
		RequireMention bool     `json:"requireMention"`
		AllowFrom      []string `json:"allowFrom"`
	} `json:"groups"`
}

// Collect는 folder(에이전트 CWD)와 session 이름으로 배선을 모은다.
// 소스가 없으면 해당 항목만 비운다 — 수집 실패로 관제를 멈추지 않는다.
func Collect(p Paths, folder, session string, labels map[string]string) Info {
	info := Info{}

	// 1) folder-bot 등록 — folder 또는 session 일치
	if b, err := os.ReadFile(p.BotsJSON); err == nil {
		var bots map[string]botEntry
		if json.Unmarshal(b, &bots) == nil {
			for name, e := range bots {
				if e.Folder == folder || (session != "" && e.Session == session) {
					info.BotName = name
					info.Engine = e.Engine
					break
				}
			}
		}
	}

	// 2) Discord 채널 — 폴더의 .discord-state/access.json
	if b, err := os.ReadFile(filepath.Join(folder, ".discord-state", "access.json")); err == nil {
		var af accessFile
		if json.Unmarshal(b, &af) == nil {
			d := &Discord{DMPolicy: af.DMPolicy}
			var ids []string
			for id := range af.Groups {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				g := af.Groups[id]
				d.Channels = append(d.Channels, Channel{
					ID: id, Label: labels[id],
					RequireMention: g.RequireMention,
					AllowCount:     len(g.AllowFrom),
				})
			}
			info.Discord = d
		}
	}

	// 2.5) codex 브리지 — 브리지 루트의 .env* 중 CODEX_WORKDIR가 이 폴더를 가리키면 연결
	for _, root := range p.BridgeRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if name != ".env" && !strings.HasPrefix(name, ".env.") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				continue
			}
			if !envPointsTo(string(b), folder) {
				continue
			}
			dataDir := "data"
			if suffix := strings.TrimPrefix(name, ".env"); suffix != "" {
				dataDir = "data" + strings.Replace(suffix, ".", "-", 1)
			}
			info.Bridge = &Bridge{Dir: root,
				Alive:    pidAlive(filepath.Join(root, dataDir, "daemon.pid")),
				Channels: envChannels(string(b))}
		}
	}

	// 3) 구동 LaunchAgent — 세션 이름·폴더·(브리지면) 브리지 경로를 언급하는 plist
	needles := []string{}
	if session != "" {
		needles = append(needles, session)
	}
	if folder != "" {
		needles = append(needles, folder)
	}
	if info.Bridge != nil {
		needles = append(needles, info.Bridge.Dir)
	}
	if entries, err := os.ReadDir(p.LaunchAgentsDir); err == nil {
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".plist") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(p.LaunchAgentsDir, e.Name()))
			if err != nil {
				continue
			}
			s := string(b)
			for _, n := range needles {
				if strings.Contains(s, n) {
					info.LaunchAgents = append(info.LaunchAgents,
						strings.TrimSuffix(e.Name(), ".plist"))
					break
				}
			}
		}
	}
	return info
}

// envPointsTo는 .env 내용의 *WORKDIR 값이 folder와 일치하는지.
func envPointsTo(env, folder string) bool {
	for _, line := range strings.Split(env, "\n") {
		if i := strings.Index(line, "WORKDIR="); i >= 0 {
			if strings.TrimSpace(line[i+len("WORKDIR="):]) == folder {
				return true
			}
		}
	}
	return false
}

// envChannels는 .env에서 채널 ID들을 모은다 (키에 CHANNEL 포함, 값은 숫자).
func envChannels(env string) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(env, "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || !strings.Contains(key, "CHANNEL") {
			continue
		}
		for _, tok := range strings.Split(val, ",") {
			tok = strings.TrimSpace(tok)
			if tok == "" || seen[tok] {
				continue
			}
			if _, err := strconv.ParseUint(tok, 10, 64); err == nil {
				seen[tok] = true
				out = append(out, tok)
			}
		}
	}
	sort.Strings(out)
	return out
}

// pidAlive는 pid 파일의 프로세스가 살아 있는지 (신호 0).
func pidAlive(pidFile string) bool {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// ShortID는 채널 ID를 표시용으로 축약한다 (앞 6자리…).
func ShortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:6] + "…"
}
