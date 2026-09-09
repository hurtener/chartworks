# Temporary development-only edits. Removed before final exact-source CI.
from pathlib import Path
import re

def add_fields(path, name, fields):
    p = Path(path)
    text = p.read_text()
    pattern = r'(type ' + re.escape(name) + r' struct \{\n)(.*?)(\n\})'
    match = re.search(pattern, text, re.S)
    if match is None:
        raise SystemExit('missing type: ' + name)
    body = match.group(2)
    for field, declaration in fields:
        if re.search(r'^\s*' + re.escape(field) + r'\s+', body, re.M) is None:
            body += '\n\t' + declaration
    p.write_text(text[:match.start()] + match.group(1) + body + match.group(3) + text[match.end():])

model = 'internal/reporting/model.go'
add_fields(model, 'ValidationRecord', [('Binding','Binding exec.Binding `json:"binding"`'),('Definitions','Definitions []topics.Definition `json:"definitions"`')])
add_fields(model, 'Attestation', [('EvidenceExpiresAt','EvidenceExpiresAt time.Time `json:"evidence_expires_at"`'),('DependencyDigest','DependencyDigest string `json:"dependency_digest"`')])
add_fields(model, 'CaptureRequest', [('Parameters','Parameters []Parameter `json:"parameters"`'),('Resolution','Resolution Resolution `json:"resolution"`')])
add_fields(model, 'WithdrawRequest', [('Revision','Revision int64 `json:"revision,omitempty"`')])
add_fields(model, 'Snapshot', [('References','References []ResourceReference')])
add_fields(model, 'Evidence', [('ResolvedAt','ResolvedAt time.Time `json:"resolved_at"`'),('Timezone','Timezone string `json:"timezone"`'),('Parameters','Parameters []BoundValue `json:"parameters"`')])
access = Path('internal/reporting/access.go')
s = access.read_text().replace('access.ErrDenied','access.ErrNotFound')
needle = 'case "health":\n\t\treturn Read'
replacement = 'case "health":\n\t\treturn Read\n\tcase "preview":\n\t\treturn Preview'
if 'case "preview":' not in s:
    if s.count(needle) != 1:
        raise SystemExit('unexpected mutation action map')
    s = s.replace(needle, replacement)
access.write_text(s)

p = Path('internal/reporting/dependencies.go')
s = p.read_text()
old = 'func snapshotsReferences(snapshot Snapshot) []ResourceReference {\n'
if 'return clone(snapshot.References)' not in s:
    if old not in s: raise SystemExit('missing references helper')
    s = s.replace(old,old+'\tif len(snapshot.References) > 0 { return clone(snapshot.References) }\n')
old = 'add("execution_context", "use", d.Context)'
if 'for _, pin := range d.Topics { add("topic", "read", pin.Topic) }' not in s:
    s = s.replace(old,old+'\n\tfor _, pin := range d.Topics { add("topic", "read", pin.Topic) }')
p.write_text(s)

p = Path('internal/reporting/parameters.go')
s = p.read_text()
if 'func parameterDigest(' not in s:
    s += '\n// parameterDigest canonicalizes zero bind values independently of nil slices.\nfunc parameterDigest(values []exec.Parameter) string { return digest(append([]exec.Parameter{}, values...)) }\n'
p.write_text(s)
for name in ['internal/reporting/service.go','internal/reporting/validation.go']:
    p = Path(name)
    s = p.read_text()
    s = s.replace('digest(resolved.Parameters)', 'parameterDigest(resolved.Parameters)').replace('digest(captured.Parameters)','parameterDigest(captured.Parameters)').replace('digest(parameters)', 'parameterDigest(parameters)')
    if name.endswith('validation.go') and 'record.Evidence.ResolvedAt =' not in s:
        needle = '\tfor _, publication := range definitions {'
        if needle not in s: raise SystemExit('validation insertion point absent')
        s = s.replace(needle,'\trecord.Evidence.ResolvedAt = resolved.At\n\trecord.Evidence.Timezone = resolved.Timezone\n\trecord.Evidence.Parameters = clone(resolved.Values)\n'+needle)
    p.write_text(s)

p = Path('internal/store/postgres/blocks_read.go')
s = p.read_text()
old = 'const blockReferenceEligibility = `NOT EXISTS ('
new = 'const blockReferenceEligibility = `EXISTS(SELECT 1 FROM chartworks.block_revision_references present WHERE (present.tenant_id,present.block_id,present.revision)=(r.tenant_id,r.block_id,r.revision)) AND NOT EXISTS ('
s = s.replace(old, new)
p.write_text(s)
