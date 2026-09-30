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

	"github.com/netwaif/agentlayer/internal/scan"
	"github.com/netwaif/agentlayer/internal/task"
)

// `agentlayer inbox wait` — "메시지 받을 준비" 명령. 데스크톱 앱에서 연 Claude 세션은 채널도 세션 간 메시지도 받지 못한다
// (2026-09-30 실측). 그래서 세션이 Bash로 이 명령을 백그라운드 실행해 두면, `agentlayer send <이름>`이 그 수신함에 편지를 넣고
// 이 명령이 편지 한 통을 stdout으로 돌려주며 끝난다 — Bash 도구의 결과로 지시가 세션에 들어간다.
// 상태 저장소(agents/)·훅은 건드리지 않는다. 주소록은 addresses/<이름>.json, 수신함은 inboxes/a<pid>/.

const InboxUsage = `사용법:
  agentlayer inbox wait [--name <이름>] [--timeout <기간, 기본 30m>]
    이 세션 앞으로 오는 편지 한 통을 기다린다(Claude 세션이 Bash로 백그라운드 실행). 편지가 오면
    "from: <보낸이>" 한 줄 + 빈 줄 + 본문을 stdout에 찍고 0으로 끝난다. 기간을 넘기면 stderr에 "답 없음"을 찍고 2로 끝난다.
    이름 기본값은 폴더명(겹치면 폴더명-<pid 끝 4자리>). 보내는 쪽: agentlayer send <이름> <메시지>
  agentlayer inbox open [--name <별칭>]
    연결 모드. 이 세션의 고유 주소 ID(al-6자)를 발급해 stdout에 찍고 바로 끝난다. 상대에게는 이 ID를 알려 준다.
    이후 wait는 주소를 유지하고(끝나도 안 지움), 대기가 꺼진 사이에 온 편지도 큐에 남겨 다음 wait가 집는다. 다시 open하면 같은 ID.
  agentlayer inbox close [--name <별칭>]
    연결 모드를 끝낸다(주소 ID·별칭 삭제).`

// Address — 주소록 항목. 기본은 `inbox wait`가 살아 있는 동안만 존재한다(끝나면 지운다).
// 연결 모드(`inbox open`)면 Keep=true: 고유 ID(al-6자)와 별칭 두 파일로 저장되고, close까지 남는다.
type Address struct {
	Name         string    `json:"name"`
	ID           string    `json:"id,omitempty"`   // 연결 모드의 고유 주소(al-xxxxxx). 상대에게 알려 주는 값
	Keep         bool      `json:"keep,omitempty"` // 연결 모드: wait가 끝나도 주소 유지, 편지는 큐에 보관
	PID          int       `json:"pid"`            // 세션(claude) 프로세스 PID — send가 생사를 본다
	CWD          string    `json:"cwd"`
	Inbox        string    `json:"inbox"`
	RegisteredAt time.Time `json:"registered_at"`
}

// newAddressID — 고유 주소 ID. 사람이 옮겨 적기 쉽게 6자(16진).
func newAddressID() string { return "al-" + task.NewID()[:6] }

// AddressesDir — 주소록 폴더.
func AddressesDir(stateDir string) string { return filepath.Join(stateDir, "addresses") }

// AddressInbox — 세션 PID 전용 수신함. pane 수신함(p<pane>)과 다른 접두(a)라 겹치지 않는다.
func AddressInbox(stateDir string, pid int) string {
	return filepath.Join(stateDir, "inboxes", fmt.Sprintf("a%d", pid))
}

// validAddressName — 파일명으로 안전한 이름만(경로 구분자·숨김 접두 금지).
func validAddressName(name string) bool {
	return name != "" && !strings.ContainsAny(name, "/\\") && !strings.HasPrefix(name, ".") && name != ".." && len(name) <= 128
}

func addressPath(stateDir, name string) string {
	return filepath.Join(AddressesDir(stateDir), name+".json")
}

