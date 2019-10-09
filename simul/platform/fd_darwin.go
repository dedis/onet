package platform

import (
	"syscall"

	"go.dedis.ch/onet/v3/log"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"
)

func init() {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Fatal("Error Getting Rlimit ", err)
	}

	// We observed that with Go 1.11.5, we were getting
	// 24576, and then with Go 1.12, we started getting "invalid argument".
	// See https://github.com/golang/go/issues/30401

	// On Darwin, the real fd max is given by sysctl.
	res, err := unix.Sysctl("kern.maxfilesperproc")
	if err != nil || len(res) != 3 {
		// In case of error, fall back to something reasonable.
		res = "10240"
	}
	// res is type string, but according to sysctl(3), it should be interpreted
	// as an int32. It seems to be little-endian. And for some reason, there are only
	// 3 bytes.
	rLimit.Max = uint64(res[0]) | uint64(res[1])<<8 | uint64(res[2])<<16

	if rLimit.Cur < rLimit.Max {
		rLimit.Cur = rLimit.Max
		err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
		if err != nil {
			log.Warn("Error Setting Rlimit:", err)
		}
	}

	err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	log.Info("File descriptor limit is:", rLimit.Cur)
}

// CheckOutOfFileDescriptors tries to duplicate the stdout file descriptor
// and throws an error if it cannot do it. This is a horrible hack mainly for
// MacOSX where the file descriptor limit is quite low and we need to tell
// people running simulations what they can do about it.
func CheckOutOfFileDescriptors() error {
	// Check if we're out of file descriptors
	newFS, err := syscall.Dup(syscall.Stdout)
	if err != nil {
		return xerrors.New(`Out of file descriptors. You might want to do something like this for Mac OSX:
    sudo sysctl -w kern.maxfiles=122880
    sudo sysctl -w kern.maxfilesperproc=102400
    sudo sysctl -w kern.ipc.somaxconn=20480`)
	}
	if err = syscall.Close(newFS); err != nil {
		return xerrors.New("Couldn't close new file descriptor: " + err.Error())
	}
	return nil
}
