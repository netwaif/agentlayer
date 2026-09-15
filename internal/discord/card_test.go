package discord

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/board"
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
		Wired:  map[string]string{"collab-bot": "⌁collab방"},
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
		"collab-bot",       // tmux 세션 이름
		"승인 대기",            // TASK
		"⎇ agent/fix-card", // worktree 브랜치
		"작업중?",             // WORK 정체 의심
		"ctx 16%",          // 게이지 대신 텍스트
		"3분",               // ctx 스냅샷 나이
		"응답 필요 1",          // 상태 집계 요약
		"기본모델",             // 기본모델 라인
		"⚠",                // claude 기본이 Fable → 경고
		"gpt-5.6-sol",      // codex 기본모델
		"Gemini 자동",        // 미설정 = 자동
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

// 봇 스레드 창(2026-09-13): 같은 세션의 메인·스레드 pane은 카드에서 한 행 — 사용자가
// "세션들이 중복돼서 나온다"고 지적한 것. 상태는 급한 쪽, 집계도 접힌 기준.
func TestBuildCardFoldsBotThreadWindow(t *testing.T) {
	d := CardData{Home: "/Users/soonho", Agents: []*state.Agent{
		{ID: "claude-35", Kind: "claude", State: state.StateWorking, Task: "스레드 작업",
			Tmux:       state.TmuxRef{Session: "dev-claudecode", Window: 1, WindowName: "t552990", PaneID: "%35"},
			CWD:        "/opt/data/ai-company/dev/claude",
			StateSince: t0.Add(-time.Minute), UpdatedAt: t0.Add(-time.Minute)},
		{ID: "claude-33", Kind: "claude", State: state.StateIdle, Task: "메인 대기",
			Tmux:       state.TmuxRef{Session: "dev-claudecode", Window: 0, WindowName: "dev-claudecode", PaneID: "%33"},
			CWD:        "/opt/data/bots/dev",
			StateSince: t0.Add(-time.Hour), UpdatedAt: t0.Add(-time.Hour)},
	}}
	b, err := json.Marshal(BuildCard(d, t0))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if n := strings.Count(s, "**dev-claudecode**"); n != 1 {
		t.Errorf("세션 행은 1개여야 함(%d):\n%s", n, s)
	}
	if !strings.Contains(s, "**dev-claudecode** (스레드 1)") {
		t.Errorf("스레드 배지 없음:\n%s", s)
	}
	if !strings.Contains(s, "작업중 1") || strings.Contains(s, "대기 1") {
		t.Errorf("집계는 접힌 행 기준(작업중 1만):\n%s", s)
	}
}

func boardFixture() *BoardData {
	return &BoardData{Name: "AI 치트키 회사", StaleLimit: 30 * time.Minute, Cards: []board.Card{
		{ID: "VIDEO-07-TOPICS", Column: board.ColBlocked, Session: "search-youtube-bot:t170966", State: "WAIT",
			Updated: t0.Add(-45 * time.Minute), LastLog: "[2026-08-25 11:15] [ASK] 참고자료 폴더 밖을 읽어도 될까요?"},
		{ID: "VIDEO-07-SCRIPT", Column: board.ColRunning, Session: "collab-bot", State: "WORK", Updated: t0.Add(-3 * time.Minute)},
		{ID: "VIDEO-07-THUMB", Column: board.ColReady, Parents: []string{"VIDEO-07-TOPICS"}, Ready: t0.Add(-41 * time.Minute)},
		{ID: "VIDEO-06", Column: board.ColDone, Updated: t0.Add(-time.Hour)},
		{ID: "VIDEO-08", Column: board.ColTodo, Parents: []string{"VIDEO-07-THUMB"}},
		{ID: "VIDEO-07-REVIEW", Column: board.ColReview, Session: "sendmanual-bot", State: "DONE", Updated: t0.Add(-time.Minute)},
	}}
}

func TestBoardContainerOrderCountsAndStale(t *testing.T) {
	c := boardContainer(boardFixture(), t0)
	if c == nil {
		t.Fatal("컨테이너 없음")
	}
	txt := containerText(c) // 기존 테스트 헬퍼가 있으면 재사용, 없으면 comps의 content를 이어 붙이는 헬퍼를 이 파일에 추가
	for _, want := range []string{
		"### 업무 보드 — AI 치트키 회사",
		"ready 1 · running 1 · blocked 1 · review 1 · todo 1 · done 1",
		"VIDEO-07-TOPICS", "search-youtube-bot:t170966", "WAIT", "⚠",
		"[ASK] 참고자료 폴더 밖을 읽어도 될까요?",
		"VIDEO-07-THUMB", "← VIDEO-07-TOPICS",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("보드 텍스트에 %q 없음:\n%s", want, txt)
		}
	}
	// 정렬: blocked → review → running → ready
	iB, iRv, iRn, iRd := strings.Index(txt, "VIDEO-07-TOPICS"), strings.Index(txt, "VIDEO-07-REVIEW"),
		strings.Index(txt, "VIDEO-07-SCRIPT"), strings.Index(txt, "VIDEO-07-THUMB")
	if !(iB < iRv && iRv < iRn && iRn < iRd) {
		t.Errorf("정렬 어긋남: %d %d %d %d", iB, iRv, iRn, iRd)
	}
	// todo·done은 행으로 안 나옴
	if strings.Contains(txt, "VIDEO-06 ") || strings.Contains(txt, "VIDEO-08") {
		t.Error("todo·done 카드는 집계에만")
	}
	// ⚠는 blocked 45분·ready 41분에만, running 3분에는 없음
	line := lineContaining(txt, "VIDEO-07-SCRIPT")
	if strings.Contains(line, "⚠") {
		t.Errorf("running에 ⚠: %s", line)
	}
}

func TestBoardContainerCapsAtEightRows(t *testing.T) {
	b := &BoardData{Name: "X", StaleLimit: time.Hour}
	for i := 0; i < 11; i++ {
		b.Cards = append(b.Cards, board.Card{ID: fmt.Sprintf("T-%02d", i), Column: board.ColRunning, Updated: t0})
	}
	txt := containerText(boardContainer(b, t0))
	if strings.Count(txt, "T-") != 8 || !strings.Contains(txt, "외 3") {
		t.Errorf("8행 상한 위반:\n%s", txt)
	}
}

func TestBuildCardOmitsBoardWhenNil(t *testing.T) {
	d := fixtureData()
	d.Board = nil
	if s := fmt.Sprint(BuildCard(d, t0)); strings.Contains(s, "업무 보드") {
		t.Error("Board nil이면 컨테이너 없음")
	}
	d.Board = &BoardData{Name: "빈 회사"}
	if s := fmt.Sprint(BuildCard(d, t0)); strings.Contains(s, "업무 보드") {
		t.Error("카드 0장이면 컨테이너 없음")
	}
	d.Board = boardFixture()
	if s := fmt.Sprint(BuildCard(d, t0)); !strings.Contains(s, "업무 보드") {
		t.Error("카드가 있으면 컨테이너 있음")
	}
}

func containerText(c map[string]any) string {
	var sb strings.Builder
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if s, ok := x["content"].(string); ok {
				sb.WriteString(s + "\n")
			}
			if comps, ok := x["components"].([]any); ok {
				for _, c := range comps {
					walk(c)
				}
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(c)
	return sb.String()
}

func lineContaining(txt, sub string) string {
	for _, l := range strings.Split(txt, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}
