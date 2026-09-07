package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// eraseUploadProfiles is called only by the authorized, fenced upload-erasure
// completion transaction, after the source is tombstoned and workspace removal
// has been observed. A source erasure covers all private derived profiles of that
// source, not just profiles created by the erasing actor. It cannot affect another
// tenant or source. The enclosing upload.erased audit covers this atomic effect.
func eraseUploadProfiles(ctx context.Context, tx pgx.Tx, tenant, source string) error {
	statements := []string{
		`UPDATE chartworks.operations SET status='cancelled',error_code='cancelled',finished_at=clock_timestamp(),lease_owner=NULL,lease_until=NULL WHERE tenant_id=$1 AND dispatch_mode='request' AND kind='profile.build' AND request_manifest->>'target'=$2 AND status IN('pending','retry','running')`,
		`UPDATE chartworks.operation_attempts a SET state='cancelled',error_code='cancelled',finished_at=clock_timestamp() FROM chartworks.operations o WHERE (a.tenant_id,a.operation_id)=(o.tenant_id,o.operation_id) AND o.tenant_id=$1 AND o.dispatch_mode='request' AND o.kind='profile.build' AND o.request_manifest->>'target'=$2 AND o.status='cancelled' AND a.state='acquiring'`,
		`DELETE FROM chartworks.profile_health_events h USING chartworks.profile_dependencies d WHERE (h.tenant_id,h.actor_id,h.session_id,h.kind,h.resource_id,h.definition_version)=(d.tenant_id,d.actor_id,d.session_id,d.kind,d.resource_id,d.definition_version) AND d.tenant_id=$1 AND d.source_id=$2`,
		`DELETE FROM chartworks.profile_health_events h USING chartworks.profile_versions p WHERE (h.tenant_id,h.profile_id)=(p.tenant_id,p.profile_id) AND p.tenant_id=$1 AND p.source_id=$2`,
		`DELETE FROM chartworks.profile_dependencies WHERE tenant_id=$1 AND source_id=$2`,
		`DELETE FROM chartworks.profile_heads WHERE tenant_id=$1 AND source_id=$2`,
		`UPDATE chartworks.profile_versions SET state='erased',result=NULL,deterministic_hash=NULL,changes='[]'::jsonb,last_read_operation=NULL,last_read_deadline=NULL WHERE tenant_id=$1 AND source_id=$2`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement, tenant, source); err != nil {
			return err
		}
	}
	return nil
}
