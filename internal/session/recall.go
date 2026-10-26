package session

import (
	"context"
	"strings"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
)
func capsuleTool(name string)bool{return strings.HasPrefix(name,"mcp__capsule-memory__")||strings.HasPrefix(name,"mcp__capsule_memory__")}
func recallResult(ctx context.Context,q memory.Queryer,ce capture.Event,all []capture.Event)(bool,error){if ce.Event.Origin=="capsule_recall"||capsuleTool(ce.Event.ToolName){return true,nil};if ce.Event.Type!="tool_result"||ce.Event.ToolCallID==""{return false,nil};for _,e:=range all{if e.Event.ToolCallID==ce.Event.ToolCallID&&capsuleTool(e.Event.ToolName){return true,nil}};var count int;e:=q.QueryRowContext(ctx,"SELECT count(*) FROM revisions r,json_each(r.body,'$.body.events') e WHERE r.kind='session_chunk' AND json_extract(e.value,'$.tool_call_id')=? AND json_extract(e.value,'$.origin')='capsule_recall'",ce.Event.ToolCallID).Scan(&count);return count>0,e}
