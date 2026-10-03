CREATE FUNCTION chartworks.exact_deleted_composition_root(p_tenant text,p_root text) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM chartworks.composition_runs c WHERE c.tenant_id=p_tenant AND c.operation_id=p_root
 AND (EXISTS(SELECT 1 FROM chartworks.document_deletion_tombstones t WHERE (t.tenant_id,t.kind,t.document_id)=(c.tenant_id,c.kind,c.document_id))
 OR EXISTS(SELECT 1 FROM chartworks.composition_run_pages p JOIN chartworks.document_deletion_tombstones t ON(t.tenant_id,t.kind,t.document_id)=(p.tenant_id,'report',p.report_id) WHERE (p.tenant_id,p.operation_id)=(c.tenant_id,c.operation_id))))
$$;
-- Stable private-query custody, independent of mutable execution operations.
CREATE TABLE chartworks.nlq_query_origins (
 tenant_id text NOT NULL, actor_id text NOT NULL, session_id text NOT NULL, query_id text NOT NULL,
 saved_copy_parent text, erased_at timestamptz,
 PRIMARY KEY(tenant_id,actor_id,session_id,query_id)
);
CREATE TABLE chartworks.nlq_query_owners (
 tenant_id text NOT NULL, actor_id text NOT NULL, session_id text NOT NULL, query_id text NOT NULL, composition_root text NOT NULL,
 PRIMARY KEY(tenant_id,actor_id,session_id,query_id,composition_root),
 FOREIGN KEY(tenant_id,actor_id,session_id,query_id) REFERENCES chartworks.nlq_query_origins
);
CREATE TABLE chartworks.nlq_query_operations (
 tenant_id text NOT NULL, actor_id text NOT NULL, session_id text NOT NULL, query_id text NOT NULL, operation text NOT NULL,
 PRIMARY KEY(tenant_id,actor_id,session_id,query_id,operation),
 FOREIGN KEY(tenant_id,actor_id,session_id,query_id) REFERENCES chartworks.nlq_query_origins
);
CREATE TABLE chartworks.composition_origin_erasure (
 tenant_id text NOT NULL, operation_id text NOT NULL, erased_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,operation_id)
);
ALTER TABLE chartworks.nlq_queries ADD COLUMN saved_copy_parent text;
INSERT INTO chartworks.nlq_query_origins(tenant_id,actor_id,session_id,query_id)
 SELECT tenant_id,actor_id,session_id,query_id FROM chartworks.nlq_queries;
-- Persisted group coordinates are authoritative even after a legacy Run changed its key.
INSERT INTO chartworks.nlq_query_owners
 SELECT DISTINCT q.tenant_id,q.actor_id,q.session_id,q.query_id,c.operation_id
 FROM chartworks.nlq_queries q JOIN chartworks.composition_runs c
 ON (c.tenant_id,c.actor_id,c.session_id)=(q.tenant_id,q.actor_id,q.session_id)
 LEFT JOIN chartworks.composition_run_groups g ON (g.tenant_id,g.operation_id)=(c.tenant_id,c.operation_id)
 WHERE q.query_id IN(convert_from(g.plan,'UTF8')::jsonb->>'query',convert_from(g.result,'UTF8')::jsonb->'query'->>'query')
 OR (g.kind='query' AND ('composition:'||c.operation_id||':'||g.group_id) IN(q.plan_operation,q.operation)) ON CONFLICT DO NOTHING;
