package discord

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// 기본 동작: 잠금이 비어 있으면 게시가 정확히 1번 일어난다.
func TestRunCoalescedPublishes(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	if err := RunCoalesced(dir, func() error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("publish 호출 %d회, 1회 기대", calls)
	}
}

// 다른 게시자가 잠금을 쥐고 있으면 게시 없이 dirty만 남기고 돌아간다
// (보유자가 dirty를 보고 다시 게시할 책임).
func TestRunCoalescedSecondCallerSkips(t *testing.T) {
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "card.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := RunCoalesced(dir, func() error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("잠금 중인데 publish %d회 호출됨", calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "card.dirty")); err != nil {
		t.Fatalf("dirty 파일이 남아 있어야 함: %v", err)
	}
}

// 게시 도중 새 트리거(dirty)가 찍히면 끝나고 한 번 더 게시한다.
func TestRunCoalescedRepublishesWhenDirtyDuringPublish(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	err := RunCoalesced(dir, func() error {
		calls++
		if calls == 1 {
			// 게시 중 도착한 동시 트리거를 흉내낸다
			if err := os.WriteFile(filepath.Join(dir, "card.dirty"), nil, 0o600); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("publish 호출 %d회, 2회 기대", calls)
	}
}
