package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/task"
)

// sendRemoteDirect — 업무(task) 등록이 없는 원격에 보내는 직송 경로. 예전에는 "먼저 task assign"으로 오류였던 입력이다.
// 어댑터는 고치지 않고 기존 함수만 쓴다: 본문은 Dispatch(카드 하나 = 임시 업무ID MSG-<8자>), 첨부는 Hermes의 Answer/upload와
// 같은 방식(원격 sh -c 'mkdir -p … && cat > …')으로 카드 작업 폴더 아래 from-company/에 올리고 본문 끝에 경로를 적는다.
// Answer 자체는 편지 카드를 "완료"로 닫아 버려 새 카드에는 쓸 수 없고, upload는 비공개라 같은 셸 한 줄을 여기서 만든다.
// 업무 등록은 만들지 않는다 — 후속 상태 추적(task watch)은 없고, 답은 원격의 편지함(Mailbox)으로 온다.
func sendRemoteDirect(ctx context.Context, w io.Writer, stateDir string, r *remote.Remote, message string, o SendOptions, now time.Time) error {
	ad, err := OpenRemote(*r, stateDir)
	if err != nil {
		return err
	}
	taskID := "MSG-" + strings.ToUpper(task.NewID()[:8])
	if o.From != "" {
		// 카드에는 from 자리가 없다 — --from을 명시했을 때만 본문 첫 줄로 남긴다(명시 없는 기존 입력은 그대로).
		message = "보낸이: " + o.From + "\n" + message
	}
	var uploaded []string
	if len(o.Files) > 0 {
		h, ok := ad.(*remote.Hermes)
		if !ok {
			return fmt.Errorf("%s(%s): 첨부 직송은 hermes 원격만 지원합니다", r.Name, r.Kind)
		}
		dir := h.WorkspaceRoot + "/" + taskID + "/" + remote.AttachDirName
		for _, local := range o.Files {
			p, err := uploadToHermes(ctx, h, dir, local)
			if err != nil {
				return fmt.Errorf("%s: %w", r.Name, err)
			}
			uploaded = append(uploaded, p)
		}
		message += "\n첨부: " + strings.Join(uploaded, ", ")
	}
	handle, derr := ad.Dispatch(ctx, remote.DispatchRequest{TaskID: taskID, Title: firstLineTitle(message), Body: message,
		Attempt: fmt.Sprintf("%d", now.Unix())})
	if derr != nil {
		if handle != "" {
			return fmt.Errorf("%s: 카드 %s를 만들었으나 기동 실패: %w", r.Name, handle, derr)
		}
		return fmt.Errorf("%s 직송 실패: %w", r.Name, derr)
	}
	if o.JSON {
		return json.NewEncoder(w).Encode(map[string]any{"session": r.Name, "remote": r.Kind, "handle": handle, "task_id": taskID,
			"files": uploaded, "state": "", "sent": true, "action": "직송", "via": "remote"})
	}
	note := ""
	if len(uploaded) > 0 {
		note = fmt.Sprintf(" 첨부 %d개", len(uploaded))
	}
	fmt.Fprintf(w, "전송 완료 → %s (%s %s) [업무 등록 없음] 직송%s\n", r.Name, r.Kind, handle, note)
	return nil
}

// uploadToHermes — Hermes.upload과 같은 셸 한 줄(로컬 셸 미경유, 표준입력으로 흘림). 크기 상한도 같다.
func uploadToHermes(ctx context.Context, h *remote.Hermes, dir, local string) (string, error) {
	f, err := os.Open(local)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil {
		return "", err
	} else if fi.IsDir() {
		return "", fmt.Errorf("첨부 %s는 폴더입니다", local)
	} else if fi.Size() > remote.MaxPullBytes {
		return "", fmt.Errorf("첨부 %s가 %dMiB를 넘습니다", local, remote.MaxPullBytes>>20)
	}
	remotePath := dir + "/" + filepath.Base(local)
	ctx2, cancel := context.WithTimeout(ctx, remote.TimeoutPull)
	defer cancel()
	cmd := fmt.Sprintf("mkdir -p %s && cat > %s", remote.ShellQuote(dir), remote.ShellQuote(remotePath))
	if _, err := h.R.Run(ctx2, f, "sh", "-c", cmd); err != nil {
		return "", fmt.Errorf("첨부 업로드 실패(%s): %w", local, err)
	}
	return remotePath, nil
}

// firstLineTitle — 카드 제목은 본문 첫 줄(최대 60자).
func firstLineTitle(message string) string {
	line := strings.TrimSpace(strings.SplitN(message, "\n", 2)[0])
	if r := []rune(line); len(r) > 60 {
		line = string(r[:60]) + "…"
	}
	return line
}