-- A legacy saved copy needs an exact recorded session-bound group, never just parent_id.
UPDATE chartworks.nlq_query_origins o SET saved_copy_parent=q.parent_id
 FROM chartworks.nlq_queries q, chartworks.composition_runs c, chartworks.composition_run_groups g, chartworks.composition_run_payloads p
 WHERE (o.tenant_id,o.actor_id,o.session_id,o.query_id)=(q.tenant_id,q.actor_id,q.session_id,q.query_id)
 AND (c.tenant_id,c.actor_id,c.session_id)=(q.tenant_id,q.actor_id,q.session_id)
 AND (g.tenant_id,g.operation_id)=(c.tenant_id,c.operation_id) AND (p.tenant_id,p.operation_id)=(c.tenant_id,c.operation_id)
 AND q.query_id IN(convert_from(g.plan,'UTF8')::jsonb->>'query',convert_from(g.result,'UTF8')::jsonb->'query'->>'query')
 AND EXISTS(SELECT 1 FROM jsonb_array_elements(convert_from(p.manifest,'UTF8')::jsonb->'groups') x WHERE x->>'id'=g.group_id AND x->'query'->>'durability'='session_bound' AND x->'origin'->>'query'=q.parent_id);
WITH RECURSIVE inherited AS (
 SELECT * FROM chartworks.nlq_query_owners
 UNION
 SELECT c.tenant_id,c.actor_id,c.session_id,c.query_id,p.composition_root FROM inherited p JOIN chartworks.nlq_query_origins c
 ON (c.tenant_id,c.actor_id,c.session_id,c.saved_copy_parent)=(p.tenant_id,p.actor_id,p.session_id,p.query_id)
) INSERT INTO chartworks.nlq_query_owners SELECT * FROM inherited ON CONFLICT DO NOTHING;
INSERT INTO chartworks.nlq_query_operations SELECT tenant_id,actor_id,session_id,query_id,operation FROM chartworks.nlq_queries WHERE operation IS NOT NULL;
INSERT INTO chartworks.nlq_query_operations SELECT tenant_id,actor_id,session_id,query_id,plan_operation FROM chartworks.nlq_queries WHERE plan_operation IS NOT NULL ON CONFLICT DO NOTHING;
-- Original checkpoint operations recover legacy journals even when current Run moved.
INSERT INTO chartworks.nlq_query_operations
 SELECT DISTINCT c.tenant_id,c.actor_id,c.session_id,gp->>'query',gp->>'operation'
 FROM chartworks.composition_runs c JOIN chartworks.composition_run_groups g USING(tenant_id,operation_id)
 CROSS JOIN LATERAL (SELECT convert_from(g.plan,'UTF8')::jsonb AS gp) v
 JOIN chartworks.nlq_query_origins o ON(o.tenant_id,o.actor_id,o.session_id,o.query_id)=(c.tenant_id,c.actor_id,c.session_id,gp->>'query')
 WHERE g.kind='query' AND gp->>'operation'='composition:'||c.operation_id||':'||g.group_id ON CONFLICT DO NOTHING;
-- Existing authored references keep content-free identity after erasure; new reuse requires a live query.
ALTER TABLE chartworks.document_query_refs DROP CONSTRAINT document_query_refs_tenant_id_actor_id_session_id_query_id_fkey;
ALTER TABLE chartworks.document_query_refs ADD FOREIGN KEY(tenant_id,actor_id,session_id,query_id) REFERENCES chartworks.nlq_query_origins;
ALTER TABLE chartworks.nlq_queries DROP CONSTRAINT nlq_intent_review_legacy_origin, DROP CONSTRAINT nlq_intent_review_preflight_origin;
ALTER TABLE chartworks.nlq_queries ADD FOREIGN KEY(tenant_id,actor_id,session_id,intent_review_legacy_id) REFERENCES chartworks.nlq_query_origins,
 ADD FOREIGN KEY(tenant_id,actor_id,session_id,intent_review_preflight_id) REFERENCES chartworks.nlq_query_origins;
