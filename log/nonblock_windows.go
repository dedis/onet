// +build windows

package log

import (
	"os"
	"syscall"
)

func setNonblock() {
	syscall.SetNonblock(syscall.Handle(os.Stderr.Fd()), true)
	syscall.SetNonblock(syscall.Handle(os.Stdout.Fd()), true)
	return
}
