// internal/task/remote.go
// 원격 직원의 상태는 훅이 아니라 task watch의 폴링이 가져온다. 여기서는 그 결과를 기존 전이·보고 함수에
// "가짜 에이전트"로 태워 inbox 형식·task.md·log.md를 로컬 직원과 똑같이 만든다.
package task

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

// MaxMessageRunes — 편지 본문 상한(넘으면 절단 + …). 보고 파일 16KiB 상한 안에 들게.
const MaxMessageRunes = 8192

func truncRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// RemoteAgent는 ApplyTransition·ReportFor가 요구하는 모양의 에이전트(저장하지 않는다).
func RemoteAgent(as *Assignment, s remote.Status, cwd string) *state.Agent {
	a := &state.Agent{ID: as.AgentID, Kind: "hermes", Task: s.Summary, Ask: s.Ask, CWD: cwd,
		Tmux: state.TmuxRef{Session: as.Session, PaneID: as.Pane}}
	if s.State == state.StateError && s.Error != "" {
		a.Task = s.Error
	}
	return a
}

// ApplyRemoteStatus는 관측 상태가 마지막 것과 다를 때만 task.md·log.md 갱신과 inbox 보고를 하고 등록 파일을 갱신한다.
func ApplyRemoteStatus(stateDir string, as *Assignment, s remote.Status, cwd string, now time.Time) (bool, error) {
	if as.Remote == nil {
		return false, nil
	}
	prev := as.Remote.LastState
	if prev == "" {
		prev = state.StateIdle
	}
	if prev == s.State && as.Remote.Seen == s.Seen {
		return false, nil
	}
	a := RemoteAgent(as, s, cwd)
	if _, err := ApplyTransition(stateDir, a, prev, s.State, now); err != nil {
		return false, err
	}
	if rep, ok := ReportFor(stateDir, a, prev, s.State, now); ok {
		if _, err := WriteReport(rep); err != nil {
			return false, err
		}
	}
	as.Remote.LastState = s.State
	as.Remote.Seen = s.Seen
	if err := Save(stateDir, *as); err != nil {
		return true, err
	}
	return true, nil
}

// MessageReport — 직원이 총괄에게 먼저 보내는 편지. taskID가 비면 "-"(watch 검증이 빈 값을 거부).
func MessageReport(inbox, from, taskID, text string, now time.Time) *Report {
	if taskID == "" {
		taskID = "-"
	}
	return &Report{Version: 1, ID: NewID(), TaskID: taskID, Session: from, Kind: "message", From: from, To: "MESSAGE",
		Task: truncRunes(text, MaxMessageRunes), At: now, Inbox: inbox}
}

// LetterReport — 원격 편지함에서 온 편지를 MESSAGE 보고로.
func LetterReport(inbox string, l remote.Letter, now time.Time) *Report {
	r := MessageReport(inbox, l.From, l.TaskID, l.Text, now)
	r.Kind = "letter"
	if !l.At.IsZero() {
		r.At = l.At
	}
	return r
}

// AdapterOpener는 이름으로 어댑터를 연다(cli가 remote.Load+remote.Open으로 구현).
type AdapterOpener func(name string) (remote.Adapter, *remote.Remote, error)

// unreachableAfter — 이 시간 넘게 연속 실패하면 ERROR를 한 번 보고한다.
const unreachableAfter = 5 * time.Minute

type outage struct {
	since    time.Time
	reported bool
}

var outages = map[string]*outage{} // 원격 이름 → 연속 실패 기록(프로세스 수명)

// PollRemotesOnce — 등록된 원격 업무마다 Poll → 전이 적용(+done이면 Pull), 등록된 원격마다 Mailbox → MESSAGE.
func PollRemotesOnce(ctx context.Context, stateDir, inbox string, open AdapterOpener, warn func(string), now time.Time) {
	list, err := List(stateDir)
	if err != nil {
		warn("등록 목록: " + err.Error())
		return
	}
	adapters := map[string]remote.Adapter{}
	get := func(name string) remote.Adapter {
		if a, ok := adapters[name]; ok {
			return a
		}
		a, _, err := open(name)
		if err != nil {
			warn(name + ": " + err.Error())
			adapters[name] = nil
			return nil
		}
		adapters[name] = a
		return a
	}
	for i := range list {
		as := &list[i]
		if as.Remote == nil || as.Remote.Handle == "" {
			continue
		}
		ad := get(as.Remote.Name)
		if ad == nil {
			continue
		}
		s, err := ad.Poll(ctx, as.Remote.Handle)
		if err != nil {
			o := outages[as.Remote.Name]
			if o == nil {
				o = &outage{since: now}
				outages[as.Remote.Name] = o
			}
			warn(fmt.Sprintf("%s 조회 실패: %v", as.Remote.Name, err))
			if !o.reported && now.Sub(o.since) >= unreachableAfter {
				o.reported = true
				a := RemoteAgent(as, remote.Status{State: state.StateError}, "")
				a.Task = fmt.Sprintf("원격 연결 실패 %d분 (%s)", int(now.Sub(o.since).Minutes()), as.Remote.Name)
				if rep, ok := ReportFor(stateDir, a, state.StateWorking, state.StateError, now); ok {
					if _, err := WriteReport(rep); err != nil {
						warn("보고 쓰기: " + err.Error())
					}
				}
			}
			continue
		}
		delete(outages, as.Remote.Name)
		cwd := ""
		if s.State == state.StateDoneUnread && as.Remote.LastState != state.StateDoneUnread && as.TaskDir != "" {
			root, id := as.BoardRootID()
			cwd = filepath.Join(root, "결과물", id, "remote")
			if err := ad.Pull(ctx, as.Remote.Handle, cwd); err != nil {
				warn(fmt.Sprintf("%s 산출물 회수 실패: %v", as.TaskID, err))
				cwd = ""
			}
		}
		if _, err := ApplyRemoteStatus(stateDir, as, s, cwd, now); err != nil {
			warn(fmt.Sprintf("%s 전이 적용 실패: %v", as.TaskID, err))
		}
	}
	remotes, err := remote.List(stateDir)
	if err != nil {
		warn("원격 목록: " + err.Error())
		return
	}
	for _, r := range remotes {
		ad := get(r.Name)
		if ad == nil {
			continue
		}
		letters, err := ad.Mailbox(ctx)
		if err != nil {
			warn(r.Name + " 편지함: " + err.Error())
			continue
		}
		for _, l := range letters {
			if l.From == "" {
				l.From = r.Name
			}
			if _, err := WriteReport(LetterReport(inbox, l, now)); err != nil {
				warn("편지 보고 쓰기: " + err.Error())
			}
		}
	}
}

// RunRemotePolling은 ctx가 끝날 때까지 PollRemotesOnce를 돈다. 간격은 등록된 원격의 poll 중 최소(기본 5초).
func RunRemotePolling(ctx context.Context, stateDir, inbox string, open AdapterOpener, warn func(string), now func() time.Time) error {
	for {
		interval := remote.DefaultPoll
		if remotes, _ := remote.List(stateDir); len(remotes) > 0 {
			for _, r := range remotes {
				if d := r.PollInterval(); d < interval {
					interval = d
				}
			}
			PollRemotesOnce(ctx, stateDir, inbox, open, warn, now())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
