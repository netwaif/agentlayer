package browser

import (
	"testing"
	"time"
)

const pmsetSample = `Assertion status system-wide:
   PreventUserIdleDisplaySleep    1
Listed by owning process:
   pid 112(powerd): [0x000000110008804d] 14:44:47 ExternalMedia named: "com.apple.powermanagement.externalmediamounted"  
   pid 97639(Google Chrome): [0x00006bb700058528] 07:05:30 NoDisplaySleepAssertion named: "Capturing"  
   pid 97639(Google Chrome): [0x0000cff80005a2c6] 00:00:15 NoDisplaySleepAssertion named: "Capturing"  
   pid 5220(caffeinate): [0x0000cf310001a1da] 00:01:02 PreventUserIdleSystemSleep named: "caffeinate command-line tool"  
`

func TestParseCaptureLocks(t *testing.T) {
	got := ParseCaptureLocks([]byte(pmsetSample))
	if len(got) != 1 {
		t.Fatalf("Chrome pid 하나만: %v", got)
	}
	if age := got[97639]; age != 7*time.Hour+5*time.Minute+30*time.Second {
		t.Errorf("가장 오래된 잠금 나이여야 함: %v", age)
	}
}

func TestHasCaptureLockMatchesAgentChromeOnly(t *testing.T) {
	lsof := func(args ...string) ([]byte, error) { return []byte("p97639\nf5\n"), nil }
	pm := func() ([]byte, error) { return []byte(pmsetSample), nil }
	if !HasCaptureLock(9222, lsof, pm) {
		t.Error("에이전트 Chrome(97639)에 잠금이 있으면 true")
	}
	other := func(args ...string) ([]byte, error) { return []byte("p123\n"), nil }
	if HasCaptureLock(9222, other, pm) {
		t.Error("다른 pid면 false")
	}
	if ChromePID(9222, lsof) != 97639 {
		t.Error("ChromePID 파싱")
	}
}

func TestThrottleOK(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if !ThrottleOK(dir, "x", time.Minute, now) {
		t.Fatal("첫 호출은 통과")
	}
	if ThrottleOK(dir, "x", time.Minute, now.Add(10*time.Second)) {
		t.Error("간격 안이면 거부")
	}
	if !ThrottleOK(dir, "x", time.Minute, now.Add(2*time.Minute)) {
		t.Error("간격 지나면 통과")
	}
}

func TestCaptureJanitorSupported(t *testing.T) {
	if !CaptureJanitorSupported("darwin") {
		t.Error("darwin은 pmset이 있어 지원")
	}
	if CaptureJanitorSupported("linux") {
		t.Error("linux는 pmset이 없어 미지원")
	}
}
