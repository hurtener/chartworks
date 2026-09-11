package reporting

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
)

// Compositions coordinates existing frozen and dynamic execution lanes. It owns
// no SQL parser, source driver, model client, renderer, or independent work queue.
type Compositions struct {
	documents *Documents
	repo      CompositionRepository
	runs      *Runs
	queries   DocumentQueries
	runner    *jobs.RequestRunner
}

// NewCompositions leaves text-only reports usable without optional query lanes.
func NewCompositions(documents *Documents, repo CompositionRepository, runs *Runs, queries DocumentQueries, runner *jobs.RequestRunner) (*Compositions, error) {
	if documents == nil || nilValue(repo) || runner == nil {
		return nil, ErrInvalid
	}
	if nilValue(queries) {
		queries = nil
	}
	return &Compositions{documents: documents, repo: repo, runs: runs, queries: queries, runner: runner}, nil
}

func normalizeCompositionRequest(in CompositionRequest) (CompositionRequest, error) {
	in = clone(in)
	if !identity.Identifier(in.Key) || in.Reference.Revision < 0 || in.Reference.Revision > 256 || len(in.Pages) > 100 ||
		!slices.Contains([]string{"", "published"}, in.Reference.Stage) || in.Reference.Revision > 0 && in.Reference.Stage != "" ||
		!slices.Contains([]string{"", "fail_closed", "allow_partial"}, in.PartialFailure) || in.Preview && in.Reference.Revision == 0 {
		return CompositionRequest{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, page := range in.Pages {
		if !identity.Identifier(page.Page) || seen[page.Page] || len(page.Filters) > 100 || len(page.Overrides) > 100 {
			return CompositionRequest{}, ErrInvalid
		}
		seen[page.Page] = true
		widgets := map[string]bool{}
		for _, override := range page.Overrides {
			if !identity.Identifier(override.Widget) || widgets[override.Widget] || len(override.Arguments) > 64 {
				return CompositionRequest{}, ErrInvalid
			}
			widgets[override.Widget] = true
		}
	}
	return in, nil
}

// Admit reserves the operation key before resolving any floating definition. A
// concurrent or later replay returns the first accepted manifest unchanged.
func (s *Compositions) Admit(ctx context.Context, e identity.Envelope, kind, id string, input CompositionRequest) (CompositionView, error) {
	if s == nil || ctx == nil {
		return CompositionView{}, ErrInvalid
	}
	in, err := normalizeCompositionRequest(input)
	if err != nil {
		return CompositionView{}, err
	}
	if err := RequireDocument(e, kind, id, Execute); err != nil {
		return CompositionView{}, err
	}
	if in.Preview {
		if err := RequireDocument(e, kind, id, Preview); err != nil {
			return CompositionView{}, err
		}
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	hash := digest([]any{CompositionVersion, kind, id, in})
	task, err := s.runner.Admit(ctx, e, in.Key, jobs.RequestInput{Kind: kind + ".run", Target: id, InputHash: hash})
	if err != nil {
		return CompositionView{}, err
	}
	if previous, err := s.repo.ReadComposition(ctx, e, task.ID); err == nil {
		return SummarizeComposition(previous), nil
	} else if !errors.Is(err, store.ErrNotFound) && !errors.Is(err, access.ErrNotFound) {
		return CompositionView{}, err
	}
	if !time.Now().Before(task.Expires) {
		return CompositionView{}, ErrExpired
	}
	manifest, err := s.resolve(ctx, e, task, kind, id, hash, in)
	if err != nil {
		return CompositionView{}, err
	}
	proof, err := prepareComposition(e, manifest)
	if err != nil {
		return CompositionView{}, err
	}
	record, err := s.repo.SealComposition(ctx, e, task, proof)
	if err != nil {
		return CompositionView{}, err
	}
	return SummarizeComposition(record), nil
}

// Get reads retained metadata only. It never calls an execution lane or source.
func (s *Compositions) Get(ctx context.Context, e identity.Envelope, id string) (CompositionView, error) {
	if s == nil || ctx == nil || !identity.Identifier(id) {
		return CompositionView{}, ErrInvalid
	}
	return s.repo.ViewComposition(ctx, e, id)
}

// Widget returns only one currently authorized saved widget/output subset.
func (s *Compositions) Widget(ctx context.Context, e identity.Envelope, id, page, widget string) (CompositionPayload, error) {
	if s == nil || ctx == nil || !identity.Identifier(id) || !identity.Identifier(page) || !identity.Identifier(widget) {
		return CompositionPayload{}, ErrInvalid
	}
	return s.repo.CompositionWidget(ctx, e, id, page, widget)
}

type compositionPageSource struct {
	id       string
	title    string
	snapshot DocumentSnapshot
	input    PageInput
}

type compositionBlockSource struct {
	snapshot Snapshot
	binding  exec.Binding
	refs     []ResourceReference
	err      error
}

func compositionFailure(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "deadline_exceeded"
	case errors.Is(err, access.ErrUnauthenticated), errors.Is(err, access.ErrForbidden), errors.Is(err, access.ErrNotFound):
		return "dependency_denied"
	case errors.Is(err, ErrBudget), errors.Is(err, exec.ErrLimit):
		return "budget_exhausted"
	case errors.Is(err, ErrStale):
		return "dependency_stale"
	case errors.Is(err, ErrInvalid), errors.Is(err, exec.ErrBinding):
		return "binding_invalid"
	default:
		return "dependency_unavailable"
	}
}

func (s *Compositions) resolve(ctx context.Context, e identity.Envelope, task jobs.RequestTask, kind, id, hash string, in CompositionRequest) (CompositionManifest, error) {
	limits := s.documents.limits
	redact := kind == "dashboard" && in.PartialFailure == "allow_partial"
	root, err := s.documents.repo.ReadDocument(ctx, e, kind, id, in.Reference, Execute, redact)
	if err != nil {
		return CompositionManifest{}, err
	}
	if root.State.Archived || root.PublishedAt == nil && !in.Preview {
		return CompositionManifest{}, ErrStale
	}
	definition, err := ProjectStoredDocument(root.Revision.Raw, kind)
	if err != nil {
		return CompositionManifest{}, err
	}
	policy := definition.PartialFailure
	if policy == "" {
		policy = limits.Composition.PartialFailure
	}
	if in.PartialFailure == "allow_partial" && policy != "allow_partial" {
		return CompositionManifest{}, ErrInvalid
	}
	if in.PartialFailure != "" {
		policy = in.PartialFailure
	}
	inputs := map[string]PageInput{}
	for _, page := range in.Pages {
		inputs[page.Page] = page
	}
	pages := []compositionPageSource{}
	if kind == "report" {
		pages = append(pages, compositionPageSource{id: "main", snapshot: root, title: documentTitle(definition), input: inputs["main"]})
		delete(inputs, "main")
	} else {
		for _, page := range definition.Pages {
			// Dashboard read reach does not create report execution authority.
			if err := RequireDocument(e, "report", page.Report, Execute); err != nil {
				return CompositionManifest{}, err
			}
			snapshot, err := s.documents.repo.ReadDocument(ctx, e, "report", page.Report, DocumentReference{Revision: page.Revision}, Execute, false)
			if err != nil {
				return CompositionManifest{}, err
			}
			if snapshot.State.Archived || snapshot.PublishedAt == nil {
				return CompositionManifest{}, ErrStale
			}
			pages = append(pages, compositionPageSource{id: page.ID, title: page.Title, snapshot: snapshot, input: inputs[page.ID]})
			delete(inputs, page.ID)
		}
	}
	if len(inputs) != 0 || len(pages) > limits.Composition.MaxPages {
		return CompositionManifest{}, ErrInvalid
	}
	retention := limits.Execution.Retention
	if in.Preview {
		retention = limits.Execution.PreviewRetention
	}
	m := CompositionManifest{Version: CompositionVersion, ID: task.ID, Tenant: e.Tenant(), Actor: e.User(), Session: e.Session(), Kind: kind, Document: id,
		Revision: root.Revision.Number, Digest: root.Revision.Digest, RequestHash: hash, TaskHash: task.Digest(), Private: in.Preview, Policy: policy,
		Redacted: kind == "dashboard" && DocumentDigest(root.Revision.Raw) != root.Revision.Digest, Created: task.Created, Expires: task.Created.Add(time.Duration(retention)),
		Limits: limits.Composition, ArtifactLimits: limits.Execution, Pages: []CompositionPage{}, Groups: []CompositionGroup{}}
	memo := map[string]compositionBlockSource{}
	groups := map[string]int{}
	widgetCount := 0
	for _, source := range pages {
		d, err := ProjectStoredDocument(source.snapshot.Revision.Raw, "report")
		if err != nil {
			return CompositionManifest{}, err
		}
		if d.PartialFailure == "fail_closed" {
			// Enclosing dashboard policy cannot weaken a page's own strictness.
			m.Policy = "fail_closed"
		}
		resolution := clone(in.Resolution)
		if resolution.At.IsZero() {
			resolution.At = task.Created
		}
		if resolution.Timezone != "" && resolution.Timezone != d.Timezone {
			return CompositionManifest{}, ErrInvalid
		}
		resolution.Timezone = d.Timezone
		if _, err := resolveReportFilters(d.Filters, source.input.Filters); err != nil {
			return CompositionManifest{}, err
		}
		overrides := map[string][]Argument{}
		for _, override := range source.input.Overrides {
			overrides[override.Widget] = override.Arguments
		}
		page := CompositionPage{ID: source.id, Report: source.snapshot.State.ID, Revision: source.snapshot.Revision.Number, Digest: source.snapshot.Revision.Digest,
			Title: source.title, Private: source.snapshot.PublishedAt == nil, Locale: d.Locale, Timezone: d.Timezone, Widgets: []CompositionWidget{}}
		for _, widget := range d.Widgets {
			widgetCount++
			if widgetCount > limits.Composition.MaxWidgets {
				return CompositionManifest{}, ErrBudget
			}
			cw := CompositionWidget{Definition: clone(widget), Parameters: []BoundValue{}}
			var group CompositionGroup
			var groupErr error
			switch widget.Kind {
			case "text":
				if len(overrides[widget.ID]) != 0 {
					return CompositionManifest{}, ErrInvalid
				}
			case "block":
				group, cw, groupErr = s.resolveBlock(ctx, e, m, d, widget, source.input.Filters, overrides[widget.ID], resolution, memo)
			case "query":
				if len(overrides[widget.ID]) != 0 {
					return CompositionManifest{}, ErrInvalid
				}
				group, groupErr = s.resolveQuery(ctx, e, m, d, widget, source.snapshot.Revision.Origins, resolution)
			}
			delete(overrides, widget.ID)
			if groupErr != nil {
				if ctx.Err() != nil || !e.Valid() {
					return CompositionManifest{}, groupErr
				}
				cw.Code = compositionFailure(groupErr)
			}
			if widget.Query != nil {
				switch {
				case !limits.Composition.LiveQueries:
					cw.Code = "live_queries_disabled"
				case widget.Query.Durability == "session_bound" && !limits.Composition.SessionBound:
					cw.Code = "session_bound_disabled"
				case widget.Query.Durability == "session_bound" && !m.Private:
					cw.Code = "session_unavailable"
				}
			}
			if group.Kind != "" && cw.Code == "" {
				key := groupIdentity(group)
				if position, found := groups[key]; found {
					for _, output := range group.Outputs {
						if !slices.Contains(m.Groups[position].Outputs, output) {
							m.Groups[position].Outputs = append(m.Groups[position].Outputs, output)
						}
					}
					cw.Group = m.Groups[position].ID
				} else if len(m.Groups) >= limits.Composition.MaxQueries {
					cw.Code = "budget_exhausted"
				} else {
					group.ID = "group-" + strconv.Itoa(len(m.Groups)+1)
					groups[key] = len(m.Groups)
					m.Groups = append(m.Groups, group)
					cw.Group = group.ID
				}
			}
			page.Widgets = append(page.Widgets, cw)
		}
		if len(overrides) != 0 {
			return CompositionManifest{}, ErrInvalid
		}
		m.Pages = append(m.Pages, page)
	}
	return m, nil
}

func documentTitle(d DocumentDefinition) string {
	for _, metadata := range d.Metadata {
		if metadata.Locale == d.Locale {
			return metadata.Title
		}
	}
	return ""
}

// Output subsets and binding provenance do not change query semantics. Exact
// parameters, partition, policy, locale/timezone and revision do. No cross-run or
// cross-context shared cache is introduced by this within-manifest grouping.
func groupIdentity(g CompositionGroup) string {
	if g.Kind == "query" {
		// Distinct query widgets retain independent generated-query evidence.
		return digest([]any{g.Kind, g.Origin, g.Query, g.Binding, g.Locale, g.Private, g.Resolution})
	}
	return digest([]any{g.Kind, g.Block, g.Revision, g.Definition, g.Execution, g.Resolved.Parameters, g.Binding, g.Policy, g.Locale, g.Private, g.Narrative, g.Resolution})
}

func (s *Compositions) resolveBlock(ctx context.Context, e identity.Envelope, m CompositionManifest, d DocumentDefinition, w Widget, filters, overrides []Argument, resolution Resolution, memo map[string]compositionBlockSource) (CompositionGroup, CompositionWidget, error) {
	cw := CompositionWidget{Definition: clone(w), Parameters: []BoundValue{}}
	if s.runs == nil || s.documents.blocks == nil {
		return CompositionGroup{}, cw, ErrUnavailable
	}
	key := w.Block.Block + ":" + strconv.FormatInt(w.Block.Revision, 10)
	entry, found := memo[key]
	if !found {
		entry.snapshot, entry.err = s.documents.blocks.repo.ReadBlock(ctx, e, w.Block.Block, Reference{Revision: w.Block.Revision}, Execute)
		if entry.err == nil {
			definition := entry.snapshot.Revision.Definition
			_, entry.refs, entry.err = s.documents.blocks.resolveDefinitions(ctx, e, definition, true)
			if entry.err == nil {
				entry.binding, entry.err = s.documents.blocks.sources.ContextBinding(ctx, e, definition.Source, definition.Context)
			}
		}
		memo[key] = entry
	}
	if entry.err != nil {
		return CompositionGroup{}, cw, entry.err
	}
	policy := w.Block.Policy
	if policy == "" {
		policy = "published"
	}
	if err := runEligibility(e, entry.snapshot, policy, time.Now()); err != nil {
		return CompositionGroup{}, cw, err
	}
	if exec.Hash(entry.binding) != entry.snapshot.Validation.BindingDigest {
		return CompositionGroup{}, cw, ErrStale
	}
	if m.Private {
		if err := Require(e, w.Block.Block, Preview); err != nil {
			return CompositionGroup{}, cw, err
		}
	}
	definition := entry.snapshot.Revision.Definition
	selected, err := s.runs.selectRunOutputs(definition, RunRequest{Outputs: w.Block.Outputs, Narrative: w.Block.Narrative})
	if err != nil {
		return CompositionGroup{}, cw, err
	}
	arguments, resolved, err := widgetArguments(definition.Parameters, d, w, filters, overrides, resolution)
	if err != nil {
		return CompositionGroup{}, cw, err
	}
	outputs := []string{}
	calls, tokens := 0, 0
	for _, output := range selected {
		outputs = append(outputs, output.ID)
		if output.Narrative != nil {
			calls += output.Narrative.MaxCalls
			tokens += output.Narrative.MaxTokens
		}
	}
	cw.Definition.Block.Revision, cw.Definition.Block.Outputs = entry.snapshot.Revision.Number, clone(outputs)
	cw.Parameters = clone(resolved.Values)
	trust := project(entry.snapshot, time.Now()).Trust
	return CompositionGroup{Kind: "block", Block: w.Block.Block, Revision: entry.snapshot.Revision.Number, Definition: entry.snapshot.Revision.Digest, Execution: entry.snapshot.Revision.ExecutionDigest,
		Outputs: outputs, Arguments: arguments, Resolved: resolved, Resolution: resolution, Binding: entry.binding, Locale: d.Locale, Policy: policy, Private: m.Private, Narrative: w.Block.Narrative,
		References: clone(entry.refs), Trust: &trust, ReservedCalls: calls, ReservedTokens: tokens}, cw, nil
}

func (s *Compositions) resolveQuery(ctx context.Context, e identity.Envelope, m CompositionManifest, d DocumentDefinition, w Widget, origins []QueryOrigin, resolution Resolution) (CompositionGroup, error) {
	if !m.Limits.LiveQueries || w.Query.Durability == "session_bound" && (!m.Limits.SessionBound || !m.Private) || s.queries == nil || s.documents.blocks == nil {
		return CompositionGroup{}, ErrUnavailable
	}
	var origin *QueryOrigin
	for _, candidate := range origins {
		if candidate.Widget == w.ID {
			value := clone(candidate)
			origin = &value
		}
	}
	if origin == nil || origin.Actor != "" && (origin.Actor != e.User() || origin.Session != e.Session()) {
		return CompositionGroup{}, access.ErrNotFound
	}
	actual, err := s.queries.InspectDocumentQuery(ctx, e, *w.Query)
	if err != nil {
		return CompositionGroup{}, err
	}
	actual.Widget = w.ID
	if digest(actual) != digest(*origin) {
		return CompositionGroup{}, ErrStale
	}
	_, refs, err := s.documents.blocks.resolveDefinitions(ctx, e, Definition{Source: origin.Source, Context: origin.Context, Topics: origin.Topics}, true)
	if err != nil {
		return CompositionGroup{}, err
	}
	binding, err := s.documents.blocks.sources.ContextBinding(ctx, e, origin.Source, origin.Context)
	if err != nil {
		return CompositionGroup{}, err
	}
	return CompositionGroup{Kind: "query", Query: clone(w.Query), Origin: origin, Binding: binding, References: refs, Private: m.Private, Locale: d.Locale,
		Outputs: []string{}, Arguments: []Argument{}, Resolution: resolution, Policy: "dynamic", ReservedCalls: 3, ReservedTokens: 1 << 20}, nil
}
