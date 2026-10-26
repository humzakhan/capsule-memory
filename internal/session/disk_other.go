//go:build !linux && !darwin

package session
import "errors"
func diskFree(string)(int64,error){return 0,errors.New("capture unsupported on this platform")}
