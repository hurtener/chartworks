from pathlib import Path

# Temporary development materialization; removed before the final read-only CI/PR.
def edit(path, fn):
    p = Path(path)
    s = p.read_text()
    out = fn(s)
    if out != s:
        p.write_text(out)

edit('internal/config/sources.go', lambda s: s.replace('return Sources{MaxConns:', 'return Sources{Connections: []SourceConnection{}, MaxConns:').replace('append([]SourceConnection(nil), s.Connections...)', 'append([]SourceConnection{}, s.Connections...)'))
edit('internal/store/postgres/errors_test.go', lambda s: s.replace('SchemaVersion() != "3"', 'SchemaVersion() != "5"'))
edit('test/acceptance/phase02_test.go', lambda s: s.replace('`SELECT count(*) FROM chartworks.schema_migrations`) != 3', '`SELECT count(*) FROM chartworks.schema_migrations`) != 5').replace('"queue_limits", "schema_migrations"}', '"queue_limits", "schema_migrations", "source_revisions", "sources", "vector_facets", "vector_generations", "vector_heads"}'))

def validator(s):
    if '"reflect"' not in s:
        s = s.replace('"encoding/json"', '"encoding/json"\n "reflect"')
    s = s.replace('if adapter == nil || config.ValidateReadValidation(limits) != nil {', 'if adapter == nil || reflect.ValueOf(adapter).Kind() == reflect.Ptr && reflect.ValueOf(adapter).IsNil() || config.ValidateReadValidation(limits) != nil {')
    if '// Initialize the pinned WASM parser' not in s:
        s = s.replace('return &Validator{', '// Initialize the pinned WASM parser at explicit capability construction, not\n // during the first user request. Disabled sources never initialize it.\n if _, err := pgquery.ParseToJSON("SELECT 1"); err != nil { return nil, ErrUnsupported }\n return &Validator{', 1)
    if 'if ctx == nil' not in s:
        s = s.replace('if !e.Valid() {', 'if ctx == nil { return Plan{}, ErrBinding }; if err := ctx.Err(); err != nil { return Plan{}, err }; if !e.Valid() {', 1)
    return s
edit('internal/exec/validator.go', validator)

def vindex(s):
    if '"reflect"' not in s:
        s = s.replace('"net/url"', '"net/url"\n "reflect"')
    return s.replace('if repo == nil {', 'if repo == nil || reflect.ValueOf(repo).Kind() == reflect.Ptr && reflect.ValueOf(repo).IsNil() {')
edit('internal/vindex/vindex.go', vindex)
edit('internal/sourceapi/http.go', lambda s: s.replace('string(data)=="null"', 'strings.TrimSpace(string(data))=="null"').replace('string(data) == "null"', 'strings.TrimSpace(string(data)) == "null"'))

p = Path('scripts/coverage-bands.conf')
s = p.read_text()
for package, minimum in [('internal/vindex',85),('internal/exec',85),('internal/sources',80),('internal/sourceapi',80)]:
    if not any(line.split('#')[0].split()[:1] == [package] for line in s.splitlines()):
        s += f'\n{package} {minimum}\n'
p.write_text(s)
