//go:build unix

package remote

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// EnsureControlDir은 ssh ControlMaster 소켓 폴더를 준비한다. 없으면 0700으로 만들고, 이미 있으면 내 것(uid)·
// 0700·진짜 폴더(심볼릭 링크 아님)인지 확인한다. /tmp 아래 예측 가능한 이름이라 다른 사용자가 먼저 만들어 둘 수
// 있다 — 그 안에 소켓을 두면 내 ssh 연결을 남이 탄다. 어긋나면 고쳐 쓰지 않고 거부한다.
func EnsureControlDir(dir string) error { return ensureControlDir(dir, os.Getuid()) }

func ensureControlDir(dir string, uid int) error {
	err := os.Mkdir(dir, 0o700)
	if err == nil {
		// umask가 소유자 비트를 깎았을 수 있다 — 방금 내가 만든 폴더라 맞춰도 안전하다.
		return os.Chmod(dir, 0o700)
	}
	if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("ssh 소켓 폴더 만들기: %w", err)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("ssh 소켓 폴더 확인: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("ssh 소켓 폴더 %s: 심볼릭 링크입니다 — 지우고 다시 실행하세요", dir)
	}
	if !fi.IsDir() {
		return fmt.Errorf("ssh 소켓 폴더 %s: 폴더가 아닙니다 — 지우고 다시 실행하세요", dir)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != uid {
		return fmt.Errorf("ssh 소켓 폴더 %s: 소유자가 현재 사용자(uid %d)가 아닙니다 — 지우고 다시 실행하세요", dir, uid)
	}
	if perm := fi.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky); perm != 0o700 {
		return fmt.Errorf("ssh 소켓 폴더 %s: 권한이 0700이 아닙니다(%04o) — 'chmod 700 %s' 뒤 다시 실행하세요", dir, fi.Mode().Perm(), dir)
	}
	return nil
}
