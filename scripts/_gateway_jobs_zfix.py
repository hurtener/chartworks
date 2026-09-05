from pathlib import Path
p=Path('internal/identity/identity.go');s=p.read_text();s=s.replace('member(p[1], "source",','member(p[1], "schedule", "source",');assert 'member(p[1], "schedule"' in s;p.write_text(s)
p=Path('internal/store/postgres/migrations/003_durable_dispatch.sql');s=p.read_text().replace('window_start IS NOT NULL AND window_end=due_at','window_start IS NOT NULL AND window_end IS NOT NULL AND window_end=due_at').replace("(schedule_id ~ '^[a-f0-9]{32}$' AND schedule_revision>0)","(schedule_id IS NOT NULL AND schedule_revision IS NOT NULL AND schedule_id ~ '^[a-f0-9]{32}$' AND schedule_revision>0)")
s=s.replace("error_code text NOT NULL DEFAULT '', started_at", "error_code text NOT NULL DEFAULT '' CHECK(error_code IN ('','authority_blocked','attempt_failed','attempt_timeout','definition_changed','cancelled','lease_lost')), started_at")
p.write_text(s)
p=Path('internal/store/postgres/jobs.go');s=p.read_text().replace('if jobs.AssertExecution(e, lease.Job) != nil {','ctx, cancel := context.WithDeadline(ctx, e.Deadline())\n defer cancel()\n if jobs.AssertExecution(e, lease.Job) != nil {',1);p.write_text(s)
p=Path('.github/workflows/gateway-jobs-work.yml');s=p.read_text().replace("-run 'TestPhase0[56]|TestGateway|TestQueue|TestJobs|TestDispatch' -count=1","-run 'TestPhase0[56]|TestGateway|TestQueue|TestJobs|TestDispatch' -count=1 -timeout=3m -v");p.write_text(s)
