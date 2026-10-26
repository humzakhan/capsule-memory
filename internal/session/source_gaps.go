package session

import (
	"context"
	"database/sql"
 "time"
 "strings"
 "github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
)
func(m *Module) MarkMissingStreams(ctx context.Context,source string,seen map[string]bool)error{m.mu.Lock();defer m.mu.Unlock();src,e:=m.Source(ctx,source);if e!=nil{return e};var flushErr error;for key,pending:=range m.buffers{prefix:=source+":";if strings.HasPrefix(key,prefix){stream:=strings.TrimPrefix(key,prefix);if !seen[stream]{if _,e:=m.flushBuffer(ctx,src,capture.Stream{Key:stream},key,pending);e!=nil&&flushErr==nil{flushErr=e}}}};e=m.svc.Update(ctx,func(tx *sql.Tx)error{rows,e:=tx.QueryContext(ctx,"SELECT stream FROM checkpoints WHERE source_id=?",source);if e!=nil{return e};var missing []string;for rows.Next(){var stream string;if rows.Scan(&stream)!=nil{rows.Close();return memory.ErrStorage};if !seen[stream]{missing=append(missing,stream)}};rows.Close();for _,stream:=range missing{if e=auditGap(ctx,tx,source,stream,"","source_lost",1);e!=nil{return e};if _,e=tx.ExecContext(ctx,"UPDATE checkpoints SET state=CASE WHEN state='quarantined' THEN state ELSE 'gap' END,reason=CASE WHEN state='quarantined' THEN reason ELSE 'source_lost' END WHERE source_id=? AND stream=?",source,stream);e!=nil{return e}};return nil});if e!=nil{return e};return flushErr}

func auditGap(ctx context.Context,tx *sql.Tx,source,stream,position,reason string,count int)error{_,e:=tx.ExecContext(ctx,"INSERT INTO source_gaps VALUES(?,?,?,?,?) ON CONFLICT(source_id,stream,position,reason) DO UPDATE SET count=max(source_gaps.count,excluded.count)",source,stream,position,reason,count);return e}

func quarantineStream(ctx context.Context,tx *sql.Tx,source,stream,reason string)error{
 _,e:=tx.ExecContext(ctx,"INSERT INTO checkpoints(source_id,stream,generation,position,prefix_digest,updated,state,reason) VALUES(?,?, '', '', '',?,'quarantined',?) ON CONFLICT(source_id,stream) DO UPDATE SET state='quarantined',reason=excluded.reason",source,stream,time.Now().Unix(),reason);return e
}
