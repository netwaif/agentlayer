//go:build darwin

package usage

import (
	"bytes"
	"os"

	"golang.org/x/sys/unix"
)

// openPTY는 가상 터미널 한 쌍(주·부)을 연다 — 공식 claude `/usage` 화면 읽기용.
func openPTY() (master *os.File, slave *os.File, err error) {
	// Open the master in blocking mode. On macOS 14, os.OpenFile can
	// leave O_NONBLOCK set even after Fd(), so the first read returns EAGAIN
	// before the child has written anything.
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	m := os.NewFile(uintptr(fd), "/dev/ptmx")
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
		m.Close()
		return nil, nil, err
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
		m.Close()
		return nil, nil, err
	}
	var name [128]byte
	if err := ioctlPtr(fd, unix.TIOCPTYGNAME, name[:]); err != nil {
		m.Close()
		return nil, nil, err
	}
	n := bytes.IndexByte(name[:], 0)
	if n < 0 {
		n = len(name)
	}
	s, err := os.OpenFile(string(name[:n]), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		m.Close()
		return nil, nil, err
	}
	return m, s, nil
}
