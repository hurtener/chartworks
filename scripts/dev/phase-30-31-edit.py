"""Temporary branch-only formatting aid; removed before final review."""
from pathlib import Path
import subprocess

p = Path('test/acceptance/phase30_fixture_test.go')
s = p.read_text().replace('\n\t"context"\n', '\n').replace('cfg.JobAudience()', 'cfg.Audiences.Jobs').replace('\nvar _ = context.Background\n', '\n')
p.write_text(s)
paths = [str(p) for p in Path('test/acceptance').glob('phase30*.go')]
subprocess.run(['gofmt', '-w', *paths], check=True)
subprocess.run(['git', 'diff', '--check'], check=True)
