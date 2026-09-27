package cli

// browserUsage — browser는 서브커맨드별 usage가 흩어져 있어 요약만 낸다.
const browserUsage = `사용법: agentlayer browser <open|pick|shot|errors|preview|cookies|mcp|control reset|restart> [...]
  상세는 'agentlayer help'의 browser 줄과 각 서브커맨드의 오류 메시지 참고`

// SubUsage는 --help 가로채기 대상 하위 명령의 사용법. send·broadcast·wake-all·close-all은 본문 인자에
// "-h"가 올 수 있어 대상에서 뺀다.
func SubUsage(cmd string) (string, bool) {
	switch cmd {
	case "task":
		return taskUsage, true
	case "remote":
		return remoteUsage, true
	case "wt":
		return wtUsage, true
	case "board":
		return boardUsage, true
	case "browser":
		return browserUsage, true
	case "channel":
		return channelUsage, true
	}
	return "", false
}

// HasHelpFlag는 인자 중 정확히 "--help" 또는 "-h"가 있으면 true.
func HasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

// HelpIntercept는 main이 하위 명령으로 분기하기 전에 부른다. 사용법이 있고 인자에 --help/-h가 있으면 (usage, true).
// 본문(편지·답장)을 위치 인자로 받는 task message·task reply는 "-h"가 본문일 수 있어 가로채지 않는다.
func HelpIntercept(args []string) (string, bool) {
	if len(args) < 2 || !HasHelpFlag(args[1:]) {
		return "", false
	}
	if args[0] == "task" && (args[1] == "message" || args[1] == "reply") {
		return "", false
	}
	return SubUsage(args[0])
}
