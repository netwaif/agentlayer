package scan

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Proc은 프로세스 표의 한 줄(ps -axo pid=,ppid=,args=).
type Proc struct {
	PID, PPID int
	Args      string
}

// ProcTable은 pid → 프로세스. 스캐너가 pane_current_command만으로 판정이 안 될 때
// (npm으로 깐 codex·gemini-cli는 pane 전면 프로세스가 `node` 래퍼다) pane_pid의
// 자손을 뒤져 2차 판정하는 데 쓴다.
type ProcTable map[int]Proc

// loadProcTable은 테스트에서 가짜 표로 바꿔 끼운다.
var loadProcTable = LoadProcTable

// LoadProcTable은 ps로 전체 프로세스 표를 읽는다(macOS·리눅스 공통 옵션).
// 실패하면 빈 표 — 판정은 1차(command)만으로 돌아간다.
func LoadProcTable() ProcTable {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,args=").Output()
	if err != nil {
		return ProcTable{}
	}
	return ParseProcTable(string(out))
}

// ParseProcTable은 "pid ppid args…" 줄들을 표로 만든다.
func ParseProcTable(out string) ProcTable {
	pt := ProcTable{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		pt[pid] = Proc{PID: pid, PPID: ppid, Args: strings.Join(f[2:], " ")}
	}
	return pt
}

// wrapperCommands: 이 이름이 pane 전면이면 실제 에이전트는 인자나 자식에 있다.
var wrapperCommands = map[string]bool{"node": true, "bun": true, "npm": true, "npx": true}

// IsWrapperCommand는 pane_current_command가 런타임 래퍼인지.
func IsWrapperCommand(cmd string) bool {
	return wrapperCommands[strings.ToLower(cmd)]
}

// maxDescendantDepth: pane_pid가 셸이면 자식이 node 래퍼(인자에 bin/codex)이고,
// pane_pid가 래퍼 자신이면 자식이 네이티브 바이너리다 — 깊이 1이면 둘 다 잡힌다.
// 더 깊이 보면 에이전트가 Bash로 띄운 다른 CLI를 오판할 수 있다.
const maxDescendantDepth = 1

// DescendantKind는 pid 자신과 그 자손(가까운 순)의 인자에서 에이전트 종류를 찾는다.
func (pt ProcTable) DescendantKind(pid int) string {
	if pid <= 0 || len(pt) == 0 {
		return ""
	}
	children := map[int][]int{}
	for _, p := range pt {
		children[p.PPID] = append(children[p.PPID], p.PID)
	}
	level := []int{pid}
	for depth := 0; depth <= maxDescendantDepth && len(level) > 0; depth++ {
		var next []int
		for _, id := range level {
			if p, ok := pt[id]; ok {
				if k := KindFromArgs(p.Args); k != "" {
					return k
				}
			}
			next = append(next, children[id]...)
		}
		level = next
	}
	return ""
}

// KindFromArgs는 명령행에서 에이전트 종류를 읽는다.
// `node /…/bin/codex` 처럼 래퍼 뒤 첫 인자, 또는 실행 파일 자체의 basename이
// claude·codex·gemini·agy면 그것. npm 패키지 경로도 신호로 쓴다.
func KindFromArgs(args string) string {
	tokens := strings.Fields(args)
	for i, tok := range tokens {
		if i > 1 {
			break
		}
		switch strings.ToLower(filepath.Base(tok)) {
		case "claude":
			return "claude"
		case "codex":
			return "codex"
		case "gemini", "agy":
			return "gemini"
		}
	}
	switch {
	case strings.Contains(args, "@anthropic-ai/claude-code"):
		return "claude"
	case strings.Contains(args, "@openai/codex"):
		return "codex"
	case strings.Contains(args, "@google/gemini-cli"):
		return "gemini"
	}
	return ""
}
