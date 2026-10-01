package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/netwaif/agentlayer/internal/remote"
)

// RunInboxReply — `agentlayer inbox reply <원격>:<카드ID> [--file <경로>]... <답|->`.
// `inbox wait --remote <이름> --app-mailbox`로 받은 편지(출력의 letter: 줄)에 답한다: 어댑터 Answer가 카드를 답으로 닫고
// 첨부는 기존 업로드 경로(서버 첨부 폴더 → kanban attach)로 붙인다. 회사 수신함(task reply)은 거치지 않는다.
// 서버의 claude-letter가 그 대화를 구독해 두었으므로 디스코드에 답 알림이 뜬다.
func RunInboxReply(ctx context.Context, stdout io.Writer, stdin io.Reader, stateDir string, args []string) error {
	var files []string
	var pos []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--file":
			if i+1 >= len(args) {
				return errors.New("--file 뒤에 경로가 필요합니다")
			}
			files = append(files, args[i+1])
			i++
		case strings.HasPrefix(args[i], "--file="):
			files = append(files, strings.TrimPrefix(args[i], "--file="))
		case strings.HasPrefix(args[i], "--") && len(pos) == 0:
			return fmt.Errorf("알 수 없는 플래그: %s\n%s", args[i], InboxUsage)
		default:
			pos = append(pos, args[i])
		}
	}
	if len(pos) < 2 {
		return errors.New("사용법: agentlayer inbox reply <원격>:<카드ID> [--file <경로>]... <답|-> (여러 줄은 '-'로 stdin)")
	}
	rname, card, ok := strings.Cut(pos[0], ":")
	if !ok || rname == "" || card == "" {
		return fmt.Errorf("대상 형식 오류: %q — <원격>:<카드ID> (inbox wait 출력의 letter: 줄)", pos[0])
	}
	text := strings.Join(pos[1:], " ")
	if text == "-" {
		if stdin == nil {
			return errors.New("stdin이 없습니다")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		text = strings.TrimRight(string(b), "\n")
	}
	if len(text) > maxMessageBytes {
		return errors.New("답이 너무 깁니다 (최대 64KiB)")
	}
	text = SanitizeMessage(text)
	if strings.TrimSpace(text) == "" {
		return errors.New("답이 비었습니다")
	}
	r, found, err := remote.Load(stateDir, rname)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("원격 %q 등록 없음 ('agentlayer remote list')", rname)
	}
	ad, err := OpenRemote(*r, stateDir)
	if err != nil {
		return fmt.Errorf("원격 %s 연결 실패: %w", rname, err)
	}
	if err := ad.Answer(ctx, card, text, files); err != nil {
		return fmt.Errorf("%s:%s 답장 실패: %w", rname, card, err)
	}
	note := ""
	if len(files) > 0 {
		note = fmt.Sprintf(" 첨부 %d개", len(files))
	}
	fmt.Fprintf(stdout, "답장 완료 → %s:%s (카드 완료%s)\n", rname, card, note)
	return nil
}
