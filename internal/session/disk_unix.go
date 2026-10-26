//go:build linux || darwin

package session
import("syscall";"github.com/humzakhan/capsule-memory/internal/memory")
func diskFree(path string)(int64,error){var s syscall.Statfs_t;if syscall.Statfs(path,&s)!=nil{return 0,memory.ErrStorage};return int64(s.Bavail)*int64(s.Bsize),nil}
