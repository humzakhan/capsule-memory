package session

import (
 "context"
 "io"
 "github.com/humzakhan/capsule-memory/internal/memory"
)
func(m *Module) Export(ctx context.Context,session string,w io.Writer)error{from:=int64(0);snapshot:="";for{page,e:=m.ExportSnapshotPage(ctx,session,from,256<<10,snapshot);if e!=nil{return e};if _,e=io.WriteString(w,page.JSONL);e!=nil{return e};if page.Done{return nil};if page.Next<=from{return memory.ErrInvalid};from=page.Next;snapshot=page.Snapshot}}
