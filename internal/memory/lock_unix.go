//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package memory

import (
 "os"
 "path/filepath"
 "syscall"
)
func lockProfile(home string,mode LockMode)(*os.File,error){
 p:=filepath.Join(home,"profile.lock");f,e:=os.OpenFile(p,os.O_CREATE|os.O_RDWR,0600);if e!=nil{return nil,e};if e=safeFile(p);e!=nil{f.Close();return nil,e};op:=syscall.LOCK_SH;if mode==LockExclusive{op=syscall.LOCK_EX};if e=syscall.Flock(int(f.Fd()),op|syscall.LOCK_NB);e!=nil{f.Close();return nil,ErrProfileBusy};return f,nil
}
func unlockProfile(f *os.File)error{if f==nil{return nil};e:=syscall.Flock(int(f.Fd()),syscall.LOCK_UN);c:=f.Close();if e!=nil{return e};return c}
