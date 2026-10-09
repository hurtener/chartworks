package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

func (s *Authoring) optionRepository() (*Service, AuthoringOptionRepository, error) {
	b, err := s.blockService()
	if err != nil {
		return nil, nil, err
	}
	r, ok := b.repo.(AuthoringOptionRepository)
	if !ok {
		return nil, nil, ErrUnavailable
	}
	return b, r, nil
}

func authoringOptionView(r AuthoringOptionRecord) AuthoringOptionView {
	out := AuthoringOptionView{Operation: r.Operation, InputDigest: r.InputDigest, Status: r.Status, Code: r.Code, ExecutionStatus: r.ExecutionStatus, RemoteState: r.RemoteState, SourceRevision: r.SourceRevision, Options: []FilterOption{}}
	if r.Status == "accepted" && !time.Now().Before(r.Deadline) {
		out.Status = "uncertain"
		out.Code = "execution_outcome_unknown"
	}
	if (r.Status == "completed" || r.Status == "failed") && (r.RemoteState == "stopped" || r.RemoteState == "not_issued") {
		out.NewOperationAllowed = true
	}
	if r.Status == "completed" {
		out.Code = "result_not_retained"
	}
	return out
}

// DatasetOptions is an explicit source read after host allocation of a new
// target. Merely editing a selection or search string does not call this method.
func (s *Authoring) DatasetOptions(ctx context.Context, e identity.Envelope, in AuthoringOptionRequest) (AuthoringOptionView, error) {
	if in.Target.Dataset == nil || in.Target.Report != nil {
		return AuthoringOptionView{}, ErrInvalid
	}
	return s.authoringOptions(ctx, e, in)
}

// ReportOptions derives immutable option coordinates from the saved
// page's actual filter bindings; caller-provided SQL/column sources are absent.
func (s *Authoring) ReportOptions(ctx context.Context, e identity.Envelope, in AuthoringOptionRequest) (AuthoringOptionView, error) {
	if in.Target.Report == nil || in.Target.Dataset != nil {
		return AuthoringOptionView{}, ErrInvalid
	}
	return s.authoringOptions(ctx, e, in)
}

