// internal/task/remote.go
// 원격 직원의 상태는 훅이 아니라 task watch의 폴링이 가져온다. 여기서는 그 결과를 기존 전이·보고 함수에
// "가짜 에이전트"로 태워 inbox 형식·task.md·log.md를 로컬 직원과 똑같이 만든다.
package task

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/netwaif/agentlayer/internal/remote"
	"github.com/netwaif/agentlayer/internal/state"
)

// MaxMessageBytes — 편지 본문 상한(바이트, 넘으면 룬 경계에서 절단 + …). 보고 파일 16KiB 상한(MaxReportBytes) 안에
// JSON 이스케이프 여유를 두고 든다. 한글은 UTF-8 3바이트라 룬 기준으로 자르면 상한을 넘겨 격리된다.
const MaxMessageBytes = 8192

func truncBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// RemoteAgent는 ApplyTransition·ReportFor가 요구하는 모양의 에이전트(저장하지 않는다). kind는 등록의 kind(hermes·exec).
func RemoteAgent(as *Assignment, s remote.Status, cwd, kind string) *state.Agent {
	if kind == "" {
		kind = "remote"
	}
	a := &state.Agent{ID: as.AgentID, Kind: kind, Task: s.Summary, Ask: s.Ask, CWD: cwd,
		Tmux: state.TmuxRef{Session: as.Session, PaneID: as.Pane}}
	if s.State == state.StateError && s.Error != "" {
		a.Task = s.Error
	}
	return a
}

// ApplyRemoteStatus는 관측 상태가 마지막 것과 다를 때만 task.md·log.md 갱신과 inbox 보고를 하고 등록 파일을 갱신한다.
// 등록 파일은 다시 읽어 확인한 뒤 Remote 필드만 갱신한다(CAS) — 폴러가 사본을 든 사이 send(handle 교체)·task done(삭제)이
// 끼어도 옛 사본으로 되돌리거나 지운 등록을 되살리지 않는다.
func ApplyRemoteStatus(stateDir string, as *Assignment, s remote.Status, cwd string, now time.Time) (bool, error) {
	kind := ""
	if as.Remote != nil {
		if r, ok, _ := remote.Load(stateDir, as.Remote.Name); ok {
			kind = r.Kind
		}
	}
	return applyRemoteStatus(stateDir, as, s, cwd, kind, now)
}

func applyRemoteStatus(stateDir string, as *Assignment, s remote.Status, cwd, kind string, now time.Time) (bool, error) {
	if as.Remote == nil {
		return false, nil
	}
	cur, ok, err := Load(stateDir, as.AgentID)
	if err != nil {
		return false, err
	}
	if !ok || cur.Remote == nil || cur.Remote.Handle != as.Remote.Handle || cur.TaskID != as.TaskID {
		return false, nil // 그 사이 마감·교체됨 — 이 관측은 버린다
	}
	prev := as.Remote.LastState
	if prev == "" {
		prev = state.StateIdle
	}
	if prev == s.State && as.Remote.Seen == s.Seen {
		return false, nil
	}
	a := RemoteAgent(as, s, cwd, kind)
	if _, err := ApplyTransition(stateDir, a, prev, s.State, now); err != nil {
		return false, err
	}
	if rep, ok := ReportFor(stateDir, a, prev, s.State, now); ok {
		if _, err := WriteReport(rep); err != nil {
			return false, err
		}
	}
	cur.Remote.LastState = s.State
	cur.Remote.Seen = s.Seen
	as.Remote.LastState, as.Remote.Seen = s.State, s.Seen
	if err := Save(stateDir, *cur); err != nil {
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
		Task: truncBytes(text, MaxMessageBytes), At: now, Inbox: inbox}
}

// LetterReport — 원격 편지함에서 온 편지를 MESSAGE 보고로. Session은 원격 이름(답장 경로), From은 보낸 쪽, Letter는 편지ID.
func LetterReport(inbox, remoteName string, l remote.Letter, now time.Time) *Report {
	r := MessageReport(inbox, l.From, l.TaskID, l.Text, now)
	r.Kind = "letter"
	r.Session = remoteName
	r.Letter = l.ID
	if !l.At.IsZero() {
		r.At = l.At
	}
	return r
}

// letterAlreadyDelivered — 같은 편지ID의 보고가 pending/·received/에 이미 있으면 true(재전달 억제).
func letterAlreadyDelivered(inbox, letterID string) bool {
	if letterID == "" {
		return false
	}
	if _, ok := findLetter(inbox, letterID); ok {
		return true
	}
	return false
}

// findLetter는 수신함(pending/·received/)에서 편지ID의 보고를 찾는다.
func findLetter(inbox, letterID string) (*Report, bool) {
	for _, sub := range []string{"received", "pending"} {
		files, _ := filepath.Glob(filepath.Join(inbox, sub, "*.json"))
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			var r Report
			if json.Unmarshal(b, &r) == nil && r.Letter == letterID {
				return &r, true
			}
		}
	}
	return nil, false
}

