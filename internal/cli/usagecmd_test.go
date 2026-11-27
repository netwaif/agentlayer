package cli

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/netwaif/agentlayer/internal/usage"
)

func f(v float64) *float64 { return &v }

func TestUsageTextNilAndEmpty(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	if s := UsageText(nil, now); !strings.Contains(s, "아직 못 읽었습니다") {
		t.Errorf("nil: %q", s)
	}
	p := &usage.Payload{Providers: map[string]usage.Provider{"claude": {OK: false}}}
	if s := UsageText(p, now); !strings.Contains(s, "읽을 수 있는 공급자가 없습니다") {
		t.Errorf("empty: %q", s)
	}
}

func TestUsageTextOrderAndWindows(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 10, 0, 0, time.UTC)
	p := &usage.Payload{TS: "2026-10-07T09:00:00Z", Providers: map[string]usage.Provider{
		"codex":  {OK: true, Action: "진행", Email: "b@x", Windows: map[string]usage.Window{"5h": {LeftPct: f(80)}}},
		"claude": {OK: true, Action: "주의", Email: "a@x", Reason: "주간 25% 남음", Windows: map[string]usage.Window{"7d": {LeftPct: f(25), ResetMin: f(90)}, "5h": {LeftPct: f(60)}, "zeta": {}}},
	}}
	s := UsageText(p, now)
	if !strings.HasPrefix(s, "(10분 전 측정)") {
		t.Errorf("age head: %q", s)
	}
	if strings.Index(s, "Claude") > strings.Index(s, "Codex") {
		t.Errorf("claude 먼저: %q", s)
	}
	lines := strings.Split(s, "\n")
	var order []string
	for _, l := range lines {
		tl := strings.TrimSpace(l)
		for _, k := range []string{"5h", "7d", "zeta"} {
			if strings.HasPrefix(tl, k+" ") {
				order = append(order, k)
			}
		}
	}
	if strings.Join(order, ",") != "5h,7d,zeta,5h" {
		t.Errorf("창 순서 5h·7d·기타 — got %v", order)
	}
	if !strings.Contains(s, "25%") || !strings.Contains(s, "리셋") || !strings.Contains(s, "—%") || !strings.Contains(s, "주간 25% 남음") {
		t.Errorf("값·리셋·빈 창·사유: %q", s)
	}
}

func TestRunUsageJSON(t *testing.T) {
	t.Setenv("AGENTLAYER_USAGE_SOURCE", "codexbar")
	dir := t.TempDir()
	runner := func() ([]byte, error) {
		return []byte(`{"ts":"2026-10-07T09:00:00Z","providers":{"claude":{"ok":true,"action":"진행","windows":{"5h":{"left_pct":70}}}}}`), nil
	}
	var out strings.Builder
	if err := RunUsage(&out, dir, runner, time.Date(2026, 10, 7, 9, 1, 0, 0, time.UTC), []string{"--json"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"left_pct":70`) || !strings.HasPrefix(out.String(), "{") {
		t.Errorf("json: %q", out.String())
	}
	var none strings.Builder
	_ = RunUsage(&none, t.TempDir(), func() ([]byte, error) { return nil, errors.New("coach 없음") }, time.Now(), []string{"--json"})
	if strings.TrimSpace(none.String()) != "null" {
		t.Errorf("페이로드 없으면 null: %q", none.String())
	}
}

// --cached: 캐시만 읽고 coach는 절대 돌리지 않는다(메뉴 막대 앱의 즉시 페인트용).
// 캐시가 낡았어도 그대로, 캐시가 없으면 null — runner가 불리면 실패.
func TestRunUsageJSONCached(t *testing.T) {
	t.Setenv("AGENTLAYER_USAGE_SOURCE", "codexbar")
	dir := t.TempDir()
	called := false
	runner := func() ([]byte, error) {
		called = true
		return []byte(`{"ts":"2026-10-07T09:00:00Z","providers":{"claude":{"ok":true,"action":"진행","windows":{"5h":{"left_pct":70}}}}}`), nil
	}
	var none strings.Builder
	if err := RunUsage(&none, dir, runner, time.Date(2026, 10, 7, 9, 1, 0, 0, time.UTC), []string{"--json", "--cached"}); err != nil {
		t.Fatal(err)
	}
	if called || strings.TrimSpace(none.String()) != "null" {
		t.Errorf("캐시 없으면 coach 안 부르고 null: called=%v out=%q", called, none.String())
	}
	// 캐시를 채운 뒤(일반 경로) 한참 지나도 --cached는 낡은 값을 즉시 준다.
	var fill strings.Builder
	_ = RunUsage(&fill, dir, runner, time.Date(2026, 10, 7, 9, 1, 0, 0, time.UTC), []string{"--json"})
	called = false
	var stale strings.Builder
	_ = RunUsage(&stale, dir, runner, time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC), []string{"--json", "--cached"})
	if called || !strings.Contains(stale.String(), `"left_pct":70`) {
		t.Errorf("낡은 캐시 그대로, coach 호출 없이: called=%v out=%q", called, stale.String())
	}
}