func (s *Authoring) authoringOptions(ctx context.Context, e identity.Envelope, in AuthoringOptionRequest) (AuthoringOptionView, error) {
	if ctx == nil || !AuthoringOptionOperationValid(in.Operation, time.Now(), false) || in.Limit < 1 || in.Limit > 199 || len(in.Search) > 256 || !utf8.ValidString(in.Search) || strings.ContainsRune(in.Search, 0) || len(in.Cursor) > filterCursorMax || !locale(in.Locale) {
		return AuthoringOptionView{}, ErrInvalid
	}
	if err := RequireAuthoringOptionTarget(e, in.Target); err != nil {
		return AuthoringOptionView{}, fmt.Errorf("option target: %w", err)
	}
	b, repo, err := s.optionRepository()
	if err != nil {
		return AuthoringOptionView{}, err
	}
	if !b.CanValidate() {
		return AuthoringOptionView{}, ErrUnavailable
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	// Read the immutable admission before resolving/validating anything native.
	prior, err := repo.ReadAuthoringOption(ctx, e, AuthoringOptionReference{Target: in.Target, Operation: in.Operation})
	if err == nil {
		if prior.InputDigest != digest(in) {
			return AuthoringOptionView{}, store.ErrConflict
		}
		return s.optionStatus(ctx, e, prior)
	}
	if !errors.Is(err, store.ErrNotFound) {
		return AuthoringOptionView{}, fmt.Errorf("option custody: %w", err)
	}
	if !AuthoringOptionOperationValid(in.Operation, time.Now(), true) {
		return AuthoringOptionView{}, store.ErrExpired
	}
	resolved, err := s.resolveAuthoringOption(ctx, e, in.Target)
	if err != nil {
		var unsupported *authoringUnsupported
		if errors.As(err, &unsupported) {
			return AuthoringOptionView{Operation: in.Operation, InputDigest: digest(in), Status: "unsupported", Code: unsupported.code, Options: []FilterOption{}}, nil
		}
		return AuthoringOptionView{}, fmt.Errorf("option resolution: %w", err)
	}
	parameters, err := s.optionQueryParameters(e, in, resolved)
	if err != nil {
		return AuthoringOptionView{}, err
	}
	statement, err := authoringOptionStatement(resolved, in.Search != "", in.Cursor != "", in.Limit+1)
	if err != nil {
		return AuthoringOptionView{}, err
	}
	now := time.Now().UTC()
	timeout := min(time.Duration(b.limits.ValidationTimeout), time.Duration(b.limits.Execution.Timeout))
	r := AuthoringOptionRecord{Tenant: e.Tenant(), Actor: e.User(), Session: e.Session(), Operation: in.Operation, InputDigest: digest(in), Target: clone(in.Target), SourceOperation: "authoring-option:" + digest([]string{e.Tenant(), e.User(), e.Session(), in.Operation}), Source: resolved.binding.Source, Context: resolved.binding.Context, SourceRevision: resolved.binding.Revision, BindingDigest: digest(resolved.binding), Topic: resolved.topic, Dataset: resolved.dataset, Dimension: resolved.dimension, Column: clone(resolved.column), ResolutionDigest: optionResolutionDigest(resolved), References: clone(resolved.refs), Blocks: clone(resolved.blocks), CreatedAt: now, Deadline: minTime(now.Add(timeout), e.Deadline()), Status: "accepted", RemoteState: "not_issued", Rows: in.Limit + 1}
	r, fresh, err := repo.ReserveAuthoringOption(ctx, e, r)
	if err != nil {
		return AuthoringOptionView{}, fmt.Errorf("option reservation: %w", err)
	}
	if !fresh {
		return s.optionStatus(ctx, e, r)
	}
	work, stop := context.WithDeadline(ctx, r.Deadline)
	defer stop()
	out, terminal := s.runAuthoringOptions(work, e, in, r, resolved, statement, parameters)
	// A failed current-pin fence suppresses values. The journal still owns all
	// source liability, and status/control remain available without those values.
	if err := repo.FinishAuthoringOption(work, e, terminal); err != nil {
		return AuthoringOptionView{}, err
	}
	if err := work.Err(); err != nil {
		return AuthoringOptionView{}, err
	}
	return out, nil
}

func (s *Authoring) optionQueryParameters(e identity.Envelope, in AuthoringOptionRequest, r authoringOptionResolution) ([]exec.Parameter, error) {
	parameters := []exec.Parameter{}
	if in.Search != "" {
		p, err := optionSearchParameter(r, in.Search)
		if err != nil {
			return nil, err
		}
		parameters = append(parameters, p)
	}
	if in.Cursor != "" {
		cursor, err := s.documents.decodeFilterCursor(in.Cursor)
		if err != nil || cursor.Report != digest(in.Target) || cursor.Filter != r.cursorField() || cursor.Search != in.Search || cursor.Limit != in.Limit || cursor.Locale != in.Locale || cursor.SourceRevision != r.binding.Revision || cursor.Authority != filterAuthority(e) || cursor.Type != r.cursorType() || cursor.Page != optionResolutionDigest(r) {
			return nil, ErrStale
		}
		p, err := optionCursorParameter(r, cursor.Last)
		if err != nil {
			return nil, err
		}
		parameters = append(parameters, p)
	}
	return parameters, nil
}

func (s *Authoring) runAuthoringOptions(ctx context.Context, e identity.Envelope, in AuthoringOptionRequest, r AuthoringOptionRecord, resolved authoringOptionResolution, statement string, parameters []exec.Parameter) (AuthoringOptionView, AuthoringOptionRecord) {
	fail := func(code string) (AuthoringOptionView, AuthoringOptionRecord) {
		r.Status = "failed"
		r.Code = code
		return authoringOptionView(r), r
	}
	b, repo, _ := s.optionRepository()
	select {
	case b.slots <- struct{}{}:
		defer func() { <-b.slots }()
	case <-ctx.Done():
		return fail("lookup_deadline")
	default:
		return fail("lookup_busy")
	}
	plan, err := b.validator.ValidateWithin(ctx, e, exec.Request{Source: r.Source, Context: r.Context, SQL: statement, Parameters: parameters}, resolved.scope)
	if err != nil {
		return fail("option_validation_rejected")
	}
	sql, binds, err := plan.SQL(e, resolved.binding)
	if err != nil || sql != statement || parameterDigest(binds) != parameterDigest(parameters) {
		return fail("source_binding_changed")
	}
	current, err := s.resolveAuthoringOption(ctx, e, in.Target)
	if err != nil || optionResolutionDigest(current) != r.ResolutionDigest || digest(current.blocks) != digest(r.Blocks) {
		return fail("option_pins_changed")
	}
	if err := repo.SealAuthoringOption(ctx, e, r, plan.Receipt()); err != nil {
		return fail("option_pins_changed")
	}
	report, runErr := b.executor.Execute(ctx, e, plan, exec.Options{Operation: r.SourceOperation, Number: 1, Preview: true, Rows: in.Limit + 1, Bytes: 512 << 10})
	if errors.Is(runErr, exec.ErrLimit) && report.Attempt.ID == "" {
		return fail("option_execution_bounds_unavailable")
	}
	r.ExecutionStatus, r.RemoteState = report.Attempt.Status, report.Attempt.RemoteState
	if runErr != nil || report.Attempt.ID == "" || report.Attempt.Status == "uncertain" || report.Attempt.Finished == nil || !slices.Contains([]string{"stopped", "not_issued"}, r.RemoteState) {
		r.Status = "uncertain"
		r.Code = "execution_outcome_unknown"
		return authoringOptionView(r), r
	}
	if report.Attempt.Manifest.Operation != r.SourceOperation || report.Attempt.Manifest.Session != e.Session() || report.Attempt.Manifest.Receipt.Manifest != plan.Receipt().Manifest || !successful(report.Attempt.Status) {
		return fail("option_read_failed")
	}
	if err := validateFilterOptionResult(report.Result, in.Limit); err != nil || resolved.column == nil && report.Result.Schema[0].Type != "text" {
		return fail("option_result_unsupported")
	}
	out := authoringOptionView(r)
	for _, row := range report.Result.Rows[:min(len(report.Result.Rows), in.Limit)] {
		if len(row) != 1 || string(row[0]) == "null" {
			return fail("option_result_unsupported")
		}
		value := row[0]
		if resolved.column != nil {
			value, err = physicalOptionValue(*resolved.column, report.Result.Schema[0], value)
			if err != nil {
				return fail("option_result_unsupported")
			}
		}
		label, err := filterLabel(value, in.Locale)
		if err != nil {
			return fail("option_result_unsupported")
		}
		if _, err := optionCursorParameter(resolved, value); err != nil {
			return fail("option_result_unsupported")
		}
		out.Options = append(out.Options, FilterOption{Value: append(json.RawMessage(nil), value...), Label: label})
	}
	out.Complete = len(report.Result.Rows) <= in.Limit
	if !out.Complete {
		last := out.Options[len(out.Options)-1].Value
		out.Next, err = s.documents.encodeFilterCursor(filterCursor{Report: digest(in.Target), Filter: resolved.cursorField(), Page: r.ResolutionDigest, Search: in.Search, Limit: in.Limit, Locale: in.Locale, SourceRevision: r.SourceRevision, Authority: filterAuthority(e), Type: resolved.cursorType(), Last: last, Expires: time.Now().Add(5 * time.Minute).Unix()})
		if err != nil {
			return fail("option_cursor_failed")
		}
	}
	current, err = s.resolveAuthoringOption(ctx, e, in.Target)
	if err != nil || optionResolutionDigest(current) != r.ResolutionDigest || digest(current.blocks) != digest(r.Blocks) {
		return fail("option_pins_changed")
	}
	r.Status, r.Code = "completed", ""
	out.Status, out.Code, out.ExecutionStatus, out.RemoteState = "completed", "", r.ExecutionStatus, r.RemoteState
	out.ValuesAvailable, out.NewOperationAllowed = true, true
	// The exact value and display label can duplicate JSON escapes. Source
	// byte limits alone do not bound this transport projection.
	wire, err := json.Marshal(out)
	if err != nil || len(wire) > 512<<10 {
		return fail("option_result_budget")
	}
	return out, r
}

// OptionStatus is pure retained metadata inspection. It never repeats native
// planning, queries or source cancellation, and never supplies cached choices.
func (s *Authoring) OptionStatus(ctx context.Context, e identity.Envelope, in AuthoringOptionReference) (AuthoringOptionView, error) {
	if ctx == nil || !AuthoringOptionOperationValid(in.Operation, time.Now(), false) {
		return AuthoringOptionView{}, ErrInvalid
	}
	_, repo, err := s.optionRepository()
	if err != nil {
		return AuthoringOptionView{}, err
	}
	r, err := repo.ReadAuthoringOption(ctx, e, in)
	if err != nil {
		return AuthoringOptionView{}, err
	}
	return s.optionStatus(ctx, e, r)
}
func (s *Authoring) optionStatus(ctx context.Context, e identity.Envelope, r AuthoringOptionRecord) (AuthoringOptionView, error) {
	b, _, err := s.optionRepository()
	if err != nil {
		return AuthoringOptionView{}, err
	}
	if err := RequireAuthoringOption(e, r); err != nil {
		return AuthoringOptionView{}, err
	}
	out := authoringOptionView(r)
	if r.Status == "completed" || r.Status == "failed" {
		return out, nil
	}
	control, ok := b.executor.(authoringPreparationControl)
	if !ok {
		return AuthoringOptionView{}, ErrUnavailable
	}
	a, err := control.ByOperation(ctx, e, r.SourceOperation)
	if errors.Is(err, store.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return AuthoringOptionView{}, err
	}
	out.ExecutionStatus, out.RemoteState = a.Status, a.RemoteState
	if a.Finished != nil && a.Status != "uncertain" && slices.Contains([]string{"stopped", "not_issued"}, a.RemoteState) {
		out.Status = "completed"
		out.Code = "result_not_retained"
		out.NewOperationAllowed = true
		if !successful(a.Status) {
			out.Status = "failed"
			out.Code = a.Code
		}
	}
	return out, nil
}

// OptionControl persists cancellation/reconciles one owned attempt. It is a
// mutation and can contact the source control lane, but never restarts a query.
func (s *Authoring) OptionControl(ctx context.Context, e identity.Envelope, in AuthoringOptionControlRequest) (AuthoringOptionView, error) {
	if ctx == nil || !AuthoringOptionOperationValid(in.Operation, time.Now(), false) || !slices.Contains([]string{"cancel", "reconcile"}, in.Action) {
		return AuthoringOptionView{}, ErrInvalid
	}
	b, repo, err := s.optionRepository()
	if err != nil {
		return AuthoringOptionView{}, err
	}
	r, err := repo.StopAuthoringOption(ctx, e, AuthoringOptionReference{Target: in.Target, Operation: in.Operation}, in.Action == "cancel")
	if err != nil {
		return AuthoringOptionView{}, err
	}
	if r.Status == "failed" || r.Status == "completed" {
		return authoringOptionView(r), nil
	}
	control, ok := b.executor.(authoringPreparationControl)
	if !ok {
		return AuthoringOptionView{}, ErrUnavailable
	}
	a, err := control.ByOperation(ctx, e, r.SourceOperation)
	if errors.Is(err, store.ErrNotFound) {
		return authoringOptionView(r), nil
	}
	if err != nil {
		return AuthoringOptionView{}, err
	}
	receipt, err := control.Control(ctx, e, a.ID, in.Action == "cancel")
	if err != nil {
		return AuthoringOptionView{}, err
	}
	if receipt.Attempt.Finished != nil && receipt.Attempt.Status != "uncertain" && slices.Contains([]string{"stopped", "not_issued"}, receipt.Attempt.RemoteState) {
		r.Status, r.Code = "failed", "result_not_retained"
		r.ExecutionStatus, r.RemoteState = receipt.Attempt.Status, receipt.Attempt.RemoteState
		if err := repo.FinishAuthoringOption(ctx, e, r); err != nil {
			return AuthoringOptionView{}, err
		}
	}
	return s.optionStatus(ctx, e, r)
}
