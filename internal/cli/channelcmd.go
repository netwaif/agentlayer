package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/netwaif/agentlayer/internal/channel"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

const channelUsage = `사용법:
  agentlayer channel serve <inbox> [--interval 200ms]
    총괄 세션에 수신함 편지를 Claude Code 채널(MCP 알림)로 밀어 넣는다 — Monitor 폴링 대체.
    등록: (회사 폴더에서) claude mcp add -s local agentlayer -- agentlayer channel serve <inbox>
    기동: claude --dangerously-load-development-channels server:agentlayer  (확인창은 봇 기동 스크립트가 넘긴다)`

// channelInstructions는 initialize 응답에 실려 세션이 채널 메시지를 무엇으로 볼지 알려준다.
const channelInstructions = `<channel source="agentlayer">로 오는 메시지는 총괄 수신함 편지다. content가 편지 JSON, ` +
	`meta.event가 종류(DONE_UNREAD·WAITING·ERROR·MESSAGE·READY), meta.origin이 local|remote. ` +
	`답은 agentlayer task reply(원격 편지) / send(로컬 세션) / task done으로 한다.`

// LetterNotification은 편지 한 건을 채널 알림으로 바꾼다. content = 편지 JSON 한 줄, meta 6키 고정.
func LetterNotification(r *task.Report) channel.Notification {
	b, _ := json.Marshal(r)
	origin := "local"
	switch r.Kind {
	case "hermes", "exec", "remote":
		origin = "remote"
	}
	if r.Letter != "" {
		origin = "remote"
	}
	return channel.Notification{Content: string(b), Meta: map[string]string{
		"task": r.TaskID, "event": r.To, "letter_id": r.ID, "origin": origin, "session": r.Session, "kind": r.Kind}}
}

// RunChannel — `agentlayer channel serve <inbox>`. stdin/stdout은 MCP 프로토콜, stderr는 로그.
func RunChannel(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, st *state.Store, stateDir, version string, args []string) error {
	if len(args) < 2 || args[0] != "serve" {
		return errors.New(channelUsage)
	}
	inbox, interval := args[1], 200*time.Millisecond
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--interval":
			if i+1 >= len(args) {
				return errors.New("--interval 뒤에 기간이 필요합니다 (예: 500ms)")
			}
			d, err := time.ParseDuration(args[i+1])
			if err != nil {
				return err
			}
			interval = d
			i++
		default:
			return fmt.Errorf("알 수 없는 인자: %s", args[i])
		}
	}
	abs, err := filepath.Abs(inbox)
	if err != nil {
		return err
	}
	logf := func(f string, a ...any) { fmt.Fprintf(stderr, "agentlayer channel: "+f+"\n", a...) }
	srv := channel.New(stdin, stdout, version, channelInstructions)
	srv.Log = func(m string) { logf("%s", m) }

	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// 원격 직원 폴링은 taskWatch와 같은 방식 — 전이가 inbox 파일로 떨어져 아래 Watch가 같은 길로 흘린다.
	opener := func(name string) (remote.Adapter, *remote.Remote, error) {
		r, ok, err := remote.Load(stateDir, name)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			return nil, nil, fmt.Errorf("원격 %q 등록 없음", name)
		}
		ad, err := OpenRemote(*r, stateDir)
		return ad, r, err
	}
	go func() {
		_ = task.RunRemotePolling(wctx, stateDir, abs, opener, func(s string) { logf("remote: %s", s) }, time.Now)
	}()
	werr := make(chan error, 1)
	go func() {
		werr <- task.Watch(wctx, abs, interval, false, func(r *task.Report) {
			srv.Notify(LetterNotification(r))
			refreshBoard(io.Discard, st, stateDir, time.Now())
		})
	}()
	serr := make(chan error, 1)
	go func() { serr <- srv.Run(wctx) }()
	logf("수신함 감시 시작: %s", abs)
	select {
	case err = <-serr:
	case err = <-werr:
		if err == nil {
			err = errors.New("수신함 감시가 예기치 않게 끝났습니다")
		}
	}
	cancel()
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}
