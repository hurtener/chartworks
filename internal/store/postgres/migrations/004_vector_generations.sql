-- pgvector must be available on the metadata server. Restricted production roles
-- require the operator to preinstall this extension before applying migrations.
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;

CREATE TABLE chartworks.vector_generations (
 tenant_id text NOT NULL CHECK (tenant_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 topic_id text NOT NULL CHECK (topic_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 context_id text NOT NULL CHECK (context_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 generation_id text NOT NULL CHECK (generation_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 version_id text NOT NULL CHECK (version_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 source_generation text NOT NULL CHECK (source_generation ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 descriptor jsonb NOT NULL,
 space_key text NOT NULL CHECK (space_key ~ '^[a-f0-9]{64}$'),
 dimensions integer NOT NULL CHECK (dimensions BETWEEN 1 AND 16000),
 manifest jsonb NOT NULL CHECK (jsonb_typeof(manifest) = 'array' AND jsonb_array_length(manifest) BETWEEN 1 AND 4096),
 manifest_hash text NOT NULL CHECK (manifest_hash ~ '^[a-f0-9]{64}$'),
 state text NOT NULL DEFAULT 'staging' CHECK (state IN ('staging','ready')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY (tenant_id,topic_id,context_id,generation_id),
 UNIQUE (tenant_id,topic_id,context_id,generation_id,space_key,dimensions)
);
CREATE TABLE chartworks.vector_heads (
 tenant_id text NOT NULL,
 topic_id text NOT NULL,
 context_id text NOT NULL,
 revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
 active_generation text,
 archived boolean NOT NULL DEFAULT false,
 PRIMARY KEY (tenant_id,topic_id,context_id),
 FOREIGN KEY (tenant_id,topic_id,context_id,active_generation)
 REFERENCES chartworks.vector_generations(tenant_id,topic_id,context_id,generation_id)
 DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE chartworks.vector_facets (
 tenant_id text NOT NULL,
 topic_id text NOT NULL,
 context_id text NOT NULL,
 generation_id text NOT NULL,
 facet_id text NOT NULL CHECK (facet_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 kind text NOT NULL CHECK (kind IN ('topic','entity','measure','dimension','kpi','relationship','rule','example')),
 source_id text NOT NULL CHECK (source_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 body text NOT NULL CHECK (octet_length(body) BETWEEN 1 AND 4096),
 space_key text NOT NULL,
 dimensions integer NOT NULL,
 embedding public.vector NOT NULL CHECK (public.vector_dims(embedding)=dimensions AND public.vector_norm(embedding) BETWEEN 1e-10 AND 1e10),
 PRIMARY KEY (tenant_id,topic_id,context_id,generation_id,facet_id),
 FOREIGN KEY (tenant_id,topic_id,context_id,generation_id,space_key,dimensions)
 REFERENCES chartworks.vector_generations(tenant_id,topic_id,context_id,generation_id,space_key,dimensions) ON DELETE CASCADE
);
CREATE INDEX vector_facets_partition ON chartworks.vector_facets(tenant_id,topic_id,context_id,generation_id,kind,facet_id);

CREATE FUNCTION chartworks.protect_vector_generation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.tenant_id,NEW.topic_id,NEW.context_id,NEW.generation_id,NEW.version_id,NEW.source_generation,NEW.descriptor,NEW.space_key,NEW.dimensions,NEW.manifest,NEW.manifest_hash,NEW.created_at)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.topic_id,OLD.context_id,OLD.generation_id,OLD.version_id,OLD.source_generation,OLD.descriptor,OLD.space_key,OLD.dimensions,OLD.manifest,OLD.manifest_hash,OLD.created_at)
 OR OLD.state='ready' AND NEW.state<>'ready' THEN RAISE EXCEPTION 'immutable vector generation' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_vector_generation BEFORE UPDATE ON chartworks.vector_generations FOR EACH ROW EXECUTE FUNCTION chartworks.protect_vector_generation();

CREATE FUNCTION chartworks.protect_vector_facet() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE current_state text;
BEGIN
 IF TG_OP IN ('UPDATE','DELETE') THEN
  SELECT state INTO current_state FROM chartworks.vector_generations
  WHERE tenant_id=OLD.tenant_id AND topic_id=OLD.topic_id AND context_id=OLD.context_id AND generation_id=OLD.generation_id FOR SHARE;
  IF current_state='ready' THEN RAISE EXCEPTION 'sealed vector generation' USING ERRCODE='23514'; END IF;
  -- A parent deletion has already removed the generation in this transaction;
  -- only that legitimate cascade may erase facets from a formerly sealed parent.
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 END IF;
 SELECT state INTO current_state FROM chartworks.vector_generations
 WHERE tenant_id=NEW.tenant_id AND topic_id=NEW.topic_id AND context_id=NEW.context_id AND generation_id=NEW.generation_id FOR SHARE;
 IF current_state IS DISTINCT FROM 'staging' THEN RAISE EXCEPTION 'sealed vector generation' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_vector_facet BEFORE INSERT OR UPDATE OR DELETE ON chartworks.vector_facets FOR EACH ROW EXECUTE FUNCTION chartworks.protect_vector_facet();

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN ('retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired','facets.generation_staged','facets.generation_published','facets.archived','facets.erased'));
