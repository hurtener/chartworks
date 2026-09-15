-- Reconcile a reviewed replacement without altering accepted occurrence rows.
ALTER TABLE chartworks.job_schedules
 ADD COLUMN change_key text NOT NULL DEFAULT '',
 ADD COLUMN change_actor text NOT NULL DEFAULT '',
 ADD COLUMN change_session text NOT NULL DEFAULT '',
 ADD COLUMN change_revision bigint NOT NULL DEFAULT 0,
 ADD CONSTRAINT schedule_change_receipt_shape CHECK(
  (change_key='' AND change_actor='' AND change_session='' AND change_revision=0) OR
  (change_key ~ '^[A-Za-z0-9_.:-]{1,128}$' AND change_actor ~ '^[A-Za-z0-9_.:-]{1,128}$' AND change_session ~ '^[A-Za-z0-9_.:-]{1,128}$' AND change_revision>0 AND change_revision<=revision));

CREATE OR REPLACE FUNCTION chartworks.protect_schedule_definition() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF ROW(NEW.tenant_id,NEW.schedule_id,NEW.creator_id,NEW.creator_session,NEW.client_key,NEW.created_at)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.schedule_id,OLD.creator_id,OLD.creator_session,OLD.client_key,OLD.created_at)
 THEN RAISE EXCEPTION 'immutable schedule identity' USING ERRCODE='55000'; END IF;
 IF ROW(NEW.request_hash,NEW.request,NEW.change_key,NEW.change_actor,NEW.change_session,NEW.change_revision)
 IS DISTINCT FROM ROW(OLD.request_hash,OLD.request,OLD.change_key,OLD.change_actor,OLD.change_session,OLD.change_revision)
 AND NOT (NEW.revision=OLD.revision+1 AND NEW.change_revision=NEW.revision AND NEW.change_key<>'' AND NEW.change_actor<>'' AND NEW.change_session<>'')
 THEN RAISE EXCEPTION 'schedule replacement requires revision receipt' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;

ALTER TABLE chartworks.engineering_proposal_effects DROP CONSTRAINT engineering_proposal_effects_kind_check;
ALTER TABLE chartworks.engineering_proposal_effects ADD CONSTRAINT engineering_proposal_effects_kind_check
 CHECK(kind IN('pipeline_draft','pipeline_publication','pipeline_run','managed_step','compensation','topic_draft','schedule'));
ALTER TABLE chartworks.engineering_proposal_effects DROP CONSTRAINT engineering_proposal_effects_observed_order_check;
ALTER TABLE chartworks.engineering_proposal_effects ADD CONSTRAINT engineering_proposal_effects_observed_order_check CHECK(observed_order BETWEEN 0 AND 6);
ALTER TABLE chartworks.engineering_proposal_references DROP CONSTRAINT engineering_proposal_references_check;
ALTER TABLE chartworks.engineering_proposal_references ADD CONSTRAINT engineering_proposal_references_check
 CHECK((kind,permission) IN(('source','query'),('dataset','query'),('execution_context','use'),('topic','read'),('execution_binding','use'),('schedule','write')));