// LoadAddress — 주소록에서 이름을 찾는다. 없으면 found=false.
func LoadAddress(stateDir, name string) (*Address, bool, error) {
	if !validAddressName(name) {
		return nil, false, nil
	}
	b, err := os.ReadFile(addressPath(stateDir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var a Address
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, false, fmt.Errorf("주소록 %s 파싱: %w", name, err)
	}
	return &a, true, nil
}

// saveAddress — 별칭 파일에 쓰고, 연결 모드(ID 있음)면 ID 파일에도 같은 내용을 쓴다.
func saveAddress(stateDir string, a Address) error {
	if err := os.MkdirAll(AddressesDir(stateDir), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	keys := []string{a.Name}
	if a.ID != "" {
		keys = append(keys, a.ID)
	}
	for _, k := range keys {
		tmp := addressPath(stateDir, k) + ".tmp"
		if err := os.WriteFile(tmp, b, 0o600); err != nil {
			return err
		}
		if err := os.Rename(tmp, addressPath(stateDir, k)); err != nil {
			return err
		}
	}
	return nil
}

func deleteAddress(stateDir, name string) { _ = os.Remove(addressPath(stateDir, name)) }

// deleteAddressAll — 별칭과 ID 파일을 함께 지운다(연결 모드 close·죽은 세션 정리).
func deleteAddressAll(stateDir string, a *Address) {
	deleteAddress(stateDir, a.Name)
	if a.ID != "" {
		deleteAddress(stateDir, a.ID)
	}
}

// SessionPID — 이 프로세스를 띄운 Claude 세션의 PID: 부모 사슬을 올라가 인자에 claude가 있는 첫 조상(가장 가까운 것).
// Bash 도구 안이면 agentlayer ← bash ← claude 순이라 두 단계 위다. 못 찾으면 0.
func SessionPID() int {
	pt := scan.LoadProcTable()
	for p, depth := os.Getppid(), 0; p > 1 && depth < 8; depth++ {
		e, ok := pt[p]
		if !ok || e.PPID == p {
			return 0
		}
		if scan.KindFromArgs(e.Args) == "claude" {
			return p
		}
		p = e.PPID
	}
	return 0
}

// sessionPIDFn·pidAliveFn — 테스트가 바꿔 끼운다.
var (
	sessionPIDFn = SessionPID
	pidAliveFn   = func(pid int) bool {
		if pid <= 0 {
			return false
		}
		err := syscall.Kill(pid, 0)
		return err == nil || errors.Is(err, syscall.EPERM)
	}
)

// ErrInboxTimeout — 기간 안에 편지가 없었다. main이 종료 코드 2로 바꾼다.
var ErrInboxTimeout = errors.New("답 없음")

// RunInboxWait — `agentlayer inbox wait|open|close`. now·interval은 테스트 주입점.
func RunInboxWait(ctx context.Context, stdout, stderr io.Writer, stateDir string, args []string, now func() time.Time) error {
	if len(args) < 1 || (args[0] != "wait" && args[0] != "open" && args[0] != "close") {
		return errors.New(InboxUsage)
	}
	sub := args[0]
	name, timeout, interval := "", 30*time.Minute, 200*time.Millisecond
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--name", "--timeout", "--interval":
			if i+1 >= len(args) {
				return fmt.Errorf("%s 뒤에 값이 필요합니다", args[i])
			}
			v := args[i+1]
			i++
			switch args[i-1] {
			case "--name":
				name = v
			case "--timeout":
				d, err := time.ParseDuration(v)
				if err != nil || d <= 0 {
					return fmt.Errorf("--timeout 형식 오류: %q (예: 30m, 2h)", v)
				}
				timeout = d
			default:
				d, err := time.ParseDuration(v)
				if err != nil || d <= 0 {
					return fmt.Errorf("--interval 형식 오류: %q", v)
				}
				interval = d
			}
		default:
			return fmt.Errorf("알 수 없는 인자: %s\n%s", args[i], InboxUsage)
		}
	}
	logf := func(f string, a ...any) { fmt.Fprintf(stderr, "agentlayer inbox: "+f+"\n", a...) }
	cwd, _ := os.Getwd()
	pid := sessionPIDFn()
	if pid <= 0 {
		// 조상에서 claude를 못 찾았다(맨 셸에서 실행 등) — 이 프로세스 자신을 세션으로 삼는다. 기다리는 동안은 살아 있으니 send의 생사 판정이 맞다.
		pid = os.Getpid()
		logf("Claude 세션 프로세스를 찾지 못해 이 프로세스(pid %d)를 주소로 씁니다", pid)
	}
	if name == "" {
		name = filepath.Base(cwd)
	}
	if !validAddressName(name) {
		return fmt.Errorf("이름 형식 오류: %q", name)
	}
	if strings.HasPrefix(name, "al-") {
		return fmt.Errorf("이름은 al-로 시작할 수 없습니다(고유 주소 ID 접두): %q", name)
	}
	inbox := AddressInbox(stateDir, pid)
	// 연결 모드의 기존 주소: 이 세션(pid)이 같은 별칭으로 open해 둔 것.
	kept, keptFound, _ := LoadAddress(stateDir, name)
	if keptFound && !(kept.Keep && kept.PID == pid) {
		kept, keptFound = nil, false
	}
	switch sub {
	case "close":
		if !keptFound {
			// 별칭이 없어도 죽은 항목·다른 세션 항목은 건드리지 않는다
			return fmt.Errorf("이 세션의 연결 주소 %q가 없습니다", name)
		}
		deleteAddressAll(stateDir, kept)
		logf("연결 종료: %s(%s)", kept.ID, name)
		return nil
	case "open":
		if keptFound {
			fmt.Fprintln(stdout, kept.ID) // 멱등 — 같은 ID
			return nil
		}
		if old, found, err := LoadAddress(stateDir, name); err == nil && found && old.PID != pid && pidAliveFn(old.PID) {
			name = fmt.Sprintf("%s-%04d", name, pid%10000)
		}
		if err := os.MkdirAll(filepath.Join(inbox, "pending"), 0o700); err != nil {
			return err
		}
		a := Address{Name: name, ID: newAddressID(), Keep: true, PID: pid, CWD: cwd, Inbox: inbox, RegisteredAt: now()}
		if err := saveAddress(stateDir, a); err != nil {
			return err
		}
		logf("연결 시작: 주소 %s(별칭 %s), pid %d, 수신함 %s — 상대는 agentlayer send %s <메시지>", a.ID, name, pid, inbox, a.ID)
		fmt.Fprintln(stdout, a.ID)
		return nil
	}
	if keptFound {
		// 연결 모드의 wait: 주소를 유지하고, 대기가 꺼진 사이에 온 편지(pending)를 치우지 않는다.
		name, inbox = kept.Name, kept.Inbox
		logf("대기 시작(연결 %s): 별칭 %s, pid %d, 최대 %s", kept.ID, name, pid, timeout)
	} else {
		// 이름 충돌: 다른 산 세션이 같은 이름을 쓰고 있으면 pid 끝 4자리를 붙인다. 죽은 항목은 덮어쓴다.
		if old, found, err := LoadAddress(stateDir, name); err == nil && found && old.PID != pid && pidAliveFn(old.PID) {
			name = fmt.Sprintf("%s-%04d", name, pid%10000)
		}
		if err := os.MkdirAll(filepath.Join(inbox, "pending"), 0o700); err != nil {
			return err
		}
		if n := purgeStale(inbox); n > 0 {
			logf("기동 전부터 있던 편지 %d건을 quarantine으로 치움(옛 세션 앞으로 온 것)", n)
		}
		if err := saveAddress(stateDir, Address{Name: name, PID: pid, CWD: cwd, Inbox: inbox, RegisteredAt: now()}); err != nil {
			return err
		}
		defer deleteAddress(stateDir, name) // 정상·타임아웃·SIGINT/SIGTERM(ctx 취소) 모두 여기서 지운다
		logf("대기 시작: 이름 %s, pid %d, 수신함 %s, 최대 %s", name, pid, inbox, timeout)
	}
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var got *task.Report
	err := task.Watch(wctx, inbox, interval, true, func(r *task.Report) { got = r })
	if got != nil {
		fmt.Fprintf(stdout, "from: %s\n\n%s\n", got.From, got.Task)
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || (err == nil && wctx.Err() != nil && ctx.Err() == nil) {
		return fmt.Errorf("%w(%s)", ErrInboxTimeout, timeout)
	}
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return errors.New("중단됨(신호) — 주소록 항목을 지웠습니다")
	}
	return err
}

