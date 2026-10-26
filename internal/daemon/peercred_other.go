//go:build !linux

package daemon
import "net"
// macOS uses private directory/socket permissions; no stdlib peer UID API.
func peerAllowed(*net.UnixConn)bool{return true}
func socketLimit()int{return 104}
