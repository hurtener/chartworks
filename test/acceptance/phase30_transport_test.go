package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/workapi"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
)

func phase30ManagementScopes(f *phase30Fixture) []string {
	scopes := append(slices.Clone(f.admissionScopes), "scheduling.read", "scheduling.execute", "scheduling.cancel", "cw.schedule.read:*", "cw.schedule.write:*", "cw.schedule.execute:*", "cw.run.write:*")
	// Reading artifacts is not needed to manage schedules. Keep run metadata
	// read for the SDK while preserving every execution dependency permission.
	scopes = slices.DeleteFunc(scopes, func(s string) bool { return s == "reporting.read" || s == "cw.block.read:*" || s == "cw.report.read:*" })
	slices.Sort(scopes)
	return slices.Compact(scopes)
}

func phase30Wire(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil { t.Fatal(err) }
	return string(b)
}

func testPhase30Transport(t *testing.T) {
	t.Run("real-http-sdk-lifecycle-and-denial-inventory", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		definition := phase27Copy(t, f.domain.base)
		definition.SQL = "SELECT id, amount FROM analytics.sales WHERE id >= $1 ORDER BY id"
		definition.Parameters = []reporting.Parameter{{Name: "minimum", Type: "integer", Required: true, Default: &reporting.Value{Literal: "1"}, Min: "0", Max: "10"}}
		f.domain.block(t, "p30-wire", definition)
		target := phase30Target("saved_sql", "p30-wire")
		target.Arguments = []jobs.ReportingArgument{{Name: "minimum", Value: jobs.ReportingValue{Literal: "1"}}}
		scopes := phase30ManagementScopes(f)
		if len(scopes) > 32 { t.Fatal("wire fixture widened signed scope ceiling", len(scopes)) }
		bearer := phase27Token(t, f.domain.f, f.actor.User(), f.actor.Session(), scopes)
		registry, err := workapi.APIRegistry(nil, f.queue)
		if err != nil { t.Fatal(err) }
		handler := workapi.Handler(f.domain.f.f.token.verifier, nil, f.queue, http.NotFoundHandler())
		server := httptest.NewServer(assertRegisteredWireSchemas(t, registry, handler))
		t.Cleanup(server.Close)
		client, err := cw.New(server.URL, server.Client(), func(context.Context) (string, error) { return bearer, nil })
		if err != nil { t.Fatal(err) }
		request := cw.ScheduleRequest{Target: cw.JobTarget{Kind: jobs.ReportingKind, BindingID: "reporting", Reporting: &target}, Spec: jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}}
		schedule, err := client.CreateSchedule(t.Context(), "wire-schedule", request)
		if err != nil || schedule.Retired || schedule.Request.Target.Reporting == nil { t.Fatal("SDK reporting schedule create", schedule, err) }
		read, err := client.Schedule(t.Context(), schedule.ID)
		if err != nil || read.Revision != schedule.Revision || phase30Wire(t, read.Request) != phase30Wire(t, request) { t.Fatal("SDK schedule projection diverged", read, err) }
		job, err := client.TestSchedule(t.Context(), schedule.ID, "wire-test", 1)
		if err != nil || job.Reporting == nil || job.Reporting.Revision != 1 { t.Fatal("SDK test admission dropped accepted pins", job, err) }
		if err := f.queue.RunOnce(t.Context()); err != nil { t.Fatal(err) }
		finished, err := client.Job(t.Context(), job.ID)
		if err != nil || finished.State != "succeeded" || finished.Delivery == nil || finished.Delivery.Catalog != "available" || finished.Delivery.Notification != "not_requested" { t.Fatal("SDK discarded truthful delivery receipt", finished, err) }
		if schedule, err = client.SetScheduleState(t.Context(), schedule.ID, 1, false); err != nil || schedule.Revision != 2 { t.Fatal("SDK pause", schedule, err) }
		request.Target.Reporting.Budget.MaxRows = 500
		if schedule, err = client.ReplaceSchedule(t.Context(), schedule.ID, 2, "wire-replacement", request); err != nil || schedule.Revision != 3 || schedule.Enabled { t.Fatal("SDK CAS replacement", schedule, err) }
		if schedule, err = client.SetScheduleState(t.Context(), schedule.ID, 3, true); err != nil || schedule.Revision != 4 { t.Fatal("SDK resume", schedule, err) }
		pending, err := client.FireSchedule(t.Context(), schedule.ID, "wire-fire")
		if err != nil { t.Fatal(err) }
		if cancelled, err := client.CancelJob(t.Context(), pending.ID); err != nil || cancelled.State != "cancelled" { t.Fatal("SDK accepted occurrence cancellation", cancelled, err) }
		if schedule, err = client.RetireSchedule(t.Context(), schedule.ID, 4); err != nil || !schedule.Retired || schedule.Enabled { t.Fatal("SDK retirement", schedule, err) }
		history, err := client.ScheduleHistory(t.Context(), schedule.ID, cw.ScheduleHistoryRequest{Kind: "revisions", Limit: 2})
		if err != nil || len(history.Revisions) != 2 || history.Revisions[0].Revision != 5 || history.Revisions[0].Request.Target.Reporting == nil || history.NextBeforeRevision != 4 { t.Fatal("SDK immutable bounded history", history, err) }

		beforeQueries, beforeModels, beforeBroker := f.domain.attemptCount(t), f.domain.f.model.requests.Load(), f.calls.Load()
		minimal := phase27Token(t, f.domain.f, "unprivileged", "unprivileged-session", []string{"reporting.read"})
		for _, operation := range workapi.Registry(false, true) {
			path := strings.ReplaceAll(operation.Path, "{id}", schedule.ID)
			body := "{}"; if operation.Method == "GET" { body = "" }
			denied := callProtected(t, handler, operation.Method, path, minimal, body, nil)
			if denied.Code != http.StatusForbidden { t.Fatal("operation missing action-first denial", operation, denied.Code, denied.Body.String()) }
			if strings.Contains(operation.Path, "event") || strings.Contains(operation.Path, "condition") || strings.Contains(operation.Path, "custom") { t.Fatal("unsupported scheduling capability advertised", operation) }
		}
		foreignClaims := f.domain.f.f.token.claims("foreign-reporting-tenant", f.actor.User(), scopes)
		foreignClaims["session"] = f.actor.Session()
		foreign := f.domain.f.f.token.sign(t, foreignClaims, nil)
		response := callProtected(t, handler, "POST", "/v1/schedules/"+schedule.ID+"/history", foreign, `{"kind":"revisions","limit":2}`, nil)
		if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "p30-wire") { t.Fatal("history crossed the tenant boundary", response.Code, response.Body.String()) }

		base := phase30Wire(t, jobs.Submission{Kind: jobs.ReportingKind, BindingID: "reporting", Reporting: &target})
		for _, path := range [][]string{{"executor"}, {"reporting", "sql"}, {"reporting", "authorization"}, {"reporting", "budget", "provider_key"}, {"reporting", "arguments", "value", "execution_binding"}} {
			var value map[string]any
			if err := json.Unmarshal([]byte(base), &value); err != nil { t.Fatal(err) }
			cursor := value
			for _, key := range path[:len(path)-1] {
				if key == "arguments" { cursor = cursor[key].([]any)[0].(map[string]any) } else { cursor = cursor[key].(map[string]any) }
			}
			cursor[path[len(path)-1]] = "SYNTHETIC_FORGED_AUTHORITY"
			response := callProtected(t, handler, "POST", "/v1/jobs", bearer, phase30Wire(t, value), map[string]string{"Idempotency-Key": "forged-input"})
			if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "SYNTHETIC_FORGED_AUTHORITY") { t.Fatal("closed nested reporting request accepted executable/authority fields", path, response.Code, response.Body.String()) }
		}
		for _, body := range []string{strings.Replace(base, `"kind":"reporting.scheduled"`, `"kind":"reporting.scheduled","kind":"retention.sweep"`, 1), strings.Repeat(" ", 8193)+base} {
			response := callProtected(t, handler, "POST", "/v1/jobs", bearer, body, map[string]string{"Idempotency-Key": "bounded-json"})
			if response.Code != http.StatusBadRequest && response.Code != http.StatusRequestEntityTooLarge { t.Fatal("duplicate/oversized request reached admission", response.Code) }
		}
		invalid := target; invalid.Outputs = []string{"missing-output"}
		if _, err := client.SubmitJob(t.Context(), "invalid-target-contract", cw.JobTarget{Kind: jobs.ReportingKind, BindingID: "reporting", Reporting: &invalid}); err == nil { t.Fatal("invalid target output was admitted") } else {
			var status *cw.StatusError
			if !errors.As(err, &status) || status.Status != http.StatusBadRequest { t.Fatal("invalid reporting target lost its typed client error", err) }
		}
		if f.domain.attemptCount(t) != beforeQueries || f.domain.f.model.requests.Load() != beforeModels || f.calls.Load() != beforeBroker { t.Fatal("denied/malformed transport performed protected execution") }

		metadata, err := jobs.NewMetadata(f.domain.f.f.db, f.limits)
		if err != nil { t.Fatal(err) }
		metadataRegistry, err := workapi.APIRegistry(nil, metadata)
		if err != nil { t.Fatal(err) }
		for _, definition := range metadataRegistry.Definitions() { if definition.ID == "testSchedule" || definition.ID == "replaceSchedule" { t.Fatal("disabled dispatch advertised admission", definition.ID) } }
		metadataHandler := assertRegisteredWireSchemas(t, metadataRegistry, workapi.Handler(f.domain.f.f.token.verifier, nil, metadata, http.NotFoundHandler()))
		response = callProtected(t, metadataHandler, "POST", "/v1/schedules/"+schedule.ID+"/history", bearer, `{"kind":"occurrences","limit":1}`, nil)
		if response.Code != http.StatusOK { t.Fatal("broker availability disabled retained history", response.Code, response.Body.String()) }
	})
	t.Run("unsupported-kinds-never-admit-work", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-closed-kinds", f.domain.base)
		for _, kind := range []string{"event", "condition", "condition_check", "custom", "shell", "webhook"} {
			target := phase30Target("saved_sql", "p30-closed-kinds")
			request := phase30Manual(target)
			request.Spec.Type = kind
			if request.Validate() == nil { t.Fatal("unsupported trigger validated", kind) }
			if _, err := f.queue.CreateSchedule(t.Context(), f.actor, "unsupported-"+kind, request); !errors.Is(err, jobs.ErrInvalid) { t.Fatal("unsupported trigger reached persistence", kind, err) }
			if _, err := f.queue.Submit(t.Context(), f.actor, "unsupported-"+kind, jobs.Submission{Kind: kind, BindingID: "reporting", Reporting: &target}); !errors.Is(err, jobs.ErrInvalid) { t.Fatal("unsupported executor reached persistence", kind, err) }
		}
		if f.calls.Load() != 0 { t.Fatal("unsupported kinds reached execution broker") }
	})
	t.Run("shared-bounded-maintenance-consumer-remains-real", func(t *testing.T) {
		q := newQueueFixture(t, func(l *jobs.Limits) { l.Batch = 1 })
		q.oldAudit(t)
		job := q.submit(t, "existing-bounded-retention")
		if err := q.service.RunOnce(t.Context()); err != nil { t.Fatal(err) }
		done, err := q.service.Get(t.Context(), q.actor, job.ID)
		if err != nil || done.State != "succeeded" || done.Batch != 1 || done.DeletedEvents != 1 || q.oldAuditCount(t) != 0 { t.Fatal("existing bounded cleanup was replaced by a stub", done, err) }
	})
}
