//go:build linux || darwin

package daemon
import("os";"path/filepath";"syscall";"github.com/humzakhan/capsule-memory/internal/memory")
func singleton(home string)(func(),error){f,e:=os.OpenFile(filepath.Join(home,"daemon.lock"),os.O_CREATE|os.O_RDWR,0600);if e!=nil{return nil,e};if e=syscall.Flock(int(f.Fd()),syscall.LOCK_EX|syscall.LOCK_NB);e!=nil{f.Close();return nil,memory.ErrProfileBusy};return func(){_ = syscall.Flock(int(f.Fd()),syscall.LOCK_UN);f.Close()},nil}