CREATE FUNCTION chartworks.register_nlq_query_origin() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE root text; root_count integer;
BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended(NEW.tenant_id,721415));
 IF EXISTS(SELECT 1 FROM chartworks.nlq_query_origins o WHERE (o.tenant_id,o.actor_id,o.session_id,o.query_id)=(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.query_id) AND o.erased_at IS NOT NULL) THEN
 RAISE EXCEPTION 'query origin erased' USING ERRCODE='23503'; END IF;
 IF TG_OP='UPDATE' AND NEW.saved_copy_parent IS DISTINCT FROM OLD.saved_copy_parent THEN RAISE EXCEPTION 'saved copy origin immutable' USING ERRCODE='55000'; END IF;
 IF TG_OP='INSERT' AND NEW.saved_copy_parent IS NOT NULL THEN
  IF NEW.saved_copy_parent IS DISTINCT FROM NEW.parent_id OR NOT EXISTS(SELECT 1 FROM chartworks.nlq_queries p
   WHERE (p.tenant_id,p.actor_id,p.session_id,p.query_id)=(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.saved_copy_parent)
   AND ROW(p.sql_text,p.parameters,p.route,p.generation,p.topic_versions,p.rule_versions,p.context_id) IS NOT DISTINCT FROM ROW(NEW.sql_text,NEW.parameters,NEW.route,NEW.generation,NEW.topic_versions,NEW.rule_versions,NEW.context_id)
   AND p.revision=NEW.parent_revision AND NEW.status='planned' AND NEW.result IS NULL AND NEW.plan_operation IS NULL) THEN
   RAISE EXCEPTION 'invalid saved copy origin' USING ERRCODE='23503'; END IF;
 END IF;
 INSERT INTO chartworks.nlq_query_origins(tenant_id,actor_id,session_id,query_id,saved_copy_parent) VALUES(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.query_id,NEW.saved_copy_parent) ON CONFLICT DO NOTHING;
 IF TG_OP='INSERT' THEN
 SELECT count(DISTINCT c.operation_id) INTO root_count FROM chartworks.composition_runs c JOIN chartworks.composition_run_groups g USING(tenant_id,operation_id)
 WHERE (c.tenant_id,c.actor_id,c.session_id)=(NEW.tenant_id,NEW.actor_id,NEW.session_id) AND g.kind='query' AND ('composition:'||c.operation_id||':'||g.group_id) IN(NEW.plan_operation,NEW.operation);
 IF root_count>1 THEN RAISE EXCEPTION 'ambiguous composition origin' USING ERRCODE='23503'; END IF;
 FOR root IN SELECT DISTINCT c.operation_id FROM chartworks.composition_runs c JOIN chartworks.composition_run_groups g USING(tenant_id,operation_id)
 WHERE (c.tenant_id,c.actor_id,c.session_id)=(NEW.tenant_id,NEW.actor_id,NEW.session_id) AND g.kind='query' AND ('composition:'||c.operation_id||':'||g.group_id) IN(NEW.plan_operation,NEW.operation) LOOP
  IF chartworks.exact_deleted_composition_root(NEW.tenant_id,root) OR EXISTS(SELECT 1 FROM chartworks.composition_origin_erasure x WHERE x.tenant_id=NEW.tenant_id AND x.operation_id=root) THEN RAISE EXCEPTION 'composition origin erased' USING ERRCODE='23503'; END IF;
  INSERT INTO chartworks.nlq_query_owners VALUES(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.query_id,root) ON CONFLICT DO NOTHING;
 END LOOP;
 IF NEW.saved_copy_parent IS NOT NULL THEN
 INSERT INTO chartworks.nlq_query_owners SELECT NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.query_id,o.composition_root FROM chartworks.nlq_query_owners o WHERE (o.tenant_id,o.actor_id,o.session_id,o.query_id)=(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.saved_copy_parent) ON CONFLICT DO NOTHING;
 END IF;
 END IF; -- ownership is immutable; later Run keys add history only
 IF NEW.operation IS NOT NULL THEN INSERT INTO chartworks.nlq_query_operations VALUES(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.query_id,NEW.operation) ON CONFLICT DO NOTHING; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER a_nlq_query_origin BEFORE INSERT OR UPDATE ON chartworks.nlq_queries FOR EACH ROW EXECUTE FUNCTION chartworks.register_nlq_query_origin();
