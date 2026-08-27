package discord

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/starter"
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

func fixtureData() CardData {
	agents := []*state.Agent{
		{ID: "claude-7", Kind: "claude", State: state.StateWaiting, Task: "승인 대기",
			Tmux: state.TmuxRef{Session: "collab-bot"}, CWD: "/Users/soonho/ai-folder/collab",
			StateSince: t0.Add(-8 * time.Minute), UpdatedAt: t0.Add(-8 * time.Minute)},
		// WORK인데 갱신이 오래 끊김 → 정체 의심 "작업중?"
		{ID: "codex-3", Kind: "codex", State: state.StateWorking,
			Tmux: state.TmuxRef{Session: "codex-bridge"}, CWD: "/Users/soonho/bridge",
			StateSince: t0.Add(-2 * time.Hour), UpdatedAt: t0.Add(-2 * time.Hour)},
		{ID: "claude-9", Kind: "claude", State: state.StateDead,
			CWD: "/Users/soonho/gone", StateSince: t0.Add(-time.Hour)},
	}
	// 에이전트 ID 키 — 같은 폴더의 다른 종류 에이전트와 오귀속되지 않게
	ctx := map[string]usage.CtxInfo{
		"claude-7": {Model: "Opus 5 (1M context)", UsedPct: pf(16), TS: t0.Add(-3 * time.Minute)},
	}
	return CardData{
		Pay:    fixturePayload(),
		Agents: agents,
		Ctx:    ctx,
		Wired:  map[string]string{"/Users/soonho/ai-folder/collab": "⌁collab방"},
		Branches: map[string]string{
			"/Users/soonho/ai-folder/collab": "agent/fix-card"},
		DefModels: map[string]string{"claude": "claude-fable-5", "codex": "gpt-5.6-sol"},
		Tasks:     []starter.Task{{Name: "hwpx-tag", Status: "진행중"}},
		Home:      "/Users/soonho",
	}
}

func TestBuildCard(t *testing.T) {
	d := fixtureData()
	comps := BuildCard(d, t0)
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
		// TUI 동등 정보
		"collab-bot",              // tmux 세션 이름
		"승인 대기",                 // TASK
		"⎇ agent/fix-card",        // worktree 브랜치
		"작업중?",                  // WORK 정체 의심
		"ctx 16%",                 // 게이지 대신 텍스트
		"3분",                     // ctx 스냅샷 나이
		"응답 필요 1",              // 상태 집계 요약
		"기본모델",                 // 기본모델 라인
		"⚠",                       // claude 기본이 Fable → 경고
		"gpt-5.6-sol",             // codex 기본모델
		"Gemini 자동",              // 미설정 = 자동
		"MultiAgent", "hwpx-tag(진행중)",
		"── claude", "── codex", // 종류 그룹 구분선
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

// 에이전트 행에는 게이지 막대를 그리지 않는다 — Discord 폰트에서 격자로
// 깨져 보이고, TUI도 행에는 "ctx N%" 텍스트만 쓴다 (게이지는 provider 창 전용).
func TestBuildCardAgentRowsHaveNoGauge(t *testing.T) {
	d := fixtureData()
	d.Pay = nil // provider 컨테이너 제외하고 에이전트 섹션만
	b, _ := json.Marshal(BuildCard(d, t0))
	out := string(b)
	if strings.Contains(out, "█") || strings.Contains(out, "░") {
		t.Error("에이전트 섹션에 게이지 막대가 있으면 안 됨")
	}
	if !strings.Contains(out, "### 에이전트") {
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