// ReplyLetter — 총괄의 답장. 수신함에서 편지ID의 보고를 찾아 그 원격 어댑터의 Answer를 부른다. 돌려주는 값은 원격 이름.
func ReplyLetter(ctx context.Context, stateDir, inbox, letterID, text string, open AdapterOpener) (string, error) {
	rep, ok := findLetter(inbox, letterID)
	if !ok {
		return "", fmt.Errorf("수신함에 편지 %q이 없습니다(%s)", letterID, inbox)
	}
	if rep.Session == "" {
		return "", fmt.Errorf("편지 %q에 원격 이름이 없습니다(로컬 직원의 편지는 send로 답합니다)", letterID)
	}
	ad, _, err := open(rep.Session)
	if err != nil {
		return rep.Session, err
	}
	if err := ad.Answer(ctx, letterID, text); err != nil {
		return rep.Session, fmt.Errorf("%s 답장 실패: %w", rep.Session, err)
	}
	return rep.Session, nil
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

// MailboxInterval — 편지함(보드 전체 목록 조회)은 카드 상태 폴링보다 드물게 본다.
const MailboxInterval = 30 * time.Second

// PollRemotesOnce — 등록된 원격 업무마다 Poll → 전이 적용(+done이면 Pull). mailbox가 참이면 등록된 원격마다 Mailbox → MESSAGE.
func PollRemotesOnce(ctx context.Context, stateDir, inbox string, open AdapterOpener, warn func(string), now time.Time, mailbox bool) {
	list, err := List(stateDir)
	if err != nil {
		warn("등록 목록: " + err.Error())
		return
	}
	type opened struct {
		ad   remote.Adapter
		kind string
	}
	adapters := map[string]opened{}
	get := func(name string) opened {
		if a, ok := adapters[name]; ok {
			return a
		}
		a, r, err := open(name)
		if err != nil {
			warn(name + ": " + err.Error())
			adapters[name] = opened{}
			return opened{}
		}
		o := opened{ad: a}
		if r != nil {
			o.kind = r.Kind
		}
		adapters[name] = o
		return o
	}
	for i := range list {
		as := &list[i]
		if as.Remote == nil || as.Remote.Handle == "" {
			continue
		}
		o := get(as.Remote.Name)
		ad := o.ad
		if ad == nil {
			continue
		}
		s, err := ad.Poll(ctx, as.Remote.Handle)
		if err != nil {
			og := outages[as.Remote.Name]
			if og == nil {
				og = &outage{since: now}
				outages[as.Remote.Name] = og
			}
			warn(fmt.Sprintf("%s 조회 실패: %v", as.Remote.Name, err))
			if !og.reported && now.Sub(og.since) >= unreachableAfter {
				og.reported = true
				a := RemoteAgent(as, remote.Status{State: state.StateError}, "", o.kind)
				a.Task = fmt.Sprintf("원격 연결 실패 %d분 (%s)", int(now.Sub(og.since).Minutes()), as.Remote.Name)
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
				// stderr는 Monitor 안에서 보이지 않는다 — 보고 본문에 실패를 남겨 총괄이 빈 폴더를 찾아 헤매지 않게.
				warn(fmt.Sprintf("%s 산출물 회수 실패: %v", as.TaskID, err))
				s.Summary = fmt.Sprintf("⚠ 산출물 회수 실패(%v) · %s", err, s.Summary)
				cwd = ""
			}
		}
		if _, err := applyRemoteStatus(stateDir, as, s, cwd, o.kind, now); err != nil {
			warn(fmt.Sprintf("%s 전이 적용 실패: %v", as.TaskID, err))
		}
	}
	if !mailbox {
		return
	}
	remotes, err := remote.List(stateDir)
	if err != nil {
		warn("원격 목록: " + err.Error())
		return
	}
	for _, r := range remotes {
		ad := get(r.Name).ad
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
			if letterAlreadyDelivered(inbox, l.ID) {
				continue
			}
			if _, err := WriteReport(LetterReport(inbox, r.Name, l, now)); err != nil {
				warn("편지 보고 쓰기: " + err.Error())
			}
		}
	}
}

// RunRemotePolling은 ctx가 끝날 때까지 PollRemotesOnce를 돈다. 간격은 등록된 원격의 poll 중 최소(기본 5초).
// 편지함은 MailboxInterval마다 한 번만 본다(보드 전체 목록 조회라 카드 폴링보다 무겁다).
func RunRemotePolling(ctx context.Context, stateDir, inbox string, open AdapterOpener, warn func(string), now func() time.Time) error {
	var lastMailbox time.Time
	for {
		interval := remote.DefaultPoll
		if remotes, _ := remote.List(stateDir); len(remotes) > 0 {
			for _, r := range remotes {
				if d := r.PollInterval(); d < interval {
					interval = d
				}
			}
			t := now()
			mailbox := t.Sub(lastMailbox) >= MailboxInterval
			if mailbox {
				lastMailbox = t
			}
			PollRemotesOnce(ctx, stateDir, inbox, open, warn, t, mailbox)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
