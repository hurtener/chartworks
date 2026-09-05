from pathlib import Path
import re
p=Path('test/acceptance/phase04_test.go');s=p.read_text();s=s.replace('access.Resource{"tenant", "block", permission, "block1"}','access.Resource{Tenant:"tenant",Kind:"block",Permission:permission,ID:"block1"}');p.write_text(s)
p=Path('test/acceptance/phase03_test.go');s=p.read_text().replace('b, err := os.ReadFile(path)','// #nosec G304 -- repository source files discovered under fixed test-owned roots, never user paths.\n b, err := os.ReadFile(path)');p.write_text(s)
p=Path('test/acceptance/security_binary_test.go');s=p.read_text().replace('\n\t"fmt"','').replace('t.Log(fmt.Sprintf("compiled JWT -> scope -> PostgreSQL -> SDK and SIGTERM path passed; operations=%d", len(checks)))','t.Logf("compiled JWT -> scope -> PostgreSQL -> SDK and SIGTERM path passed; operations=%d", len(checks))');p.write_text(s)
# Separate standard-library and module imports; gofmt then supplies canonical ordering.
paths=list(Path('internal').rglob('*.go'))+list(Path('sdk').rglob('*.go'))+list(Path('test').rglob('*.go'))
for p in paths:
 s=p.read_text();m=re.search(r'import\s*\((.*?)\)',s,re.S)
 if not m:continue
 raw=[line.strip() for line in m[1].splitlines() if line.strip()]
 if any('"' not in line or line.startswith('//') for line in raw):continue
 std=[];external=[]
 for line in raw:
  package=re.search(r'"([^"]+)"',line)[1]
  (external if '.' in package.split('/')[0] else std).append(line)
 block='import (\n'+'\n'.join(std)+ ('\n\n' if std and external else '')+'\n'.join(external)+'\n)'
 s=s[:m.start()]+block+s[m.end():];p.write_text(s)
