-- Reporting extends the existing queue; no bearer, account, SQL, or model data
-- is stored in the dispatch ledger. Earlier applied migrations stay unchanged.
DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.operations'::regclass AND conname='operations_kind_check';
 ALTER TABLE chartworks.operations DROP CONSTRAINT operations_kind_check;
 EXECUTE format('ALTER TABLE chartworks.operations ADD CONSTRAINT operations_kind_check CHECK ((%s) OR kind=''reporting.scheduled'')',previous);
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.operations'::regclass AND conname='request_manifest_shape';
 ALTER TABLE chartworks.operations DROP CONSTRAINT request_manifest_shape;
 EXECUTE format($shape$ALTER TABLE chartworks.operations ADD CONSTRAINT request_manifest_shape CHECK ((%s) OR
 (dispatch_mode='queued' AND kind='reporting.scheduled' AND COALESCE(
  jsonb_typeof(dispatch_manifest)='object' AND octet_length(dispatch_manifest::text)<=65536 AND
  jsonb_typeof(request_manifest)='object' AND octet_length(request_manifest::text)<=4096 AND
  dispatch_manifest->>'id'=operation_id AND dispatch_manifest->>'tenant'=tenant_id AND
  dispatch_manifest->>'kind'=kind AND dispatch_manifest->>'binding_id'=binding_id AND
  dispatch_manifest->>'executor'=actor_id AND dispatch_manifest->>'manifest_hash'=manifest_hash AND
  jsonb_typeof(dispatch_manifest->'reporting')='object' AND
  dispatch_manifest->'pipeline' IS NULL AND
  request_manifest=dispatch_manifest->'reporting'->'input' AND
  request_manifest->>'kind' IN('reporting.run','report.run') AND
  request_manifest->>'target'=dispatch_manifest->'reporting'->'target'->>'id',false)))$shape$,previous);
END $$;

-- Counters are pre-call reservations, not retrospective provider invoices.
-- A failed/unknown external attempt stays charged across owners and restarts.
CREATE TABLE chartworks.reporting_occurrence_delivery (
 tenant_id text NOT NULL,
 operation_id text NOT NULL,
 artifact_kind text NOT NULL CHECK(artifact_kind IN('block','report')),
 query_state text NOT NULL DEFAULT 'pending' CHECK(query_state IN('pending','succeeded','partial','failed')),
 artifact_state text NOT NULL DEFAULT 'pending' CHECK(artifact_state IN('pending','retained')),
 catalog_state text NOT NULL DEFAULT 'pending' CHECK(catalog_state IN('pending','available','unavailable')),
 notification_state text NOT NULL DEFAULT 'not_requested' CHECK(notification_state='not_requested'),
 query_limit integer NOT NULL CHECK(query_limit BETWEEN 1 AND 8),
 model_call_limit integer NOT NULL CHECK(model_call_limit BETWEEN 0 AND 8),
 model_token_limit integer NOT NULL CHECK(model_token_limit BETWEEN 0 AND 131072),
 query_reservations integer NOT NULL DEFAULT 0 CHECK(query_reservations BETWEEN 0 AND 8),
 model_call_reservations integer NOT NULL DEFAULT 0 CHECK(model_call_reservations BETWEEN 0 AND 8),
 model_token_reservations integer NOT NULL DEFAULT 0 CHECK(model_token_reservations BETWEEN 0 AND 131072),
 published_at timestamptz,
 PRIMARY KEY(tenant_id,operation_id),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.operations(tenant_id,operation_id) ON DELETE CASCADE,
 CHECK(query_reservations<=query_limit AND model_call_reservations<=model_call_limit AND model_token_reservations<=model_token_limit),
 CHECK((model_call_limit=0)=(model_token_limit=0)),
 CHECK((catalog_state='available')=(published_at IS NOT NULL)),
 CHECK(catalog_state<>'available' OR artifact_state='retained')
);
CREATE FUNCTION chartworks.protect_reporting_reservations() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF ROW(NEW.tenant_id,NEW.operation_id,NEW.artifact_kind,NEW.query_limit,NEW.model_call_limit,NEW.model_token_limit,NEW.notification_state)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.operation_id,OLD.artifact_kind,OLD.query_limit,OLD.model_call_limit,OLD.model_token_limit,OLD.notification_state)
 OR NEW.query_reservations<OLD.query_reservations OR NEW.model_call_reservations<OLD.model_call_reservations OR NEW.model_token_reservations<OLD.model_token_reservations
 OR (OLD.published_at IS NOT NULL AND ROW(NEW.query_state,NEW.artifact_state,NEW.catalog_state,NEW.published_at)
 IS DISTINCT FROM ROW(OLD.query_state,OLD.artifact_state,OLD.catalog_state,OLD.published_at))
 THEN RAISE EXCEPTION 'immutable reporting receipt or refunded reservation' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER reporting_reservations_monotonic BEFORE UPDATE ON chartworks.reporting_occurrence_delivery
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_reporting_reservations();

ALTER TABLE chartworks.job_schedules ADD COLUMN retired boolean NOT NULL DEFAULT false,
 ADD CONSTRAINT retired_schedule_disabled CHECK(NOT retired OR NOT enabled);
CREATE TABLE chartworks.job_schedule_revisions (
 tenant_id text NOT NULL, schedule_id text NOT NULL, revision bigint NOT NULL CHECK(revision>0),
 enabled boolean NOT NULL, retired boolean NOT NULL,
 request jsonb NOT NULL CHECK(jsonb_typeof(request)='object' AND octet_length(request::text)<=8192),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 baseline boolean NOT NULL DEFAULT false,
 PRIMARY KEY(tenant_id,schedule_id,revision),
 FOREIGN KEY(tenant_id,schedule_id) REFERENCES chartworks.job_schedules(tenant_id,schedule_id),
 CHECK(NOT retired OR NOT enabled)
);
-- Existing schedules get one explicitly labeled baseline, not invented history.
INSERT INTO chartworks.job_schedule_revisions(tenant_id,schedule_id,revision,enabled,retired,request,baseline)
 SELECT tenant_id,schedule_id,revision,enabled,retired,request,true FROM chartworks.job_schedules;
CREATE FUNCTION chartworks.record_schedule_revision() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='UPDATE' THEN
  IF OLD.retired AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'retired schedule is immutable' USING ERRCODE='55000'; END IF;
  IF NEW.revision=OLD.revision THEN
   IF ROW(NEW.enabled,NEW.retired,NEW.request) IS DISTINCT FROM ROW(OLD.enabled,OLD.retired,OLD.request)
   THEN RAISE EXCEPTION 'schedule change requires revision CAS' USING ERRCODE='55000'; END IF;
   RETURN NEW;
  END IF;
  IF NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'schedule revision gap' USING ERRCODE='55000'; END IF;
 END IF;
 INSERT INTO chartworks.job_schedule_revisions(tenant_id,schedule_id,revision,enabled,retired,request)
 VALUES(NEW.tenant_id,NEW.schedule_id,NEW.revision,NEW.enabled,NEW.retired,NEW.request);
 RETURN NEW;
END $$;
CREATE TRIGGER schedule_revision_history AFTER INSERT OR UPDATE ON chartworks.job_schedules
 FOR EACH ROW EXECUTE FUNCTION chartworks.record_schedule_revision();
CREATE FUNCTION chartworks.protect_schedule_history() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 RAISE EXCEPTION 'immutable schedule revision history' USING ERRCODE='55000';
END $$;
CREATE TRIGGER schedule_history_immutable BEFORE UPDATE OR DELETE ON chartworks.job_schedule_revisions
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_schedule_history();
