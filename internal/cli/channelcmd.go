package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/netwaif/agentlayer/internal/channel"
	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/task"
)

const channelUsage = `사용법:
  agentlayer channel serve --self [--interval 200ms]
    직원 세션용 — 이 세션(tmux pane) 전용 수신함을 감시한다. 'agentlayer send'가 tmux 키 입력 대신 여기로 지시를 넣는다.
    등록: (직원 폴더에서) claude mcp add -s local agentlayer -- agentlayer channel serve --self
  agentlayer channel serve <inbox> [--interval 200ms]
    총괄 세션에 수신함 편지를 Claude Code 채널(MCP 알림)로 밀어 넣는다 — Monitor 폴링 대체.
    등록: (회사 폴더에서) claude mcp add -s local agentlayer -- agentlayer channel serve <inbox>
    기동: claude --dangerously-load-development-channels server:agentlayer  (확인창은 봇 기동 스크립트가 넘긴다)`

// channelInstructions는 initialize 응답에 실려 세션이 채널 메시지를 무엇으로 볼지 알려준다.
const channelInstructions = `<channel source="agentlayer">로 오는 메시지는 총괄 수신함 편지다. content가 편지 JSON, ` +
	`meta.event가 종류(DONE_UNREAD·WAITING·ERROR·MESSAGE·READY), meta.origin이 local|remote. ` +
	`답은 agentlayer task reply(원격 편지) / send(로컬 세션) / task done으로 한다.`

// selfInstructions는 직원 세션(--self)의 initialize 응답에 실린다.
const selfInstructions = `<channel source="agentlayer" event="SEND">로 오는 메시지는 총괄(또는 사용자)이 agentlayer send로 보낸 지시다. ` +
	`content가 지시 본문 그대로이며, 사용자가 입력창에 직접 친 것과 똑같이 받아 수행한다. meta.from이 보낸 세션, meta.task가 업무ID(있을 때). ` +
	`이 채널로 회신하지 않는다 — 결과는 평소 규칙대로 보고한다. ` +
	`content 안에 <channel source="plugin:discord:discord"> 태그가 들어 있으면 메인 봇이 넘긴 디스코드 스레드 메시지다 — 그 태그의 글을 스레드 사용자의 말로 처리한다.`

// DirectiveEvent — 직원 수신함에 떨어지는 지시의 to 값.
const DirectiveEvent = "SEND"

// maxChannelDirective — 채널로 보내는 지시 본문 상한. 보고 파일 상한(16KiB)에 머리말 여유를 뺀 값. 넘으면 tmux 입력으로 간다.
const maxChannelDirective = 12000

