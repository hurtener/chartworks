package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// Ordinary signed scope is necessary but cannot replace a service-prepared
// mutation or a live operation fence. JSON cannot reconstruct those proofs.
func TestDocumentStorageRejectsCallerPreparedProofs(t *testing.T) {
	f := newPhase18Fixture(t)
	ctx := context.Background()
	e := phase27Actor(t, f, f.f.e.User(), phase29DocumentScopes())
	claimed := []byte(`{"encoded":"e30=","authority":"caller-claimed","deadline":"2099-01-01T00:00:00Z","lease":{}}`)
	var document reporting.PreparedDocument
	var quarantine reporting.PreparedQuarantine
	var composition reporting.PreparedComposition
	var checkpoint reporting.PreparedCompositionWrite
	for _, target := range []any{&document, &quarantine, &composition, &checkpoint} {
		if err := json.Unmarshal(claimed, target); err != nil || !reflect.ValueOf(target).Elem().IsZero() {
			t.Fatal("caller JSON populated a sealed proof", err)
		}
	}
	if out, err := f.f.db.CommitDocument(ctx, e, document); !errors.Is(err, access.ErrUnauthenticated) || out.ID != "" {
		t.Fatal("caller document proof reached storage", out, err)
	}
	if out, err := f.f.db.QuarantineDocument(ctx, e, quarantine); !errors.Is(err, access.ErrUnauthenticated) || out != "" {
		t.Fatal("caller quarantine proof reached storage", out, err)
	}
	if out, err := f.f.db.SealComposition(ctx, e, jobs.RequestTask{}, composition); !errors.Is(err, access.ErrUnauthenticated) || out.Manifest.ID != "" {
		t.Fatal("caller composition proof reached storage", out.State, err)
	}
	if out, err := f.f.db.CheckpointComposition(ctx, jobs.Invocation{}, checkpoint); !errors.Is(err, reporting.ErrInvalid) || out.Manifest.ID != "" {
		t.Fatal("unfenced caller checkpoint reached storage", out.State, err)
	}
	raw := support.Raw(t, f.f.dsn)
	if count(t, raw, `SELECT (SELECT count(*) FROM chartworks.document_heads) + (SELECT count(*) FROM chartworks.document_quarantine) + (SELECT count(*) FROM chartworks.composition_runs)`) != 0 {
		t.Fatal("rejected proof left document or composition state")
	}
}

