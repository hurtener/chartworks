from pathlib import Path

p = Path('internal/store/postgres/postgres.go')
s = p.read_text()
old = 'func (d *DB) transaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {\n'
if 'func (d *DB) transactionOptions(' not in s:
    assert old in s
    s = s.replace(old, old + '\treturn d.transactionOptions(ctx, pgx.TxOptions{}, fn)\n}\nfunc (d *DB) transactionOptions(ctx context.Context, options pgx.TxOptions, fn func(context.Context, pgx.Tx) error) error {\n', 1)
    s = s.replace('d.pool.BeginTx(ctx, pgx.TxOptions{})', 'd.pool.BeginTx(ctx, options)', 1)
if 'readexec.ErrUnsafe' not in s:
    s = s.replace('"github.com/hurtener/chartworks/internal/jobs"', '"github.com/hurtener/chartworks/internal/access"\n readexec "github.com/hurtener/chartworks/internal/exec"\n "github.com/hurtener/chartworks/internal/jobs"')
    s = s.replace('[]error{jobs.ErrInvalid', '[]error{readexec.ErrUnsafe, readexec.ErrUnsupported, readexec.ErrBinding, readexec.ErrLimit, access.ErrUnauthenticated, access.ErrForbidden, access.ErrNotFound, jobs.ErrInvalid')
p.write_text(s)

p = Path('internal/config/config.go')
s = p.read_text()
if 'Sources   Sources' not in s and 'Sources Sources' not in s:
    s = s.replace('type Values struct {', 'type Values struct {\n Sources Sources `json:"sources"`\n Exec ReadValidation `json:"exec"`', 1)
    s = s.replace('v := c.values', 'v := c.values\n v.Sources = c.values.Sources.Clone()', 1)
    s = s.replace('return Values{', 'return Values{\n Sources: DefaultSources(),\n Exec: DefaultReadValidation(),', 1)
    s = s.replace('return ValidateGateway(v.Gateway, v.Features.Gateway)', 'if err := ValidateSources(v.Sources); err != nil { return err }; if err := ValidateReadValidation(v.Exec); err != nil { return err }; return ValidateGateway(v.Gateway, v.Features.Gateway)', 1)
p.write_text(s)

p = Path('internal/store/postgres/migrations/004_vector_generations.sql')
s = p.read_text()
if 'audit_events_action_check' not in s:
    s += "\nALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;\nALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN ('retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired','facets.generation_staged','facets.generation_published','facets.archived','facets.erased'));\n"
p.write_text(s)

p = Path('scripts/coverage-bands.conf')
s = p.read_text()
for package, minimum in [('internal/vindex',85),('internal/exec',85),('internal/sources',80)]:
    if not any(line.split('#')[0].split()[:1] == [package] for line in s.splitlines()):
        s += f'\n{package} {minimum}\n'
p.write_text(s)
