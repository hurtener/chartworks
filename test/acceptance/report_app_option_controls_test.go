package acceptance

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
)

type pausedOptionValidator struct {
	delegate reporting.Validator
	ready    chan struct{}
	release  chan struct{}
}

func (p *pausedOptionValidator) ValidateWithin(ctx context.Context, e identity.Envelope, r readexec.Request, scope []readexec.RelationScope) (readexec.Plan, error) {
	plan, err := p.delegate.ValidateWithin(ctx, e, r, scope)
	close(p.ready)
	select {
	case <-ctx.Done():
		return readexec.Plan{}, ctx.Err()
	case <-p.release:
		return plan, err
	}
}

func optionAuthoringWithBoundaries(t *testing.T, f *phase29ExecutionFixture, v reporting.Validator, x reporting.Executor) *reporting.Authoring {
	t.Helper()
	_, topics := newPhase18Service(t, f.f)
	blocks, err := reporting.New(f.f.f.db, topics, f.f.f.s, v, x, nil, f.limits)
	if err != nil {
		t.Fatal(err)
	}
	documents, err := reporting.NewDocuments(f.f.f.db, blocks, nil, f.limits)
	if err != nil {
		t.Fatal(err)
	}
	s, err := reporting.NewAuthoring(documents, f.compositions)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func optionsForPreparation(in reporting.AuthoringPrepareRequest, n int) reporting.AuthoringOptionRequest {
	return reporting.AuthoringOptionRequest{Target: reporting.AuthoringOptionTarget{Dataset: &reporting.AuthoringDatasetOptionTarget{NewBlock: in.NewBlock, Topic: in.Intent.Topic, Dataset: in.Intent.Dataset, Dimension: "region"}}, Operation: optionKey(n), Limit: 20, Locale: "en-US"}
}

func TestReportAppOptionCancellationDuringValidation(t *testing.T) {
	f, normal, author, prepare, _, scopes := filteredDatasetFixture(t)
	ctx := t.Context()
	before := f.attemptCount(t)
	gate := &pausedOptionValidator{delegate: f.f.f.validator, ready: make(chan struct{}), release: make(chan struct{})}
	s := optionAuthoringWithBoundaries(t, f, gate, f.f.f.executor)
	in := optionsForPreparation(prepare, 21)
	type result struct {
		view reporting.AuthoringOptionView
		err  error
	}
	done := make(chan result, 1)
	go func() { v, err := s.DatasetOptions(ctx, author, in); done <- result{v, err} }()
	defer func() {
		select {
		case <-gate.release:
		default:
			close(gate.release)
		}
	}()
	select {
	case <-gate.ready:
	case got := <-done:
		t.Fatal("lookup failed before native planning", got)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	status, err := s.OptionStatus(ctx, author, reporting.AuthoringOptionReference{Target: in.Target, Operation: in.Operation})
	if err != nil || status.Status != "accepted" || status.NewOperationAllowed || status.ValuesAvailable {
		t.Fatal("accepted lookup status", status, err)
	}
	other := phase27Copy(t, in)
	other.Operation = optionKey(22)
	other.Search = "East"
	if _, err := normal.DatasetOptions(ctx, author, other); !errors.Is(err, readexec.ErrUncertain) {
		t.Fatal("changed search bypassed active liability", err)
	}
	otherSession, err := f.f.f.token.verifier.Verify(ctx, phase27Token(t, f.f, author.User(), "other-option-session", scopes), auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := normal.DatasetOptions(ctx, otherSession, other); !errors.Is(err, readexec.ErrUncertain) {
		t.Fatal("new session bypassed active liability", err)
	}
	denied, err := normal.OptionStatus(ctx, otherSession, reporting.AuthoringOptionReference{Target: in.Target, Operation: in.Operation})
	if err == nil || !reflect.DeepEqual(denied, reporting.AuthoringOptionView{}) {
		t.Fatal("other session read original custody", denied, err)
	}
	cancelled, err := s.OptionControl(ctx, author, reporting.AuthoringOptionControlRequest{Target: in.Target, Operation: in.Operation, Action: "cancel"})
	if err != nil || cancelled.Status != "failed" || cancelled.Code != "cancelled" || !cancelled.NewOperationAllowed || cancelled.ValuesAvailable {
		t.Fatal("pre-admission cancel", cancelled, err)
	}
	close(gate.release)
	got := <-done
	if got.view.ValuesAvailable || len(got.view.Options) != 0 || f.attemptCount(t) != before {
		t.Fatal("cancelled planner dispatched source SELECT", got, f.attemptCount(t)-before)
	}
	replay, err := normal.DatasetOptions(ctx, author, in)
	if err != nil || replay.Status != "failed" || replay.ValuesAvailable || f.attemptCount(t) != before {
		t.Fatal("cancelled operation reran", replay, err)
	}
	if fresh, err := normal.DatasetOptions(ctx, author, other); err != nil || !fresh.ValuesAvailable || f.attemptCount(t) != before+1 {
		t.Fatal("explicit fresh search after confirmed cancellation", fresh, err)
	}
}

func TestReportAppOptionReceiptAndRuleFences(t *testing.T) {
	f, s, author, prepare, publication, scopes := filteredDatasetFixture(t)
	ctx := t.Context()
	in := optionsForPreparation(prepare, 31)
	if page, err := s.DatasetOptions(ctx, author, in); err != nil || !page.ValuesAvailable {
		t.Fatal(page, err)
	}
	base, err := f.f.f.db.ReadAuthoringOption(ctx, author, reporting.AuthoringOptionReference{Target: in.Target, Operation: in.Operation})
	if err != nil || base.Receipt == nil {
		t.Fatal("sealed receipt", err)
	}
	scope, _ := store.NewScope(author.Tenant(), author.User())
	original, err := f.f.f.db.GetReadOperation(ctx, scope, base.SourceOperation)
	if err != nil {
		t.Fatal(err)
	}
	reserve := func(n int) reporting.AuthoringOptionRecord {
		t.Helper()
		r := phase27Copy(t, base)
		r.Operation = optionKey(n)
		r.SourceOperation = "authoring-option:" + readexec.Hash([]string{author.Tenant(), author.User(), author.Session(), r.Operation})
		r.Status = "accepted"
		r.Code = ""
		r.ExecutionStatus = ""
		r.RemoteState = "not_issued"
		r.Receipt = nil
		r.CreatedAt = time.Now().UTC()
		r.Deadline = r.CreatedAt.Add(30 * time.Second)
		r, fresh, err := f.f.f.db.ReserveAuthoringOption(ctx, author, r)
		if err != nil || !fresh {
			t.Fatal("reserve native fence fixture", err)
		}
		if err := f.f.f.db.SealAuthoringOption(ctx, author, r, *base.Receipt); err != nil {
			t.Fatal("seal native fence fixture", err)
		}
		return r
	}
	r := reserve(32)
	before := f.attemptCount(t)
	for i, change := range []func(*readexec.Attempt, *store.Scope){
		func(a *readexec.Attempt, _ *store.Scope) { a.Manifest.Receipt.Manifest = strings.Repeat("0", 64) },
		func(a *readexec.Attempt, _ *store.Scope) { a.Manifest.Receipt.Context = "other-context" },
		func(a *readexec.Attempt, _ *store.Scope) { a.Manifest.Receipt.Source = "other-source" },
		func(a *readexec.Attempt, _ *store.Scope) { a.Manifest.Session = "other-session" },
		func(_ *readexec.Attempt, s *store.Scope) { *s, _ = store.NewScope(author.Tenant(), "other-actor") },
		func(a *readexec.Attempt, _ *store.Scope) { a.Manifest.Preview = false },
		func(a *readexec.Attempt, _ *store.Scope) { a.Manifest.Limits.Rows++ },
		func(a *readexec.Attempt, _ *store.Scope) { a.Manifest.Limits.Bytes = 1 << 20 },
		func(a *readexec.Attempt, _ *store.Scope) { a.Number = 2 },
	} {
		a := phase27Copy(t, original)
		a.ID = fmt.Sprintf("%032x", 100+i)
		a.Number = 1
		a.Manifest.Operation = r.SourceOperation
		a.Created = time.Now().UTC()
		a.Deadline = r.Deadline
		a.Status = "accepted"
		a.Code = ""
		a.Remote = nil
		a.RemoteState = "not_issued"
		a.Finished = nil
		a.Rows = 0
		a.Bytes = 0
		a.SourceDurationNS = nil
		s := scope
		change(&a, &s)
		if err := f.f.f.db.BeginRead(ctx, s, a, 3); err == nil {
			t.Fatal("mismatched native read admitted", i)
		}
	}
	if f.attemptCount(t) != before {
		t.Fatal("denied native inserts changed attempts")
	}
	if _, err := s.OptionControl(ctx, author, reporting.AuthoringOptionControlRequest{Target: in.Target, Operation: r.Operation, Action: "cancel"}); err != nil {
		t.Fatal(err)
	}
	// A rule publication during the native read invalidates values, while the
	// original physical attempt stays available for content-free inspection.
	gate := &pausedPreparedValidation{delegate: f.f.f.executor, ready: make(chan struct{}), release: make(chan struct{})}
	racing := optionAuthoringWithBoundaries(t, f, f.f.f.validator, gate)
	in.Operation = optionKey(33)
	type result struct {
		view reporting.AuthoringOptionView
		err  error
	}
	done := make(chan result, 1)
	go func() { v, err := racing.DatasetOptions(ctx, author, in); done <- result{v, err} }()
	defer func() {
		select {
		case <-gate.release:
		default:
			close(gate.release)
		}
	}()
	select {
	case <-gate.ready:
	case got := <-done:
		t.Fatal("lookup did not execute", got)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	rules, err := rulesets.New(f.f.f.db, f.f.f.db, f.f.f.db)
	if err != nil {
		t.Fatal(err)
	}
	reviewer := f.f.f.token.envelope(t, author.Tenant(), author.User(), topicScopes(author.Tenant())...)
	phase17PublishRules(t, rules, reviewer, publication)
	close(gate.release)
	got := <-done
	if got.view.ValuesAvailable || len(got.view.Options) > 0 || f.attemptCount(t) != before+1 {
		t.Fatal("rule race released values or repeated query", got)
	}
	if status, err := s.OptionStatus(ctx, author, reporting.AuthoringOptionReference{Target: in.Target, Operation: in.Operation}); err != nil || status.ValuesAvailable || !status.NewOperationAllowed {
		t.Fatal("terminal rule rejection status", status, err)
	}
	in.Operation = optionKey(34)
	if page, err := s.DatasetOptions(ctx, author, in); err != nil || page.Status != "unsupported" || page.Code != "reviewed_rules_unsupported" || f.attemptCount(t) != before+1 {
		t.Fatal("active rules performed option query", page, err)
	}
	limited := phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(scopes), func(x string) bool { return x == "cw.topic.read:"+publication.Definition.Topic }))
	if page, err := s.DatasetOptions(ctx, limited, in); err == nil || !reflect.DeepEqual(page, reporting.AuthoringOptionView{}) {
		t.Fatal("hidden rule metadata leaked", page, err)
	}
}
