package discord

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/state"
	"github.com/netwaif/agentlayer/internal/usage"
)

var t0 = time.Date(2026, 8, 25, 12, 0, 0, 0, time.FixedZone("KST", 9*3600))

func pf(v float64) *float64 { return &v }

func fixturePayload() *usage.Payload {
	return &usage.Payload{Providers: map[string]usage.Provider{
		"claude": {OK: true, Plan: "Max", Email: "kshxxthm@gmail.com", Level: "green",
			Action: "지금 큰 작업 돌리세요", Reason: "여유 있어요.",
			Windows: map[string]usage.Window{
				"5h": {LeftPct: pf(77), ResetMin: pf(197)},
				"7d": {LeftPct: pf(16), ResetMin: pf(7)}}},
		"antigravity": {OK: true, Email: "know@x.com", Level: "green", Action: "OK",
			Windows: map[string]usage.Window{
				"knowhackking": {LeftPct: pf(64)}, "aitipking": {}}},
	}}
}

func fixtureAgents() ([]*state.Agent, map[string]usage.CtxInfo) {
	agents := []*state.Agent{
		{ID: "claude-7", Kind: "claude", State: state.StateWaiting, Task: "승인 대기",
			Tmux: state.TmuxRef{Session: "collab-bot"}, CWD: "/Users/soonho/ai-folder/collab",
			StateSince: t0.Add(-8 * time.Minute)},
		{ID: "claude-9", Kind: "claude", State: state.StateDead,
			CWD: "/Users/soonho/gone", StateSince: t0.Add(-time.Hour)},
	}
	// 에이전트 ID 키 — 같은 폴더의 다른 종류 에이전트와 오귀속되지 않게
	ctx := map[string]usage.CtxInfo{
		"claude-7": {Model: "Opus 5 (1M context)", UsedPct: pf(16)},
	}
	return agents, ctx
}

func TestBuildComponents(t *testing.T) {
	agents, ctx := fixtureAgents()
	comps := BuildComponents(fixturePayload(), agents, ctx, map[string]string{"/Users/soonho/ai-folder/collab": "⌁collab방"}, "/Users/soonho", t0)
	b, err := json.Marshal(comps)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, want := range []string{
		"Claude — 지금 큰 작업 돌리세요", "kshxxthm@gmail.com",
		"5h", "77", "리셋 3시간 후",
		"knowhackking", // antigravity 계정 행
		"### 에이전트", "~/ai-folder/collab", "Opus 5 (1M context)", "응답 필요", "8분",
		"갱신 \\u003ct:", // json.Marshal이 <를 이스케이프 — Discord는 정상 해석
		"⌁collab방",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("카드에 %q 있어야 함", want)
		}
	}
	if strings.Contains(out, "/Users/soonho/gone") {
		t.Error("DEAD 에이전트는 카드에서 제외")
	}
	// 컨테이너 구조 확인
	first := comps[0].(map[string]any)
	if first["type"] != typeContainer || first["accent_color"] == nil {
		t.Errorf("컨테이너 형식: %+v", first)
	}
}

func TestBuildComponentsNoUsage(t *testing.T) {
	agents, ctx := fixtureAgents()
	comps := BuildComponents(nil, agents, ctx, nil, "/Users/soonho", t0)
	b, _ := json.Marshal(comps)
	if !strings.Contains(string(b), "### 에이전트") {
		t.Error("coach 없이도 에이전트 섹션은 나옴")
	}
}

func TestWorsenedPings(t *testing.T) {
	pay := fixturePayload()
	// green → 첫 관찰: 핑 없음
	pings, lv := WorsenedPings(pay, map[string]string{})
	if len(pings) != 0 {
		t.Errorf("첫 관찰 핑 없음: %v", pings)
	}
	// green → red 악화: 핑
	p := pay.Providers["claude"]
	p.Level = "red"
	p.Action = "미루세요"
	pay.Providers["claude"] = p
	pings, lv = WorsenedPings(pay, lv)
	if len(pings) != 1 || !strings.Contains(pings[0], "Claude") {
		t.Errorf("악화 핑 1건: %v", pings)
	}
	// red 유지: 중복 핑 없음
	pings, _ = WorsenedPings(pay, lv)
	if len(pings) != 0 {
		t.Errorf("유지 상태는 무음: %v", pings)
	}
}

func TestUpsertPatchThenPost(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPatch {
			w.WriteHeader(404) // 메시지 삭제됨 가정
			return
		}
		fmt.Fprint(w, `{"id":"999"}`)
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	id, err := c.Upsert([]any{}, "123")
	if err != nil {
		t.Fatal(err)
	}
	if id != "999" {
		t.Errorf("404 후 새 POST id: %s", id)
	}
	if len(calls) != 2 || !strings.HasPrefix(calls[0], "PATCH") || !strings.HasPrefix(calls[1], "POST") {
		t.Errorf("PATCH→POST 순서: %v", calls)
	}
}

func TestUpsertPatchSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()
	id, err := NewClient(srv.URL).Upsert([]any{}, "123")
	if err != nil || id != "123" {
		t.Errorf("PATCH 성공 시 기존 id 유지: %s, %v", id, err)
	}
}

func TestCardStateRoundTrip(t *testing.T) {
	p := CardStatePath(t.TempDir())
	if err := SaveCardState(p, &CardState{MessageID: "42", LastLevels: map[string]string{"claude": "green"}}); err != nil {
		t.Fatal(err)
	}
	s := LoadCardState(p)
	if s.MessageID != "42" || s.LastLevels["claude"] != "green" {
		t.Errorf("round-trip: %+v", s)
	}
}
