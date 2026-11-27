//go:build darwin || linux

package usage

import (
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func ioctlPtr(fd int, req uint, buf []byte) error {
	_, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(unsafe.Pointer(&buf[0])))
	if e != 0 {
		return e
	}
	return nil
}

// setWinsize는 가상 터미널 크기(행·열).
func setWinsize(f *os.File, rows, cols uint16) error {
	return unix.IoctlSetWinsize(int(f.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: rows, Col: cols})
}

// tryLock은 파일 잠금(다른 프로세스가 쥐고 있으면 false) — 동시에 하나만 띄우게.
func tryLock(path string) (*os.File, bool) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false
	}
	if unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		f.Close()
		return nil, false
	}
	return f, true
}

// ptyAttr — 새 세션의 제어 터미널로 slave를 쓴다(claude가 TTY로 그린다).
func ptyAttr(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true} }

// killPTYGroup — 새 세션(=프로세스 그룹)째 강제 종료.
func killPTYGroup(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

func detachProbe(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
