//go:build darwin

package usage

import (
	"bytes"
	"os"

	"golang.org/x/sys/unix"
)

// openPTY는 가상 터미널 한 쌍(주·부)을 연다 — 공식 claude `/usage` 화면 읽기용.
func openPTY() (master *os.File, slave *os.File, err error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	fd := int(m.Fd())
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
