from pathlib import Path

p = Path('internal/store/postgres/postgres.go')
s = p.read_text()
old = 'func (d *DB) transaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {\n'
if 'func (d *DB) transactionOptions(' not in s:
    assert old in s
    s = s.replace(old, old + '\treturn d.transactionOptions(ctx, pgx.TxOptions{}, fn)\n}\nfunc (d *DB) transactionOptions(ctx context.Context, options pgx.TxOptions, fn func(context.Context, pgx.Tx) error) error {\n', 1)
    s = s.replace('d.pool.BeginTx(ctx, pgx.TxOptions{})', 'd.pool.BeginTx(ctx, options)', 1)
    p.write_text(s)

p = Path('scripts/coverage-bands.conf')
s = p.read_text()
for package, minimum in [('internal/vindex',85)]:
    if not any(line.split('#')[0].split()[:1] == [package] for line in s.splitlines()):
        s += f'\n{package} {minimum}\n'
p.write_text(s)
