//go:build !js && !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !windows

package assetbuild

import (
	"errors"
	"os"
)

func tryLock(_ *os.File) error {
	return errors.New("asset build locking is unsupported on this platform")
}
