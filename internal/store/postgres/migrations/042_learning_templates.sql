-- CW-08 replaces fixed-increment examples with version-pinned, reviewed
-- learning evidence. Existing rows remain inspectable candidates but are not
-- applicable to generation until reviewed against a current origin.
ALTER TABLE chartworks.nlq_examples
 ADD COLUMN uncertainty double precision NOT NULL DEFAULT 1 CHECK(uncertainty>=0 AND uncertainty<=1),
 ADD COLUMN positive_evidence integer NOT NULL DEFAULT 0 CHECK(positive_evidence>=0),
 ADD COLUMN negative_evidence integer NOT NULL DEFAULT 0 CHECK(negative_evidence>=0),
 ADD COLUMN origin jsonb NOT NULL DEFAULT '{"schema_version":1,"locale":"en","topic_version":"legacy","context":"legacy","source_binding_digest":"","rule_versions":[],"templates":[]}'::jsonb,
 ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 ADD COLUMN reviewed_by text NOT NULL DEFAULT '' CHECK(octet_length(reviewed_by)<=256),
 ADD COLUMN review_note text NOT NULL DEFAULT '' CHECK(octet_length(review_note)<=4096),
 ADD COLUMN reviewed_at timestamptz;

-- The migration may encounter active rows protected by migration 017's
-- active-to-candidate guard. Replace that function while ALTER TABLE still
-- holds the table lock with a guard that permits only this exact legacy
-- normalization; the final lifecycle guard below replaces it before commit.
CREATE OR REPLACE FUNCTION chartworks.protect_nlq_example() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR NEW.tenant_id<>OLD.tenant_id OR NEW.example_id<>OLD.example_id OR NEW.topic_id<>OLD.topic_id OR
    NEW.question<>OLD.question OR NEW.sql_text<>OLD.sql_text OR NEW.digest<>OLD.digest OR NEW.created_at<>OLD.created_at OR
    NOT (NEW.state='candidate' AND NEW.evidence_count=OLD.evidence_count AND NEW.positive_evidence=OLD.evidence_count AND
      NEW.negative_evidence=0 AND NEW.weight=(OLD.evidence_count+1)::double precision/(OLD.evidence_count+2) AND
      NEW.uncertainty=1/sqrt((OLD.evidence_count+2)::double precision) AND NEW.origin=OLD.origin AND
      NEW.version=OLD.version AND NEW.reviewed_by=OLD.reviewed_by AND NEW.review_note=OLD.review_note AND
      NEW.reviewed_at IS NOT DISTINCT FROM OLD.reviewed_at AND NEW.provenance=OLD.provenance AND NEW.updated_at=OLD.updated_at) THEN
  RAISE EXCEPTION 'nlq example immutable fields changed' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;

UPDATE chartworks.nlq_examples
 SET positive_evidence=evidence_count,
     negative_evidence=0,
     weight=(evidence_count+1)::double precision/(evidence_count+2),
     uncertainty=1/sqrt((evidence_count+2)::double precision),
     state='candidate';

ALTER TABLE chartworks.nlq_examples
 ADD CONSTRAINT nlq_examples_origin_shape CHECK(
   jsonb_typeof(origin)='object' AND origin->>'schema_version'='1'
   AND octet_length(origin::text)<=32768
 ),
 ADD CONSTRAINT nlq_examples_evidence_total CHECK(evidence_count=positive_evidence+negative_evidence);

ALTER TABLE chartworks.nlq_examples
 ADD CONSTRAINT nlq_examples_active_review CHECK(
   state<>'active' OR (reviewed_by<>'' AND review_note<>'' AND reviewed_at IS NOT NULL)
 );

ALTER TABLE chartworks.nlq_examples
 ALTER COLUMN uncertainty DROP DEFAULT,
 ALTER COLUMN positive_evidence DROP DEFAULT,
 ALTER COLUMN negative_evidence DROP DEFAULT,
 ALTER COLUMN origin DROP DEFAULT,
 ALTER COLUMN version DROP DEFAULT;

DROP INDEX chartworks.nlq_feedback_once;
CREATE UNIQUE INDEX nlq_feedback_outcome_once ON chartworks.nlq_feedback(tenant_id,actor_id,session_id,query_id,verdict,COALESCE(correction_sql,''));

ALTER TABLE chartworks.nlq_queries
 ADD COLUMN example_selection jsonb NOT NULL DEFAULT '{}'::jsonb
 CHECK(jsonb_typeof(example_selection)='object' AND octet_length(example_selection::text)<=262144);
ALTER TABLE chartworks.nlq_queries ALTER COLUMN example_selection DROP DEFAULT;

CREATE OR REPLACE FUNCTION chartworks.protect_nlq_example() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
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

CREATE OR REPLACE FUNCTION chartworks.protect_nlq_query() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR NEW.tenant_id<>OLD.tenant_id OR NEW.actor_id<>OLD.actor_id OR NEW.session_id<>OLD.session_id OR NEW.query_id<>OLD.query_id OR
    NEW.parent_id IS DISTINCT FROM OLD.parent_id OR NEW.topic_id<>OLD.topic_id OR NEW.topics IS DISTINCT FROM OLD.topics OR
    NEW.topic_versions IS DISTINCT FROM OLD.topic_versions OR NEW.rule_versions IS DISTINCT FROM OLD.rule_versions OR
    NEW.template_selections IS DISTINCT FROM OLD.template_selections OR NEW.example_selection IS DISTINCT FROM OLD.example_selection OR
    NEW.context_id<>OLD.context_id OR NEW.locale<>OLD.locale OR NEW.question<>OLD.question OR NEW.route IS DISTINCT FROM OLD.route OR
    NEW.created_at<>OLD.created_at OR NEW.revision<>OLD.revision+1 THEN
  RAISE EXCEPTION 'nlq query immutable fields changed' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;

CREATE INDEX nlq_examples_applicable ON chartworks.nlq_examples
 (tenant_id,topic_id,state,weight DESC,updated_at DESC,example_id)
 WHERE state='active';
