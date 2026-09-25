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
	"time"

	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

const remoteUsage = `사용법:
  agentlayer remote add <이름> --kind hermes (--ssh <호스트> | --local) --profile <프로필> --workspace-root <절대경로>
                      [--exec "<원격 명령 접두어>"] [--board <보드>] [--mailbox <담당자>] [--max-runtime 2h] [--poll 5s] [--no-check]
  agentlayer remote add <이름> --kind exec --file <어댑터 정의 JSON> [--no-check]
  agentlayer remote list [--json]
  agentlayer remote check <이름>
  agentlayer remote rm <이름>`

// RunRemote — 원격 직원 등록·점검·삭제. open은 remote.Open(테스트는 페이크).
func RunRemote(ctx context.Context, w io.Writer, st *state.Store, stateDir string,
	open func(remote.Remote, string) (remote.Adapter, error), args []string, now time.Time) error {
	if len(args) == 0 {
		return errors.New(remoteUsage)
	}
	switch args[0] {
	case "add":
		return remoteAdd(ctx, w, st, stateDir, open, args[1:], now)
	case "list":
		return remoteList(w, stateDir, args[1:])
	case "check":
		if len(args) != 2 {
			return errors.New(remoteUsage)
		}
		r, ok, err := remote.Load(stateDir, args[1])
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("원격 %q이 등록돼 있지 않습니다", args[1])
		}
		return remoteCheck(ctx, w, *r, stateDir, open)
	case "rm":
		if len(args) != 2 {
			return errors.New(remoteUsage)
		}
		found, err := remote.Delete(stateDir, args[1])
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("원격 %q이 등록돼 있지 않습니다", args[1])
		}
		fmt.Fprintf(w, "원격 %s 삭제\n", args[1])
		return nil
	default:
		return fmt.Errorf("알 수 없는 remote 명령: %s\n%s", args[0], remoteUsage)
	}
}

func remoteAdd(ctx context.Context, w io.Writer, st *state.Store, stateDir string,
	open func(remote.Remote, string) (remote.Adapter, error), args []string, now time.Time) error {
	if len(args) == 0 {
		return errors.New(remoteUsage)
	}
	r := remote.Remote{Name: args[0], AddedAt: now}
	file, noCheck := "", false
	for i := 1; i < len(args); i++ {
		flag := args[i]
		if flag == "--no-check" {
			noCheck = true
			continue
		}
		if flag == "--local" {
			r.Local = true
			continue
		}
		if !strings.HasPrefix(flag, "--") {
			return fmt.Errorf("알 수 없는 인자: %s\n%s", flag, remoteUsage)
		}
		if i+1 >= len(args) {
			return fmt.Errorf("%s 뒤에 값이 필요합니다", flag)
		}
		i++
		v := args[i]
		switch flag {
		case "--kind":
			r.Kind = v
		case "--ssh":
			r.SSH = v
		case "--profile":
			r.Profile = v
		case "--exec":
			r.Exec = strings.Fields(v)
		case "--workspace-root":
			r.WorkspaceRoot = v
		case "--board":
			r.Board = v
		case "--mailbox":
			r.Mailbox = v
		case "--max-runtime":
			r.MaxRuntime = v
		case "--poll":
			r.Poll = v
		case "--file":
			file = v
		default:
			return fmt.Errorf("알 수 없는 플래그: %s\n%s", flag, remoteUsage)
		}
	}
	if r.Kind == "" {
		r.Kind = "hermes"
	}
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		var def remote.Remote
		if err := json.Unmarshal(b, &def); err != nil {
			return fmt.Errorf("어댑터 정의 파일 파싱: %w", err)
		}
		r.Commands = def.Commands
		if r.Poll == "" {
			r.Poll = def.Poll
		}
		// 상대경로 명령은 정의 파일 위치 기준으로 절대화 — task watch는 회사 루트에서 돌기 때문.
		base := filepath.Dir(file)
		if abs, err := filepath.Abs(base); err == nil {
			base = abs
		}
		for name, argv := range r.Commands {
			if len(argv) > 0 && strings.Contains(argv[0], "/") && !filepath.IsAbs(argv[0]) {
				argv[0] = filepath.Join(base, argv[0])
				r.Commands[name] = argv
			}
		}
	}
	if err := r.Validate(); err != nil {
		return err
	}
	agents, err := st.List()
	if err != nil {
		return err
	}
	for _, a := range agents {
		if a.Tmux.Session == r.Name && a.State != state.StateDead {
			return fmt.Errorf("이름 %q은 산 tmux 세션과 같습니다 — 다른 이름을 쓰세요(원격이 로컬 세션을 가립니다)", r.Name)
		}
	}
	if !noCheck {
		if err := remoteCheck(ctx, w, r, stateDir, open); err != nil {
			return fmt.Errorf("점검 실패로 저장하지 않음(--no-check로 생략 가능): %w", err)
		}
	}
	if err := remote.Save(stateDir, r); err != nil {
		return err
	}
	fmt.Fprintf(w, "원격 %s 등록 (%s). 배정은 'agentlayer task assign <업무ID> %s …', 지시는 'agentlayer send %s …'\n", r.Name, r.Kind, r.Name, r.Name)
	return nil
}

func remoteCheck(ctx context.Context, w io.Writer, r remote.Remote, stateDir string, open func(remote.Remote, string) (remote.Adapter, error)) error {
	ad, err := open(r, stateDir)
	if err != nil {
		return err
	}
	info, err := ad.Check(ctx)
	if info.Version != "" {
		fmt.Fprintf(w, "  %s · 왕복 %s\n", info.Version, info.RoundTrip.Round(time.Millisecond))
	}
	if info.Detail != "" {
		fmt.Fprintf(w, "  %s\n", info.Detail)
	}
	if err != nil {
		return err
	}
	if r.Profile != "" {
		fmt.Fprintf(w, "  프로필 %s: 사용 가능\n", r.Profile)
	}
	return nil
}

func remoteList(w io.Writer, stateDir string, args []string) error {
	list, err := remote.List(stateDir)
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "--json" {
		if list == nil {
			list = []remote.Remote{}
		}
		return json.NewEncoder(w).Encode(list)
	}
	if len(list) == 0 {
		fmt.Fprintln(w, "등록된 원격 직원 없음. ('agentlayer remote add …')")
		return nil
	}
	fmt.Fprintln(w, PadRight("이름", 20)+PadRight("종류", 8)+PadRight("대상", 40)+"폴링")
	for _, r := range list {
		target := r.SSH + " " + r.Profile
		if r.Local {
			target = "(local) " + r.Profile
		}
		if r.Kind == "exec" {
			target = strings.Join(r.Commands["dispatch"], " ")
		}
		fmt.Fprintln(w, PadRight(r.Name, 20)+PadRight(r.Kind, 8)+PadRight(target, 40)+r.PollInterval().String())
	}
	return nil
}
