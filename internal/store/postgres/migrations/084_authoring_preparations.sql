-- Private metadata custody;
-- the existing read_attempts journal remains the sole physical execution owner.
CREATE TABLE chartworks.authoring_preparations (
 tenant_id text NOT NULL,
 preparation_id text NOT NULL CHECK(preparation_id ~ '^[a-f0-9]{32}$'),
 actor_id text NOT NULL, session_id text NOT NULL,
 target_id text NOT NULL CHECK(target_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 operation_id text NOT NULL CHECK(operation_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 input_digest text NOT NULL CHECK(input_digest ~ '^[a-f0-9]{64}$'),
 source_operation text NOT NULL CHECK(source_operation ~ '^chart-prepare:[a-f0-9]{64}$'),
 status text NOT NULL CHECK(status IN('accepted','prepared','failed','uncertain','consumed')),
 record jsonb NOT NULL CHECK(jsonb_typeof(record)='object' AND octet_length(record::text)<=2097152),
 created_at timestamptz NOT NULL,
 deadline timestamptz NOT NULL CHECK(deadline>created_at AND deadline<=created_at+interval '61 seconds'),
 expires_at timestamptz NOT NULL CHECK(expires_at>created_at AND expires_at<=created_at+interval '15 minutes'),
 source_id text GENERATED ALWAYS AS(record#>>'{binding,source}') STORED NOT NULL,
 source_revision bigint GENERATED ALWAYS AS((record#>>'{binding,revision}')::bigint) STORED NOT NULL,
 topic_id text GENERATED ALWAYS AS(record#>>'{topics,0,topic}') STORED NOT NULL,
 topic_version text GENERATED ALWAYS AS(record#>>'{topics,0,version}') STORED NOT NULL,
 created_block text GENERATED ALWAYS AS(CASE WHEN status='consumed' THEN target_id ELSE NULL END) STORED,
 PRIMARY KEY(tenant_id,preparation_id), UNIQUE(tenant_id,actor_id,session_id,operation_id),
 FOREIGN KEY(tenant_id,source_id,source_revision) REFERENCES chartworks.source_revisions(tenant_id,source_id,revision),
 FOREIGN KEY(tenant_id,topic_id,topic_version) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,created_block) REFERENCES chartworks.block_heads(tenant_id,block_id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
 CHECK(record->>'id'=preparation_id AND record->>'actor'=actor_id AND record->>'session'=session_id AND record->>'target'=target_id AND record->>'operation'=operation_id AND record->>'input_digest'=input_digest AND record->>'source_operation'=source_operation AND record->>'status'=status)
);
CREATE FUNCTION chartworks.protect_authoring_preparation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.tenant_id,NEW.preparation_id,NEW.actor_id,NEW.session_id,NEW.target_id,NEW.operation_id,NEW.input_digest,NEW.source_operation,NEW.created_at,NEW.deadline,NEW.expires_at)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.preparation_id,OLD.actor_id,OLD.session_id,OLD.target_id,OLD.operation_id,OLD.input_digest,OLD.source_operation,OLD.created_at,OLD.deadline,OLD.expires_at)
 OR (NEW.record-ARRAY['status','code','digest','revision','attempt']) IS DISTINCT FROM (OLD.record-ARRAY['status','code','digest','revision','attempt'])
 OR NOT (OLD.status='accepted' AND NEW.status IN('prepared','failed','uncertain') OR OLD.status='prepared' AND NEW.status='consumed')
 OR OLD.status='prepared' AND (NEW.record-'status') IS DISTINCT FROM (OLD.record-'status') THEN
  RAISE EXCEPTION 'immutable chart preparation' USING ERRCODE='23514';
 END IF; RETURN NEW;
END $$;
CREATE TRIGGER immutable_authoring_preparation BEFORE UPDATE ON chartworks.authoring_preparations FOR EACH ROW EXECUTE FUNCTION chartworks.protect_authoring_preparation();
