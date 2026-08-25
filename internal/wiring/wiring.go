// Package wiring은 에이전트 하나의 "배선"을 읽기 전용으로 수집한다:
// folder-bot 등록 정보, 담당 Discord 채널·정책, 구동 LaunchAgent.
// 어떤 파일도 수정하지 않는다 — 관리(등록·pairing)는 각 도구의 영역이다.
package wiring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// Info는 에이전트 하나의 배선 전체.
type Info struct {
	BotName      string // folder-bot 등록 이름 (미등록이면 빈 값)
	Engine       string
	Discord      *Discord // .discord-state 없으면 nil
	LaunchAgents []string // 이 세션·폴더를 언급하는 plist 라벨들
}

// Paths는 수집 소스 위치. 테스트에서 교체한다.
type Paths struct {
	BotsJSON        string // ~/.config/folder-bot/bots.json
	LaunchAgentsDir string // ~/Library/LaunchAgents
}

func DefaultPaths() Paths {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}
	}
	return Paths{
		BotsJSON:        filepath.Join(home, ".config", "folder-bot", "bots.json"),
		LaunchAgentsDir: filepath.Join(home, "Library", "LaunchAgents"),
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

	// 3) 구동 LaunchAgent — 세션 이름 또는 폴더 경로를 언급하는 plist
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
			if (session != "" && strings.Contains(s, session)) ||
				(folder != "" && strings.Contains(s, folder)) {
				info.LaunchAgents = append(info.LaunchAgents,
					strings.TrimSuffix(e.Name(), ".plist"))
			}
		}
	}
	return info
}

// ShortID는 채널 ID를 표시용으로 축약한다 (앞 6자리…).
func ShortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:6] + "…"
}
