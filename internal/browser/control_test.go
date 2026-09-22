// internal/browser/control_test.go
package browser

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestApplyTransitions(t *testing.T) {
	t0 := time.Date(2026, 9, 22, 14, 0, 0, 0, time.Local)
	idle := Control{Owner: OwnerIdle}
	a := Apply(idle, EvCall, "claude-%1", "검색", t0)
	if a.Owner != OwnerAgent || a.Agent != "claude-%1" || a.Label != "검색" || !a.Since.Equal(t0) || !a.LastCall.Equal(t0) {
		t.Fatalf("idle→agent: %+v", a)
	}
	// 다른 에이전트 호출도 막지 않고 공유 — since는 유지, last_call·agent 갱신
	b := Apply(a, EvCall, "codex-%2", "정리", t0.Add(3*time.Second))
	if b.Owner != OwnerAgent || b.Agent != "codex-%2" || !b.Since.Equal(t0) || !b.LastCall.Equal(t0.Add(3*time.Second)) {
		t.Fatalf("agent→agent 공유: %+v", b)
	}
	u := Apply(b, EvUserTake, "", "", t0.Add(4*time.Second))
	if u.Owner != OwnerUser || u.Stopped || !u.Since.Equal(t0.Add(4*time.Second)) {
		t.Fatalf("→user: %+v", u)
	}
	s := Apply(u, EvUserStop, "", "", t0.Add(5*time.Second))
	if s.Owner != OwnerUser || !s.Stopped {
		t.Fatalf("→stopped: %+v", s)
	}
	// stopped 중 호출은 상태를 못 바꾼다
	if got := Apply(s, EvCall, "claude-%1", "x", t0.Add(6*time.Second)); got.Owner != OwnerUser || !got.Stopped {
		t.Fatalf("stopped 중 call은 무시: %+v", got)
	}
	r := Apply(s, EvUserReturn, "", "", t0.Add(7*time.Second))
	if r.Owner != OwnerAgent || r.Stopped || r.Agent != "codex-%2" {
		t.Fatalf("user→agent 복귀(마지막 에이전트 유지): %+v", r)
	}
}

func TestExpiredAndEffective(t *testing.T) {
	t0 := time.Now()
	c := Control{Owner: OwnerAgent, Agent: "a", LastCall: t0}
	if c.Expired(t0.Add(19 * time.Second)) {
		t.Fatal("19초는 만료 아님")
	}
	if !c.Expired(t0.Add(ControlExpiry)) {
		t.Fatal("20초는 만료")
	}
	if e := c.Effective(t0.Add(30 * time.Second)); e.Owner != OwnerIdle {
		t.Fatalf("만료면 idle: %+v", e)
	}
	u := Control{Owner: OwnerUser, LastCall: t0}
	if u.Expired(t0.Add(time.Hour)) {
		t.Fatal("user 소유는 만료 없음")
	}
}

func TestSaveLoadControl(t *testing.T) {
	dir := t.TempDir()
	if got := LoadControl(dir); got.Owner != OwnerIdle {
		t.Fatalf("없으면 idle: %+v", got)
	}
	c := Control{Owner: OwnerUser, Agent: "claude-%1", Label: "a:b", Since: time.Unix(1, 0), LastCall: time.Unix(2, 0), Stopped: true, Waiting: 2}
	if err := SaveControl(dir, c); err != nil {
		t.Fatal(err)
	}
	got := LoadControl(dir)
	if got.Owner != OwnerUser || got.Agent != c.Agent || got.Label != c.Label || !got.Stopped || got.Waiting != 2 || !got.Since.Equal(c.Since) {
		t.Fatalf("왕복: %+v", got)
	}
	if _, err := filepath.Abs(dir); err != nil {
		t.Fatal(err)
	}
}

func TestMirrorValueAndParseRequest(t *testing.T) {
	c := Control{Owner: OwnerAgent, Agent: "claude-%1", Label: "제목: 검색", Since: time.UnixMilli(1000), LastCall: time.UnixMilli(2000), Waiting: 1}
	v := MirrorValue(c, true, "JustWatch: 신작")
	want := "agent:claude-%1:제목∶ 검색:1000:2000:0:1:1:JustWatch∶ 신작"
	if v != want {
		t.Fatalf("mirror = %q, want %q", v, want)
	}
	if strings.Count(v, ":") != 8 {
		t.Fatalf("구분자 수: %q", v)
	}
	for in, want := range map[string]Event{"user:123": EvUserTake, "agent:5": EvUserReturn, "stop:9": EvUserStop} {
		ev, ms, ok := ParseRequest(in)
		if !ok || ev != want || ms == 0 {
			t.Errorf("ParseRequest(%q) = %v %d %v", in, ev, ms, ok)
		}
	}
	if _, _, ok := ParseRequest("bogus"); ok {
		t.Error("잘못된 요청은 false")
	}
	if _, _, ok := ParseRequest(""); ok {
		t.Error("빈 요청은 false")
	}
}
