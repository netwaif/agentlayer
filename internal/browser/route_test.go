package browser

import (
	"fmt"
	"testing"

	"github.com/netwaif/agentlayer/internal/state"
)

func fakeLsof(portOut, cwdOut string) RunLsof {
	return func(args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "cwd" {
				return []byte(cwdOut), nil
			}
		}
		return []byte(portOut), nil
	}
}

func TestPortCWD(t *testing.T) {
	run := fakeLsof("p8231\n", "p8231\nfcwd\nn/Users/x/proj\n")
	cwd, err := PortCWD(3000, run)
	if err != nil || cwd != "/Users/x/proj" {
		t.Fatalf("got %q %v", cwd, err)
	}
}

func TestPortCWDNoListener(t *testing.T) {
	if _, err := PortCWD(3000, fakeLsof("", "")); err == nil {
		t.Error("리스너 없으면 에러")
	}
}

func TestCandidatesLongestMatch(t *testing.T) {
	agents := []*state.Agent{
		{ID: "a", CWD: "/Users/x", State: state.StateWaiting},
		{ID: "b", CWD: "/Users/x/proj", State: state.StateWorking},
		{ID: "dead", CWD: "/Users/x/proj", State: state.StateDead},
	}
	run := fakeLsof("p1\n", "p1\nfcwd\nn/Users/x/proj/sub\n")
	got := Candidates(agents, "http://localhost:3000/page", run)
	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("최장일치 산 에이전트 1명이어야: %v", ids(got))
	}
}

func TestCandidatesFallbackAll(t *testing.T) {
	agents := []*state.Agent{
		{ID: "a", CWD: "/Users/x", State: state.StateWaiting},
		{ID: "dead", CWD: "/Users/y", State: state.StateDead},
	}
	got := Candidates(agents, "https://example.com/", nil)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("외부 사이트는 산 에이전트 전원 폴백: %v", ids(got))
	}
}

func ids(as []*state.Agent) (r []string) {
	for _, a := range as {
		r = append(r, a.ID)
	}
	return
}

var _ = fmt.Sprintf