CREATE FUNCTION chartworks.guard_live_query_reference() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended(NEW.tenant_id,721415));
 IF NOT EXISTS(SELECT 1 FROM chartworks.nlq_queries q WHERE (q.tenant_id,q.actor_id,q.session_id,q.query_id)=(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.query_id)) THEN RAISE EXCEPTION 'borrowed query unavailable' USING ERRCODE='23503'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER document_query_live BEFORE INSERT ON chartworks.document_query_refs FOR EACH ROW EXECUTE FUNCTION chartworks.guard_live_query_reference();
CREATE FUNCTION chartworks.guard_owned_read_start() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended(NEW.tenant_id,721415));
 IF EXISTS(SELECT 1 FROM chartworks.nlq_query_operations k JOIN chartworks.nlq_query_origins o USING(tenant_id,actor_id,session_id,query_id)
 WHERE k.tenant_id=NEW.tenant_id AND k.actor_id=NEW.actor_id AND k.operation=NEW.operation_id AND o.erased_at IS NOT NULL) THEN RAISE EXCEPTION 'query read origin erased' USING ERRCODE='23503'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER owned_read_start BEFORE INSERT ON chartworks.read_attempts FOR EACH ROW EXECUTE FUNCTION chartworks.guard_owned_read_start();
CREATE OR REPLACE FUNCTION chartworks.protect_nlq_query() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' AND EXISTS(SELECT 1 FROM chartworks.nlq_query_origins o WHERE (o.tenant_id,o.actor_id,o.session_id,o.query_id)=(OLD.tenant_id,OLD.actor_id,OLD.session_id,OLD.query_id) AND o.erased_at IS NOT NULL) THEN RETURN OLD; END IF;
 IF TG_OP='DELETE' OR NEW.tenant_id<>OLD.tenant_id OR NEW.actor_id<>OLD.actor_id OR NEW.session_id<>OLD.session_id OR NEW.query_id<>OLD.query_id OR
    NEW.parent_id IS DISTINCT FROM OLD.parent_id OR NEW.parent_revision IS DISTINCT FROM OLD.parent_revision OR NEW.parent_digest IS DISTINCT FROM OLD.parent_digest OR
    NEW.topic_id<>OLD.topic_id OR NEW.topics IS DISTINCT FROM OLD.topics OR NEW.topic_versions IS DISTINCT FROM OLD.topic_versions OR NEW.rule_versions IS DISTINCT FROM OLD.rule_versions OR
    NEW.template_selections IS DISTINCT FROM OLD.template_selections OR NEW.example_selection IS DISTINCT FROM OLD.example_selection OR
    NEW.context_id<>OLD.context_id OR NEW.locale<>OLD.locale OR NEW.question<>OLD.question OR NEW.route IS DISTINCT FROM OLD.route OR
    NEW.created_at<>OLD.created_at OR NEW.revision<>OLD.revision+1 THEN
  RAISE EXCEPTION 'nlq query immutable fields changed' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION chartworks.protect_nlq_feedback() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' AND EXISTS(SELECT 1 FROM chartworks.nlq_queries q
  WHERE (q.tenant_id,q.actor_id,q.session_id,q.query_id)=(OLD.tenant_id,OLD.actor_id,OLD.session_id,OLD.query_id)
  AND EXISTS(SELECT 1 FROM chartworks.nlq_query_origins o WHERE (o.tenant_id,o.actor_id,o.session_id,o.query_id)=(q.tenant_id,q.actor_id,q.session_id,q.query_id) AND o.erased_at IS NOT NULL)) THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'nlq feedback is immutable' USING ERRCODE='55000';
END $$;


