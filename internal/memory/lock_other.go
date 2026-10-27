//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package memory

import (
 "errors"
 "os"
)
func lockProfile(home string,mode LockMode)(*os.File,error){if mode==LockExclusive{return nil,errors.New("daemon unsupported on this platform")};return nil,nil}
func unlockProfile(f *os.File)error{return nil}
