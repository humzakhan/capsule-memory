package session

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"time"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)
type ProjectSuggestion struct{Path,Name,ProjectID string;Approved bool}
func(m *Module) MapProject(ctx context.Context,path,projectID,name string)(string,error){m.mu.Lock();defer m.mu.Unlock();if !filepath.IsAbs(path){return "",memory.ErrInvalid};path=filepath.Clean(path);if name==""{name=filepath.Base(path)};scope:="";if projectID!=""{e:=m.svc.Read(ctx,func(q memory.Queryer)error{return q.QueryRowContext(ctx,"SELECT scope FROM projects WHERE project_id=?",projectID).Scan(&scope)});if e!=nil{return "",memory.ErrInvalid}}else{var e error;projectID,e=memory.NewID();if e!=nil{return "",e};s,e:=m.svc.AddScope(ctx,name,"project");if e!=nil{return "",e};scope=s.ID;p,e:=m.svc.Prepare(ctx,memory.PutInput{Scope:scope,Kind:records.KindProject,Record:projectID,Body:records.Project{ProjectID:projectID,Name:name},Provenance:memory.Provenance{SourceKind:"manual",SourceRef:"project-map"}});if e!=nil{return "",e};if e=m.svc.Commit(ctx,[]memory.Prepared{p},nil);e!=nil{return "",e}}
	e:=m.svc.Update(ctx,func(tx *sql.Tx)error{_,e:=tx.ExecContext(ctx,"INSERT INTO project_paths(path,project_id,approved_at) VALUES(?,?,?) ON CONFLICT(path) DO UPDATE SET project_id=excluded.project_id,approved_at=excluded.approved_at",path,projectID,time.Now().UTC().Format(time.RFC3339Nano));return e});return projectID,e}
func(m *Module) UnmapProject(ctx context.Context,path string)error{m.mu.Lock();defer m.mu.Unlock();return m.svc.Update(ctx,func(tx *sql.Tx)error{_,e:=tx.ExecContext(ctx,"DELETE FROM project_paths WHERE path=?",filepath.Clean(path));return e})}
func(m *Module) ListProjects(ctx context.Context)([]ProjectSuggestion,error){var out []ProjectSuggestion;e:=m.svc.Read(ctx,func(q memory.Queryer)error{rows,e:=q.QueryContext(ctx,"SELECT path,project_id FROM project_paths ORDER BY path");if e!=nil{return e};defer rows.Close();for rows.Next(){var v ProjectSuggestion;if rows.Scan(&v.Path,&v.ProjectID)!=nil{return memory.ErrStorage};v.Approved=true;out=append(out,v)};return rows.Err()});return out,e}
func(m *Module) SuggestProjects(ctx context.Context,sourceID string)([]ProjectSuggestion,error){s,e:=m.Source(ctx,sourceID);if e!=nil{return nil,e};a:=m.adapters[s.Harness];if a==nil{return nil,memory.ErrInvalid};p,e:=a.Preview(ctx,s);if e!=nil{return nil,e};var out []ProjectSuggestion;for _,path:=range p.Locations{out=append(out,ProjectSuggestion{Path:path,Name:filepath.Base(path)})};return out,nil}
func projectFor(ctx context.Context,q memory.Queryer,cwd string)(project,scope string,err error){if cwd==""||!filepath.IsAbs(cwd){return "","",nil};rows,e:=q.QueryContext(ctx,"SELECT p.path,p.project_id,j.scope FROM project_paths p JOIN projects j ON j.project_id=p.project_id ORDER BY length(p.path) DESC");if e!=nil{return "","",e};defer rows.Close();cwd=filepath.Clean(cwd);for rows.Next(){var prefix,id,s string;if rows.Scan(&prefix,&id,&s)!=nil{return "","",memory.ErrStorage};if cwd==prefix||strings.HasPrefix(cwd,prefix+string(filepath.Separator)){var denied int;if e=q.QueryRowContext(ctx,"SELECT count(*) FROM project_exclusions WHERE (?=path OR substr(?,1,length(path)+1)=path||'/') AND length(path)>=?",cwd,cwd,len(prefix)).Scan(&denied);e!=nil{return "","",e};if denied>0{return "excluded","",nil};return id,s,nil}};if e=rows.Err();e!=nil{return "","",e};var denied int;if e=q.QueryRowContext(ctx,"SELECT count(*) FROM project_exclusions WHERE ?=path OR substr(?,1,length(path)+1)=path||'/'",cwd,cwd).Scan(&denied);e!=nil{return "","",e};if denied>0{return "excluded","",nil};return "","",nil}

func(m *Module) ExcludeProject(ctx context.Context,path string,exclude bool)error{if !filepath.IsAbs(path){return memory.ErrInvalid};m.mu.Lock();defer m.mu.Unlock();path=filepath.Clean(path);return m.svc.Update(ctx,func(tx *sql.Tx)error{if exclude{_,e:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO project_exclusions VALUES(?)",path);return e};_,e:=tx.ExecContext(ctx,"DELETE FROM project_exclusions WHERE path=?",path);return e})}