CREATE OR REPLACE FUNCTION chartworks.protect_composition_payload() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' AND (EXISTS(SELECT 1 FROM chartworks.composition_runs h WHERE h.tenant_id=OLD.tenant_id AND h.operation_id=OLD.operation_id AND (h.expires_at<=clock_timestamp() OR EXISTS(SELECT 1 FROM chartworks.composition_origin_erasure x WHERE (x.tenant_id,x.operation_id)=(h.tenant_id,h.operation_id))))
 OR EXISTS(SELECT 1 FROM chartworks.composition_runs h JOIN chartworks.document_deletion_tombstones t
  ON(t.tenant_id,t.kind,t.document_id)=(h.tenant_id,h.kind,h.document_id)
  WHERE h.tenant_id=OLD.tenant_id AND h.operation_id=OLD.operation_id)
 OR EXISTS(SELECT 1 FROM chartworks.composition_run_pages p JOIN chartworks.document_deletion_tombstones t
  ON(t.tenant_id,t.kind,t.document_id)=(p.tenant_id,'report',p.report_id)
  WHERE p.tenant_id=OLD.tenant_id AND p.operation_id=OLD.operation_id)) THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'immutable composition payload' USING ERRCODE='55000';
END $$;

CREATE OR REPLACE FUNCTION chartworks.protect_composition_group() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN
  IF EXISTS(SELECT 1 FROM chartworks.composition_runs h WHERE h.tenant_id=OLD.tenant_id AND h.operation_id=OLD.operation_id AND (h.expires_at<=clock_timestamp() OR EXISTS(SELECT 1 FROM chartworks.composition_origin_erasure x WHERE (x.tenant_id,x.operation_id)=(h.tenant_id,h.operation_id))))
  OR EXISTS(SELECT 1 FROM chartworks.composition_runs h JOIN chartworks.document_deletion_tombstones t
   ON(t.tenant_id,t.kind,t.document_id)=(h.tenant_id,h.kind,h.document_id)
   WHERE h.tenant_id=OLD.tenant_id AND h.operation_id=OLD.operation_id)
  OR EXISTS(SELECT 1 FROM chartworks.composition_run_pages p JOIN chartworks.document_deletion_tombstones t
   ON(t.tenant_id,t.kind,t.document_id)=(p.tenant_id,'report',p.report_id)
   WHERE p.tenant_id=OLD.tenant_id AND p.operation_id=OLD.operation_id) THEN RETURN OLD; END IF;
  RAISE EXCEPTION 'composition group requires retention expiry' USING ERRCODE='55000';
 END IF;
 IF ROW(NEW.tenant_id,NEW.operation_id,NEW.group_id,NEW.ordinal,NEW.kind) IS DISTINCT FROM ROW(OLD.tenant_id,OLD.operation_id,OLD.group_id,OLD.ordinal,OLD.kind)
 OR (OLD.started AND NOT NEW.started) OR (OLD.plan IS NOT NULL AND NEW.plan IS DISTINCT FROM OLD.plan)
 OR (OLD.result IS NOT NULL AND ROW(NEW.result,NEW.result_digest) IS DISTINCT FROM ROW(OLD.result,OLD.result_digest))
 THEN RAISE EXCEPTION 'immutable composition checkpoint' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION chartworks.protect_query_origin_identity() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='UPDATE' AND ROW(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.query_id,NEW.saved_copy_parent) IS NOT DISTINCT FROM ROW(OLD.tenant_id,OLD.actor_id,OLD.session_id,OLD.query_id,OLD.saved_copy_parent)
 AND OLD.erased_at IS NULL AND NEW.erased_at IS NOT NULL AND EXISTS(SELECT 1 FROM chartworks.nlq_query_owners k WHERE (k.tenant_id,k.actor_id,k.session_id,k.query_id)=(OLD.tenant_id,OLD.actor_id,OLD.session_id,OLD.query_id) AND chartworks.exact_deleted_composition_root(k.tenant_id,k.composition_root)) THEN RETURN NEW; END IF;
 -- Repeated deletion of another owner is idempotent.
 IF TG_OP='UPDATE' AND NEW IS NOT DISTINCT FROM OLD THEN RETURN NEW; END IF;
 RAISE EXCEPTION 'query origin identity immutable' USING ERRCODE='55000';
