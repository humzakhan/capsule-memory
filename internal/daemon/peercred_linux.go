//go:build linux

package daemon

import (
	"net"
	"os"
	"syscall"
)

func peerAllowed(c *net.UnixConn) bool {
	raw, e := c.SyscallConn()
	if e != nil {
		return false
	}
	allowed := false
	e = raw.Control(func(fd uintptr) {
		cred, e := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		allowed = e == nil && cred.Uid == uint32(os.Getuid())
	})
	return e == nil && allowed
}
func socketLimit() int { return 108 }
