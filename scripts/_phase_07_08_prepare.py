from pathlib import Path

# Temporary development materialization; removed before final read-only CI and PR.
def edit(path, fn):
    p = Path(path)
    s = p.read_text()
    out = fn(s)
    if out != s:
        p.write_text(out)

# Extend the actual merged composition root; preserve its broker/gateway lifecycle APIs.
def work(s):
    if 'sourceService' in s:
        return s
    s = s.replace('"github.com/hurtener/chartworks/internal/config"', '"github.com/hurtener/chartworks/internal/config"\n readexec "github.com/hurtener/chartworks/internal/exec"\n "github.com/hurtener/chartworks/internal/sourceapi"\n "github.com/hurtener/chartworks/internal/sources"')
    s = s.replace('type work struct {', 'type work struct {\n sourceService *sources.Service')
    marker = '\tw.handler = workapi.Handler(verifier, w.engine, w.queue, next)'
    assert marker in s
    s = s.replace(marker, '''	w.sourceService, err = sources.New(db, v.Sources, lookup)
	if err != nil { w.close(); return nil, err }
	var validator *readexec.Validator
	if v.Sources.Enabled {
		validator, err = readexec.NewValidator(w.sourceService, v.Exec)
		if err != nil { w.close(); return nil, err }
	}
	w.handler = sourceapi.Handler(verifier, w.sourceService, validator, workapi.Handler(verifier, w.engine, w.queue, next))''', 1)
    s = s.replace('w.wait.Wait()', 'w.wait.Wait()\n if w.sourceService != nil { w.sourceService.Close() }', 1)
    return s
edit('internal/foundation/work.go', work)

def vector(s):
    if '"unicode/utf8"' not in s:
        s = s.replace('"time"', '"time"\n "unicode/utf8"')
    s = s.replace('if len(v) < 1 ||', 'if !utf8.ValidString(v) || len(v) < 1 ||', 1)
    s = s.replace('len(f.Text) < 1', '!utf8.ValidString(f.Text) || len(f.Text) < 1') if '!utf8.ValidString(f.Text)' not in s else s
    start = s.index('func VectorLiteral(')
    end = s.index('\n}\n', start) + 3
    s = s[:start] + '''func VectorLiteral(v []float32, dimensions int) (string, error) {
	if dimensions < 1 || dimensions > 16000 || len(v) != dimensions { return "", store.ErrInvalid }
	var b strings.Builder
	b.WriteByte('[')
	squaredNorm := float64(0)
	for i, n := range v {
		if math.IsNaN(float64(n)) || math.IsInf(float64(n), 0) { return "", store.ErrInvalid }
		squaredNorm += float64(n)*float64(n)
		if i != 0 { b.WriteByte(',') }
		b.WriteString(strconv.FormatFloat(float64(n), 'g', -1, 32))
	}
	b.WriteByte(']')
	// pgvector 0.8.2 accumulates cosine norms in float32. Finite components alone
	// do not prevent zero/overflowed accumulators and NaN distances. This declared
	// domain leaves ample margin for all supported dimensions without rescaling evidence.
	if squaredNorm < 1e-20 || squaredNorm > 1e20 { return "", store.ErrInvalid }
	return b.String(), nil
}
''' + s[end:]
    return s
edit('internal/vindex/vindex.go', vector)

def vector_store(s):
    if '"math"' not in s:
        s = s.replace('"errors"', '"errors"\n "math"')
    if 'math.IsNaN(h.Distance)' not in s:
        s = s.replace('bytes += len(h.Text)', 'if math.IsNaN(h.Distance) || math.IsInf(h.Distance, 0) || h.Distance < 0 || h.Distance > 2 { rows.Close(); return store.ErrInvalid }\n bytes += len(h.Text)', 1)
    return s
edit('internal/store/postgres/vindex.go', vector_store)

def vector_migration(s):
    s = s.replace('public.vector_norm(embedding)>0', 'public.vector_norm(embedding) BETWEEN 1e-10 AND 1e10')
    if "TG_OP IN ('UPDATE','DELETE')" not in s:
        start = s.index('CREATE FUNCTION chartworks.protect_vector_facet()')
        end_marker = 'CREATE TRIGGER immutable_vector_facet BEFORE INSERT OR UPDATE ON chartworks.vector_facets FOR EACH ROW EXECUTE FUNCTION chartworks.protect_vector_facet();'
        end = s.index(end_marker, start) + len(end_marker)
        s = s[:start] + '''CREATE FUNCTION chartworks.protect_vector_facet() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE current_state text;
BEGIN
 IF TG_OP IN ('UPDATE','DELETE') THEN
  SELECT state INTO current_state FROM chartworks.vector_generations
  WHERE tenant_id=OLD.tenant_id AND topic_id=OLD.topic_id AND context_id=OLD.context_id AND generation_id=OLD.generation_id FOR SHARE;
  IF current_state='ready' THEN RAISE EXCEPTION 'sealed vector generation' USING ERRCODE='23514'; END IF;
  -- A parent deletion has already removed the generation in this transaction;
  -- only that legitimate cascade may erase facets from a formerly sealed parent.
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 END IF;
 SELECT state INTO current_state FROM chartworks.vector_generations
 WHERE tenant_id=NEW.tenant_id AND topic_id=NEW.topic_id AND context_id=NEW.context_id AND generation_id=NEW.generation_id FOR SHARE;
 IF current_state IS DISTINCT FROM 'staging' THEN RAISE EXCEPTION 'sealed vector generation' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_vector_facet BEFORE INSERT OR UPDATE OR DELETE ON chartworks.vector_facets FOR EACH ROW EXECUTE FUNCTION chartworks.protect_vector_facet();''' + s[end:]
    return s
edit('internal/store/postgres/migrations/004_vector_generations.sql', vector_migration)

def source_types(s):
    # Do not invoke a custom type's typmod formatter while merely discovering it.
    s = s.replace("format_type(a.atttypid,a.atttypmod)", "CASE WHEN n.nspname='pg_catalog' THEN format_type(a.atttypid,a.atttypmod) ELSE n.nspname||'.'||t.typname END") if "ELSE n.nspname||'.'||t.typname END" not in s else s
    return s
edit('internal/sources/postgres.go', source_types)

# Closed JSON binding checks must also reject absent keys at the SQL constraint.
edit('internal/store/postgres/migrations/005_sources.sql', lambda s: s.replace("CHECK(binding->>'tenant'=tenant_id", "CHECK(binding ?& ARRAY['tenant','source','context','dialect','revision'] AND binding->>'tenant'=tenant_id"))
