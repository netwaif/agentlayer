//go:build linux

package usage

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openPTY() (master *os.File, slave *os.File, err error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	fd := int(m.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		m.Close()
		return nil, nil, err
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		m.Close()
		return nil, nil, err
	}
	s, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		m.Close()
		return nil, nil, err
	}
	return m, s, nil
}
