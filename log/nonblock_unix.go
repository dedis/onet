// +build !windows

package log

import (
	"os"
	"syscall"
)

func setNonblock() {
	syscall.SetNonblock(int(os.Stderr.Fd()), true)
	syscall.SetNonblock(int(os.Stdout.Fd()), true)
	return
}
