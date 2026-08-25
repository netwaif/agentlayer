package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/tmuxx"
	"github.com/netwaif/agentlayer/internal/wt"
)

const wtUsage = `사용법: agentlayer wt <명령>

  new <task>     worktree+브랜치+tmux window+에이전트 실행
                 [--agent claude|codex|gemini] [--repo 경로] [--base 브랜치] [--test '명령']
  list           태스크 목록과 상태
  diff <task>    base 대비 변경 보기
  test <task>    테스트 실행·기록  [--cmd '명령']
  review <task>  리뷰 파일 생성 (diff에 "#> 코멘트"를 적는 파일)
  send <task>    리뷰 코멘트를 에이전트에게 수정 지시로 전송
  merge <task>   merge 안내 + 확인 후 실행 (자동 merge 없음)
  clean <task>   보존 우선 정리 (미커밋·미병합 있으면 거부)
`

// RunWT는 `agentlayer wt ...`의 본체.
func RunWT(w io.Writer, stateDir string, st *state.Store, tm tmuxx.Tmux, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(w, wtUsage)
		return nil
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "new":
		fs := flag.NewFlagSet("wt new", flag.ContinueOnError)
		agent := fs.String("agent", "claude", "에이전트 종류")
		repo := fs.String("repo", ".", "대상 저장소")
		base := fs.String("base", "", "base 브랜치 (기본: 현재 브랜치)")
		test := fs.String("test", "", "테스트 명령")
		task, err := parseTaskAndFlags(fs, rest)
		if err != nil {
			return err
		}
		m, err := wt.New(stateDir, wt.NewOptions{Task: task, Repo: *repo, Base: *base,
			Agent: *agent, TestCmd: *test, Tmux: tm})
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "태스크 %s 시작\n  worktree: %s\n  branch:   %s (base %s)\n  agent:    %s (tmux window %q)\n",
			m.Task, ShortenHome(m.Path), m.Branch, m.Base, m.Agent, m.Task)
		return nil

	case "list":
		metas, err := wt.ListMetas(stateDir)
		if err != nil {
			return err
		}
		if len(metas) == 0 {
			fmt.Fprintln(w, "worktree 태스크 없음 — agentlayer wt new <task>로 시작하세요.")
			return nil
		}
		tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "TASK\tAGENT\tBRANCH\tDIRTY\tUNMERGED\tTEST\tPATH")
		for _, m := range metas {
			dirty, _ := wt.Dirty(m.Path)
			unmerged, _ := wt.Unmerged(m.Repo, m.Base, m.Branch)
			test := "—"
			if m.TestPass != nil {
				if *m.TestPass {
					test = "✔"
				} else {
					test = "✖"
				}
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\t%s\n",
				m.Task, m.Agent, m.Branch, len(dirty), unmerged, test, ShortenHome(m.Path))
		}
		return tw.Flush()

	case "diff":
		task, err := oneTask(rest)
		if err != nil {
			return err
		}
		m, err := wt.LoadMeta(stateDir, task)
		if err != nil {
			return err
		}
		diff, err := wt.Diff(m.Path, m.Base)
		if err != nil {
			return err
		}
		fmt.Fprintln(w, diff)
		return nil

	case "test":
		fs := flag.NewFlagSet("wt test", flag.ContinueOnError)
		cmdOverride := fs.String("cmd", "", "테스트 명령 (기록됨)")
		task, err := parseTaskAndFlags(fs, rest)
		if err != nil {
			return err
		}
		pass, err := wt.RunTest(w, stateDir, task, *cmdOverride)
		if err != nil {
			return err
		}
		if pass {
			fmt.Fprintln(w, "✔ 테스트 통과")
		} else {
			fmt.Fprintln(w, "✖ 테스트 실패")
		}
		return nil

	case "review":
		task, err := oneTask(rest)
		if err != nil {
			return err
		}
		path, err := wt.WriteReviewFile(stateDir, task)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "리뷰 파일: %s\n에디터로 열어 \"#> 코멘트\"를 적고, agentlayer wt send %s 로 보내세요.\n",
			ShortenHome(path), task)
		return nil

	case "send":
		task, err := oneTask(rest)
		if err != nil {
			return err
		}
		n, err := wt.SendComments(stateDir, task, st, tm)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "코멘트 %d건을 에이전트에게 보냈습니다.\n", n)
		return nil

	case "merge":
		fs := flag.NewFlagSet("wt merge", flag.ContinueOnError)
		yes := fs.Bool("yes", false, "확인 없이 진행")
		task, err := parseTaskAndFlags(fs, rest)
		if err != nil {
			return err
		}
		confirm := func() bool {
			if *yes {
				fmt.Fprintln(w, "y (--yes)")
				return true
			}
			r := bufio.NewReader(os.Stdin)
			line, _ := r.ReadString('\n')
			return strings.TrimSpace(strings.ToLower(line)) == "y"
		}
		return wt.MergeGuide(w, stateDir, task, confirm)

	case "clean":
		task, err := oneTask(rest)
		if err != nil {
			return err
		}
		if err := wt.Clean(stateDir, task); err != nil {
			return err
		}
		fmt.Fprintf(w, "태스크 %s 정리 완료 (worktree·브랜치·메타 제거)\n", task)
		return nil

	default:
		fmt.Fprint(w, wtUsage)
		return fmt.Errorf("알 수 없는 wt 명령: %s", cmd)
	}
}

// oneTask는 인자에서 태스크 이름 하나를 꺼낸다.
func oneTask(args []string) (string, error) {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return "", fmt.Errorf("태스크 이름이 필요합니다")
	}
	return args[0], nil
}

// parseTaskAndFlags는 "task --flag ..." 또는 "--flag ... task" 순서를 허용한다.
func parseTaskAndFlags(fs *flag.FlagSet, args []string) (string, error) {
	var task string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		task, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if task == "" && fs.NArg() > 0 {
		task = fs.Arg(0)
	}
	if task == "" {
		return "", fmt.Errorf("태스크 이름이 필요합니다")
	}
	return task, nil
}
