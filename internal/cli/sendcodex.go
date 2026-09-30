package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/netwaif/agentlayer/internal/config"
	"github.com/netwaif/agentlayer/internal/usage"
)

// 코덱스 데스크톱 앱 세션에는 훅도 tmux도 없어 상태 저장소에 레코드가 없다. 그러나 rollout(~/.codex/sessions)은 남고
// `codex queue --thread <ID>`는 떠 있는 어떤 세션에든 들어간다. 그래서 `send <세션 ID>`는 기존 해석(ResolveTarget)이
// 실패한 뒤에만 이 파일의 직송 경로를 탄다 — 예전에는 오류였던 입력이다.

// minSessionIDPrefix — 세션 ID 접두로 대상을 찾을 때 요구하는 최소 길이(UUID 첫 묶음). 짧은 접두가 우연히 맞는 일을 막는다.
const minSessionIDPrefix = 8

// LooksLikeSessionID — UUID(8-4-4-4-12, 16진수) 전체이거나 그 앞자리 접두(8자 이상)인가. 세션 이름과 구별하는 기준이라
// 16진수·하이픈만 허용한다.
func LooksLikeSessionID(s string) bool {
	if len(s) < minSessionIDPrefix || len(s) > 36 {
		return false
	}
	const shape = "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
	for i, r := range s {
		if shape[i] == '-' {
			if r != '-' {
				return false
			}
			continue
		}
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}

// sendCodexDirect — 기록 없는 코덱스 세션에 세션 ID만으로 `codex queue --thread <ID>`를 보낸다.
// 접두만 받았으면 rollout에서 전체 ID와 작업 폴더를 찾는다. 전체 UUID면 rollout이 없어도 그대로 보낸다
// (작업 폴더는 --cwd 또는 rollout, 없으면 빈 값 = 현재 폴더). 상태를 모르니 관문(작업 중·승인 대기)은 없다 —
// 큐는 현재 턴 뒤에 처리되고, 승인창이 떠 있으면 그 뒤에 처리된다. tmux 폴백은 없다: 큐가 실패하면 오류로 끝난다.
func sendCodexDirect(ctx context.Context, w io.Writer, spec string, o SendOptions, message string) error {
	cfg := config.Load()
	if !cfg.CodexQueueEnabled() {
		return fmt.Errorf("코덱스 세션 %s: 기록이 없는 세션은 codex queue로만 보낼 수 있는데 설정(codex_queue)이 꺼져 있습니다", spec)
	}
	id, cwd := "", o.CWD
	full := len(spec) == 36
	rid, rcwd, err := usage.CodexSessionByPrefix(codexSessionsRootFn(), spec)
	switch {
	case err == nil:
		id = rid
		if cwd == "" {
			cwd = rcwd
		}
	case full && errors.Is(err, usage.ErrCodexSessionNotFound):
		id = spec // rollout이 없어도(다른 계정 폴더·아직 안 쓰임) 전체 ID면 큐에 맡긴다
	case errors.Is(err, usage.ErrCodexSessionNotFound):
		return fmt.Errorf("코덱스 세션 %q을 rollout에서 찾지 못했습니다 — 전체 세션 ID를 쓰거나 'agentlayer status'로 이름 확인", spec)
	default:
		return err
	}
	if qerr := codexQueueFn(ctx, id, cwd, message); qerr != nil {
		return fmt.Errorf("코덱스 세션 %s 전송 실패(codex queue, 폴백 없음): %w", id, qerr)
	}
	if o.JSON {
		return json.NewEncoder(w).Encode(map[string]any{"session": id, "window": "", "pane": "", "session_id": id,
			"kind": "codex", "cwd": cwd, "recorded": false, "state": "", "sent": true, "via": "queue"})
	}
	fmt.Fprintf(w, "전송 완료 → codex %s [기록 없음] (codex queue)\n", id)
	return nil
}