// sendToAddress — `send <이름>`의 주소록 분기(ResolveTarget이 실패한 뒤에만). PID가 살아 있으면 SendViaChannel과 같은 방식으로
// 편지를 넣고 받는 쪽이 집어 갈 때까지 기다린다(못 집어 가면 회수하고 오류). 죽었으면 항목을 지우고 오류.
func sendToAddress(w io.Writer, stateDir string, addr *Address, from, message string, o SendOptions) error {
	if !pidAliveFn(addr.PID) {
		deleteAddressAll(stateDir, addr)
		return fmt.Errorf("%s: 세션(pid %d)이 죽어 주소록에서 지웠습니다 — 받는 쪽에서 'agentlayer inbox wait' 다시 실행", addr.Name, addr.PID)
	}
	label := addr.Name
	if addr.Keep {
		// 연결 모드: 대기가 꺼져 있어도 편지를 회수하지 않고 큐(pending)에 남긴다 — 다음 wait가 집는다.
		label = addr.ID
		if _, err := task.WriteReport(DirectiveReport(addr.Inbox, from, "", message, time.Now())); err != nil {
			return fmt.Errorf("%s 전송 실패: %w", label, err)
		}
		if o.JSON {
			return json.NewEncoder(w).Encode(map[string]any{"session": label, "name": addr.Name, "window": "", "pane": "", "pid": addr.PID,
				"state": "", "sent": true, "via": "inbox"})
		}
		fmt.Fprintf(w, "전송 완료 → %s(%s, pid %d) [연결 큐] (inbox)\n", label, addr.Name, addr.PID)
		return nil
	}
	if err := SendViaChannel(addr.Inbox, from, "", message, time.Now()); err != nil {
		if errors.Is(err, errNotConsumed) {
			return fmt.Errorf("%s: 받는 쪽이 %s 안에 집어 가지 않아 편지를 회수했습니다 — 'agentlayer inbox wait'가 돌고 있는지 확인", addr.Name, channelDeliverWait)
		}
		return fmt.Errorf("%s 전송 실패: %w", addr.Name, err)
	}
	if o.JSON {
		return json.NewEncoder(w).Encode(map[string]any{"session": addr.Name, "window": "", "pane": "", "pid": addr.PID,
			"state": "", "sent": true, "via": "inbox"})
	}
	fmt.Fprintf(w, "전송 완료 → %s (pid %d) [inbox wait] (inbox)\n", addr.Name, addr.PID)
	return nil
}
