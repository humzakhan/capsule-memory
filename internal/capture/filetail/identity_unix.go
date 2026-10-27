//go:build linux || darwin

package filetail
import("fmt";"os";"syscall")
func Identity(fi os.FileInfo) string { if s,ok:=fi.Sys().(*syscall.Stat_t);ok{return fmt.Sprintf("%d:%d",s.Dev,s.Ino)};return "-" }
