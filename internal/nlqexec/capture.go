package nlqexec

import (
	"context"
	"slices"
	"strings"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// CaptureEvidence is a private, source-backed handoff to definition authoring.
// It carries no publication, certification or result-retention authority. Public
// query routes continue using their existing SQL-redacted response projections.
type CapturedAmountCompleteness struct {
	MetricLabel        string
	Metric             string
	ValueColumn        int
	UnknownCountMetric string
	UnknownCountColumn int
}

type CaptureEvidence struct {
	AmountCompleteness []CapturedAmountCompleteness
	SQL                string
	Parameters         []exec.Parameter
	Schema             []exec.Field
	Source             string
	Context            string
	Publications       []topics.Published
	RuleVersions       []string
	Templates          []rulesets.TemplateSelection
	Question           string
}

// CaptureDefinition reads a completed, same-actor/same-session query and rechecks
// its original resource set with current signed authority. It neither regenerates
// SQL nor opens a new source execution, and rejects planned/failed/stale results.
func (s *Service) CaptureDefinition(ctx context.Context, e identity.Envelope, id string) (CaptureEvidence, error) {
	var out CaptureEvidence
	if ctx == nil || s == nil || !identity.Identifier(id) {
		return out, ErrInvalid
	}
	if !e.Valid() {
		return out, access.ErrUnauthenticated
	}
	if !e.Has("reporting.sql.read") {
		return out, ErrInspectionRequired
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	scope, err := scope(e)
	if err != nil {
		return out, err
	}
	q, err := s.repo.ReadQuery(ctx, scope, id)
	if err != nil {
		return out, err
	}
	if q.Session != e.Session() {
		return out, ErrForeignSession
	}
	if !slices.Contains([]string{"succeeded", "empty", "truncated"}, q.Status) || q.Result == nil || q.EvidenceStale || strings.TrimSpace(q.SQL) == "" || len(q.SQL) > maxSQLBytes || len(q.Result.Schema) == 0 {
		return out, ErrNoPlan
	}
	admitted, err := s.retainedAdmission(ctx, e, q)
	if err != nil {
		return out, err
	}
	if err := access.Require(e, "query.execute", admitted.resources...); err != nil {
		return out, err
	}
	if err := s.verifyAnalyticalOutputReplay(ctx, e, q, admitted); err != nil {
		return out, err
	}
	out = CaptureEvidence{SQL: q.SQL, Parameters: append([]exec.Parameter{}, q.Parameters...), Schema: append([]exec.Field{}, q.Result.Schema...), Source: admitted.source, Context: admitted.context, Question: q.Question, Publications: []topics.Published{}, RuleVersions: append([]string{}, q.RuleVersions...), Templates: append([]rulesets.TemplateSelection{}, q.Templates...)}
	for _, item := range resultAmountCompleteness(q.Analytical, nil, false) {
		out.AmountCompleteness = append(out.AmountCompleteness, CapturedAmountCompleteness{Metric: item.Metric, ValueColumn: item.ValueColumn, UnknownCountMetric: item.UnknownCountMetric, UnknownCountColumn: item.UnknownCountColumn})
	}
	for i, topic := range q.Topics {
		contract, err := s.topics.RetainedContract(ctx, e, topic, q.TopicVersions[i])
		if err != nil {
			return CaptureEvidence{}, err
		}
		out.Publications = append(out.Publications, contract.Publication)
	}
	for i := range out.AmountCompleteness {
		item := &out.AmountCompleteness[i]
		for _, publication := range out.Publications {
			for _, measure := range publication.Definition.Measures {
				if item.Metric == publication.Definition.Topic+":measure:"+measure.ID {
					item.MetricLabel = measure.Name
				}
			}
			for _, kpi := range publication.Definition.KPIs {
				if item.Metric == publication.Definition.Topic+":kpi:"+kpi.ID {
					item.MetricLabel = kpi.Name
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return CaptureEvidence{}, err
	}
	if !e.Valid() {
		return CaptureEvidence{}, access.ErrUnauthenticated
	}
	return out, nil
}