// Exercise the actual repository against a valid retained artifact before
// denying authority, cancelling requests, and closing its real connection pool.
// None of these failures may return values, mutate state, or rerun a source.
func TestReportingCompositionStorageFailureSurfaces(t *testing.T) {
	f := newPhase18Fixture(t)
	ctx := context.Background()
	scopes := append(phase29DocumentScopes(), "reporting.execute", "cw.report.execute:*", "cw.run.read:*", "jobs.read", "jobs.cancel", "reporting.retention", "cw.tenant.erase:*")
	e := phase27Actor(t, f, f.f.e.User(), scopes)
	documents, err := reporting.NewDocuments(f.f.db, nil, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRequestRunner(f.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	compositions, err := reporting.NewCompositions(documents, f.f.db, nil, nil, runner)
	if err != nil {
		t.Fatal(err)
	}
	state, err := documents.Create(ctx, e, "report", "failure-surface", phase29Text("Retained failure boundary"))
	if err != nil {
		t.Fatal(err)
	}
	state = phase29Publish(t, documents, e, state)
	run, err := compositions.Admit(ctx, e, "report", state.ID, reporting.CompositionRequest{Key: "failure-surface"})
	if err != nil {
		t.Fatal(err)
	}
	run, err = compositions.Run(ctx, e, run.ID, false)
	if err != nil || !run.Complete {
		t.Fatal("valid control artifact", run.State, err)
	}
	if payload, err := compositions.Widget(ctx, e, run.ID, "main", "intro"); err != nil || payload.Text == nil {
		t.Fatal("valid control read", err)
	}
	raw := support.Raw(t, f.f.dsn)
	readState := func() string {
		t.Helper()
		var snapshot string
		if err := raw.QueryRow(ctx, `SELECT jsonb_build_object(
 'state',c.state,'retained',c.retained_bytes,'reserved',c.reserved_bytes,
 'audits',(SELECT count(*) FROM chartworks.audit_events WHERE tenant_id=$1),
 'documents',(SELECT count(*) FROM chartworks.document_heads WHERE tenant_id=$1),
 'operations',(SELECT count(*) FROM chartworks.operations WHERE tenant_id=$1))::text
 FROM chartworks.composition_runs c WHERE c.tenant_id=$1 AND c.operation_id=$2`, e.Tenant(), run.ID).Scan(&snapshot); err != nil {
			t.Fatal("inspect unchanged durable state", err)
		}
		return snapshot
	}
	before, beforeSource, beforeModel := readState(), f.f.lookups.Load(), f.model.requests.Load()
	operations := []struct {
		name string
		call func(context.Context, identity.Envelope) (bool, error)
	}{
		{"document-read", func(ctx context.Context, e identity.Envelope) (bool, error) {
			out, err := f.f.db.ReadDocument(ctx, e, "report", state.ID, reporting.DocumentReference{}, reporting.Read, false)
			return out.State.ID == "" && len(out.Revision.Raw) == 0, err
		}},
		{"document-list", func(ctx context.Context, e identity.Envelope) (bool, error) {
			out, err := f.f.db.ListDocuments(ctx, e, "report", "", 10)
			return len(out.Items) == 0 && out.Next == "", err
		}},
		{"execution-recovery", func(ctx context.Context, e identity.Envelope) (bool, error) {
			out, err := f.f.db.ReadComposition(ctx, e, run.ID)
			return out.Manifest.ID == "" && len(out.Results) == 0, err
		}},
		{"artifact-metadata", func(ctx context.Context, e identity.Envelope) (bool, error) {
			out, err := f.f.db.ViewComposition(ctx, e, run.ID)
			return out.ID == "" && len(out.Pages) == 0, err
		}},
		{"widget-payload", func(ctx context.Context, e identity.Envelope) (bool, error) {
			out, err := f.f.db.CompositionWidget(ctx, e, run.ID, "main", "intro")
			return reflect.ValueOf(out).IsZero(), err
		}},
		{"cancel", func(ctx context.Context, e identity.Envelope) (bool, error) {
			out, err := f.f.db.CancelComposition(ctx, e, run.ID)
			return out.ID == "" && len(out.Pages) == 0, err
		}},
		{"retention", func(ctx context.Context, e identity.Envelope) (bool, error) {
			removed, err := f.f.db.ExpireCompositions(ctx, e, 10)
			return removed == 0, err
		}},
		{"create", func(ctx context.Context, e identity.Envelope) (bool, error) {
			out, err := documents.Create(ctx, e, "report", "must-not-exist", phase29Text("Denied mutation"))
			return out.ID == "", err
		}},
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	for _, stage := range []struct {
		name string
		ctx  context.Context
		e    identity.Envelope
		want error
	}{
		{"unverified", ctx, identity.Envelope{}, access.ErrUnauthenticated},
		{"cancelled", cancelled, e, context.Canceled},
		{"store-unavailable", ctx, e, store.ErrUnavailable},
	} {
		t.Run(stage.name, func(t *testing.T) {
			if stage.name == "store-unavailable" {
				f.f.db.Close()
			}
			for _, operation := range operations {
				t.Run(operation.name, func(t *testing.T) {
					empty, err := operation.call(stage.ctx, stage.e)
					if !errors.Is(err, stage.want) || !empty {
						t.Fatal("failure returned data or wrong error category", empty, err)
					}
				})
			}
			if readState() != before || f.f.lookups.Load() != beforeSource || f.model.requests.Load() != beforeModel {
				t.Fatal("failed storage operation mutated state or reached source/model work")
			}
		})
	}
}
