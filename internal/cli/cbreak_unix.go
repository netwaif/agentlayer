//go:build darwin || linux

package cli

import "golang.org/x/sys/unix"

// makeCbreak은 키를 한 글자씩 즉시 읽되(ICANON·ECHO 끔) 출력 후처리(OPOST, \n→\r\n)와
// Ctrl-C(ISIG)는 그대로 둔다. raw 모드(term.MakeRaw)는 OPOST까지 꺼서 pick 안내문이
// 계단식으로 밀려 관제탑 팝업 화면이 깨졌다(2026-09-03 촬영 실측).
func makeCbreak(fd uintptr) (restore func(), err error) {
	t, err := unix.IoctlGetTermios(int(fd), ioctlReadTermios)
	if err != nil {
		return nil, err
	}
	old := *t
	t.Lflag &^= unix.ICANON | unix.ECHO
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(int(fd), ioctlWriteTermios, t); err != nil {
		return nil, err
	}
	return func() { _ = unix.IoctlSetTermios(int(fd), ioctlWriteTermios, &old) }, nil
}
