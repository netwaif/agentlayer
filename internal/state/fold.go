package state

import (
	"fmt"
	"regexp"
)

// threadWindowRe는 Discord 봇이 스레드마다 띄우는 tmux 창 이름 규약:
// folder-bot bot-thread.sh·codex-discord tui-up.sh --window 모두 "t<스레드ID 끝 6자리>".
var threadWindowRe = regexp.MustCompile(`^t[0-9]{6}$`)

// IsThreadWindow는 창 이름이 봇 스레드 창 규약인지.
func IsThreadWindow(name string) bool { return threadWindowRe.MatchString(name) }

// IsThread는 이 레코드가 봇 스레드 창의 pane인지.
func (a *Agent) IsThread() bool { return IsThreadWindow(a.Tmux.WindowName) }

// ThreadBadge는 세션 이름 옆에 붙는 스레드 표기. 접힌 행이면 "스레드 N",
// 메인 없이 홀로 선 스레드 pane이면 "스레드 t552990", 그 외 빈 문자열.
func (a *Agent) ThreadBadge() string {
	switch {
	case a.Threads > 0:
		return fmt.Sprintf("스레드 %d", a.Threads)
	case a.IsThread():
		return "스레드 " + a.Tmux.WindowName
	}
	return ""
}

// Fold는 표시용으로 봇 스레드 pane을 같은 kind·세션의 메인 행에 접는다.
// 레코드(정본)는 pane마다 그대로이고, 반환 슬라이스의 접힌 행만 복사본이다.
//
// 규칙: 산(DEAD 아닌) 레코드 중 창 이름이 스레드 규약인 것만 접는다 — 같은 세션의
// 사용자 창(worktree 작업 등)은 건드리지 않는다. 대표는 메인+스레드 중 급한 쪽
// (Priority 낮은 쪽), 동률이면 메인. 메인이 여럿이면 스레드와 같은 cwd인 메인,
// 없으면 첫 메인. 순서는 입력(List 정렬) 그대로 — 대표는 그룹의 첫 자리에 선다.
// 컨테이너처럼 메인 cwd와 스레드 cwd가 달라도 세션이 같으면 접힌다.
func Fold(agents []*Agent) []*Agent {
	type group struct {
		mains, threads []*Agent
	}
	key := func(a *Agent) string { return a.Kind + "|" + a.Tmux.Session }
	groups := map[string]*group{}
	for _, a := range agents {
		if a.State == StateDead || a.Tmux.Session == "" {
			continue
		}
		g := groups[key(a)]
		if g == nil {
			g = &group{}
			groups[key(a)] = g
		}
		if a.IsThread() {
			g.threads = append(g.threads, a)
		} else {
			g.mains = append(g.mains, a)
		}
	}

	// 접히는 그룹마다: 흡수 메인 → 대표 결정
	rep := map[*Agent]*Agent{}   // 그룹 구성원 → 대표(복사본)
	emitted := map[*Agent]bool{} // 대표를 이미 출력했나
	for _, g := range groups {
		if len(g.threads) == 0 || len(g.mains) == 0 {
			continue
		}
		host := g.mains[0]
		for _, m := range g.mains {
			if m.CWD == g.threads[0].CWD {
				host = m
				break
			}
		}
		best := host
		for _, t := range g.threads {
			if t.State.Priority() < best.State.Priority() {
				best = t
			}
		}
		cp := *best
		cp.Threads = len(g.threads)
		rep[host] = &cp
		for _, t := range g.threads {
			rep[t] = &cp
		}
	}

	out := make([]*Agent, 0, len(agents))
	for _, a := range agents {
		r, ok := rep[a]
		if !ok {
			out = append(out, a)
			continue
		}
		if emitted[r] {
			continue
		}
		emitted[r] = true
		out = append(out, r)
	}
	return out
}
