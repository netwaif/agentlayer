package usage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// 실측 coach --json 축약 fixture
const coachFixture = `{"ts":"2026-08-25T06:54:43Z","providers":{
  "claude":{"ok":true,"plan":"Claude Max 5x","email":"kshxxthm@gmail.com","level":"green",
    "action":"지금 큰 작업 돌리세요","reason":"단기 한도 77% 남고 장기도 여유라 끊길 걱정 없어요.",
    "windows":{"5h":{"left_pct":77,"reset_min":197},"7d":{"left_pct":16,"reset_min":7},"fable_7d":{"left_pct":36,"reset_min":7}}},
  "codex":{"ok":true,"plan":"plus","email":"kshxxthm@gmail.com","level":"green",
    "action":"큰 작업 돌려도 돼요","reason":"주간 한도가 80% 남아요.",
    "windows":{"7d":{"left_pct":80,"reset_min":8204}}},
  "antigravity":{"ok":true,"plan":"Starter","email":"knowhackking@gmail.com","level":"green",
    "action":"큰 작업 돌려도 돼요","reason":"주간 한도가 64% 남아요.",
    "windows":{"knowhackking":{"left_pct":64,"reset_min":5725},"aitipking":{"left_pct":null,"reset_min":null}}}}}`

func TestFetchParsesPayload(t *testing.T) {
	p, err := Fetch(func() ([]byte, error) { return []byte(coachFixture), nil })
	if err != nil {
		t.Fatal(err)
	}
	cl, ok := p.Providers["claude"]
	if !ok {
		t.Fatal("claude provider 있어야 함")
	}
	if cl.Level != "green" || cl.Action != "지금 큰 작업 돌리세요" {
		t.Errorf("level/action: %+v", cl)
	}
	w := cl.Windows["5h"]
	if w.LeftPct == nil || *w.LeftPct != 77 {
		t.Errorf("5h left_pct: %+v", w)
	}
	// null 윈도우 허용
	ag := p.Providers["antigravity"].Windows["aitipking"]
	if ag.LeftPct != nil {
		t.Errorf("null left_pct는 nil: %+v", ag)
	}
}

func TestFetchCoachMissing(t *testing.T) {
	p, err := Fetch(func() ([]byte, error) { return nil, errors.New("exec: coach not found") })
	if err != nil {
		t.Fatal("coach 부재는 에러 아님(패널만 생략)")
	}
	if p != nil {
		t.Error("부재 시 nil payload")
	}
}

func TestGauge(t *testing.T) {
	pct := 50.0
	g := Gauge(&pct, 10)
	if g != "█████░░░░░" {
		t.Errorf("50%%/10칸: %q", g)
	}
	if Gauge(nil, 4) != "░░░░" {
		t.Errorf("nil은 빈 바: %q", Gauge(nil, 4))
	}
	full := 100.0
	if Gauge(&full, 4) != "████" {
		t.Errorf("100%%: %q", Gauge(&full, 4))
	}
}

func TestResetLabel(t *testing.T) {
	cases := []struct {
		min  float64
		want string
	}{
		{30, "30분 후"},
		{197, "3시간 후"},
		{8204, "6일 후"},
	}
	for _, c := range cases {
		m := c.min
		if got := ResetLabel(&m); got != c.want {
			t.Errorf("ResetLabel(%v) = %q, want %q", c.min, got, c.want)
		}
	}
	if ResetLabel(nil) != "" {
		t.Error("nil은 빈 문자열")
	}
}

// nvm 사용자(시스템 node 없음)도 npx를 찾아야 한다 — 최신 버전 bin을 후보에 넣는다.
func TestToolDirsIncludesNvmLatest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, v := range []string{"v20.1.0", "v22.3.0", "v9.9.9"} {
		os.MkdirAll(filepath.Join(home, ".nvm", "versions", "node", v, "bin"), 0o755)
	}
	dirs := toolDirs()
	want := filepath.Join(home, ".nvm", "versions", "node", "v22.3.0", "bin")
	found := false
	for _, d := range dirs {
		if d == want {
			found = true
		}
	}
	if !found {
		t.Errorf("nvm 최신 bin 누락: %v", dirs)
	}
}
