from pathlib import Path
p=Path('internal/store/postgres/migrations/006_read_attempts.sql');s=p.read_text();assert 'DROP CONSTRAINT audit_events_action_check' not in s
s+='''
ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN (
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired',
 'facets.generation_staged','facets.generation_published','facets.archived','facets.erased','source.created','source.rotated',
 'read.accepted','read.cancel_requested','read.succeeded','read.empty','read.truncated','read.cancelled','read.timed_out','read.failed','read.uncertain','read.interrupted'
));
''';p.write_text(s)
p=Path('internal/exec/execution.go');s=p.read_text();needle='case errors.Is(runErr, ErrType):';assert needle in s;s=s.replace(needle,'case errors.Is(runErr, ErrUncertain):\n status,code="uncertain","remote_outcome_unknown"\n '+needle,1);p.write_text(s)
