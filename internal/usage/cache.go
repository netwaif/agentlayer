package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// coach는 콜드 실행에 분 단위가 걸릴 수 있다(codexbar 라이브 조회).
// TUI가 2초 폴링을 하므로 반드시 캐시와 실행 중복 방지가 필요하다.

var fetchMu sync.Mutex // coach 동시 실행 방지 (TryLock)

type cacheFile struct {
	TS      time.Time `json:"ts"`
	Payload *Payload  `json:"payload"`
}

func readCacheFile(dir string) cacheFile {
	var cached cacheFile
	if b, err := os.ReadFile(filepath.Join(dir, "usage-cache.json")); err == nil {
		_ = json.Unmarshal(b, &cached)
	}
	return cached
}

// ReadCached는 나이 불문 캐시를 읽기만 한다 — coach를 절대 실행하지 않는다.
// "일단 낡은 값이라도 즉시 그리고, 갱신은 뒤에서" 하는 표시용 경로
// (TUI 첫 페인트·카드 1차 게시). 나이는 Payload.TS로 화면이 밝힌다.
func ReadCached(dir string) *Payload {
	return readCacheFile(dir).Payload
}

// FetchCached는 dir의 캐시가 ttl 안이면 그걸 쓰고, 아니면 runner를 돌려
// 갱신한다. 다른 goroutine/프로세스가 이미 돌리는 중이면(락 실패) 낡은
// 캐시라도 즉시 돌려준다 — 관제 화면은 블로킹되면 안 된다.
func FetchCached(dir string, ttl time.Duration, runner func() ([]byte, error), now time.Time) *Payload {
	path := filepath.Join(dir, "usage-cache.json")
	cached := readCacheFile(dir)
	if cached.Payload != nil && now.Sub(cached.TS) < ttl {
		return cached.Payload
	}
	if !fetchMu.TryLock() {
		return cached.Payload // 갱신 중 — 낡은 값으로 버틴다
	}
	defer fetchMu.Unlock()
	pay, _ := Fetch(runner)
	if pay == nil {
		return cached.Payload
	}
	if b, err := json.Marshal(cacheFile{TS: now, Payload: pay}); err == nil {
		tmp := path + ".tmp"
		if os.WriteFile(tmp, b, 0o600) == nil {
			_ = os.Rename(tmp, path)
		}
	}
	return pay
}
