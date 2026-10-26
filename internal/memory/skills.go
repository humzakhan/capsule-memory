package memory

import (
	"context"
	"encoding/json"
	"errors"
)

type Release struct {
	Version int `json:"version"`
	Vault string `json:"vault_id"`
	Scope string `json:"scope_id"`
	Record string `json:"record_id"`
	Revision string `json:"revision_id"`
	Digest string `json:"sha256"`
	Runtime string `json:"runtime"`
	Capabilities []string `json:"capabilities"`
}
func (s *Service) ApproveSkill(ctx context.Context,scope,record,revision string) (map[string]any,error) {
	s.mu.Lock();defer s.mu.Unlock();owner,err:=s.ownerKey();if err!=nil{return nil,err}
	v,err:=s.Get(ctx,scope,record,true);if err!=nil{return nil,err};if v.Deleted||v.Conflict||len(v.Heads)!=1||v.Heads[0].ID!=revision||v.Heads[0].Kind!="skill"{return nil,errors.New("approval requires the unique current visible skill revision")}
	r:=v.Heads[0];release:=Release{1,s.config.Vault,scope,record,revision,digest([]byte(r.Payload.Text)),"text/markdown",[]string{"read-as-text"}}
	signed,err:=sign("agent-memory/skill-approval/v1",release,owner);if err!=nil{return nil,err};b,err:=json.Marshal(signed);if err!=nil{return nil,err}
	if _,err=s.db.ExecContext(ctx,"INSERT INTO approvals(revision,release) VALUES(?,?) ON CONFLICT(revision) DO UPDATE SET release=excluded.release,active=0",revision,b);err!=nil{return nil,ErrStorage}
	return map[string]any{"record_id":record,"revision_id":revision,"sha256":release.Digest,"approved":true,"staged":true,"active":false,"runtime":"text/markdown","capabilities":release.Capabilities},nil
}
func (s *Service) ActivateSkill(context.Context,string,string,string) error {
	return errors.New("skill activation disabled in v1: approval is local text review only; no trusted harness installer is implemented")
}
func (s *Service) ListSkills(ctx context.Context,scope string) ([]RecordView,error) {
	if err:=s.scope(scope);err!=nil{return nil,err};rows,err:=s.db.QueryContext(ctx,"SELECT DISTINCT record FROM revisions WHERE scope=? AND kind='skill' ORDER BY record LIMIT 100",scope);if err!=nil{return nil,ErrStorage}
	ids:=[]string{};for rows.Next(){var id string;if rows.Scan(&id)!=nil{rows.Close();return nil,ErrStorage};ids=append(ids,id)};err=rows.Err();rows.Close();if err!=nil{return nil,ErrStorage}
	out:=[]RecordView{};for _,id:=range ids{v,err:=s.Get(ctx,scope,id,false);if err!=nil{return nil,err};if !v.Deleted{out=append(out,v)}};return out,nil
}
