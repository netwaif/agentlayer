package discord

import (
	"os"
	"path/filepath"
	"syscall"
)

// RunCoalesced는 카드 게시를 단일 비행으로 합친다.
// dirty를 먼저 찍고 잠금을 비차단으로 시도한다. 잠금을 못 잡으면 다른
// 게시자가 실행 중이라는 뜻 — dirty만 남기고 돌아가면 보유자가 루프
// 조건에서 그것을 보고 최신 상태로 다시 게시한다. 잠금을 잡으면
// dirty가 없어질 때까지 게시를 반복한다(게시 중 도착한 트리거 수습).
func RunCoalesced(dir string, publish func() error) error {
	dirty := filepath.Join(dir, "card.dirty")
	if err := os.WriteFile(dirty, nil, 0o600); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "card.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil // 보유자에게 위임
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	for {
		if _, err := os.Stat(dirty); err != nil {
			return nil
		}
		if err := os.Remove(dirty); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := publish(); err != nil {
			return err
		}
	}
}
