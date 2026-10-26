package claude

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/capture/filetail"
)
func previewVersions(ctx context.Context,c filetail.Config)[]string{ss,_,e:=filetail.Discover(ctx,c,"",64,accept);if e!=nil{return nil};versions:=map[string]bool{};for _,st:=range ss{b,e:=filetail.Header(c.Root,st.Key);if e!=nil{continue};var v struct{Version string `json:"version"`};if json.Unmarshal(b,&v)!=nil{continue};if v.Version==""{v.Version="unknown"};versions[v.Version]=true};out:=make([]string,0,len(versions));for v:=range versions{out=append(out,v)};sort.Strings(out);return out}
var _ capture.Adapter=Adapter{}
