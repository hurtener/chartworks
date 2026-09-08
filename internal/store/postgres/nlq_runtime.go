package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ nlqexec.Repository = (*DB)(nil)

func marshalNLQ(value any) ([]byte, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, store.ErrInvalid
	}
	if len(b) == 0 {
		return nil, store.ErrInvalid
	}
	return b, nil
}

func (d *DB) CreateSession(ctx context.Context, scope store.Scope, s nlqexec.SessionRecord) error {
	if err := checkScope(scope); err != nil || !identity.Identifier(s.ID) || s.Tenant != scope.Tenant() || s.Actor != scope.Actor() || !identity.Identifier(s.Context) || len(s.Topics) < 1 || len(s.Topics) > 4 || (s.Locale != "en" && s.Locale != "es") {
		return store.ErrInvalid
	}
	topics, err := marshalNLQ(s.Topics)
	if err != nil {
		return err
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.nlq_sessions(tenant_id,actor_id,session_id,context_id,topics,locale,created_at,updated_at) VALUES($1,$2,$3,$4,$5::jsonb,$6,$7,$8) ON CONFLICT DO NOTHING`, scope.Tenant(), scope.Actor(), s.ID, s.Context, topics, s.Locale, s.Created, s.Updated)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return auditJob(ctx, tx, scope, "nlq.session_created", s.ID)
	})
}

func (d *DB) ReadSession(ctx context.Context, scope store.Scope, id string) (out nlqexec.SessionRecord, err error) {
	if err = checkScope(scope); err != nil || id == "" {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		if e := tx.QueryRow(ctx, `SELECT session_id,tenant_id,actor_id,context_id,topics,locale,created_at,updated_at FROM chartworks.nlq_sessions WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3`, scope.Tenant(), scope.Actor(), id).Scan(&out.ID, &out.Tenant, &out.Actor, &out.Context, &raw, &out.Locale, &out.Created, &out.Updated); e != nil {
			return e
		}
		if json.Unmarshal(raw, &out.Topics) != nil {
			return store.ErrMigration
		}
		return nil
	})
	return out, err
}

func (d *DB) CreateQuery(ctx context.Context, scope store.Scope, q nlqexec.QueryRecord) error {
	if err := checkScope(scope); err != nil || !identity.Identifier(q.ID) || !identity.Identifier(q.Session) || !identity.Identifier(q.Topic) || !identity.Identifier(q.Context) || q.Revision != 1 || q.Status == "" {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := insertNLQQuery(ctx, tx, scope, q); err != nil {
			return err
		}
		return auditJob(ctx, tx, scope, "nlq.query_planned", q.ID)
	})
}

func insertNLQQuery(ctx context.Context, tx pgx.Tx, scope store.Scope, q nlqexec.QueryRecord) error {
	if q.Topics == nil {
		q.Topics = []string{}
	}
	if q.TopicVersions == nil {
		q.TopicVersions = []string{}
	}
	if q.RuleVersions == nil {
		q.RuleVersions = []string{}
	}
	if q.Parameters == nil {
		q.Parameters = []exec.Parameter{}
	}
	if q.Assumptions == nil {
		q.Assumptions = []string{}
	}
	if q.Ambiguities == nil {
		q.Ambiguities = []string{}
	}
	if q.Errors == nil {
		q.Errors = []string{}
	}
	topics, e := marshalNLQ(q.Topics)
	if e != nil {
		return e
	}
	versions, e := marshalNLQ(q.TopicVersions)
	if e != nil {
		return e
	}
	rules, e := marshalNLQ(q.RuleVersions)
	if e != nil {
		return e
	}
	route, e := marshalNLQ(q.Route)
	if e != nil {
		return e
	}
	generation, e := marshalNLQ(q.Generation)
	if e != nil {
		return e
	}
	params, e := marshalNLQ(q.Parameters)
	if e != nil {
		return e
	}
	receipt, e := marshalNLQ(q.Receipt)
	if e != nil {
		return e
	}
	assumptions, e := marshalNLQ(q.Assumptions)
	if e != nil {
		return e
	}
	ambiguities, e := marshalNLQ(q.Ambiguities)
	if e != nil {
		return e
	}
	errorsJSON, e := marshalNLQ(q.Errors)
	if e != nil {
		return e
	}
	var result any
	if q.Result != nil {
		result, e = marshalNLQ(q.Result)
		if e != nil {
			return e
		}
	}
	var operation any
	if q.Operation != "" {
		operation = q.Operation
	}
	var sqlText any
	if q.SQL != "" {
		sqlText = q.SQL
	}
	_, e = tx.Exec(ctx, `INSERT INTO chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id,parent_id,operation,topic_id,topics,topic_versions,rule_versions,context_id,locale,question,route,generation,sql_text,parameters,receipt,status,result,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10::jsonb,$11,$12,$13,$14::jsonb,$15::jsonb,$16,$17::jsonb,$18::jsonb,$19,$20::jsonb,$21::jsonb,$22::jsonb,$23::jsonb,$24,$25,$26,$27,$28)`, scope.Tenant(), scope.Actor(), q.Session, q.ID, nullableString(q.Parent), operation, q.Topic, topics, versions, rules, q.Context, q.Locale, q.Question, route, generation, sqlText, params, receipt, q.Status, result, assumptions, ambiguities, errorsJSON, q.ValidationFixes, q.ExecutionFixes, q.Revision, q.Created, q.Updated)
	return e
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

const nlqQueryColumns = `tenant_id,actor_id,session_id,query_id,parent_id,operation,topic_id,topics,topic_versions,rule_versions,context_id,locale,question,route,generation,sql_text,parameters,receipt,status,result,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision,created_at,updated_at`

func (d *DB) ReadQuery(ctx context.Context, scope store.Scope, id string) (out nlqexec.QueryRecord, err error) {
	if err = checkScope(scope); err != nil || id == "" {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanNLQQuery(tx.QueryRow(ctx, `SELECT `+nlqQueryColumns+` FROM chartworks.nlq_queries WHERE tenant_id=$1 AND actor_id=$2 AND query_id=$3`, scope.Tenant(), scope.Actor(), id), &out); err != nil {
			return err
		}
		return markRuleEvidenceStale(ctx, tx, scope.Tenant(), &out)
	})
	return out, err
}

func (d *DB) ReadOperation(ctx context.Context, scope store.Scope, operation string) (out nlqexec.QueryRecord, err error) {
	if err = checkScope(scope); err != nil || operation == "" {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanNLQQuery(tx.QueryRow(ctx, `SELECT `+nlqQueryColumns+` FROM chartworks.nlq_queries WHERE tenant_id=$1 AND actor_id=$2 AND operation=$3`, scope.Tenant(), scope.Actor(), operation), &out); err != nil {
			return err
		}
		return markRuleEvidenceStale(ctx, tx, scope.Tenant(), &out)
	})
	return out, err
}

// markRuleEvidenceStale is a read-side consumer of the atomic Phase 16
// invalidation ledger. The relation is optional while a Phase 18-only schema
// is bootstrapped; once migration 018 is present, every matching topic/rule
// pin marks the retained query evidence stale without rewriting immutable
// query fields. The marker is surfaced on the shared query projection and is
// preserved if a subsequent replay updates the mutable execution metadata.
func markRuleEvidenceStale(ctx context.Context, tx pgx.Tx, tenant string, out *nlqexec.QueryRecord) error {
	if out == nil || len(out.Topics) == 0 || len(out.RuleVersions) == 0 {
		return nil
	}
	var available bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('chartworks.topic_rule_evidence_invalidations') IS NOT NULL`).Scan(&available); err != nil {
		return err
	}
	if !available {
		return nil
	}
	var stale bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM unnest($1::text[]) WITH ORDINALITY AS t(topic_id,ordinal)
		JOIN unnest($2::text[]) WITH ORDINALITY AS r(rule_version,ordinal) USING (ordinal)
		JOIN chartworks.topic_rule_evidence_invalidations i
		  ON i.tenant_id=$3 AND i.topic_id=t.topic_id AND i.old_rule_version=r.rule_version
	)`, out.Topics, out.RuleVersions, tenant).Scan(&stale); err != nil {
		return err
	}
	if stale {
		out.EvidenceStale = true
		for _, code := range out.Errors {
			if code == "rule_evidence_stale" {
				return nil
			}
		}
		out.Errors = append(out.Errors, "rule_evidence_stale")
	}
	return nil
}

func scanNLQQuery(row pgx.Row, out *nlqexec.QueryRecord) error {
	var tenantValue, actorValue string
	var parent, operation, sqlText *string
	var topics, versions, rules, route, generation, params, receipt, result, assumptions, ambiguities, queryErrors []byte
	if err := row.Scan(&tenantValue, &actorValue, &out.Session, &out.ID, &parent, &operation, &out.Topic, &topics, &versions, &rules, &out.Context, &out.Locale, &out.Question, &route, &generation, &sqlText, &params, &receipt, &out.Status, &result, &assumptions, &ambiguities, &queryErrors, &out.ValidationFixes, &out.ExecutionFixes, &out.Revision, &out.Created, &out.Updated); err != nil {
		return err
	}
	_ = tenantValue
	_ = actorValue
	out.Parent, out.Operation, out.SQL = stringValue(parent), stringValue(operation), stringValue(sqlText)
	for _, value := range []struct {
		raw    []byte
		target any
	}{
		{topics, &out.Topics}, {versions, &out.TopicVersions}, {rules, &out.RuleVersions}, {route, &out.Route},
		{generation, &out.Generation}, {params, &out.Parameters}, {receipt, &out.Receipt}, {assumptions, &out.Assumptions},
		{ambiguities, &out.Ambiguities}, {queryErrors, &out.Errors},
	} {
		if len(value.raw) == 0 || json.Unmarshal(value.raw, value.target) != nil {
			return store.ErrMigration
		}
	}
	if len(result) > 0 {
		var parsed exec.Result
		if json.Unmarshal(result, &parsed) != nil {
			return store.ErrMigration
		}
		out.Result = &parsed
	}
	return nil
}
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (d *DB) UpdateQuery(ctx context.Context, scope store.Scope, q nlqexec.QueryRecord, expected int64) error {
	if err := checkScope(scope); err != nil || q.ID == "" || q.Session == "" || expected < 1 || q.Revision != expected+1 {
		return store.ErrInvalid
	}
	generation, e := marshalNLQ(q.Generation)
	if e != nil {
		return e
	}
	params, e := marshalNLQ(q.Parameters)
	if e != nil {
		return e
	}
	receipt, e := marshalNLQ(q.Receipt)
	if e != nil {
		return e
	}
	assumptions, e := marshalNLQ(q.Assumptions)
	if e != nil {
		return e
	}
	ambiguities, e := marshalNLQ(q.Ambiguities)
	if e != nil {
		return e
	}
	errorsJSON, e := marshalNLQ(q.Errors)
	if e != nil {
		return e
	}
	var result any
	if q.Result != nil {
		result, e = marshalNLQ(q.Result)
		if e != nil {
			return e
		}
	}
	var operation any
	if q.Operation != "" {
		operation = q.Operation
	}
	var sqlText any
	if q.SQL != "" {
		sqlText = q.SQL
	}
	err := d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `UPDATE chartworks.nlq_queries SET operation=$5,generation=$6::jsonb,sql_text=$7,parameters=$8::jsonb,receipt=$9::jsonb,status=$10,result=$11::jsonb,assumptions=$12::jsonb,ambiguities=$13::jsonb,errors=$14::jsonb,validation_fixes=$15,execution_fixes=$16,revision=$17,updated_at=$18 WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND query_id=$4 AND revision=$19`, scope.Tenant(), scope.Actor(), q.Session, q.ID, operation, generation, sqlText, params, receipt, q.Status, result, assumptions, ambiguities, errorsJSON, q.ValidationFixes, q.ExecutionFixes, q.Revision, time.Now().UTC(), expected)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return auditJob(ctx, tx, scope, "nlq.query_executed", q.ID)
	})
	return err
}

func (d *DB) RecordFeedback(ctx context.Context, scope store.Scope, f nlqexec.FeedbackRecord) error {
	if err := checkScope(scope); err != nil || f.ID == "" || f.Session == "" || f.QueryID == "" || (f.Verdict != "positive" && f.Verdict != "negative") {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.nlq_feedback(tenant_id,actor_id,session_id,feedback_id,query_id,verdict,correction_sql,note,provenance,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`, scope.Tenant(), scope.Actor(), f.Session, f.ID, f.QueryID, f.Verdict, nullableString(f.Correction), nullableString(f.Note), f.Provenance, f.Created)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		event, err := newID()
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id) VALUES($1,$2,$3,'nlq.feedback_recorded',$4)`, scope.Tenant(), event, scope.Actor(), f.QueryID)
		return err
	})
}

func (d *DB) UpsertExample(ctx context.Context, scope store.Scope, x nlqexec.ExampleRecord) (out nlqexec.ExampleRecord, err error) {
	if err = checkScope(scope); err != nil || x.ID == "" || x.Topic == "" || x.Question == "" || x.SQL == "" || x.Digest == "" || x.State != "candidate" {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `INSERT INTO chartworks.nlq_examples(tenant_id,example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,provenance,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,'candidate',$7,$8,$9,$10,$11) ON CONFLICT(tenant_id,topic_id,digest) DO UPDATE SET evidence_count=chartworks.nlq_examples.evidence_count+1,weight=LEAST(1.0,GREATEST(chartworks.nlq_examples.weight,EXCLUDED.weight)+0.05),provenance=EXCLUDED.provenance,updated_at=clock_timestamp() RETURNING example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,provenance,created_at,updated_at`, scope.Tenant(), x.ID, x.Topic, x.Question, x.SQL, x.Digest, x.Weight, x.EvidenceCount, x.Provenance, x.Created, x.Updated)
		return scanExample(e, &out)
	})
	return out, err
}

func scanExample(row pgx.Row, out *nlqexec.ExampleRecord) error {
	return row.Scan(&out.ID, &out.Topic, &out.Question, &out.SQL, &out.Digest, &out.State, &out.Weight, &out.EvidenceCount, &out.Provenance, &out.Created, &out.Updated)
}

func (d *DB) ListExamples(ctx context.Context, scope store.Scope, topic string, limit int) (out []nlqexec.ExampleRecord, err error) {
	if err = checkScope(scope); err != nil || topic == "" || limit < 1 || limit > 64 {
		return nil, store.ErrInvalid
	}
	out = []nlqexec.ExampleRecord{}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,provenance,created_at,updated_at FROM chartworks.nlq_examples WHERE tenant_id=$1 AND topic_id=$2 AND state IN('candidate','active') ORDER BY weight DESC,evidence_count DESC,updated_at DESC,example_id LIMIT $3`, scope.Tenant(), topic, limit)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var x nlqexec.ExampleRecord
			if e = scanExample(rows, &x); e != nil {
				return e
			}
			out = append(out, x)
		}
		return rows.Err()
	})
	return out, err
}

func (d *DB) SetExampleState(ctx context.Context, scope store.Scope, id, state string) (out nlqexec.ExampleRecord, err error) {
	if err = checkScope(scope); err != nil || id == "" || (state != "candidate" && state != "active" && state != "retired") {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if e := scanExample(tx.QueryRow(ctx, `UPDATE chartworks.nlq_examples SET state=$3,updated_at=clock_timestamp() WHERE tenant_id=$1 AND example_id=$2 RETURNING example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,provenance,created_at,updated_at`, scope.Tenant(), id, state), &out); e != nil {
			return e
		}
		event, e := newID()
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id) VALUES($1,$2,$3,'nlq.example_changed',$4)`, scope.Tenant(), event, scope.Actor(), id)
		return e
	})
	return out, err
}
