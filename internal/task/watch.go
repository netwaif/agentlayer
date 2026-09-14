// internal/task/watch.go
package task

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// MaxReportBytes — 보고 한 건 상한. 넘으면 격리.
const MaxReportBytes = 16384

// Poll은 pending/의 json 파일을 이름순으로 하나씩 검사해 첫 정상 건을 received/로 옮기고 반환한다.
// 불량(심볼릭 링크·과대·깨진 JSON·version≠1·파일명≠id)은 quarantine/로 옮기고 계속 본다.
func Poll(inbox string) (*Report, bool, error) {
	for _, sub := range []string{"pending", "received", "quarantine"} {
		if err := os.MkdirAll(filepath.Join(inbox, sub), 0o700); err != nil {
			return nil, false, err
		}
	}
	files, err := filepath.Glob(filepath.Join(inbox, "pending", "*.json"))
	if err != nil {
		return nil, false, err
	}
	sort.Strings(files)
	for _, f := range files {
		r, verr := readReport(f)
		if verr != nil {
			_ = os.Rename(f, filepath.Join(inbox, "quarantine", filepath.Base(f)))
			continue
		}
		if err := os.Rename(f, filepath.Join(inbox, "received", filepath.Base(f))); err != nil {
			return nil, false, err
		}
		return r, true, nil
	}
	return nil, false, nil
}

func readReport(f string) (*Report, error) {
	fi, err := os.Lstat(f)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("symlink")
	}
	if fi.Size() > MaxReportBytes {
		return nil, errors.New("oversize")
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}
	var r Report
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	base := filepath.Base(f)
	if r.Version != 1 || len(r.ID) != 32 || base != r.ID+".json" || r.TaskID == "" || r.To == "" {
		return nil, errors.New("identity")
	}
	return &r, nil
}

// Watch는 interval마다 Poll하고 정상 건마다 emit을 부른다. once면 첫 건 뒤 nil로 종료,
// 아니면 ctx가 끝날 때까지 돈다(ctx.Err() 반환).
func Watch(ctx context.Context, inbox string, interval time.Duration, once bool, emit func(*Report)) error {
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		for {
			r, ok, err := Poll(inbox)
			if err != nil {
				return err
			}
			if !ok {
				break
			}
			emit(r)
			if once {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
