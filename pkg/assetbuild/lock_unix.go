//go:build !js && (linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package assetbuild

import (
	"errors"
	"os"
	"syscall"
)

func tryLock(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return errLockBusy
	}

	return err
}
