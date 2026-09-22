package postgres

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
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

// CreateSession persists one tenant- and actor-scoped NLQ session anchor.
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

// ReadSession returns one tenant- and actor-scoped NLQ session anchor.
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

// CreateQuery persists protected generation evidence for one planned query.
func (d *DB) CreateQuery(ctx context.Context, scope store.Scope, q nlqexec.QueryRecord) error {
	decodedDigest, digestErr := hex.DecodeString(q.ParentDigest)
	lineageValid := q.Parent == "" && q.ParentRevision == 0 && q.ParentDigest == "" || identity.Identifier(q.Parent) && q.ParentRevision > 0 && digestErr == nil && len(decodedDigest) == 32 && q.ParentDigest == strings.ToLower(q.ParentDigest)
	if err := checkScope(scope); err != nil || !identity.Identifier(q.ID) || !identity.Identifier(q.Session) || !identity.Identifier(q.Topic) || !identity.Identifier(q.Context) || q.Revision != 1 || q.Status == "" || !lineageValid || !validTemplateSelectionEvidence(q.Templates, q.Route.Templates, q.Route.Request.Templates, q.Topics, q.TopicVersions, q.RuleVersions) {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if q.Parent != "" {
			var parent nlqexec.QueryRecord
			if err := scanNLQQuery(tx.QueryRow(ctx, `SELECT `+nlqQueryColumns+` FROM chartworks.nlq_queries WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND query_id=$4 FOR UPDATE`, scope.Tenant(), scope.Actor(), q.Session, q.Parent), &parent); err != nil {
				return err
			}
			if parent.Revision != q.ParentRevision || nlqexec.QueryLineageDigest(parent) != q.ParentDigest {
				return store.ErrConflict
			}
		}
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
	if q.Templates == nil {
		q.Templates = []rulesets.TemplateSelection{}
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
	templates, e := marshalNLQ(q.Templates)
	if e != nil {
		return e
	}
	selection, e := marshalNLQ(q.ExampleSelection)
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
	var clarification any
	if q.Clarification != nil {
		if q.Clarification.SchemaVersion != 1 {
			return store.ErrInvalid
		}
		clarification, e = marshalNLQ(q.Clarification)
		if e != nil {
			return e
		}
	}
	_, e = tx.Exec(ctx, `INSERT INTO chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id,parent_id,parent_revision,parent_digest,operation,topic_id,topics,topic_versions,rule_versions,template_selections,example_selection,context_id,locale,question,route,generation,sql_text,parameters,receipt,status,result,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision,created_at,updated_at,clarification) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11::jsonb,$12::jsonb,$13::jsonb,$14::jsonb,$15,$16,$17,$18::jsonb,$19::jsonb,$20,$21::jsonb,$22::jsonb,$23,$24::jsonb,$25::jsonb,$26::jsonb,$27::jsonb,$28,$29,$30,$31,$32,$33::jsonb)`, scope.Tenant(), scope.Actor(), q.Session, q.ID, nullableString(q.Parent), nullableInt64(q.ParentRevision), nullableString(q.ParentDigest), operation, q.Topic, topics, versions, rules, templates, selection, q.Context, q.Locale, q.Question, route, generation, sqlText, params, receipt, q.Status, result, assumptions, ambiguities, errorsJSON, q.ValidationFixes, q.ExecutionFixes, q.Revision, q.Created, q.Updated, clarification)
	return e
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

const nlqQueryColumns = `tenant_id,actor_id,session_id,query_id,parent_id,parent_revision,parent_digest,operation,topic_id,topics,topic_versions,rule_versions,template_selections,example_selection,context_id,locale,question,route,generation,sql_text,parameters,receipt,status,result,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision,created_at,updated_at,clarification`

// ReadQuery returns protected query metadata and consumes rule invalidation fences.
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

// ReadOperation returns the durable query receipt bound to an operation key.
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
	var parent, parentDigest, operation, sqlText *string
	var parentRevision *int64
	var topics, versions, rules, templates, selection, route, generation, params, receipt, result, assumptions, ambiguities, queryErrors, clarification []byte
	if err := row.Scan(&tenantValue, &actorValue, &out.Session, &out.ID, &parent, &parentRevision, &parentDigest, &operation, &out.Topic, &topics, &versions, &rules, &templates, &selection, &out.Context, &out.Locale, &out.Question, &route, &generation, &sqlText, &params, &receipt, &out.Status, &result, &assumptions, &ambiguities, &queryErrors, &out.ValidationFixes, &out.ExecutionFixes, &out.Revision, &out.Created, &out.Updated, &clarification); err != nil {
		return err
	}
	_ = tenantValue
	_ = actorValue
	out.Parent, out.Operation, out.SQL = stringValue(parent), stringValue(operation), stringValue(sqlText)
	out.ParentDigest = stringValue(parentDigest)
	if parentRevision != nil {
		out.ParentRevision = *parentRevision
	}
	for _, value := range []struct {
		raw    []byte
		target any
	}{
		{topics, &out.Topics}, {versions, &out.TopicVersions}, {rules, &out.RuleVersions}, {templates, &out.Templates}, {selection, &out.ExampleSelection}, {route, &out.Route},
		{generation, &out.Generation}, {params, &out.Parameters}, {receipt, &out.Receipt}, {assumptions, &out.Assumptions},
		{ambiguities, &out.Ambiguities}, {queryErrors, &out.Errors},
	} {
		if len(value.raw) == 0 || json.Unmarshal(value.raw, value.target) != nil {
			return store.ErrMigration
		}
	}
	// Migration 039 backfilled the new column with an explicit empty array, but
	// pre-migration route JSON omitted empty template fields and decodes them as
	// nil. Canonicalize only that legacy no-selection state. Any actual governed
	// selection must still match all three immutable evidence projections.
	if !normalizeTemplateSelectionEvidence(out, templates, route) {
		return store.ErrMigration
	}
	if len(clarification) > 0 && string(clarification) != "null" {
		var evidence nlqexec.ClarificationEvidence
		if json.Unmarshal(clarification, &evidence) != nil || evidence.SchemaVersion != 1 {
			return store.ErrMigration
		}
		out.Clarification = &evidence
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

func normalizeTemplateSelectionEvidence(out *nlqexec.QueryRecord, columnJSON, routeJSON []byte) bool {
	column, route, request, emptyShape, ok := decodeTemplateSelectionEvidence(columnJSON, routeJSON)
	if !ok || !sameTemplateSelections(column, out.Templates) || !sameTemplateSelections(route, out.Route.Templates) || !sameTemplateSelections(request, out.Route.Request.Templates) {
		return false
	}
	if len(column) == 0 {
		if !emptyShape {
			return false
		}
		out.Templates = []rulesets.TemplateSelection{}
		out.Route.Templates = []rulesets.TemplateSelection{}
		out.Route.Request.Templates = []rulesets.TemplateSelection{}
		return true
	}
	return validTemplateSelectionEvidence(column, route, request, out.Topics, out.TopicVersions, out.RuleVersions)
}

func decodeTemplateSelectionEvidence(columnJSON, routeJSON []byte) (column, route, request []rulesets.TemplateSelection, emptyShape, ok bool) {
	if column, ok = decodeTemplateSelectionArray(columnJSON); !ok {
		return nil, nil, nil, false, false
	}
	var routeObject map[string]json.RawMessage
	if json.Unmarshal(routeJSON, &routeObject) != nil || routeObject == nil {
		return nil, nil, nil, false, false
	}
	routeRaw, routePresent := routeObject["templates"]
	if routePresent {
		if route, ok = decodeTemplateSelectionArray(routeRaw); !ok {
			return nil, nil, nil, false, false
		}
	}
	requestRaw, requestPresent := routeObject["request"]
	requestTemplatesPresent := false
	if requestPresent {
		var requestObject map[string]json.RawMessage
		if json.Unmarshal(requestRaw, &requestObject) != nil || requestObject == nil {
			return nil, nil, nil, false, false
		}
		requestTemplatesRaw, present := requestObject["templates"]
		requestTemplatesPresent = present
		if present {
			if request, ok = decodeTemplateSelectionArray(requestTemplatesRaw); !ok {
				return nil, nil, nil, false, false
			}
		}
	}
	if len(column) == 0 {
		legacyOmission := !routePresent && !requestTemplatesPresent
		explicitEmpty := routePresent && requestTemplatesPresent && len(route) == 0 && len(request) == 0
		return column, route, request, legacyOmission || explicitEmpty, true
	}
	if !routePresent || !requestTemplatesPresent {
		return nil, nil, nil, false, false
	}
	return column, route, request, false, true
}

func decodeTemplateSelectionArray(raw []byte) ([]rulesets.TemplateSelection, bool) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, false
	}
	var selections []rulesets.TemplateSelection
	if json.Unmarshal(raw, &selections) != nil || selections == nil {
		return nil, false
	}
	return selections, true
}

func validTemplateSelectionEvidence(column, route, request []rulesets.TemplateSelection, queryTopics, topicVersions, ruleVersions []string) bool {
	if !sameTemplateSelections(column, route) || !sameTemplateSelections(column, request) || len(column) > 4 {
		return false
	}
	seenTopics := make(map[string]bool, len(column))
	for _, selection := range column {
		if !identity.Identifier(selection.ID) || !identity.Identifier(selection.Topic) || !identity.Identifier(selection.TopicVersion) || !identity.Identifier(selection.RuleVersion) || !topics.DigestValid(selection.PackDigest) || !topics.DigestValid(selection.RuleDigest) || seenTopics[selection.Topic] {
			return false
		}
		aligned := false
		for i, topic := range queryTopics {
			if i < len(topicVersions) && i < len(ruleVersions) && selection.Topic == topic && selection.TopicVersion == topicVersions[i] && selection.RuleVersion == ruleVersions[i] {
				aligned = true
				break
			}
		}
		if !aligned {
			return false
		}
		seenTopics[selection.Topic] = true
	}
	return true
}

func sameTemplateSelections(left, right []rulesets.TemplateSelection) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// UpdateQuery advances mutable execution metadata under a query revision CAS.
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

// RecordFeedback stores one scoped review receipt without changing a publication.
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

// ApplyFeedback atomically stores a feedback receipt and its learning effect.
// An exact or same-outcome replay is a successful no-op, so retries cannot
// amplify evidence.
func (d *DB) ApplyFeedback(ctx context.Context, scope store.Scope, f nlqexec.FeedbackRecord, x nlqexec.ExampleRecord) (out nlqexec.ExampleRecord, applied bool, err error) {
	if err = checkScope(scope); err != nil || f.ID == "" || f.Session == "" || f.QueryID == "" || (f.Verdict != "positive" && f.Verdict != "negative") {
		return out, false, store.ErrInvalid
	}
	if x.ID != "" && (x.Topic == "" || x.Question == "" || x.SQL == "" || x.Digest == "" || x.State != "candidate" || x.Version != 1 || x.Origin.SchemaVersion != 1 || (x.EvidenceOutcome != "positive" && x.EvidenceOutcome != "negative")) {
		return out, false, store.ErrInvalid
	}
	var origin []byte
	if x.ID != "" {
		origin, err = marshalNLQ(x.Origin)
		if err != nil {
			return out, false, err
		}
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `INSERT INTO chartworks.nlq_feedback(tenant_id,actor_id,session_id,feedback_id,query_id,verdict,correction_sql,note,provenance,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`, scope.Tenant(), scope.Actor(), f.Session, f.ID, f.QueryID, f.Verdict, nullableString(f.Correction), nullableString(f.Note), f.Provenance, f.Created)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		applied = true
		if x.ID != "" {
			row := tx.QueryRow(ctx, `INSERT INTO chartworks.nlq_examples(tenant_id,example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,provenance,created_at,updated_at,uncertainty,positive_evidence,negative_evidence,origin,version) VALUES($1,$2,$3,$4,$5,$6,'candidate',$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,1) ON CONFLICT(tenant_id,topic_id,digest) DO UPDATE SET evidence_count=chartworks.nlq_examples.evidence_count+1,positive_evidence=chartworks.nlq_examples.positive_evidence+EXCLUDED.positive_evidence,negative_evidence=chartworks.nlq_examples.negative_evidence+EXCLUDED.negative_evidence,weight=(chartworks.nlq_examples.positive_evidence+EXCLUDED.positive_evidence+1)::double precision/(chartworks.nlq_examples.positive_evidence+EXCLUDED.positive_evidence+chartworks.nlq_examples.negative_evidence+EXCLUDED.negative_evidence+2),uncertainty=1/sqrt((chartworks.nlq_examples.positive_evidence+EXCLUDED.positive_evidence+chartworks.nlq_examples.negative_evidence+EXCLUDED.negative_evidence+2)::double precision),provenance=EXCLUDED.provenance,updated_at=clock_timestamp(),version=chartworks.nlq_examples.version+1 WHERE chartworks.nlq_examples.origin=EXCLUDED.origin RETURNING example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,uncertainty,positive_evidence,negative_evidence,origin,version,reviewed_by,review_note,reviewed_at,provenance,created_at,updated_at`, scope.Tenant(), x.ID, x.Topic, x.Question, x.SQL, x.Digest, x.Weight, x.EvidenceCount, x.Provenance, x.Created, x.Updated, x.Uncertainty, x.PositiveEvidence, x.NegativeEvidence, origin)
			if e = scanExample(row, &out); e != nil {
				if e == pgx.ErrNoRows {
					return store.ErrConflict
				}
				return e
			}
		}
		event, e := newID()
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id) VALUES($1,$2,$3,'nlq.feedback_recorded',$4)`, scope.Tenant(), event, scope.Actor(), f.QueryID)
		return e
	})
	if err != nil {
		applied = false
	}
	return out, applied, err
}

// UpsertExample deduplicates one candidate learning example by topic and digest.
func (d *DB) UpsertExample(ctx context.Context, scope store.Scope, x nlqexec.ExampleRecord) (out nlqexec.ExampleRecord, err error) {
	if err = checkScope(scope); err != nil || x.ID == "" || x.Topic == "" || x.Question == "" || x.SQL == "" || x.Digest == "" || x.State != "candidate" || x.Version != 1 || x.Origin.SchemaVersion != 1 || (x.EvidenceOutcome != "positive" && x.EvidenceOutcome != "negative") {
		return out, store.ErrInvalid
	}
	origin, err := marshalNLQ(x.Origin)
	if err != nil {
		return out, err
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `INSERT INTO chartworks.nlq_examples(tenant_id,example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,provenance,created_at,updated_at,uncertainty,positive_evidence,negative_evidence,origin,version) VALUES($1,$2,$3,$4,$5,$6,'candidate',$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,1) ON CONFLICT(tenant_id,topic_id,digest) DO UPDATE SET evidence_count=chartworks.nlq_examples.evidence_count+1,positive_evidence=chartworks.nlq_examples.positive_evidence+EXCLUDED.positive_evidence,negative_evidence=chartworks.nlq_examples.negative_evidence+EXCLUDED.negative_evidence,weight=(chartworks.nlq_examples.positive_evidence+EXCLUDED.positive_evidence+1)::double precision/(chartworks.nlq_examples.positive_evidence+EXCLUDED.positive_evidence+chartworks.nlq_examples.negative_evidence+EXCLUDED.negative_evidence+2),uncertainty=1/sqrt((chartworks.nlq_examples.positive_evidence+EXCLUDED.positive_evidence+chartworks.nlq_examples.negative_evidence+EXCLUDED.negative_evidence+2)::double precision),provenance=EXCLUDED.provenance,updated_at=clock_timestamp(),version=chartworks.nlq_examples.version+1 WHERE chartworks.nlq_examples.origin=EXCLUDED.origin RETURNING example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,uncertainty,positive_evidence,negative_evidence,origin,version,reviewed_by,review_note,reviewed_at,provenance,created_at,updated_at`, scope.Tenant(), x.ID, x.Topic, x.Question, x.SQL, x.Digest, x.Weight, x.EvidenceCount, x.Provenance, x.Created, x.Updated, x.Uncertainty, x.PositiveEvidence, x.NegativeEvidence, origin)
		if e := scanExample(e, &out); e != nil {
			if e == pgx.ErrNoRows {
				return store.ErrConflict
			}
			return e
		}
		return nil
	})
	return out, err
}

// ImportExample inserts a portable candidate once. A replay returns the exact
// existing row without adding evidence, and a digest collision with another
// origin fails closed.
func (d *DB) ImportExample(ctx context.Context, scope store.Scope, x nlqexec.ExampleRecord) (out nlqexec.ExampleRecord, applied bool, err error) {
	if err = checkScope(scope); err != nil || x.ID == "" || x.Topic == "" || x.Question == "" || x.SQL == "" || x.Digest == "" || x.State != "candidate" || x.Version != 1 || x.Origin.SchemaVersion != 1 {
		return out, false, store.ErrInvalid
	}
	origin, err := marshalNLQ(x.Origin)
	if err != nil {
		return out, false, err
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `INSERT INTO chartworks.nlq_examples(tenant_id,example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,provenance,created_at,updated_at,uncertainty,positive_evidence,negative_evidence,origin,version) VALUES($1,$2,$3,$4,$5,$6,'candidate',$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,1) ON CONFLICT(tenant_id,topic_id,digest) DO NOTHING`, scope.Tenant(), x.ID, x.Topic, x.Question, x.SQL, x.Digest, x.Weight, x.EvidenceCount, x.Provenance, x.Created, x.Updated, x.Uncertainty, x.PositiveEvidence, x.NegativeEvidence, origin)
		if e != nil {
			return e
		}
		applied = tag.RowsAffected() == 1
		if e = scanExample(tx.QueryRow(ctx, `SELECT example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,uncertainty,positive_evidence,negative_evidence,origin,version,reviewed_by,review_note,reviewed_at,provenance,created_at,updated_at FROM chartworks.nlq_examples WHERE tenant_id=$1 AND topic_id=$2 AND digest=$3`, scope.Tenant(), x.Topic, x.Digest), &out); e != nil {
			return e
		}
		if exec.Hash(out.Origin) != exec.Hash(x.Origin) {
			return store.ErrConflict
		}
		return nil
	})
	if err != nil {
		applied = false
	}
	return out, applied, err
}

func scanExample(row pgx.Row, out *nlqexec.ExampleRecord) error {
	var origin []byte
	if err := row.Scan(&out.ID, &out.Topic, &out.Question, &out.SQL, &out.Digest, &out.State, &out.Weight, &out.EvidenceCount, &out.Uncertainty, &out.PositiveEvidence, &out.NegativeEvidence, &origin, &out.Version, &out.ReviewedBy, &out.ReviewNote, &out.ReviewedAt, &out.Provenance, &out.Created, &out.Updated); err != nil {
		return err
	}
	if json.Unmarshal(origin, &out.Origin) != nil || out.Origin.SchemaVersion != 1 {
		return store.ErrMigration
	}
	return nil
}

// ReadExample loads one exact tenant-scoped learning row for a state change.
// The caller must still reauthorize its resolved topic and dependencies before
// invoking any mutation; this method never exposes an unscoped list as a
// substitute for exact identity.
func (d *DB) ReadExample(ctx context.Context, scope store.Scope, id string) (out nlqexec.ExampleRecord, err error) {
	if err = checkScope(scope); err != nil || id == "" {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanExample(tx.QueryRow(ctx, `SELECT example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,uncertainty,positive_evidence,negative_evidence,origin,version,reviewed_by,review_note,reviewed_at,provenance,created_at,updated_at FROM chartworks.nlq_examples WHERE tenant_id=$1 AND example_id=$2`, scope.Tenant(), id), &out)
	})
	return out, err
}

// ListExamples returns bounded candidate and active examples for one topic.
func (d *DB) ListExamples(ctx context.Context, scope store.Scope, topic string, limit int) (out []nlqexec.ExampleRecord, err error) {
	if err = checkScope(scope); err != nil || topic == "" || limit < 1 || limit > 64 {
		return nil, store.ErrInvalid
	}
	out = []nlqexec.ExampleRecord{}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,uncertainty,positive_evidence,negative_evidence,origin,version,reviewed_by,review_note,reviewed_at,provenance,created_at,updated_at FROM chartworks.nlq_examples WHERE tenant_id=$1 AND topic_id=$2 AND state IN('candidate','active') ORDER BY weight DESC,evidence_count DESC,updated_at DESC,example_id LIMIT $3`, scope.Tenant(), topic, limit)
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

// SetExampleState advances one candidate example through its explicit review state.
func (d *DB) SetExampleState(ctx context.Context, scope store.Scope, in nlqexec.ExampleStateRequest, reviewer string) (out nlqexec.ExampleRecord, err error) {
	if err = checkScope(scope); err != nil || in.ExampleID == "" || reviewer == "" || (in.State != "candidate" && in.State != "active" && in.State != "retired") {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if e := scanExample(tx.QueryRow(ctx, `UPDATE chartworks.nlq_examples SET state=$3,reviewed_by=CASE WHEN $3='active' THEN $4 ELSE reviewed_by END,review_note=CASE WHEN $3='active' THEN $5 ELSE review_note END,reviewed_at=CASE WHEN $3='active' THEN clock_timestamp() ELSE reviewed_at END,updated_at=clock_timestamp(),version=version+1 WHERE tenant_id=$1 AND example_id=$2 AND ($6=0 OR version=$6) AND ($3<>'active' OR (positive_evidence>=1 AND weight>=0.60 AND positive_evidence>negative_evidence)) RETURNING example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,uncertainty,positive_evidence,negative_evidence,origin,version,reviewed_by,review_note,reviewed_at,provenance,created_at,updated_at`, scope.Tenant(), in.ExampleID, in.State, reviewer, in.ReviewNote, in.ExpectedVersion), &out); e != nil {
			if e == pgx.ErrNoRows {
				var exists bool
				if existsErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.nlq_examples WHERE tenant_id=$1 AND example_id=$2)`, scope.Tenant(), in.ExampleID).Scan(&exists); existsErr != nil {
					return existsErr
				}
				if !exists {
					return store.ErrNotFound
				}
				return store.ErrConflict
			}
			return e
		}
		event, e := newID()
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id) VALUES($1,$2,$3,'nlq.example_changed',$4)`, scope.Tenant(), event, scope.Actor(), in.ExampleID)
		return e
	})
	return out, err
}
