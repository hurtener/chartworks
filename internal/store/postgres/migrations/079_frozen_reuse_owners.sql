-- One durable source-start custodian for equivalent frozen runs. This is not a
-- queue or an authority grant. Private sessions never share cold ownership.
-- Deliberately independent of payload and read-journal retention: an absent
-- receipt cannot prove that reserved native work was never dispatched.
CREATE TABLE chartworks.frozen_reuse_owners (
 tenant_id text NOT NULL,
 reuse_key text NOT NULL CHECK(reuse_key ~ '^[a-f0-9]{64}$'),
 private_session text NOT NULL CHECK(private_session='' OR private_session ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 owner_operation text NOT NULL,
 owner_fence bigint NOT NULL CHECK(owner_fence>0),
 reservation_number integer NOT NULL DEFAULT 0 CHECK(reservation_number BETWEEN 0 AND 3),
 reservation_fence bigint NOT NULL DEFAULT 0 CHECK(reservation_fence>=0),
 settlement text NOT NULL DEFAULT 'none' CHECK(settlement IN('none','unresolved','retryable','successful','retained')),
 settled_attempt text CHECK(settled_attempt ~ '^[a-f0-9]{32}$'),
 completed boolean NOT NULL DEFAULT false,
 PRIMARY KEY(tenant_id,reuse_key,private_session),
 FOREIGN KEY(tenant_id,owner_operation) REFERENCES chartworks.frozen_runs(tenant_id,operation_id),
 CHECK((reservation_number=0 AND reservation_fence=0 AND settlement='none' AND settled_attempt IS NULL AND NOT completed)
    OR (reservation_number>0 AND reservation_fence>0 AND settlement<>'none')),
 CHECK((settlement IN('retryable','successful','retained'))=(settled_attempt IS NOT NULL)),
 CHECK(NOT completed OR settlement='retained')
);
CREATE INDEX frozen_reuse_owner_operation ON chartworks.frozen_reuse_owners(tenant_id,owner_operation);

CREATE FUNCTION chartworks.protect_frozen_reuse_owner() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN
  RAISE EXCEPTION 'frozen source custody cannot be erased' USING ERRCODE='23514';
 END IF;
 IF ROW(NEW.tenant_id,NEW.reuse_key,NEW.private_session) IS DISTINCT FROM ROW(OLD.tenant_id,OLD.reuse_key,OLD.private_session)
 OR (NEW.owner_operation<>OLD.owner_operation AND OLD.reservation_number>0 AND OLD.settlement<>'retryable' AND NOT OLD.completed) THEN
  RAISE EXCEPTION 'unresolved frozen source custody cannot be replaced' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER frozen_reuse_custody BEFORE UPDATE OR DELETE ON chartworks.frozen_reuse_owners FOR EACH ROW EXECUTE FUNCTION chartworks.protect_frozen_reuse_owner();
