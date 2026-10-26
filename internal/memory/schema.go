package memory

const SchemaV2 = 2
const schemaV2 = `
CREATE TABLE IF NOT EXISTS scope_labels(scope TEXT PRIMARY KEY,label TEXT NOT NULL,role TEXT NOT NULL CHECK(role IN('default','project','personal','ledger')));
CREATE TABLE IF NOT EXISTS deps(child TEXT NOT NULL,child_revision TEXT NOT NULL,parent TEXT NOT NULL,parent_revision TEXT NOT NULL,event_from TEXT NOT NULL DEFAULT '',event_to TEXT NOT NULL DEFAULT '',evidence INTEGER NOT NULL DEFAULT 1,PRIMARY KEY(child_revision,parent,parent_revision,event_from,event_to,evidence));
CREATE INDEX IF NOT EXISTS deps_parent ON deps(parent);
CREATE TABLE IF NOT EXISTS retracted_events(scope TEXT NOT NULL,session TEXT NOT NULL,event_id TEXT NOT NULL,correction_revision TEXT NOT NULL,PRIMARY KEY(scope,session,event_id,correction_revision));
CREATE TABLE IF NOT EXISTS stale(record TEXT PRIMARY KEY,reason TEXT NOT NULL,since INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS jobs(job_key TEXT PRIMARY KEY,kind TEXT NOT NULL,scope TEXT NOT NULL,subject TEXT NOT NULL,inputs BLOB NOT NULL,destination TEXT NOT NULL,priority INTEGER NOT NULL,state TEXT NOT NULL CHECK(state IN('pending','leased','done','failed','cancelled','blocked')),not_before INTEGER NOT NULL,attempt_token TEXT,lease_until INTEGER,attempts INTEGER NOT NULL DEFAULT 0,error_code TEXT,result_record TEXT,assigned_device TEXT NOT NULL,seq INTEGER NOT NULL,versions TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS jobs_ready ON jobs(state,priority,not_before,seq);
CREATE TABLE IF NOT EXISTS local_seq(name TEXT PRIMARY KEY,value INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS policy(name TEXT PRIMARY KEY,version INTEGER NOT NULL,body BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS sources(source_id TEXT PRIMARY KEY,harness TEXT NOT NULL,adapter TEXT NOT NULL,store_identity TEXT NOT NULL,config BLOB NOT NULL,enabled INTEGER NOT NULL DEFAULT 0,consent_at TEXT,state TEXT NOT NULL DEFAULT 'disabled',reason TEXT);
CREATE TABLE IF NOT EXISTS checkpoints(source_id TEXT NOT NULL REFERENCES sources(source_id),stream TEXT NOT NULL,generation TEXT NOT NULL,position TEXT NOT NULL,prefix_digest TEXT NOT NULL,file_identity TEXT,adapter_state TEXT NOT NULL DEFAULT '',observed_modified TEXT NOT NULL DEFAULT '',observed_size INTEGER NOT NULL DEFAULT 0,reconciling INTEGER NOT NULL DEFAULT 0,discarding INTEGER NOT NULL DEFAULT 0,caught_up INTEGER NOT NULL DEFAULT 0,updated INTEGER NOT NULL,state TEXT NOT NULL DEFAULT 'ok',reason TEXT,PRIMARY KEY(source_id,stream));
CREATE TABLE IF NOT EXISTS source_gaps(source_id TEXT NOT NULL,stream TEXT NOT NULL,position TEXT NOT NULL,reason TEXT NOT NULL,count INTEGER NOT NULL,PRIMARY KEY(source_id,stream,position,reason));
CREATE TABLE IF NOT EXISTS backfills(source_id TEXT PRIMARY KEY,since TEXT NOT NULL,remaining INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS backfill_streams(source_id TEXT NOT NULL,stream TEXT NOT NULL,end_position INTEGER NOT NULL,active INTEGER NOT NULL,PRIMARY KEY(source_id,stream));
CREATE TABLE IF NOT EXISTS event_ids(source_id TEXT NOT NULL,stream TEXT NOT NULL,generation TEXT NOT NULL,position_key TEXT NOT NULL,event_id TEXT NOT NULL,content_digest TEXT NOT NULL,PRIMARY KEY(source_id,stream,generation,position_key));
CREATE TABLE IF NOT EXISTS seen_events(session TEXT NOT NULL,event_id TEXT NOT NULL,chunk TEXT NOT NULL,PRIMARY KEY(session,event_id));
CREATE TABLE IF NOT EXISTS origin_paths(session_key TEXT PRIMARY KEY,path TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS project_exclusions(path TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS project_paths(path TEXT PRIMARY KEY,project_id TEXT NOT NULL,approved_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS pauses(source_id TEXT NOT NULL,started TEXT NOT NULL,ended TEXT,resolution TEXT CHECK(resolution IN('skip','include')),PRIMARY KEY(source_id,started));
CREATE TABLE IF NOT EXISTS projects(project_id TEXT PRIMARY KEY,scope TEXT NOT NULL,record TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions(session TEXT PRIMARY KEY,session_key TEXT NOT NULL,scope TEXT NOT NULL,project_id TEXT NOT NULL,harness TEXT NOT NULL,state TEXT NOT NULL,last_event_time TEXT,view_of TEXT,deleted INTEGER NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS sessions_key ON sessions(session_key);
CREATE TABLE IF NOT EXISTS chunks(record TEXT NOT NULL,session TEXT NOT NULL,branch TEXT NOT NULL,seq INTEGER NOT NULL,revision TEXT NOT NULL,events INTEGER NOT NULL,bytes INTEGER NOT NULL,PRIMARY KEY(record,revision));
CREATE TABLE IF NOT EXISTS bindings(harness TEXT NOT NULL,native_id TEXT NOT NULL,session TEXT NOT NULL,PRIMARY KEY(harness,native_id,session));
CREATE TABLE IF NOT EXISTS suppressions(session_key TEXT PRIMARY KEY,record TEXT NOT NULL,revision TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS candidate_keys(candidate_key TEXT PRIMARY KEY,record TEXT NOT NULL,review TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS budget(day TEXT PRIMARY KEY,input_tokens INTEGER NOT NULL,output_tokens INTEGER NOT NULL,jobs INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS projections(link_id TEXT PRIMARY KEY,source_profile TEXT NOT NULL,source_record TEXT NOT NULL,source_revision TEXT NOT NULL,target_profile TEXT NOT NULL,target_scope TEXT NOT NULL,target_record TEXT,target_revision TEXT,state TEXT NOT NULL CHECK(state IN('pending','projected','invalidating','withdrawn','hidden')),verified_head TEXT,verified_at TEXT,error_code TEXT);
CREATE TABLE IF NOT EXISTS ledger(link_id TEXT PRIMARY KEY,target_record TEXT NOT NULL,state TEXT NOT NULL,bridge TEXT NOT NULL,grant_record TEXT NOT NULL,verified_head TEXT NOT NULL,verified_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS bridge_grants(record TEXT PRIMARY KEY,bridge TEXT NOT NULL,ledger_scope TEXT NOT NULL,target_scopes TEXT NOT NULL,revoked INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS knowledge(link_id TEXT PRIMARY KEY,scope TEXT NOT NULL,record TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS aggregation_grants(record TEXT PRIMARY KEY,destination TEXT NOT NULL,sources TEXT NOT NULL,revoked INTEGER NOT NULL);
CREATE VIRTUAL TABLE IF NOT EXISTS session_search USING fts5(scope UNINDEXED,record UNINDEXED,revision UNINDEXED,kind UNINDEXED,session UNINDEXED,text);
CREATE TABLE IF NOT EXISTS replay_block(scope TEXT PRIMARY KEY,reason TEXT NOT NULL,at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS outbox_source(revision TEXT PRIMARY KEY REFERENCES revisions(revision),source_id TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS outbox_hold(revision TEXT PRIMARY KEY REFERENCES revisions(revision),reason TEXT NOT NULL);
PRAGMA user_version=2;
`