// PaneInbox는 tmux pane 하나의 전용 수신함 경로. pane ID("%12")는 tmux 서버 수명 안에서 고유하다.
func PaneInbox(stateDir, paneID string) string {
	id := strings.TrimPrefix(paneID, "%")
	if id == "" {
		return ""
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return filepath.Join(stateDir, "inboxes", "p"+id)
}

// DirectiveNotification은 지시 한 건을 채널 알림으로 바꾼다. content = 지시 본문 그대로.
func DirectiveNotification(r *task.Report) channel.Notification {
	meta := map[string]string{"event": DirectiveEvent, "from": r.From, "letter_id": r.ID}
	if r.TaskID != "" && r.TaskID != "-" {
		meta["task"] = r.TaskID
	}
	return channel.Notification{Content: r.Task, Meta: meta}
}

// DirectiveReport는 직원 수신함에 넣을 지시 편지.
func DirectiveReport(inbox, from, taskID, text string, now time.Time) *task.Report {
	if taskID == "" {
		taskID = "-"
	}
	return &task.Report{Version: 1, ID: task.NewID(), TaskID: taskID, Session: from, Kind: "directive", From: from,
		To: DirectiveEvent, Task: text, At: now, Inbox: inbox}
}

// ChannelLive — 그 수신함을 쥔 채널 서버가 살아 있는가(잠금이 잡혀 있는가). 죽은 서버의 잠금은 커널이 풀어 준다.
func ChannelLive(inbox string) bool {
	if inbox == "" {
		return false
	}
	f, err := os.OpenFile(filepath.Join(inbox, ".channel.lock"), os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return false
	}
	return true
}

// channelDeliverWait — 넣은 지시를 채널 서버가 집어 가기를 기다리는 상한. 테스트가 줄인다.
var channelDeliverWait = 3 * time.Second

// errNotConsumed — 서버가 지시를 집어 가지 않았다(편지는 회수했다). 호출자는 tmux 입력으로 되돌아간다.
var errNotConsumed = errors.New("채널 서버가 지시를 집어 가지 않음")

// SendViaChannel은 지시를 수신함에 넣고 서버가 집어 갈 때까지 기다린다. 못 집어 가면 편지를 회수하고 오류.
// 회수에 실패하면(이미 집어 갔다) 전달된 것으로 본다 — 두 경로로 두 번 보내지 않는다.
func SendViaChannel(inbox, from, taskID, text string, now time.Time) error {
	r := DirectiveReport(inbox, from, taskID, text, now)
	p, err := task.WriteReport(r)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(channelDeliverWait)
	for {
		if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := os.Remove(p); err != nil {
		return nil
	}
	return errNotConsumed
}

// purgeStale은 서버가 뜨기 전부터 있던 pending을 quarantine으로 치운다. pane ID는 tmux 서버가 다시 뜨면 재사용되므로
// 옛 세션 앞으로 남은 지시가 새 세션에 들어가면 안 된다. send는 서버가 살아 있을 때만 넣으므로 기동 시점의 pending은 전부 낡은 것이다.
func purgeStale(inbox string) int {
	files, _ := filepath.Glob(filepath.Join(inbox, "pending", "*.json"))
	if len(files) == 0 {
		return 0
	}
	_ = os.MkdirAll(filepath.Join(inbox, "quarantine"), 0o700)
	n := 0
	for _, f := range files {
		if os.Rename(f, filepath.Join(inbox, "quarantine", "stale-"+filepath.Base(f))) == nil {
			n++
		}
	}
	return n
}

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

// channelSettle — initialize 뒤 이만큼 살아 있어야 수신함을 소비한다(상태 점검용 짧은 접속 거르기).
// channelLockRetry — 다른 채널 서버가 수신함을 쥐고 있을 때 다시 시도하는 간격. 테스트가 줄인다.
var (
	channelSettle    = 3 * time.Second
	channelLockRetry = 2 * time.Second
)

// acquireInboxLock은 <inbox>/.channel.lock을 배타로 잡을 때까지 기다린다. 잡으면 푸는 함수를 돌려준다.
// 프로세스가 죽으면 커널이 잠금을 풀어 주므로 낡은 잠금이 남지 않는다.
func acquireInboxLock(ctx context.Context, inbox string, logf func(string, ...any)) (func(), bool) {
	if err := os.MkdirAll(inbox, 0o700); err != nil {
		logf("수신함 폴더 만들기 실패: %v", err)
		return nil, false
	}
	f, err := os.OpenFile(filepath.Join(inbox, ".channel.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		logf("수신함 잠금 파일 열기 실패: %v", err)
		return nil, false
	}
	waited := false
	for {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, true
		}
		if !waited {
			logf("다른 채널 서버가 수신함을 쥐고 있어 기다립니다: %s", inbox)
			waited = true
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, false
		case <-time.After(channelLockRetry):
		}
	}
}

// channelFlag — 이 서버를 채널로 받겠다는 Claude Code 기동 인자.
const channelFlag = "--dangerously-load-development-channels"

// HasChannelFlag — pid의 조상 중에 이 서버(server:<이름>)를 채널로 띄운 프로세스가 있는가.
// 초기화 요청은 플래그가 있든 없든 똑같아서(2026-09-29 실측) 프로토콜로는 가릴 수 없다. 플래그 없이 뜬 세션은
// 채널 알림을 버리므로, 그런 세션의 서버가 수신함을 쥐면 지시·편지가 소비만 되고 화면에 나타나지 않는다.
func HasChannelFlag(pt scan.ProcTable, pid int) bool {
	want := "server:" + channel.Name
	for p, depth := pid, 0; p > 1 && depth < 8; depth++ {
		e, ok := pt[p]
		if !ok || e.PPID == p {
			return false
		}
		f := strings.Fields(e.Args)
		for i, a := range f {
			if a == channelFlag && i+1 < len(f) && f[i+1] == want {
				return true
			}
			if a == channelFlag+"="+want {
				return true
			}
		}
		p = e.PPID
	}
	return false
}

// channelEnabledFn — 이 프로세스가 채널로 떠 있는가. 테스트가 바꿔 끼운다.
var channelEnabledFn = func() bool { return HasChannelFlag(scan.LoadProcTable(), os.Getppid()) }

// servePassive는 수신함을 건드리지 않고 핸드셰이크만 받는다.
func servePassive(ctx context.Context, stdin io.Reader, stdout io.Writer, version, instructions string, logf func(string, ...any)) error {
	srv := channel.New(stdin, stdout, version, instructions)
	srv.Log = func(m string) { logf("%s", m) }
	err := srv.Run(ctx)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

// RunChannel — `agentlayer channel serve <inbox>`. stdin/stdout은 MCP 프로토콜, stderr는 로그.
func RunChannel(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, st *state.Store, stateDir, version string, args []string) error {
	if len(args) < 2 || args[0] != "serve" {
		return errors.New(channelUsage)
	}
	inbox, interval := args[1], 200*time.Millisecond
	self := inbox == "--self"
	// pane 수신함 — 직원(--self)은 이것만, 총괄(serve <inbox>)도 tmux pane 안이면 함께 쥔다. 총괄 메인·총괄 스레드 세션에도
	// `send`가 채널로 들어가게(tmux 붙여넣기는 이미지 경로가 든 여러 줄에서 접힌 채 제출되지 않는다, 2026-09-30).
	pane := PaneInbox(stateDir, os.Getenv("TMUX_PANE"))
	if self {
		inbox = pane
	}
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
	logf := func(f string, a ...any) { fmt.Fprintf(stderr, "agentlayer channel: "+f+"\n", a...) }
	if self && inbox == "" {
		// tmux 밖 세션 — 받을 주소가 없다. 핸드셰이크만 받고 조용히 머문다(send는 tmux pane만 대상으로 한다).
		logf("tmux pane이 아니라 수신함 없이 뜹니다(TMUX_PANE 없음)")
		return servePassive(ctx, stdin, stdout, version, selfInstructions, logf)
	}
	if !channelEnabledFn() {
		logf("이 세션은 %s server:%s 없이 떴습니다 — 수신함을 쥐지 않습니다(지시·편지는 다른 경로로 간다)", channelFlag, channel.Name)
		ins := channelInstructions
		if self {
			ins = selfInstructions
		}
		return servePassive(ctx, stdin, stdout, version, ins, logf)
	}
	if self {
		if err := os.MkdirAll(inbox, 0o700); err != nil {
			return err
		}
	}
	abs, err := filepath.Abs(inbox)
	if err != nil {
		return err
	}
	// 경로 오타를 조용히 새 폴더로 만들어 "편지가 안 온다"로 헤매지 않게 — 상위 폴더가 없으면
	// 거부하고, 수신함만 없을 때는 만든다고 알린다(새 회사의 첫 기동은 정상 경로다).
	if _, serr := os.Stat(abs); errors.Is(serr, os.ErrNotExist) {
		if _, perr := os.Stat(filepath.Dir(abs)); perr != nil {
			return fmt.Errorf("수신함의 상위 폴더가 없습니다: %s — 경로를 확인하세요", filepath.Dir(abs))
		}
		logf("수신함 폴더가 없어 새로 만듭니다: %s (경로가 맞는지 확인하세요)", abs)
	}
	instructions := channelInstructions
	if self {
		instructions = selfInstructions
	} else if pane != "" {
		instructions = channelInstructions + " " + selfInstructions
	}
	srv := channel.New(stdin, stdout, version, instructions)
	srv.Log = func(m string) { logf("%s", m) }

	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
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
	// 수신함은 "진짜 채널 세션" 하나만 소비한다. `claude mcp get`·`mcp list`·doctor의 상태 점검도 이 서버를
	// 띄워 initialize까지 하고 바로 죽인다(2026-09-29 실측: initialize→initialized→tools/list 뒤 1초 안에 종료).
	// 그 짧은 인스턴스가 pending을 집으면 편지가 received/로 옮겨진 채 아무에게도 전달되지 않는다.
	// 그래서 (1) initialize 뒤 channelSettle 동안 살아남은 뒤에만, (2) 수신함 잠금을 쥔 하나만 감시를 시작한다.
	werr := make(chan error, 2)
	// watchDirectives — pane 수신함 하나를 쥐고 지시(SEND)를 알림으로 흘린다. 감시가 끝나면 werr로 알린다.
	watchDirectives := func(box string) {
		unlock, ok := acquireInboxLock(wctx, box, logf)
		if !ok {
			return
		}
		defer unlock()
		logf("수신함 감시 시작: %s", box)
		if n := purgeStale(box); n > 0 {
			logf("기동 전부터 있던 지시 %d건을 quarantine으로 치움(옛 세션 앞으로 온 것)", n)
		}
		werr <- task.Watch(wctx, box, interval, false, func(r *task.Report) {
			if r.To == DirectiveEvent {
				srv.Notify(DirectiveNotification(r))
			}
		})
	}
	go func() {
		select {
		case <-srv.Initialized():
		case <-wctx.Done():
			return
		}
		select {
		case <-time.After(channelSettle):
		case <-wctx.Done():
			return
		}
		if self {
			watchDirectives(abs)
			return
		}
		if pane != "" {
			if err := os.MkdirAll(pane, 0o700); err != nil {
				logf("pane 수신함 만들기 실패(%s): %v — 지시는 tmux 입력으로 온다", pane, err)
			} else {
				go watchDirectives(pane)
			}
		}
		// 회사 수신함은 총괄 세션 하나만 쥔다 — 다른 서버(총괄 스레드 세션 등)는 잠금이 풀릴 때까지 기다린다.
		unlock, ok := acquireInboxLock(wctx, abs, logf)
		if !ok {
			return
		}
		defer unlock()
		logf("수신함 감시 시작: %s", abs)
		// 원격 직원 폴링은 taskWatch와 같은 방식 — 전이가 inbox 파일로 떨어져 아래 Watch가 같은 길로 흘린다.
		go func() {
			_ = task.RunRemotePolling(wctx, stateDir, abs, opener, func(s string) { logf("remote: %s", s) }, time.Now)
		}()
		werr <- task.Watch(wctx, abs, interval, false, func(r *task.Report) {
			srv.Notify(LetterNotification(r))
			refreshBoard(io.Discard, st, stateDir, time.Now())
		})
	}()
	serr := make(chan error, 1)
	go func() { serr <- srv.Run(wctx) }()
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
