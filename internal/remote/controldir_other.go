//go:build !unix

package remote

import "os"

// EnsureControlDir — 소유자·권한 비트가 없는 OS에서는 만들기만 한다(배포 대상은 darwin·linux).
func EnsureControlDir(dir string) error { return os.MkdirAll(dir, 0o700) }
