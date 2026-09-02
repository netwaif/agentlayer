//go:build !(darwin || linux)

package cli

import "errors"

func makeCbreak(fd uintptr) (func(), error) { return nil, errors.New("cbreak 미지원 플랫폼") }
