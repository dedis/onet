// +build !windows

package platform

import (
	"errors"
	"syscall"

	"go.dedis.ch/onet/v3/log"
)

// By default in simulation we update the per-process file descriptor limit
// to the maximal limit.
func init() {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Fatal("Error Getting Rlimit ", err)
	}

	if rLimit.Cur < rLimit.Max {
		rLimit.Cur = rLimit.Max
		err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
		if err != nil {
			log.Warn("Error Setting Rlimit:", err)
		}
	}

	err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Error("Couldn't raise Rlimit: " + err.Error())
	}
}

// CheckOutOfFileDescriptors tries to duplicate the stdout file descriptor
// and throws an error if it cannot do it. This is a horrible hack mainly for
// MacOSX where the file descriptor limit is quite low and we need to tell
// people running simulations what they can do about it.
func CheckOutOfFileDescriptors() error {
	// Check if we're out of file descriptors
	newFS, err := syscall.Dup(syscall.Stdout)
	if err != nil {
		return errors.New(`Out of file descriptors. You might want to do something like this for Mac OSX:
    sudo sysctl -w kern.maxfiles=122880
    sudo sysctl -w kern.maxfilesperproc=102400
    sudo sysctl -w kern.ipc.somaxconn=20480`)
	}
	if err = syscall.Close(newFS); err != nil {
		return errors.New("Couldn't close new file descriptor: " + err.Error())
	}
	return nil
}