END $$;
CREATE TRIGGER query_origin_identity BEFORE UPDATE OR DELETE ON chartworks.nlq_query_origins FOR EACH ROW EXECUTE FUNCTION chartworks.protect_query_origin_identity();
CREATE TRIGGER query_owners_immutable BEFORE UPDATE OR DELETE ON chartworks.nlq_query_owners FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE TRIGGER query_operations_immutable BEFORE UPDATE OR DELETE ON chartworks.nlq_query_operations FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE TRIGGER composition_erasure_immutable BEFORE UPDATE OR DELETE ON chartworks.composition_origin_erasure FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE FUNCTION chartworks.guard_erased_query_key() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF EXISTS(SELECT 1 FROM chartworks.nlq_query_operations k JOIN chartworks.nlq_query_origins o USING(tenant_id,actor_id,session_id,query_id)
 WHERE k.tenant_id=NEW.tenant_id AND k.actor_id=NEW.actor_id AND k.operation IN(NEW.operation,NEW.plan_operation) AND o.erased_at IS NOT NULL) THEN RAISE EXCEPTION 'erased query operation unavailable' USING ERRCODE='23503'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER b_erased_query_key BEFORE INSERT OR UPDATE ON chartworks.nlq_queries FOR EACH ROW EXECUTE FUNCTION chartworks.guard_erased_query_key();
CREATE FUNCTION chartworks.guard_erased_rendition() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended(NEW.tenant_id,721415));
 IF convert_from(NEW.request,'UTF8')::jsonb->'view'->>'kind' IN('report','dashboard') AND EXISTS(SELECT 1 FROM chartworks.composition_origin_erasure x JOIN chartworks.composition_runs c USING(tenant_id,operation_id) WHERE (x.tenant_id,x.operation_id)=(NEW.tenant_id,NEW.run_id) AND c.kind=convert_from(NEW.request,'UTF8')::jsonb->'view'->>'kind') THEN RAISE EXCEPTION 'rendition origin erased' USING ERRCODE='23503'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER rendition_origin_live BEFORE INSERT ON chartworks.render_renditions FOR EACH ROW EXECUTE FUNCTION chartworks.guard_erased_rendition();

-- Learning contributions are immutable content-free origins, never inferred
-- from the mutable last-provenance label of an aggregated example.
CREATE TABLE chartworks.nlq_example_contributions (
 tenant_id text NOT NULL, example_id text NOT NULL, feedback_id text NOT NULL,
 actor_id text NOT NULL, session_id text NOT NULL, query_id text NOT NULL,
 PRIMARY KEY(tenant_id,example_id,feedback_id)
);
CREATE TABLE chartworks.nlq_example_quarantine (
 tenant_id text NOT NULL, example_id text NOT NULL,
 reason text NOT NULL CHECK(reason IN('legacy_lineage_unproved','erased_contributor')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(tenant_id,example_id)
);
CREATE TABLE chartworks.nlq_example_erasures (
 tenant_id text NOT NULL, example_id text NOT NULL,
 erased_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(tenant_id,example_id)
);
INSERT INTO chartworks.nlq_example_contributions
 SELECT x.tenant_id,x.example_id,f.feedback_id,f.actor_id,f.session_id,f.query_id
 FROM chartworks.nlq_examples x JOIN chartworks.nlq_feedback f ON f.tenant_id=x.tenant_id AND x.provenance='feedback:'||f.feedback_id
 WHERE x.evidence_count=1;
INSERT INTO chartworks.nlq_example_quarantine(tenant_id,example_id,reason)
 SELECT x.tenant_id,x.example_id,'legacy_lineage_unproved' FROM chartworks.nlq_examples x
 WHERE x.reviewed_at IS NULL AND x.evidence_count<>(SELECT count(*) FROM chartworks.nlq_example_contributions c WHERE(c.tenant_id,c.example_id)=(x.tenant_id,x.example_id));
CREATE TRIGGER example_contribution_immutable BEFORE UPDATE OR DELETE ON chartworks.nlq_example_contributions FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE TRIGGER example_quarantine_immutable BEFORE UPDATE OR DELETE ON chartworks.nlq_example_quarantine FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE TRIGGER example_erasure_immutable BEFORE UPDATE OR DELETE ON chartworks.nlq_example_erasures FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE FUNCTION chartworks.guard_example_contribution() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended(NEW.tenant_id,721415));
 IF NOT EXISTS(SELECT 1 FROM chartworks.nlq_feedback f JOIN chartworks.nlq_queries q USING(tenant_id,actor_id,session_id,query_id)
 WHERE (f.tenant_id,f.feedback_id,f.actor_id,f.session_id,f.query_id)=(NEW.tenant_id,NEW.feedback_id,NEW.actor_id,NEW.session_id,NEW.query_id))
 OR NOT EXISTS(SELECT 1 FROM chartworks.nlq_examples x WHERE (x.tenant_id,x.example_id)=(NEW.tenant_id,NEW.example_id)) THEN RAISE EXCEPTION 'invalid example contribution' USING ERRCODE='23503'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER example_contribution_live BEFORE INSERT ON chartworks.nlq_example_contributions FOR EACH ROW EXECUTE FUNCTION chartworks.guard_example_contribution();
