package session

import (
 "context"
 "database/sql"
 "encoding/json"
 "github.com/humzakhan/capsule-memory/internal/memory"
 "github.com/humzakhan/capsule-memory/internal/records"
)

func(a *applier) indexManifest(ctx context.Context,tx *sql.Tx,r memory.Revision)error{
 if r.Operation=="delete"{return nil};var m records.SessionManifest;if json.Unmarshal(r.Body,&m)!=nil{return memory.ErrInvalid}
 if _,e:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO manifest_origins VALUES(?,?,?,?,?)",r.Record,r.Scope,m.SessionKey,m.Origin.Harness,m.Origin.NativeSessionID);e!=nil{return e}
 for _,b:=range m.Bindings{if b.Strength=="strong"{if _,e:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO manifest_bindings VALUES(?,?,?,?)",r.Record,r.Scope,b.Harness,b.NativeID);e!=nil{return e}}};return nil
}

func manifestAffected(ctx context.Context,tx *sql.Tx,seeds []string)error{
 if _,e:=tx.ExecContext(ctx,"CREATE TEMP TABLE IF NOT EXISTS manifest_seed(record TEXT PRIMARY KEY); DELETE FROM manifest_seed; CREATE TEMP TABLE IF NOT EXISTS manifest_affected(record TEXT PRIMARY KEY); DELETE FROM manifest_affected");e!=nil{return e}
 for _,id:=range seeds{if _,e:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO manifest_seed VALUES(?)",id);e!=nil{return e}}
 _,e:=tx.ExecContext(ctx,`WITH RECURSIVE related(record) AS (
 SELECT record FROM manifest_seed UNION
 SELECT b.record FROM related x JOIN manifest_origins a ON a.record=x.record JOIN manifest_origins b ON b.scope=a.scope AND b.session_key=a.session_key UNION
 SELECT b.record FROM related x JOIN manifest_origins a ON a.record=x.record JOIN manifest_bindings b ON b.scope=a.scope AND b.harness=a.harness AND b.native_id=a.native_id UNION
 SELECT b.record FROM related x JOIN manifest_bindings a ON a.record=x.record JOIN manifest_origins b ON b.scope=a.scope AND b.harness=a.harness AND b.native_id=a.native_id UNION
 SELECT b.record FROM related x JOIN manifest_bindings a ON a.record=x.record JOIN manifest_bindings b ON b.scope=a.scope AND b.harness=a.harness AND b.native_id=a.native_id
 ) INSERT INTO manifest_affected SELECT record FROM related LIMIT 100001`);if e!=nil{return e};var n int;if e=tx.QueryRowContext(ctx,"SELECT count(*) FROM manifest_affected").Scan(&n);e!=nil{return e};if n>100000{return memory.ErrStaleOverflow};return nil
}

func(a *applier) rebuildManifestGroups(ctx context.Context,tx *sql.Tx)error{
 if _,e:=tx.ExecContext(ctx,"CREATE TEMP TABLE IF NOT EXISTS manifest_rebuilt(record TEXT PRIMARY KEY); DELETE FROM manifest_rebuilt");e!=nil{return e}
 for{var id string;e:=tx.QueryRowContext(ctx,"SELECT record FROM manifest_origins WHERE record NOT IN (SELECT record FROM manifest_rebuilt) ORDER BY record LIMIT 1").Scan(&id);if e==sql.ErrNoRows{return nil};if e!=nil{return e}
  if e=a.reconcileManifests(ctx,tx,id);e!=nil{return e};if _,e=tx.ExecContext(ctx,"INSERT OR IGNORE INTO manifest_rebuilt SELECT record FROM manifest_affected");e!=nil{return e}
 }
}
