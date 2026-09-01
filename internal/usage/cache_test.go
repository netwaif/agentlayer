package usage

import (
	"sync"
	"testing"
	"time"
)

func TestFetchCachedFreshSkipsRunner(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	runner := func() ([]byte, error) { calls++; return []byte(coachFixture), nil }
	now := time.Now()
	p1 := FetchCached(dir, 5*time.Minute, runner, now)
	if p1 == nil || calls != 1 {
		t.Fatalf("첫 호출은 runner 실행: calls=%d", calls)
	}
	p2 := FetchCached(dir, 5*time.Minute, runner, now.Add(time.Minute))
	if p2 == nil || calls != 1 {
		t.Errorf("TTL 안이면 캐시 사용: calls=%d", calls)
	}
	FetchCached(dir, 5*time.Minute, runner, now.Add(6*time.Minute))
	if calls != 2 {
		t.Errorf("TTL 지나면 재실행: calls=%d", calls)
	}
}

func TestFetchCachedRunnerFailKeepsStale(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	FetchCached(dir, time.Minute, func() ([]byte, error) { return []byte(coachFixture), nil }, now)
	p := FetchCached(dir, time.Minute, func() ([]byte, error) { return nil, errTest }, now.Add(2*time.Minute))
	if p == nil {
		t.Error("runner 실패 시 낡은 캐시라도 반환")
	}
}

func TestFetchCachedInFlightReturnsStale(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	FetchCached(dir, time.Minute, func() ([]byte, error) { return []byte(coachFixture), nil }, now)

	slowStarted := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		FetchCached(dir, time.Minute, func() ([]byte, error) {
			close(slowStarted)
			<-release
			return []byte(coachFixture), nil
		}, now.Add(2*time.Minute))
	}()
	<-slowStarted
	// 갱신 중인 상태에서 두 번째 호출 — 즉시 낡은 캐시 반환해야 함
	done := make(chan *Payload, 1)
	go func() {
		done <- FetchCached(dir, time.Minute, func() ([]byte, error) {
			t.Error("중복 실행 금지")
			return nil, errTest
		}, now.Add(2*time.Minute))
	}()
	select {
	case p := <-done:
		if p == nil {
			t.Error("낡은 캐시 반환")
		}
	case <-time.After(2 * time.Second):
		t.Error("블로킹되면 안 됨")
	}
	close(release)
	wg.Wait()
}

func TestReadCachedReturnsExpiredCache(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Add(-24 * time.Hour) // 캐시를 하루 묵힌다
	FetchCached(dir, time.Minute, func() ([]byte, error) { return []byte(coachFixture), nil }, now)
	p := ReadCached(dir)
	if p == nil {
		t.Error("나이 불문 캐시 반환 — 표시용 stale 읽기")
	}
}

func TestReadCachedEmptyDir(t *testing.T) {
	if p := ReadCached(t.TempDir()); p != nil {
		t.Error("캐시 없으면 nil")
	}
}

var errTest = &testErr{}

type testErr struct{}

func (*testErr) Error() string { return "test" }
