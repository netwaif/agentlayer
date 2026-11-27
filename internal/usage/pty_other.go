//go:build !darwin && !linux

package usage

import (
	"errors"
	"os"
)

func openPTY() (*os.File, *os.File, error) {
	return nil, nil, errors.New("이 운영체제에서는 가상 터미널을 열 수 없음")
}
