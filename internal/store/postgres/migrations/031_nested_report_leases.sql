-- Sequential frozen children share their owning report's counted root slot.
-- Parentage is immutable operational provenance, never an identity grant.
ALTER TABLE chartworks.operations ADD COLUMN nested_parent text;
ALTER TABLE chartworks.operations ADD COLUMN nested_fence bigint;
ALTER TABLE chartworks.operations ADD CONSTRAINT nested_parent_reference
 FOREIGN KEY(tenant_id,nested_parent) REFERENCES chartworks.operations(tenant_id,operation_id);
ALTER TABLE chartworks.operations ADD CONSTRAINT nested_child_shape CHECK (
 (nested_parent IS NULL AND nested_fence IS NULL) OR
 (nested_parent IS NOT NULL AND nested_fence IS NOT NULL AND nested_fence>0 AND nested_parent<>operation_id
  AND dispatch_mode='request' AND kind='reporting.run'));
CREATE UNIQUE INDEX one_unfinished_report_child ON chartworks.operations(tenant_id,nested_parent)
 WHERE nested_parent IS NOT NULL AND status IN('pending','retry','running');
CREATE INDEX report_child_parent ON chartworks.operations(tenant_id,nested_parent) WHERE nested_parent IS NOT NULL;
CREATE FUNCTION chartworks.protect_report_child() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE parent chartworks.operations%ROWTYPE;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.nested_parent IS DISTINCT FROM OLD.nested_parent OR
     (NEW.nested_fence IS NOT NULL AND NEW.nested_fence<OLD.nested_fence) THEN
   RAISE EXCEPTION 'immutable report child parent' USING ERRCODE='55000';
  END IF;
 END IF;
 IF NEW.nested_parent IS NOT NULL AND (TG_OP='INSERT' OR NEW.nested_fence IS DISTINCT FROM OLD.nested_fence) THEN
  SELECT * INTO STRICT parent FROM chartworks.operations
   WHERE tenant_id=NEW.tenant_id AND operation_id=NEW.nested_parent FOR KEY SHARE;
  IF parent.nested_parent IS NOT NULL OR parent.kind NOT IN('report.run','dashboard.run','reporting.scheduled') OR
     (parent.kind='reporting.scheduled' AND parent.request_manifest->>'kind'<>'report.run') OR
     parent.actor_id<>NEW.actor_id OR
     CASE WHEN parent.dispatch_mode='queued' THEN parent.operation_id ELSE parent.initiator_session END <> NEW.initiator_session OR
     parent.status<>'running' OR parent.fence<>NEW.nested_fence OR parent.lease_until<=clock_timestamp() OR parent.expires_at<=clock_timestamp() OR
     NEW.expires_at>parent.expires_at THEN
   RAISE EXCEPTION 'invalid report child ownership' USING ERRCODE='55000';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER report_child_immutable BEFORE INSERT OR UPDATE ON chartworks.operations
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_report_child();

-- An orphan/old-fence child is counted independently until its live lease ends.
-- It cannot continue publishing: the domain transaction also checks its parent.
CREATE VIEW chartworks.active_execution_roots AS
 SELECT o.tenant_id,o.operation_id FROM chartworks.operations o
 WHERE o.dispatch_mode IN('queued','request') AND o.status='running'
 AND o.lease_until>clock_timestamp() AND o.expires_at>clock_timestamp()
 AND NOT EXISTS(SELECT 1 FROM chartworks.operations p
  WHERE p.tenant_id=o.tenant_id AND p.operation_id=o.nested_parent
  AND p.nested_parent IS NULL AND p.status='running' AND p.fence=o.nested_fence
  AND p.lease_until>clock_timestamp() AND p.expires_at>clock_timestamp());
CREATE VIEW chartworks.pending_execution_roots AS
 SELECT o.tenant_id,o.operation_id FROM chartworks.operations o
 WHERE o.dispatch_mode IN('queued','request') AND o.status IN('pending','retry','running')
 AND NOT EXISTS(SELECT 1 FROM chartworks.operations p
  WHERE p.tenant_id=o.tenant_id AND p.operation_id=o.nested_parent
  AND p.nested_parent IS NULL AND p.status IN('pending','retry','running'));
