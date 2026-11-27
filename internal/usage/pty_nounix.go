//go:build !darwin && !linux

package usage

import (
	"os"
	"os/exec"
)

func setWinsize(*os.File, uint16, uint16) error { return nil }
func tryLock(string) (*os.File, bool)           { return nil, false }

// openPTY가 오류를 내므로 부르지 않는다(빌드만 맞춘다).
func ptyAttr(*exec.Cmd)      {}
func killPTYGroup(*exec.Cmd) {}

func detachProbe(cmd *exec.Cmd) {}