CREATE FUNCTION chartworks.guard_erased_example() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended(NEW.tenant_id,721415));
 IF EXISTS(SELECT 1 FROM chartworks.nlq_example_erasures x WHERE (x.tenant_id,x.example_id)=(NEW.tenant_id,NEW.example_id)) THEN RAISE EXCEPTION 'example erased' USING ERRCODE='23503'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER example_live BEFORE INSERT ON chartworks.nlq_examples FOR EACH ROW EXECUTE FUNCTION chartworks.guard_erased_example();
ALTER TABLE chartworks.document_deletion_tombstones ADD COLUMN erased_examples integer NOT NULL DEFAULT 0 CHECK(erased_examples>=0), ADD COLUMN quarantined_examples integer NOT NULL DEFAULT 0 CHECK(quarantined_examples>=0);

CREATE OR REPLACE FUNCTION chartworks.protect_nlq_example() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' AND OLD.reviewed_at IS NULL AND EXISTS(SELECT 1 FROM chartworks.nlq_example_erasures e WHERE (e.tenant_id,e.example_id)=(OLD.tenant_id,OLD.example_id)) THEN RETURN OLD; END IF;
 IF TG_OP='DELETE' OR NEW.tenant_id<>OLD.tenant_id OR NEW.example_id<>OLD.example_id OR NEW.topic_id<>OLD.topic_id OR
    NEW.question<>OLD.question OR NEW.sql_text<>OLD.sql_text OR NEW.digest<>OLD.digest OR NEW.origin IS DISTINCT FROM OLD.origin OR
    NEW.created_at<>OLD.created_at OR NEW.version<>OLD.version+1 OR NEW.positive_evidence<OLD.positive_evidence OR
    NEW.negative_evidence<OLD.negative_evidence OR OLD.state='retired' OR (OLD.state='active' AND NEW.state='candidate') OR
    ((NEW.reviewed_by,NEW.review_note,NEW.reviewed_at) IS DISTINCT FROM (OLD.reviewed_by,OLD.review_note,OLD.reviewed_at)
      AND NOT (OLD.state='candidate' AND NEW.state='active')) THEN
  RAISE EXCEPTION 'nlq example immutable fields changed' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;

CREATE INDEX nlq_query_operation_lookup ON chartworks.nlq_query_operations(tenant_id,actor_id,operation);
CREATE INDEX nlq_query_owner_root ON chartworks.nlq_query_owners(tenant_id,composition_root);
CREATE INDEX nlq_query_saved_parent ON chartworks.nlq_query_origins(tenant_id,actor_id,session_id,saved_copy_parent) WHERE saved_copy_parent IS NOT NULL;
CREATE INDEX nlq_example_contribution_origin ON chartworks.nlq_example_contributions(tenant_id,actor_id,session_id,query_id);
