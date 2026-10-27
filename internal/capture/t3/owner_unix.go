//go:build linux || darwin

package t3
import("os";"syscall")
func owned(fi os.FileInfo)bool{s,ok:=fi.Sys().(*syscall.Stat_t);return ok&&int(s.Uid)==os.Getuid()}
