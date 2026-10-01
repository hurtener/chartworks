package postgres

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
	"strings"
)

var _ nlqexec.GenerationExampleReader = (*DB)(nil)

func (d *DB) SelectGenerationExamples(ctx context.Context, scope store.Scope, q nlqexec.GenerationExampleQuery) (out []nlqexec.ExampleRecord, err error) {
	digest, e := hex.DecodeString(q.SourceBindingDigest)
	if checkScope(scope) != nil || !identity.Identifier(q.Topic) || !identity.Identifier(q.Context) || q.TopicVersion == "" || len(q.TopicVersion) > 256 || len(digest) != 32 || e != nil || strings.ToLower(q.SourceBindingDigest) != q.SourceBindingDigest || q.Limit < 1 || q.Limit > 64 || len(q.SearchText) > 20000 || len(q.RuleVersions) > 64 || len(q.Templates) > 64 || (q.Locale != "" && q.Locale != nlq.LanguageEnglish && q.Locale != nlq.LanguageSpanish) {
		return nil, store.ErrInvalid
	}
	rules := q.RuleVersions
	if rules == nil {
		rules = []string{}
	}
	templates := q.Templates
	if templates == nil {
		templates = []rulesets.TemplateSelection{}
	}
	rj, e := json.Marshal(rules)
	if e != nil {
		return nil, store.ErrInvalid
	}
	tj, e := json.Marshal(templates)
	if e != nil {
		return nil, store.ErrInvalid
	}
	out = []nlqexec.ExampleRecord{}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// FTS eligibility is exact provenance/evidence, independent of similarity.
		// Match phase uses the partial search index; fallback never admits stale or
		// unreviewed rows just to fill the optional example budget.
		const eligible = ` tenant_id=$1 AND topic_id=$2 AND state='active' AND positive_evidence>=1 AND weight>=0.60 AND positive_evidence>negative_evidence AND reviewed_by<>'' AND review_note<>'' AND reviewed_at IS NOT NULL AND sql_text<>'' AND question<>'' AND origin->>'schema_version'='1' AND origin->>'context'=$3 AND origin->>'topic_version'=$4 AND origin->>'source_binding_digest'=$5 AND ($6='' OR origin->>'locale'=$6) AND COALESCE(origin->'rule_versions','[]'::jsonb)=$7::jsonb AND COALESCE(origin->'templates','[]'::jsonb)=$8::jsonb AND (COALESCE(origin->>'binding_policy','')='' OR ($9 AND origin->>'binding_policy'=$10))`
		const columns = `example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,uncertainty,positive_evidence,negative_evidence,origin,version,reviewed_by,review_note,reviewed_at,provenance,created_at,updated_at,parameter_schema`
		for _, matched := range []bool{true, false} {
			if len(out) >= q.Limit {
				break
			}
			if matched && q.SearchText == "" {
				continue
			}
			where := ` AND (to_tsvector('simple',question) @@ websearch_to_tsquery('simple',$11))=$12`
			ordering := `weight DESC,evidence_count DESC,updated_at DESC,example_id`
			if matched {
				ordering = `ts_rank_cd(to_tsvector('simple',question),websearch_to_tsquery('simple',$11)) DESC,` + ordering
			}
			rows, e := tx.Query(ctx, `SELECT `+columns+` FROM chartworks.nlq_examples WHERE `+eligible+where+` ORDER BY `+ordering+` LIMIT $13`, scope.Tenant(), q.Topic, q.Context, q.TopicVersion, q.SourceBindingDigest, string(q.Locale), rj, tj, q.AllowOwned, nlqexec.OwnedExamplePolicy, q.SearchText, matched, q.Limit-len(out))
			if e != nil {
				return e
			}
			for rows.Next() {
				var x nlqexec.ExampleRecord
				if e = scanExample(rows, &x); e != nil {
					rows.Close()
					return e
				}
				out = append(out, x)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
		}
		return nil
	})
	return out, err
}
